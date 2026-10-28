// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import test from 'node:test';
import assert from 'node:assert/strict';
import { emptyHealthData, healthEvents, filterHealthEvents, pendingDetailReferences, categoryCoverage,
  validAcquisitionQuery, healthDataEntryCount, replaceDetailReport, laboratoryEvents } from '../src/health-data.ts';
import { acquireHealthData } from '../src/data-acquisition.ts';

const entry = (title, date = '115/10/28', extra = {}) => ({ title, date, institution: '合成院所', source: '合成來源', fields: [], ...extra });
const report = (category, entries) => ({ category, entries, complete: false, fetched_at: '2026-10-28T00:00:00Z', coverage: '合成來源，總數未知' });
const catalog = [{ id: 'medicine', label: '用藥', group: '測試', range_mode: 'date_search' }, { id: 'hospital', label: '住院', group: '測試', range_mode: 'date_search' }];
const noFilter = { text: '', category: '', institution: '', from: '', to: '', includeUnknownDates: true };
const classifyFailure = error => ({ message: error.message, expired: error.message === 'expired', retryable: error.message === 'transient' });
function options(extra = {}) {
  return { snapshot: emptyHealthData(), categories: ['medicine', 'hospital'], query: { from: '', to: '' }, mode: 'refresh', signal: new AbortController().signal,
    classifyFailure, onUpdate() {}, onProgress() {}, ...extra };
}

test('source timeline preserves same-day duplicate rows and explicit parent links without inferred encounters', () => {
  const snapshot = emptyHealthData();
  snapshot.reports.medicine = report('medicine', [entry('藥甲', '115/10/28', { detail_id: 'ref' }), entry('藥甲')]);
  snapshot.reports.hospital = report('hospital', [entry('入院', '2026-10-28'), entry('日期未知', '不明')]);
  snapshot.details.medicine = { ref: report('medicine_detail', [entry('藥甲', '115/10/28'), entry('藥甲', '115/10/28')]) };
  const events = healthEvents(snapshot, catalog);
  assert.equal(events.length, 6);
  assert.equal(new Set(events.map(event => event.id)).size, 6);
  assert.equal(events.filter(event => event.parentReference === 'ref').length, 2);
  assert.equal(events.filter(event => event.date === '2026-10-28').length, 5);
  assert.equal(events.at(-1).entry.title, '日期未知');
  assert.equal(events[0].entry.date, '2026-10-28');
});

test('timeline display filters never modify source records or fabricate unknown dates', () => {
  const snapshot = emptyHealthData();
  snapshot.reports.medicine = report('medicine', [entry('藥甲', '115/10/28', { fields: [{ name: '給藥日數', value: '28' }] }), entry('藥乙', '不明')]);
  snapshot.reports.hospital = report('hospital', [entry('病房', '115/10/20')]);
  const events = healthEvents(snapshot, catalog);
  assert.equal(filterHealthEvents(events, { ...noFilter, text: '藥甲 28' }).length, 1);
  assert.equal(filterHealthEvents(events, { ...noFilter, category: 'medicine', from: '2026-10-27' }).length, 2);
  assert.equal(filterHealthEvents(events, { ...noFilter, category: 'medicine', from: '2026-10-27', includeUnknownDates: false }).length, 1);
  assert.equal(filterHealthEvents(events, { ...noFilter, institution: '其他院所' }).length, 0);
  assert.equal(snapshot.reports.medicine.entries[0].fields[0].value, '28');
  assert.equal(snapshot.reports.medicine.entries.length, 2);
});

test('coverage distinguishes an unqueried category, successful empty list and incomplete detail retrieval', () => {
  const snapshot = emptyHealthData();
  assert.equal(categoryCoverage(snapshot, 'medicine').queried, false);
  snapshot.reports.medicine = report('medicine', []);
  assert.equal(categoryCoverage(snapshot, 'medicine').queried, true);
  snapshot.reports.medicine.entries = [entry('a', '', { detail_id: 'a' }), entry('same a', '', { detail_id: 'a' }), entry('b', '', { detail_id: 'b' })];
  snapshot.details.medicine = { a: report('medicine_detail', []) };
  snapshot.states.medicine = { status: 'ready', detail_errors: { b: '來源失敗' } };
  assert.deepEqual(pendingDetailReferences(snapshot, 'medicine'), ['b']);
  assert.deepEqual(categoryCoverage(snapshot, 'medicine'), { count: 3, queried: true, detailCount: 2, loadedCount: 1, failedCount: 1, pendingCount: 1 });
});

