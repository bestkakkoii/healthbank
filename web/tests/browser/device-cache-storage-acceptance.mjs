// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import assert from 'node:assert/strict';
import { mkdtemp, readFile, rm } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { exportDataset } from '../../src/health-dataset.ts';
import { command, root } from './toolchain.mjs';

const MODULE_PATH = '/synthetic-device-cache-storage.js';
const PAGE_PATH = '/synthetic-device-cache-storage';
const KEY_A = 'a'.repeat(64), KEY_B = 'b'.repeat(64);

// The source module is bundled only into an isolated synthetic fixture route.
// Production contains no diagnostic hook or public cache-testing endpoint.
export async function runDeviceCacheStorageAcceptance({ createContext, baseURL, fixture, importLaboratory }) {
  const temporary = await mkdtemp(path.join(os.tmpdir(), 'healthbank-cache-module-'));
  let context;
  try {
    const bundle = path.join(temporary, 'synthetic-cache.js');
    await command(path.join(root, '.tools/bin/esbuild'), ['web/src/device-health-cache.ts', '--bundle', '--format=esm', '--target=es2022,safari16.4', `--outfile=${bundle}`]);
    const source = await readFile(bundle, 'utf8');
    context = await createContext();
    await context.route(`**${MODULE_PATH}`, route => route.fulfill({ status: 200, contentType: 'text/javascript;charset=utf-8', body: source }));
    await context.route(`**${PAGE_PATH}`, route => route.fulfill({ status: 200, contentType: 'text/html;charset=utf-8',
      body: '<!doctype html><html lang="zh-Hant"><meta charset="utf-8"><title>合成裝置快取儲存驗證</title><p>合成資料／非真實帳號</p></html>' }));
    const first = await context.newPage(), second = await context.newPage();
    await Promise.all([first.goto(baseURL + PAGE_PATH), second.goto(baseURL + PAGE_PATH)]);
    const call = (page, method, ...args) => page.evaluate(async ({ method, args, modulePath }) => (await import(modulePath))[method](...args), { method, args, modulePath: MODULE_PATH });
    const outcome = (page, method, ...args) => page.evaluate(async ({ method, args, modulePath }) => {
      try { await (await import(modulePath))[method](...args); return 'unexpected_success'; }
      catch (error) { return error.code ?? error.name; }
    }, { method, args, modulePath: MODULE_PATH });
    const loaded = (page, key) => page.evaluate(async ({ key, modulePath }) => {
      const { dataset, state } = await (await import(modulePath)).loadDeviceCache(key);
      return { enabled: state.enabled, lease: state.lease, metadata: state.metadata,
        values: dataset ? JSON.parse(dataset.laboratoryJSON).records.map(row => row.result) : [],
        sources: Object.keys(dataset?.snapshot.reports ?? {}) };
    }, { key, modulePath: MODULE_PATH });
    const report = await importLaboratory(context, baseURL, { schema: 'healthbank.lab.v1', records: [fixture.laboratory.records[0], fixture.laboratory.records[0]] });
    const content = await exportDataset(report.report, { reports: {}, details: {}, states: {}, query: { from: '', to: '' } }, [], []);
    const values = [fixture.laboratory.records[0].result, fixture.laboratory.records[0].result];

    assert.equal(await call(first, 'verifiedCacheProfile', { authenticated: true, mode: 'official', profile_key: KEY_A }), KEY_A);
    assert.equal(await call(first, 'verifiedCacheProfile', { authenticated: false, mode: 'official', profile_key: KEY_A }), null);
    assert.equal(await call(first, 'verifiedCacheProfile', { authenticated: true, mode: 'import', profile_key: KEY_A }), null);
    assert.equal(await outcome(first, 'inspectDeviceCache', 'unverified'), 'invalid');
    assert.deepEqual(await call(first, 'inspectDeviceCache', KEY_A), { enabled: false, lease: null, metadata: null });
    const consent = await call(first, 'enableDeviceCache', KEY_A);
    assert.equal(consent.enabled, true); assert.equal(consent.metadata, null);
    const saved = await call(first, 'saveDeviceCache', consent.lease, content);
    assert.equal(saved.metadata.records, 2); assert.equal(saved.metadata.size, Buffer.byteLength(content));
    assert.deepEqual((await loaded(second, KEY_A)).values, values, 'Exact duplicate records must survive a real IDB Blob round trip.');
    assert.equal((await loaded(second, KEY_B)).enabled, false, 'A different pseudonymous user cannot inherit the first user\'s payload.');
    assert.equal(await outcome(second, 'saveDeviceCache', consent.lease, content), 'stale', 'Earlier save revisions cannot overwrite a newer snapshot.');

    await second.evaluate(async modulePath => {
      globalThis.syntheticCacheEvents = [];
      globalThis.syntheticUnwatch = (await import(modulePath)).watchDeviceCache(value => globalThis.syntheticCacheEvents.push(value));
    }, MODULE_PATH);
    const cleared = await call(first, 'clearDeviceCache', KEY_A, true);
    assert.equal(cleared.enabled, true); assert.equal(cleared.metadata, null);
    assert.notEqual(cleared.lease.profile_epoch, saved.lease.profile_epoch);
    assert.equal(await outcome(second, 'saveDeviceCache', saved.lease, content), 'stale', 'Clearing for refresh rejects another tab\'s old write.');
    assert.deepEqual((await loaded(second, KEY_A)).values, []);
    await second.waitForFunction(() => globalThis.syntheticCacheEvents.length > 0);
    const events = await second.evaluate(() => globalThis.syntheticCacheEvents);
    assert.ok(events.every(event => Object.keys(event).every(key => ['kind', 'profile_key'].includes(key))), 'No observations or credentials enter cross-tab messages.');

    const savedAgain = await call(first, 'saveDeviceCache', cleared.lease, content);
    const consentB = await call(second, 'enableDeviceCache', KEY_B);
    await call(second, 'saveDeviceCache', consentB.lease, content);
    await call(second, 'clearAllDeviceCaches');
    assert.equal((await loaded(first, KEY_A)).enabled, false); assert.equal((await loaded(first, KEY_B)).enabled, false);
    const reenabled = await call(first, 'enableDeviceCache', KEY_A);
    assert.notEqual(reenabled.lease.global_epoch, savedAgain.lease.global_epoch);
    assert.equal(await outcome(first, 'saveDeviceCache', savedAgain.lease, content), 'stale', 'Re-enabling after clear-all must not resurrect an old lease.');
    assert.equal(await first.evaluate(async ({ lease, content, modulePath }) => {
      const controller = new AbortController(); controller.abort();
      try { await (await import(modulePath)).saveDeviceCache(lease, content, controller.signal); return 'unexpected_success'; }
      catch (error) { return error.name; }
    }, { lease: reenabled.lease, content, modulePath: MODULE_PATH }), 'AbortError');
    const unsafe = JSON.parse(content); unsafe.password = 'SYNTHETIC_FORBIDDEN_FIELD';
    assert.equal(await outcome(first, 'saveDeviceCache', reenabled.lease, JSON.stringify(unsafe)), 'Error');
    assert.equal((await loaded(first, KEY_A)).metadata, null, 'Cancelled or invalid saves cannot create any payload.');

    await call(first, 'saveDeviceCache', reenabled.lease, content);
    await first.evaluate(async ({ key, modulePath }) => {
      const module = await import(modulePath), original = Blob.prototype.arrayBuffer;
      globalThis.syntheticReadEntered = false;
      Blob.prototype.arrayBuffer = async function() {
        globalThis.syntheticReadEntered = true;
        await new Promise(resolve => { globalThis.syntheticReleaseRead = resolve; });
        return original.call(this);
      };
      globalThis.syntheticPendingRead = module.loadDeviceCache(key)
        .then(() => 'unexpected_success', error => error.code ?? error.name)
        .finally(() => { Blob.prototype.arrayBuffer = original; });
    }, { key: KEY_A, modulePath: MODULE_PATH });
    await first.waitForFunction(() => globalThis.syntheticReadEntered);
    await call(second, 'clearAllDeviceCaches');
    assert.equal(await first.evaluate(async () => { globalThis.syntheticReleaseRead(); return globalThis.syntheticPendingRead; }), 'stale',
      'An asynchronously decoded Blob cannot reappear after another tab cleared it.');

    const concurrent = await call(first, 'enableDeviceCache', KEY_A);
    await Promise.all([outcome(first, 'saveDeviceCache', concurrent.lease, content), call(second, 'clearAllDeviceCaches')]);
    assert.equal((await loaded(second, KEY_A)).enabled, false, 'Atomic transaction ordering must leave clear-all final under a concurrent save.');
    for (let index = 1; index <= 8; index++) await call(first, 'enableDeviceCache', index.toString(16).repeat(64));
    assert.equal(await outcome(second, 'enableDeviceCache', KEY_A), 'capacity');
    await call(first, 'clearAllDeviceCaches');

    const durable = await call(first, 'enableDeviceCache', KEY_A);
    await call(first, 'saveDeviceCache', durable.lease, content);
    await first.close();
    const reopened = await context.newPage(); await reopened.goto(baseURL + PAGE_PATH);
    assert.deepEqual((await loaded(reopened, KEY_A)).values, values, 'Explicitly consented IDB data survives closing the producing page.');
    await reopened.reload(); assert.deepEqual((await loaded(reopened, KEY_A)).values, values);
    await reopened.evaluate(async ({ key, modulePath }) => {
      const module = await import(modulePath);
      await new Promise((resolve, reject) => {
        const open = globalThis.indexedDB.open(module.DEVICE_CACHE_DATABASE, 1);
        open.onerror = () => reject(open.error);
        open.onsuccess = () => {
          const db = open.result, tx = db.transaction('payloads', 'readwrite'), store = tx.objectStore('payloads'), get = store.get(key);
          get.onsuccess = () => {
            const value = get.result;
            value.content = new Blob([new Uint8Array(value.content.size)]); store.put(value);
          };
          tx.oncomplete = () => { db.close(); resolve(); }; tx.onabort = () => { db.close(); reject(tx.error); };
        };
      });
    }, { key: KEY_A, modulePath: MODULE_PATH });
    assert.equal(await outcome(reopened, 'loadDeviceCache', KEY_A), 'invalid', 'Same-size corrupted storage must fail the exact SHA-256 check.');
    await call(second, 'clearDeviceCache', KEY_A);
    assert.equal((await loaded(reopened, KEY_A)).enabled, false);
    return { synthetic_only: true, actual_indexeddb: true, tabs: 2, duplicate_values_preserved: true,
      per_user_consent_and_isolation: true, save_revision_cas: true, clear_refresh_generation: true, clear_all_revokes_consent: true,
      stale_async_read_rejected: true, concurrent_clear_wins: true, profile_limit_enforced: true,
      page_close_and_reload_retention: true, payload_checksum_verified: true, credentials_rejected: true };
  } finally {
    await context?.close(); await rm(temporary, { recursive: true, force: true });
  }
}
