// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';

const json = body => ({ status: 200, contentType: 'application/json', body: JSON.stringify(body) });
const unavailable = () => ({ status: 503, contentType: 'application/json', body: JSON.stringify({ error: { code: 'synthetic_unavailable', message: '合成測試：本次來源暫時無法取得。' } }) });
const escapeRegex = value => value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
const row = (source, date, detail) => ({ date, institution: '合成分批院所', title: source, source,
  fields: [{ name: '合成資料', value: '不屬於任何真實個人' }], ...(detail ? { detail_id: detail } : {}) });
const report = (category, entries, range) => ({ category, entries, fetched_at: '2026-10-28T00:00:00Z', complete: false,
  coverage: '合成批次；來源歷史完整性未驗證', start_date: range.from, end_date: range.to });

/** Explicit controller fixture only; never represents an authenticated provider test. */
export async function runBatchAcceptance({ createContext, baseURL, evidenceDir, fixture, importLaboratory, session }) {
  const context = await createContext();
  const imported = await importLaboratory(context, baseURL, { schema: 'healthbank.lab.v1', records: fixture.laboratory.records.slice(0, 1) });
  let token = imported.session.csrf_token;
  const template = imported.report.records[0];
  const catalogResponse = await context.request.get(`${baseURL}/api/data/catalog`);
  const { categories } = await catalogResponse.json();
  const page = await context.newPage(); page.setDefaultTimeout(15000);
  let presentOfficial = true;
  let unsolicitedLogouts = 0;
  page.on('request', request => { if (new URL(request.url()).pathname === '/api/auth/logout') unsolicitedLogouts++; });
  let closing = false;
  const routeFailures = [];
  const guarded = handler => async route => {
    try { await handler(route); }
    catch (failure) {
      if (!closing) routeFailures.push(failure);
      await route.abort().catch(() => {});
    }
  };
  const calls = [], laboratoryAttempts = new Map();
  const januaryNested = { reached: Promise.withResolvers(), release: Promise.withResolvers() };
  const februaryCancel = { reached: Promise.withResolvers(), release: Promise.withResolvers() };
  const marchExpired = { reached: Promise.withResolvers(), release: Promise.withResolvers() };
  const ordinaryExpired = { reached: Promise.withResolvers(), release: Promise.withResolvers() };
  const waitReached = async pending => {
    const timer = setTimeout(() => pending.reject(new Error('Synthetic batch request did not arrive.')), 15000);
    try { await pending.promise; } finally { clearTimeout(timer); }
  };
  const finishHeld = async (route, hold, response) => {
    hold.reached.resolve(); await hold.release.promise;
    try { await route.fulfill(response); }
    catch (failure) {
      // An intentional cancel/unmount may close the request before its fixture
      // response is released. No response is replayed into another generation.
      if (!page.isClosed() && !route.request().failure()) throw failure;
    }
  };
  try {
    await page.route('**/api/session', guarded(async route => {
      const response = await route.fetch(), value = await response.json();
      await route.fulfill({ response, json: presentOfficial ? { ...value, mode: 'official', authenticated: true } : value });
    }));
    await page.route('**/api/reports?*', guarded(async route => {
      const url = new URL(route.request().url()), range = { from: url.searchParams.get('from'), to: url.searchParams.get('to') };
      assert.ok(route.request().headers()['x-csrf-token'] === token, 'Synthetic batch retains the real local generation token.');
      calls.push({ kind: 'laboratory', ...range });
      const attempt = (laboratoryAttempts.get(range.from) ?? 0) + 1;
      laboratoryAttempts.set(range.from, attempt);
      const suffix = !range.from ? 'ORDINARY' : range.from === '2024-01-15' ? 'JAN' : range.from === '2024-02-01' ? 'FEB' : 'MARCH';
      const date = range.from || '2024-04-01';
      const response = json({ records: [{ ...template, id: `${template.id}-${suffix}`, exam_date: date, raw_check_date: date,
        visit_date: '', raw_visit_date: '', source: `BATCH_${suffix}_LAB`, result: '101.01', numeric_value: 101.01 }],
      session_token: token, fetched_at: '2026-10-28T00:00:00Z', complete: false, warnings: ['合成分批檢驗，不代表來源歷史完整'],
      official_query: suffix === 'ORDINARY'
        ? { mode: 'all_history', start_date: '1912-01-01', end_date: '2026-10-28', reported_count: null }
        : { mode: 'custom', start_date: range.from, end_date: range.to, reported_count: null } });
      if (suffix === 'FEB' && attempt === 1) return route.fulfill(unavailable());
      if (suffix === 'FEB' && attempt === 2) return finishHeld(route, februaryCancel, response);
      if (suffix === 'MARCH') return finishHeld(route, marchExpired, response);
      if (suffix === 'ORDINARY') return finishHeld(route, ordinaryExpired, response);
      return route.fulfill(response);
    }));
    await page.route('**/api/supplementary?*', guarded(async route => {
      const url = new URL(route.request().url());
      const category = url.searchParams.get('category'), detail = url.searchParams.get('detail') ?? '';
      const range = { from: url.searchParams.get('from') ?? '', to: url.searchParams.get('to') ?? '' };
      calls.push({ kind: 'source', category, detail, ...range });
      assert.ok(route.request().headers()['x-csrf-token'] === token, 'Synthetic source request remains generation-bound.');
      if (!range.from) {
        assert.ok(['chronic', 'dental'].includes(category) && !detail, 'Only the explicitly selected preload fixture sources are allowed.');
        return route.fulfill(json(report(category, [row(`SYNTHETIC_RETAINED_${category}`, '2023-01-02')], range)));
      }
      assert.ok(['medicine', 'western'].includes(category), 'Source-only or unselected sources must not enter date batches.');
      const suffix = range.from === '2024-01-15' ? 'JAN' : 'FEB';
      const reference = (suffix === 'JAN' ? '1' : '2').repeat(47);
      if (category === 'western') {
        const attempts = calls.filter(call => call.kind === 'source' && call.category === category && call.from === range.from).length;
        if (suffix === 'FEB' && attempts <= 2) return route.fulfill(unavailable());
        return route.fulfill(json(report(category, [row(`BATCH_${suffix}_WESTERN`, range.from)], range)));
      }
      if (!detail) return route.fulfill(json(report(category, [row(`BATCH_${suffix}_MEDICINE`, range.from, `${reference}a`)], range)));
      if (detail === `${reference}a`) return route.fulfill(json(report(category, [row(`BATCH_${suffix}_DETAIL`, range.from, `${reference}b`)], range)));
      assert.equal(detail, `${reference}b`, 'The nested reference must come from the previous detail.');
      const response = json(report(category, [row(`BATCH_${suffix}_NESTED`, range.from)], range));
      if (suffix === 'JAN') return finishHeld(route, januaryNested, response);
      return route.fulfill(response);
    }));
    await page.goto(baseURL, { waitUntil: 'networkidle' });
    await page.getByRole('button', { name: '醫療紀錄', exact: true }).click();
    await page.locator('.health-data-management:not(.health-data-batches) > summary').click();
    await page.locator('.health-data-options > summary').click();
    const options = page.locator('.health-data-options');
    const selectCategory = async (id, selected = true) => {
      const label = categories.find(category => category.id === id)?.label;
      assert.ok(label, `Expected public catalog category ${id}.`);
      await options.getByRole('checkbox', { name: new RegExp(`^${escapeRegex(label)}`) }).setChecked(selected);
    };
    await options.getByRole('button', { name: '清除來源選取', exact: true }).click();
    await selectCategory('chronic'); await selectCategory('dental');
    await options.getByRole('checkbox', { name: '一併取得全部檢驗結果', exact: true }).uncheck();
    const normal = page.getByRole('button', { name: '取得所選來源的全部年度', exact: true });
    await normal.click();
    await page.getByRole('strong').filter({ hasText: 'SYNTHETIC_RETAINED_chronic' }).waitFor();
    await page.getByRole('strong').filter({ hasText: 'SYNTHETIC_RETAINED_dental' }).waitFor();
    await options.getByRole('button', { name: '清除來源選取', exact: true }).click();
    for (const id of ['medicine', 'western', 'chronic']) await selectCategory(id);
    await page.locator('.health-data-management:not(.health-data-batches) > summary').click();
    const batch = page.locator('.health-data-batches');
    await batch.locator(':scope > summary').click();
    await batch.getByLabel('分批起日', { exact: true }).fill('2024-01-15');
    await batch.getByLabel('分批迄日', { exact: true }).fill('2024-02-20');
    await batch.getByRole('combobox', { name: '每批期間', exact: true }).selectOption('month');
    await batch.getByRole('checkbox', { name: '本計畫一併取得檢驗結果', exact: true }).check();
    assert.match(await batch.innerText(), /1 個已選來源不支援日期搜尋/);
    await batch.getByRole('button', { name: '開始第一批', exact: true }).click();
    await waitReached(januaryNested.reached);
    const next = batch.getByRole('button', { name: '開始下一批並釋放這批資料', exact: true });
    const ack = batch.getByRole('checkbox', { name: '我已確認本批資料包已保存', exact: true });
    assert.ok(await next.isDisabled() && await ack.isDisabled(), 'Incomplete nested detail must block advance/export acknowledgement.');
    assert.ok(await normal.isDisabled(), 'An active batch owns ordinary acquisition controls.');
    januaryNested.release.resolve();
    await batch.getByText('本批可讀取的清單與明細已處理，可匯出本批資料包。來源歷史完整性仍依各項說明。', { exact: true }).waitFor();
    assert.ok(await next.isDisabled() && await ack.isDisabled(), 'A completed batch still requires an actual export.');
    const save = async filename => {
      const [download] = await Promise.all([page.waitForEvent('download'), batch.getByRole('button', { name: '匯出本批資料包 v2', exact: true }).click()]);
      const output = path.join(evidenceDir, filename); await download.saveAs(output);
      const document = JSON.parse(await readFile(output, 'utf8'));
      assert.equal(document.schema, 'healthbank.dataset.v2');
      assert.ok(!JSON.stringify(document).includes(token), 'Batch archive must strip the live generation token.');
      return document;
    };
    const january = await save('synthetic-batch-january.json');
    assert.deepEqual(january.data.query, { from: '2024-01-15', to: '2024-01-31' });
    assert.equal(january.laboratory.records.length, 1);
    assert.deepEqual(Object.keys(january.data.reports).sort(), ['medicine', 'western']);
    assert.equal(Object.keys(january.data.details.medicine).length, 2);
    assert.ok(await next.isDisabled() && !await ack.isDisabled(), 'Export alone is not confirmation of a saved download.');
    await ack.check(); await next.click();
    await batch.getByText('本批仍有未完成項目', { exact: false }).waitFor();
    const incomplete = await save('synthetic-batch-failed-february.json');
    assert.deepEqual(incomplete.data.query, { from: '2024-02-01', to: '2024-02-20' });
    assert.equal(incomplete.laboratory.records.length, 0);
    assert.equal(incomplete.data.states.western.status, 'failed');
    assert.ok(JSON.stringify(incomplete).includes('BATCH_FEB_MEDICINE') && !JSON.stringify(incomplete).includes('BATCH_JAN'), 'Successful new-batch sibling survives; no prior-batch lab or source is archived.');
    await ack.check();
    assert.ok(await batch.getByRole('button', { name: '完成分批計畫', exact: true }).isDisabled(), 'Saving partial data cannot advance an incomplete batch.');
    await batch.getByRole('button', { name: '接續本批未完成項目', exact: true }).click();
    await waitReached(februaryCancel.reached);
    await batch.getByRole('button', { name: '停止本批取得', exact: true }).click();
    februaryCancel.release.resolve();
    await batch.getByText('已停止本批取得；成功清單與明細仍保留，可接續本批。', { exact: true }).waitFor();
    const cancelled = await save('synthetic-batch-cancelled-february.json');
    assert.equal(cancelled.laboratory.records.length, 0);
    assert.ok(!JSON.stringify(cancelled).includes('BATCH_JAN') && JSON.stringify(cancelled).includes('BATCH_FEB_MEDICINE'));
    const medicineBefore = calls.filter(call => call.category === 'medicine' && call.from === '2024-02-01').length;
    await batch.getByRole('button', { name: '接續本批未完成項目', exact: true }).click();
    await batch.getByText('本批可讀取的清單與明細已處理，可匯出本批資料包。來源歷史完整性仍依各項說明。', { exact: true }).waitFor();
    assert.equal(calls.filter(call => call.category === 'medicine' && call.from === '2024-02-01').length, medicineBefore, 'Resume must not refetch successful sibling lists/details.');
    const february = await save('synthetic-batch-complete-february.json');
    assert.equal(february.laboratory.records[0].source, 'BATCH_FEB_LAB');
    assert.ok(!JSON.stringify(february).includes('BATCH_JAN'));
    await ack.check(); await batch.getByRole('button', { name: '完成分批計畫', exact: true }).click();
    await batch.getByText('分批計畫已結束', { exact: false }).waitFor({ state: 'attached' });
    assert.ok(await normal.isEnabled(), 'Ending the plan unlocks ordinary acquisition.');
    await page.getByRole('strong').filter({ hasText: 'SYNTHETIC_RETAINED_chronic' }).waitFor();
    await page.getByRole('strong').filter({ hasText: 'SYNTHETIC_RETAINED_dental' }).waitFor();
    const ranges = [...new Set(calls.filter(call => call.from).map(call => `${call.from}:${call.to}`))];
    assert.deepEqual(ranges, ['2024-01-15:2024-01-31', '2024-02-01:2024-02-20']);
    assert.ok(!calls.some(call => call.from && ['chronic', 'dental'].includes(call.category)), 'Source-only and unselected data are never queried as date batches.');

    if (!await batch.evaluate(element => element.open)) await batch.locator(':scope > summary').click();
    await batch.getByLabel('分批起日', { exact: true }).fill('2024-03-01');
    await batch.getByLabel('分批迄日', { exact: true }).fill('2024-03-10');
    await batch.getByRole('button', { name: '開始第一批', exact: true }).click();
    await waitReached(marchExpired.reached);
    const before = await session(context, baseURL);
    const ended = await context.request.post(`${baseURL}/api/auth/logout`, { headers: { Origin: baseURL, 'X-CSRF-Token': before.csrf_token }, data: {} });
    assert.ok(ended.ok(), 'The isolated fixture still uses real Go logout/session retirement.');
    presentOfficial = false; marchExpired.release.resolve();
    await page.getByText('使用階段已到期或更換，先前報告與醫療資料已清除。請確認目前使用者後重新取得資料。', { exact: true }).waitFor();
    await batch.waitFor({ state: 'hidden' });
    assert.equal(await page.locator('.health-data-event').count(), 0);
    if (routeFailures.length > 0) throw routeFailures[0];
    assert.ok(!calls.some(call => call.kind === 'source' && call.from === '2024-03-01'), 'Unmounted old-generation batch must not continue to source requests.');

    // The normal all-history workflow shares the laboratory callback. Exercise
    // its independently owned catch with a fresh real local generation too.
    const restarted = await importLaboratory(context, baseURL, { schema: 'healthbank.lab.v1', records: fixture.laboratory.records.slice(0, 1) });
    token = restarted.session.csrf_token; presentOfficial = true;
    await page.reload({ waitUntil: 'networkidle' });
    await page.getByRole('button', { name: '醫療紀錄', exact: true }).click();
    await page.locator('.health-data-management:not(.health-data-batches) > summary').click();
    await options.locator(':scope > summary').click();
    await options.getByRole('button', { name: '清除來源選取', exact: true }).click();
    await selectCategory('medicine');
    await options.getByRole('checkbox', { name: '一併取得全部檢驗結果', exact: true }).check();
    const beforeOrdinary = calls.length;
    await normal.click(); await waitReached(ordinaryExpired.reached);
    const ordinarySession = await session(context, baseURL);
    const ordinaryEnded = await context.request.post(`${baseURL}/api/auth/logout`, { headers: { Origin: baseURL, 'X-CSRF-Token': ordinarySession.csrf_token }, data: {} });
    assert.ok(ordinaryEnded.ok());
    presentOfficial = false; ordinaryExpired.release.resolve();
    await page.getByText('使用階段已到期或更換，先前報告與醫療資料已清除。請確認目前使用者後重新取得資料。', { exact: true }).waitFor();
    await batch.waitFor({ state: 'hidden' });
    assert.equal(await page.locator('.health-data-event').count(), 0);
    if (routeFailures.length > 0) throw routeFailures[0];
    assert.ok(!calls.slice(beforeOrdinary).some(call => call.kind === 'source'), 'A stale ordinary laboratory callback must not continue to source requests.');
    assert.equal(unsolicitedLogouts, 0, 'Reconciliation must not replay logout against the newly observed session.');
    return { fixture_only: true, actual_official_login: false, source_calls_are_synthetic: true,
      real_generation_tokens_preserved: true, completed_ranges: ranges, nested_details: 2,
      failed_and_cancelled_archives_exclude_prior_batch: true, unselected_sources_retained: 2,
      real_session_retirement_cancels_batch: true, real_session_retirement_cancels_ordinary_acquisition: true,
      reconciliation_does_not_replay_logout: true };
  } catch (failure) {
    if (!page.isClosed()) await page.screenshot({ path: path.join(evidenceDir, 'synthetic-batch-failure.png') }).catch(() => {});
    throw routeFailures[0] ?? failure;
  } finally {
    closing = true;
    for (const hold of [januaryNested, februaryCancel, marchExpired, ordinaryExpired]) hold.release.resolve();
    await context.close();
  }
}
