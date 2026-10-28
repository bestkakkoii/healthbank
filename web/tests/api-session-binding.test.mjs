// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import ts from '../node_modules/typescript/lib/typescript.js';

// The API error class uses TypeScript parameter properties, which Node's
// erase-only loader does not support. Compile with the project's pinned TS.
const source = await readFile(new URL('../src/api.ts', import.meta.url), 'utf8');
const compiled = ts.transpileModule(source, {
  compilerOptions: { target: ts.ScriptTarget.ESNext, module: ts.ModuleKind.ESNext },
}).outputText;
const { api, ApiError, isSessionInvalidation, setCsrfToken } = await import(
  `data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`
);
const tokenA = 'a'.repeat(64);
const tokenB = 'b'.repeat(64);
const emptyReport = { records: [], fetched_at: '', complete: false, warnings: [] };
const packageID = 'c'.repeat(48);
const packageStatus = () => ({ id: packageID, state: 'ready', total: 1, completed: 0, failed: 0, bytes: 0,
  files: [{ archive_id: 'archive_1', state: 'queued', bytes: 0 }], expires_at: '2026-10-28T00:30:00Z', package_bytes: 0 });

test('stateful reads and downloaded attachment bytes stay bound to the page generation', async t => {
  const calls = [];
  t.mock.method(globalThis, 'fetch', async (path, options) => {
    calls.push({ path, token: new Headers(options.headers).get('X-CSRF-Token'), options });
    return Response.json({ ...emptyReport, session_token: tokenA });
  });
  t.after(() => setCsrfToken());
  setCsrfToken(tokenA);
  await api.currentReports();
  await api.catalog();
  await api.supplementary('medicine', '', '', '', new AbortController().signal);
  await api.document('medicine', 'c'.repeat(48), new AbortController().signal);
  await api.session();
  assert.deepEqual(calls.map(call => call.token), [tokenA, tokenA, tokenA, tokenA, null]);
  for (const call of calls) {
    assert.equal(call.options.credentials, 'same-origin');
    assert.equal(call.options.cache, 'no-store');
  }
});

test('all laboratory response paths require an explicit valid generation proof', async t => {
  let responseToken;
  t.mock.method(globalThis, 'fetch', async () => Response.json({ ...emptyReport, session_token: responseToken }));
  const operations = [
    () => api.reports('', ''),
    () => api.currentReports(),
    () => api.demo(),
    () => api.importReports(new File(['{"records":[]}'], 'synthetic.json')),
  ];
  for (responseToken of [undefined, '', tokenA.slice(1), 'G'.repeat(64), 123]) {
    for (const operation of operations) {
      await assert.rejects(operation(), error => error instanceof ApiError &&
        error.code === 'invalid_report_session' && isSessionInvalidation(error));
    }
  }
  responseToken = tokenB;
  for (const operation of operations) assert.equal((await operation()).session_token, tokenB);
});

test('a changed browser cookie rejects an old page read without treating ordinary conflicts as logout', async t => {
  t.mock.method(globalThis, 'fetch', async (_path, options) => {
    if (new Headers(options.headers).get('X-CSRF-Token') !== tokenB)
      return Response.json({ error: { code: 'session_changed', message: '合成工作階段已更換' } }, { status: 409 });
    return Response.json({ ...emptyReport, session_token: tokenB });
  });
  t.after(() => setCsrfToken());
  setCsrfToken(tokenA);
  await assert.rejects(api.currentReports(), error => error instanceof ApiError && isSessionInvalidation(error));
  setCsrfToken(tokenB);
  assert.equal((await api.currentReports()).session_token, tokenB);
  assert.equal(isSessionInvalidation(new ApiError('合成設定版本衝突', 'settings_conflict', 409)), false);
  assert.equal(isSessionInvalidation(new ApiError('合成來源限制', 'origin_rejected', 403)), false);
});