test('acquisition dates are independent complete ranges and reject invalid or reversed dates', () => {
  assert.equal(validAcquisitionQuery({ from: '', to: '' }), true);
  assert.equal(validAcquisitionQuery({ from: '2024-02-29', to: '2024-02-29' }), true);
  for (const query of [{ from: '2026-02-29', to: '2026-10-28' }, { from: '', to: '2026-10-28' },
    { from: '2026-10-28', to: '2026-10-27' }, { from: '1911-01-01', to: '2201-01-01' }]) assert.equal(validAcquisitionQuery(query), false);
});

test('acquisition reads all category lists first then every detail beyond the old 24-record boundary', async () => {
  const calls = [];
  const result = await acquireHealthData(options({ fetchReport: async (category, reference, query) => {
    calls.push([category, reference, query]);
    return reference ? report(`${category}_detail`, [entry(reference)]) : report(category, Array.from({ length: category === 'medicine' ? 31 : 2 }, (_, index) => entry(`${index}`, '', { detail_id: `${category}${index}` })));
  } }));
  assert.deepEqual(calls.slice(0, 2).map(([category, reference]) => [category, reference]), [['medicine', ''], ['hospital', '']]);
  assert.equal(calls.length, 35);
  assert.ok(calls.every(([, , query]) => query.from === '' && query.to === ''));
  assert.equal(Object.keys(result.snapshot.details.medicine).length, 31);
  assert.equal(Object.keys(result.snapshot.details.hospital).length, 2);
  assert.equal(result.expired, false);
});

test('transient list failure retains prior data while other categories succeed and resume only fetches failures', async () => {
  const snapshot = emptyHealthData();
  snapshot.reports.medicine = report('medicine', [entry('舊成功資料')]);
  const first = await acquireHealthData(options({ snapshot, fetchReport: async category => {
    if (category === 'medicine') throw new Error('protocol change');
    return report(category, []);
  } }));
  assert.equal(first.snapshot.reports.medicine.entries[0].title, '舊成功資料');
  assert.equal(first.snapshot.states.medicine.status, 'failed');
  assert.equal(first.snapshot.states.hospital.status, 'ready');
  const calls = [];
  const second = await acquireHealthData(options({ mode: 'resume', snapshot: first.snapshot, fetchReport: async category => { calls.push(category); return report(category, []); } }));
  assert.deepEqual(calls, ['medicine']);
  assert.equal(second.snapshot.states.medicine.status, 'ready');
});

test('detail failure preserves earlier successes and independent later details; resume skips successes', async () => {
  const initial = await acquireHealthData(options({ categories: ['medicine'], fetchReport: async (category, reference) => {
    if (reference === 'b') throw new Error('unavailable');
    return reference ? report(`${category}_detail`, [entry(reference)]) : report(category, ['a', 'b', 'c'].map(id => entry(id, '', { detail_id: id })));
  } }));
  assert.deepEqual(Object.keys(initial.snapshot.details.medicine), ['a', 'c']);
  assert.equal(initial.snapshot.states.medicine.detail_errors.b, 'unavailable');
  const calls = [];
  const resumed = await acquireHealthData(options({ snapshot: initial.snapshot, categories: ['medicine'], mode: 'resume', fetchReport: async (category, reference) => {
    calls.push(reference); return report(`${category}_detail`, [entry(reference)]);
  } }));
  assert.deepEqual(calls, ['b']);
  assert.deepEqual(resumed.snapshot.states.medicine.detail_errors, {});
});

test('cancellation prevents a late response replacing data and resume continues from completed requests', async () => {
  const controller = new AbortController();
  const updates = [];
  let release;
  const deferred = new Promise(resolve => { release = resolve; });
  const task = acquireHealthData(options({ signal: controller.signal, categories: ['medicine'], onUpdate: data => updates.push(data),
    fetchReport: async (category, reference) => reference ? deferred : report(category, [entry('a', '', { detail_id: 'a' })]) }));
  await new Promise(resolve => setImmediate(resolve));
  controller.abort(); release(report('medicine_detail', [entry('過時資料')]));
  const result = await task;
  assert.equal(Object.keys(result.snapshot.details.medicine).length, 0);
  assert.equal(updates.at(-1).reports.medicine.entries.length, 1);
  assert.equal(Object.keys(updates.at(-1).details.medicine).length, 0);
});

test('authentication expiry clears all accumulated categories and stops further requests', async () => {
  const calls = [];
  const result = await acquireHealthData(options({ categories: ['medicine', 'hospital', 'adult'], fetchReport: async category => {
    calls.push(category); if (category === 'hospital') throw new Error('expired'); return report(category, [entry('private synthetic')]);
  } }));
  assert.equal(result.expired, true);
  assert.deepEqual(result.snapshot, emptyHealthData());
  assert.deepEqual(calls, ['medicine', 'hospital']);
});

