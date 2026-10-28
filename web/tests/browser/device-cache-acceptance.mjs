// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import assert from 'node:assert/strict';
import { mkdtemp, readFile, rm } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { exportDataset } from '../../src/health-dataset.ts';
import { command, root } from './toolchain.mjs';

const MODULE_PATH = '/synthetic-device-cache-ui.js';
const PROFILE_A = 'a'.repeat(64), PROFILE_B = 'b'.repeat(64);
const json = value => ({ status: 200, contentType: 'application/json', body: JSON.stringify(value) });
const open = async element => { if (!await element.evaluate(node => node.open)) await element.locator(':scope > summary').click(); };
async function bounded(promise) {
  let timeout;
  try { await Promise.race([promise, new Promise((_, reject) => { timeout = setTimeout(() => reject(new Error('Synthetic cache acquisition did not reach its expected boundary.')), 20_000); })]); }
  finally { clearTimeout(timeout); }
}
const categories = [{ id: 'medicine', label: '合成處方資料', label_en: 'Synthetic prescriptions', group: '合成', range_mode: 'date_search', has_details: true },
  { id: 'calendar_events', label: '合成未來行程', label_en: 'Synthetic scheduled events', group: '合成', range_mode: 'date_search' }];

/** Actual App/IndexedDB/downloads; explicitly mocked verified-source API boundary. */
export async function runDeviceCacheAcceptance({ createContext, baseURL, evidenceDir, fixture, importLaboratory }) {
  const temporary = await mkdtemp(path.join(os.tmpdir(), 'healthbank-cache-ui-'));
  let context, closing = false;
  const held = { reached: Promise.withResolvers(), release: Promise.withResolvers() };
  try {
    const bundle = path.join(temporary, 'synthetic-cache.js');
    await command(path.join(root, '.tools/bin/esbuild'), ['web/src/device-health-cache.ts', '--bundle', '--format=esm', '--target=es2022,safari16.4', `--outfile=${bundle}`]);
    context = await createContext();
    const moduleSource = await readFile(bundle, 'utf8');
    await context.route(`**${MODULE_PATH}`, route => route.fulfill({ status: 200, contentType: 'text/javascript', body: moduleSource }));
    const raw = { ...fixture.laboratory.records[0], item_name: 'ALT', order_name: '合成快取檢驗', order_code: '09026C', check_date: '113/02/03', visit_date: '113/01/19',
      result: '27.000000000000000001', unit: 'U/L', source: 'SYNTHETIC_OLD_CODE', source_name: '合成快取來源名稱', upload_date: '113/03/20 09:41:22' };
    const imported = await importLaboratory(context, baseURL, { schema: 'healthbank.lab.v2', records: [raw, { ...raw }] });
    let token = imported.session.csrf_token, profile = PROFILE_A, presentOfficial = true, revision = 0, holdNextLaboratory = true;
    const original = { ...imported.report, fetched_at: '2024-12-20T01:02:03Z', complete: true, warnings: [],
      official_query: { mode: 'all_history', start_date: '1912-01-01', end_date: '2024-12-20', reported_count: 2 } };
    let currentReport = original;
    const calls = [], routeFailures = [];
    const guard = handler => async route => {
      try { await handler(route); }
      catch (failure) { if (!closing) routeFailures.push(failure); await route.abort().catch(() => {}); }
    };
    const bound = route => assert.ok(route.request().headers()['x-csrf-token'] === token, 'Synthetic cache API must preserve the real local session generation.');
    await context.route('**/api/session', guard(async route => {
      const response = await route.fetch(), data = await response.json();
      await route.fulfill({ response, json: presentOfficial ? { ...data, mode: 'official', authenticated: true, profile_key: profile, report_revision: revision } : data });
    }));
    await context.route('**/api/reports/current', guard(async route => { bound(route); await route.fulfill(json({ ...currentReport, session_token: token })); }));
    await context.route('**/api/data/catalog', route => route.fulfill(json({ categories,
      field_terms: [{ label: '合成欄位', label_en: 'Synthetic field' }], terminology_basis: 'healthbank_editorial_translation' })));
    await context.route('**/api/reports/cache', guard(async route => {
      bound(route); const input = route.request().postDataJSON();
      assert.equal(input.expected_profile_key, profile);
      if (route.request().method() === 'DELETE') {
        calls.push({ kind: 'clear', scope: input.scope }); revision++;
        if (input.scope === 'all_reports' || currentReport.cache_provenance) currentReport = { records: [], fetched_at: '', complete: false, warnings: [] };
        await route.fulfill(json({ status: 'cleared' })); return;
      }
      assert.equal(input.expected_report_revision, revision);
      assert.equal(input.document.schema, 'healthbank.lab.v2');
      calls.push({ kind: 'restore' }); revision++;
      currentReport = { records: input.document.records.map((row, index) => ({ ...original.records[index % original.records.length], result: row.result,
        source: row.source, source_name: row.source_name, raw_upload_date: row.upload_date })), fetched_at: input.source_fetched_at, complete: false,
        warnings: ['合成快取：未重新拉取官方資料'], cache_provenance: { kind: 'device_cache', saved_at: input.saved_at,
          restored_at: '2026-10-28T05:06:07Z', source_fetched_at: input.source_fetched_at, source_complete: input.source_complete,
          ...(input.source_query ? { source_query: input.source_query } : {}) } };
      await route.fulfill(json({ ...currentReport, session_token: token }));
    }));
    await context.route('**/api/reports?*', guard(async route => {
      bound(route); const url = new URL(route.request().url()); assert.equal(url.searchParams.get('from'), ''); assert.equal(url.searchParams.get('to'), '');
      calls.push({ kind: 'laboratory' });
      if (holdNextLaboratory) { holdNextLaboratory = false; held.reached.resolve(); await held.release.promise; }
      if (route.request().failure()) return;
      revision++;
      currentReport = { ...original, fetched_at: '2026-10-28T06:07:08Z', records: original.records.map(row => ({ ...row,
        result: '37.250000000000000001', numeric_value: 37.25, source: 'SYNTHETIC_FRESH_CODE' })) };
      await route.fulfill(json({ ...currentReport, session_token: token }));
    }));
    await context.route('**/api/supplementary?*', guard(async route => {
      bound(route); const url = new URL(route.request().url()), category = url.searchParams.get('category'), detail = url.searchParams.get('detail');
      assert.equal(url.searchParams.get('from'), ''); assert.equal(url.searchParams.get('to'), '', 'Future-calendar maximum scope must retain empty application dates.');
      calls.push({ kind: 'source', category, detail: Boolean(detail) });
      await route.fulfill(json({ category, fetched_at: '2026-10-28T06:07:08Z', complete: false, coverage: '合成來源沒有獨立總數', entries: [{
        date: category === 'calendar_events' ? '2099-01-01' : '2026-01-01', title: detail ? 'SYNTHETIC_FRESH_DETAIL' : `SYNTHETIC_FRESH_${category}`,
        source: 'SYNTHETIC_ONLY', institution: '合成院所', fields: [{ name: '合成欄位', value: '合成原文' }],
        ...(category === 'medicine' && !detail ? { detail_id: 'd'.repeat(48) } : {}),
      }] }));
    }));
    await context.route('**/api/auth/logout', guard(async route => {
      const response = await route.fetch(), value = await response.json();
      token = value.csrf_token; presentOfficial = false; currentReport = { records: [], fetched_at: '', complete: false, warnings: [] };
      await route.fulfill({ response, json: value });
    }));
    const stored = page => page.evaluate(async ({ modulePath, key }) => (await import(modulePath)).inspectDeviceCache(key), { modulePath: MODULE_PATH, key: PROFILE_A });
    let page = await context.newPage(); page.setDefaultTimeout(20000);
    await page.goto(baseURL, { waitUntil: 'networkidle' });
    let panel = page.locator('#device-health-cache'); await open(panel);
    const consent = panel.getByRole('checkbox'); await consent.waitFor();
    assert.equal(await consent.isChecked(), false);
    assert.equal((await stored(page)).enabled, false, 'Opening verified data must not opt into persistence.');
    await consent.click();
    await panel.getByText('目前已取得的資料已保存在此裝置。', { exact: true }).waitFor();
    const enabled = await stored(page); assert.equal(enabled.metadata.records, 2);
    const originalFile = new TextEncoder().encode('SYNTHETIC_CACHE_ATTACHMENT_中文\n');
    const snapshot = { reports: { medicine: { category: 'medicine', fetched_at: '2024-12-20T01:02:03Z', complete: true, coverage: '合成過往來源範圍', entries: [{
      date: '113/02/03', title: 'SYNTHETIC_OLD_SOURCE', institution: '合成院所', source: 'SYNTHETIC_ONLY', fields: [],
      attachments: [{ id: 'c'.repeat(48), name: '合成快取附件.txt', content_type: 'text/plain' }], document_actions: [{ id: 'e'.repeat(48), name: '合成不可保存能力', kind: 'prepare_export' }],
    }] } }, details: {}, states: { medicine: { status: 'ready', detail_errors: {}, query: { from: '', to: '' } } }, query: { from: '', to: '' } };
    const content = await exportDataset(original, snapshot, [{ category: 'medicine', id: 'c'.repeat(48), name: '合成快取附件.txt', content_type: 'text/plain', bytes: originalFile }], categories);
    await page.evaluate(async ({ lease, content, modulePath }) => (await import(modulePath)).saveDeviceCache(lease, content), { lease: enabled.lease, content, modulePath: MODULE_PATH });
    await page.close();
    currentReport = { records: [], fetched_at: '', complete: false, warnings: [] };
    page = await context.newPage(); page.setDefaultTimeout(20000);
    await page.goto(baseURL, { waitUntil: 'networkidle' });
    panel = page.locator('#device-health-cache'); await open(panel);
    await panel.getByText('已開啟這位使用者在此裝置保存的資料；本次登入未自動重新拉取全部歷史。', { exact: true }).waitFor();
    assert.equal(calls.filter(call => call.kind === 'laboratory').length, 0);
    assert.equal(calls.filter(call => call.kind === 'source').length, 0);
    assert.equal(calls.filter(call => call.kind === 'restore').length, 1);
    await page.getByRole('button', { name: '醫療紀錄', exact: true }).click();
    await page.locator('.health-data-event').filter({ hasText: 'SYNTHETIC_OLD_SOURCE' }).waitFor();
    assert.equal(await page.locator('.health-data-full-acquisition').count(), 0, 'Retired cache handles must not be offered as live acquisition capabilities.');
    const tools = page.locator('#dataset-tools'); await open(tools);
    const [download] = await Promise.all([page.waitForEvent('download'), tools.getByRole('button', { name: '匯出全部已取得資料', exact: true }).click()]);
    const output = path.join(evidenceDir, 'synthetic-device-cache-roundtrip.json'); await download.saveAs(output);
    const downloaded = JSON.parse(await readFile(output, 'utf8'));
    assert.equal(downloaded.laboratory.complete, false); assert.equal(downloaded.laboratory.cache_provenance.source_complete, true);
    assert.equal(downloaded.data.reports.medicine.complete, false); assert.equal(downloaded.data.reports.medicine.cache_provenance.source_complete, true);
    assert.equal(downloaded.laboratory.records.length, 2); assert.equal(downloaded.attachments.length, 1);
    assert.equal(downloaded.laboratory.records[0].upload_date, raw.upload_date); assert.equal(downloaded.laboratory.records[0].source_name, raw.source_name);
    for (const forbidden of [token, PROFILE_A, 'document_actions', 'e'.repeat(48)]) assert.ok(!JSON.stringify(downloaded).includes(forbidden));

    await panel.getByRole('button', { name: '清除舊資料並重新取得全部來源', exact: true }).click();
    await bounded(held.reached.promise);
    assert.equal((await stored(page)).metadata, null, 'Old local payload is gone before the first fresh source request.');
    assert.equal(await page.locator('.health-data-event').filter({ hasText: 'SYNTHETIC_OLD_SOURCE' }).count(), 0);
    assert.ok(calls.findIndex(call => call.kind === 'clear' && call.scope === 'all_reports') < calls.findIndex(call => call.kind === 'laboratory'));
    await panel.getByRole('button', { name: '停止重新取得', exact: true }).click(); held.release.resolve();
    await panel.getByText('重新取得已停止；目前只保留本輪成功取得的內容，舊快取沒有復原。', { exact: true }).waitFor();
    assert.equal((await stored(page)).metadata, null);
    await panel.getByRole('button', { name: '清除舊資料並重新取得全部來源', exact: true }).click();
    await panel.getByText('目前已取得的資料已保存在此裝置。', { exact: true }).waitFor();
    const after = await stored(page); assert.equal(after.metadata.sources, 2); assert.equal(after.metadata.records, 2);
    assert.deepEqual(new Set(calls.filter(call => call.kind === 'source').map(call => call.category)), new Set(['medicine', 'calendar_events']));
    assert.equal(calls.filter(call => call.kind === 'source' && call.detail).length, 1);
    await page.locator('.health-data-event').filter({ hasText: 'SYNTHETIC_FRESH_DETAIL' }).waitFor();
    assert.equal(await page.locator('.health-data-event').filter({ hasText: 'SYNTHETIC_OLD_SOURCE' }).count(), 0);
    await page.screenshot({ path: path.join(evidenceDir, 'synthetic-device-cache-fresh.png') });

    const other = await context.newPage(); other.setDefaultTimeout(20000);
    await other.goto(baseURL, { waitUntil: 'networkidle' });
    await other.getByText('醫療來源已從此裝置快取開啟；檢驗工作區保留目前另行取得的官方資料。各來源保留原始取得時間。需要更新時，請使用「清除舊資料並重新取得全部來源」。', { exact: true }).waitFor();
    await panel.getByRole('button', { name: '清除此使用者快取', exact: true }).click();
    const otherPanel = other.locator('#device-health-cache'); await open(otherPanel);
    await otherPanel.getByText('本機快取已在另一個分頁清除或重新取得；本頁的舊快取已移除，沒有自動重新建立。', { exact: true }).waitFor();
    await panel.getByText('已清除此使用者的健康快取、保存同意與目前工作區。', { exact: true }).waitFor();
    assert.equal((await stored(other)).enabled, false);
    assert.equal(await other.locator('.health-data-event').filter({ hasText: /SYNTHETIC_FRESH_(medicine|calendar_events|DETAIL)/ }).count(), 0);
    assert.equal(await other.locator('.health-data-event').filter({ hasText: 'SYNTHETIC_FRESH_CODE' }).count(), 2,
      'Cross-tab cache revocation retains the separate fresh official laboratory view.');
    assert.equal(currentReport.records.length, 0, 'An explicit clear-all-reports request clears this shared server workspace.');
    await other.close();
    await panel.getByRole('checkbox').click();
    await panel.getByText('已同意此裝置保存目前官方驗證使用者的健康資料。', { exact: true }).waitFor();
    await panel.getByRole('button', { name: '清除舊資料並重新取得全部來源', exact: true }).click();
    await panel.getByText('目前已取得的資料已保存在此裝置。', { exact: true }).waitFor();
    await page.getByRole('button', { name: '登出', exact: true }).click();
    await page.getByText('已結束使用階段並清除目前報告。', { exact: true }).waitFor();
    assert.equal((await stored(page)).enabled, true); assert.ok((await stored(page)).metadata, 'Logout retains explicitly consented device data.');
    profile = PROFILE_B; presentOfficial = true;
    await page.reload({ waitUntil: 'networkidle' }); panel = page.locator('#device-health-cache'); await open(panel);
    assert.equal(await panel.getByRole('checkbox').isChecked(), false);
    assert.equal(await page.locator('.health-data-event').count(), 0, 'A later verified user must not inherit the prior user\'s saved workspace.');
    await panel.getByRole('button', { name: '清除此裝置所有健康快取', exact: true }).click();
    await panel.getByText('已清除此網站在這部裝置的所有健康快取與保存同意。自行下載的檔案須另外管理。', { exact: true }).waitFor();
    assert.equal((await stored(page)).enabled, false);
    assert.deepEqual(routeFailures.map(failure => failure.message), []);
    return { synthetic_only: true, actual_official_login: false, real_app_and_indexeddb: true, consent_required: true,
      cached_login_without_full_refetch: true, official_mode_retained: true, cached_live_capabilities_removed: true,
      actual_download_preserves_provenance_duplicates_and_raw_source_fields: true, clear_before_refresh: true,
      cancelled_refresh_does_not_revive_old_data: true, future_calendar_empty_query: true, nested_details_acquired: true,
      cross_tab_clear_removes_cached_workspace: true, logout_retains_consented_cache: true, other_user_isolated: true, clear_all_revokes: true };
  } finally {
    closing = true; held.release.resolve(); await context?.close(); await rm(temporary, { recursive: true, force: true });
  }
}