test('package creation inserts exact dataset JSON as an object and all control operations retain the page generation', async t => {
  const calls = [];
  t.mock.method(globalThis, 'fetch', async (path, options) => {
    calls.push({ path, token: new Headers(options.headers).get('X-CSRF-Token'), options });
    return Response.json(options.method === 'DELETE' ? { status: 'cancelled' } : packageStatus());
  });
  t.after(() => setCsrfToken()); setCsrfToken(tokenA);
  const content = '{"schema":"healthbank.dataset.v2","精確原文":"400.0000000000001"}\n';
  const metadata = { files: [{ category: 'imaging', id: 'd'.repeat(48), archive_id: 'archive_1' }],
    plan: { id: 'e'.repeat(32), index: 1, total: 1, files_total: 1 },
    acquisition: { scope: 'all_available', started_at: '2026-10-28T00:00:00Z', finished_at: '2026-10-28T00:01:00Z', statuses: [] } };
  assert.equal((await api.createAcquisitionPackage({ ...metadata, dataset: content })).id, packageID);
  await api.acquisitionPackageStatus({ id: packageID });
  await api.acquisitionPackageStatus({ plan: metadata.plan.id, part: 1 });
  await api.cancelAcquisitionPackage(packageID);
  const parsed = JSON.parse(calls[0].options.body);
  assert.deepEqual(parsed.dataset, JSON.parse(content));
  assert.equal(JSON.stringify(parsed.dataset) + '\n', content);
  assert.ok(calls.every(call => call.token === tokenA && call.options.credentials === 'same-origin' && call.options.cache === 'no-store'));
  assert.match(calls[2].path, /plan=e{32}&part=1$/);
  assert.equal(calls[3].options.method, 'DELETE');
  const external = { name: 'healthbank.dataset.v2.json', size: 12, sha256: 'f'.repeat(64) };
  await api.createAcquisitionPackage({ ...metadata, external_dataset: external });
  assert.deepEqual(JSON.parse(calls[4].options.body).external_dataset, external);
  assert.ok(!Object.hasOwn(JSON.parse(calls[4].options.body), 'dataset'));
  await assert.rejects(api.createAcquisitionPackage({ ...metadata, dataset: content, external_dataset: external }));
  await assert.rejects(api.createAcquisitionPackage(metadata));
  await assert.rejects(api.createAcquisitionPackage({ ...metadata, dataset: content,
    files: [{ ...metadata.files[0], archive_id: 'archive_2' }] }), error => error.code === 'invalid_response');
});

test('package status cannot turn inconsistent counts or missing file/ZIP digests into successful completion', async t => {
  let value;
  t.mock.method(globalThis, 'fetch', async () => Response.json(value));
  const invalid = [
    item => { item.total = 2; },
    item => { item.completed = 1; },
    item => { item.bytes = -1; },
    item => { item.bytes = 1; },
    item => { item.id = 'd'.repeat(48); },
    item => { item.files[0].bytes = 501 * 1024 * 1024; },
    item => { item.files[0].state = 'completed'; item.completed = 1; },
    item => { item.state = 'completed'; },
    item => { item.package_sha256 = 'a'.repeat(64); },
    item => { item.total = 2; item.files.push({ ...item.files[0] }); },
    item => { item.files[0].archive_id = 'archive_1234567890'; },
  ];
  for (const mutate of invalid) {
    value = packageStatus(); mutate(value);
    await assert.rejects(api.acquisitionPackageStatus({ id: packageID }), error => error instanceof ApiError && error.code === 'invalid_response');
  }
  value = { ...packageStatus(), state: 'completed', completed: 1, bytes: 12, package_bytes: 1024,
    package_sha256: 'f'.repeat(64), files: [{ archive_id: 'archive_1', state: 'completed', bytes: 12, sha256: 'e'.repeat(64) }] };
  assert.equal((await api.acquisitionPackageStatus({ id: packageID })).state, 'completed');
  delete value.package_sha256;
  await assert.rejects(api.acquisitionPackageStatus({ id: packageID }), error => error.code === 'invalid_response');
});

test('preparation refresh metadata only permits exact parent correlation or the explicitly uncorrelated official job list', async t => {
  let value = { status: 'requested', message: '合成產製受理', refresh: { category: 'imaging', detail: 'd'.repeat(48), correlation: 'same_record' } };
  t.mock.method(globalThis, 'fetch', async () => Response.json(value));
  assert.equal((await api.prepareDocument('imaging', 'e'.repeat(48))).refresh.detail, 'd'.repeat(48));
  value = { ...value, refresh: { category: 'official_files', correlation: 'unavailable' } };
  assert.equal((await api.prepareDocument('imaging', 'e'.repeat(48))).refresh.correlation, 'unavailable');
  for (const refresh of [undefined, { category: 'imaging', correlation: 'same_record' },
    { category: 'another_source', detail: 'd'.repeat(48), correlation: 'same_record' },
    { category: 'official_files', detail: 'd'.repeat(48), correlation: 'unavailable' }]) {
    value = { ...value, refresh };
    await assert.rejects(api.prepareDocument('imaging', 'e'.repeat(48)), error => error.code === 'invalid_response');
  }
  assert.equal(isSessionInvalidation(new ApiError('合成下載包已清除', 'package_unavailable', 404)), false);
  assert.equal(isSessionInvalidation(new ApiError('合成下載包衝突', 'package_plan_conflict', 409)), false);
});