test('only classified transient failures receive one bounded retry', async () => {
  let attempts = 0;
  const result = await acquireHealthData(options({ categories: ['medicine'], fetchReport: async () => { attempts++; throw new Error('transient'); } }));
  assert.equal(attempts, 2);
  assert.equal(result.snapshot.states.medicine.status, 'failed');
  assert.equal(result.snapshot.reports.medicine, undefined);
});

test('nested detail references are acquired once, remain visible and retain explicit parent linkage', async () => {
  const calls = [];
  const result = await acquireHealthData(options({ categories: ['medicine'], fetchReport: async (category, reference) => {
    calls.push(reference);
    if (!reference) return report(category, [entry('來源', '', { detail_id: 'parent' })]);
    if (reference === 'parent') return report(`${category}_detail`, [entry('日記錄', '', { detail_id: 'child' }), entry('同一日連結', '', { detail_id: 'child' })]);
    return report(`${category}_detail`, [entry('原值', '', { detail_id: 'parent', fields: [{ name: '量測', value: '1.0000000001', unit: '原單位' }] })]);
  } }));
  assert.deepEqual(calls, ['', 'parent', 'child']);
  assert.equal(categoryCoverage(result.snapshot, 'medicine').loadedCount, 2);
  assert.equal(categoryCoverage(result.snapshot, 'medicine').pendingCount, 0);
  const events = healthEvents(result.snapshot, catalog);
  assert.equal(events.length, 4);
  assert.equal(events.find(event => event.entry.title === '原值').parentReference, 'child');
  assert.equal(events.find(event => event.entry.title === '原值').entry.fields[0].value, '1.0000000001');
});

test('resuming cancelled refresh re-fetches its list instead of pretending older data fulfilled the new query', async () => {
  const snapshot = emptyHealthData();
  snapshot.reports.medicine = report('medicine', [entry('舊資料')]);
  snapshot.states.medicine = { status: 'cancelled', detail_errors: {} };
  const calls = [];
  const result = await acquireHealthData(options({ snapshot, mode: 'resume', categories: ['medicine'], query: { from: '2020-01-01', to: '2026-10-28' },
    fetchReport: async (category, reference, query) => { calls.push([reference, query]); return report(category, [entry('新資料')]); } }));
  assert.equal(calls.length, 1);
  assert.deepEqual(calls[0], ['', { from: '2020-01-01', to: '2026-10-28' }]);
  assert.equal(result.snapshot.reports.medicine.entries[0].title, '新資料');
});

test('resume retains each failed category requested range after another category was refreshed with all-history', async () => {
  const customQuery = { from: '2020-01-01', to: '2020-12-31' };
  const snapshot = emptyHealthData();
  snapshot.states.medicine = { status: 'failed', error: 'previous failure', detail_errors: {}, query: customQuery };
  snapshot.reports.hospital = report('hospital', []);
  snapshot.states.hospital = { status: 'ready', detail_errors: {}, query: { from: '', to: '' } };
  const calls = [];
  await acquireHealthData(options({ snapshot, mode: 'resume', fetchReport: async (category, reference, query) => {
    calls.push([category, reference, query]); return report(category, []);
  } }));
  assert.deepEqual(calls, [['medicine', '', customQuery]]);
});

test('aggregate source limit rejects a whole new response and preserves earlier successful data', async () => {
  const result = await acquireHealthData(options({ limits: { entries: 3, detailRequests: 100 }, fetchReport: async category =>
    report(category, [entry('first'), entry('second')]) }));
  assert.equal(healthDataEntryCount(result.snapshot), 2);
  assert.equal(result.snapshot.reports.medicine.entries.length, 2);
  assert.equal(result.snapshot.reports.hospital, undefined);
  assert.equal(result.snapshot.states.hospital.status, 'failed');
  assert.match(result.snapshot.states.hospital.error, /容量上限/);
});

test('detail capacity limit retains completed siblings and leaves rejected detail explicitly pending', async () => {
  const result = await acquireHealthData(options({ categories: ['medicine'], limits: { entries: 3, detailRequests: 100 },
    fetchReport: async (category, reference) => reference ? report(`${category}_detail`, [entry(reference)]) :
      report(category, [entry('a', '', { detail_id: 'a' }), entry('b', '', { detail_id: 'b' })]) }));
  assert.equal(healthDataEntryCount(result.snapshot), 3);
  assert.deepEqual(Object.keys(result.snapshot.details.medicine), ['a']);
  assert.match(result.snapshot.states.medicine.detail_errors.b, /容量上限/);
  assert.deepEqual(pendingDetailReferences(result.snapshot, 'medicine'), ['b']);
});

test('fresh recursive references stop at an explicit per-run request budget and can resume', async () => {
  const calls = [];
  const fetchReport = async (category, reference) => {
    calls.push(reference);
    const next = reference ? String(Number(reference) + 1) : '1';
    return report(category, [entry(next, '', { detail_id: next })]);
  };
  const first = await acquireHealthData(options({ categories: ['medicine'], limits: { entries: 100, detailRequests: 3 }, fetchReport }));
  assert.deepEqual(calls, ['', '1', '2', '3']);
  assert.deepEqual(pendingDetailReferences(first.snapshot, 'medicine'), ['4']);
  assert.match(first.snapshot.states.medicine.detail_errors['4'], /請求上限/);
  const resumed = await acquireHealthData(options({ categories: ['medicine'], snapshot: first.snapshot, mode: 'resume', limits: { entries: 100, detailRequests: 2 }, fetchReport }));
  assert.deepEqual(calls.slice(4), ['4', '5']);
  assert.deepEqual(pendingDetailReferences(resumed.snapshot, 'medicine'), ['6']);
});

test('refreshed detail prunes retired descendants but keeps a child reachable from another parent', () => {
  const snapshot = emptyHealthData();
  snapshot.reports.medicine = report('medicine', [entry('p', '', { detail_id: 'parent' }), entry('q', '', { detail_id: 'other' })]);
  snapshot.details.medicine = {
    parent: report('medicine_detail', [entry('old child', '', { detail_id: 'retired' }), entry('shared', '', { detail_id: 'shared' })]),
    other: report('medicine_detail', [entry('other link', '', { detail_id: 'shared' })]),
    retired: report('medicine_detail', [entry('retired leaf')]), shared: report('medicine_detail', [entry('shared leaf')]),
  };
  snapshot.states.medicine = { status: 'ready', detail_errors: { retired: 'old failure', shared: 'shared failure' } };
  const next = replaceDetailReport(snapshot, 'medicine', 'parent', report('medicine_detail', [entry('new child', '', { detail_id: 'new' })]));
  assert.deepEqual(Object.keys(next.details.medicine), ['parent', 'other', 'shared']);
  assert.deepEqual(next.states.medicine.detail_errors, { shared: 'shared failure' });
  assert.deepEqual(pendingDetailReferences(next, 'medicine'), ['new']);
  assert.equal(snapshot.details.medicine.retired.entries[0].title, 'retired leaf');
  assert.throws(() => replaceDetailReport(snapshot, 'medicine', 'unknown', report('medicine_detail', [])), /不屬於目前清單/);
});

test('unified laboratory events preserve duplicate raw inequalities, source units and separate visit dates', () => {
  const record = { id: 'same-id', item_name: '合成檢驗原名', display_name: '合成檢驗', result: '>50',
    unit: 'mg/dL', reference_text: '<40', reference_low: null, reference_high: '40', official_flag: '1',
    exam_date: '2026-10-28', visit_date: '2026-10-20', institution: '合成院所', source: '合成檢驗來源',
    order_code: 'SYNTHETIC', order_name: '合成醫令' };
  const events = laboratoryEvents({ records: [record, { ...record }], fetched_at: '2026-10-28', complete: false, warnings: [] });
  assert.equal(events.length, 2);
  assert.notEqual(events[0].id, events[1].id);
  assert.equal(events[0].category, 'laboratory');
  assert.equal(events[0].date, '2026-10-28');
  assert.equal(events[0].parentReference, '');
  assert.deepEqual(events[0].entry.fields.find(field => field.name === '原始結果'), { name: '原始結果', value: '>50', unit: 'mg/dL' });
  assert.equal(events[0].entry.fields.find(field => field.name === '就醫日期（非採檢日期）').value, '2026-10-20');
  assert.equal(events[0].entry.fields.find(field => field.name === '原院旗標（原文）').value, '1');
  assert.equal(filterHealthEvents(events, { ...noFilter, text: '>50', category: 'laboratory' }).length, 2);
});

test('known source timestamps remain selectable by date while their full original values survive', () => {
  const snapshot = emptyHealthData();
  snapshot.reports.physiology = report('physiology', [entry('設備紀錄', '2025-01-02 08:30'), entry('自行記錄', '115/10/01 08:30')]);
  const events = healthEvents(snapshot, []);
  assert.equal(filterHealthEvents(events, { ...noFilter, from: '2025-01-02', to: '2025-01-02', includeUnknownDates: false }).length, 1);
  assert.equal(events[0].date, '2026-10-01');
  assert.equal(events[0].entry.date, '115/10/01 08:30');
});
