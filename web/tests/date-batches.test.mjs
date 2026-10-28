// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import test from 'node:test';
import assert from 'node:assert/strict';
import { splitDateBatches, selectedDateCategories, beginDateBatch, replaceBatchCategories, batchEntryAllowance,
  stoppedDateBatch, dateBatchStatus, assertBatchAdvance, batchSummary, missingBatchLaboratory } from '../src/date-batches.ts';
import { acquireHealthData } from '../src/data-acquisition.ts';
import { emptyHealthData, MAX_HEALTH_DATA_ENTRIES } from '../src/health-data.ts';
import { exportDataset, importDataset, DATASET_SCHEMA } from '../src/health-dataset.ts';

const range = { from: '2024-01-01', to: '2024-12-31' };
const entry = (title, extra = {}) => ({ title, date: '113/02/29', institution: '合成院所', source: '合成來源', fields: [], ...extra });
const report = (category, entries = []) => ({ category, entries, complete: false, fetched_at: '2026-10-28T00:00:00Z', coverage: '合成來源，總數未知' });
const catalog = [{ id: 'medicine', label: '用藥', group: '合成', range_mode: 'date_search' }, { id: 'chronic', label: '慢箋', group: '合成', range_mode: 'source' }];
const laboratory = { records: [], fetched_at: '2026-10-28T00:00:00Z', complete: false, warnings: ['合成來源總數未知'],
  official_query: { mode: 'custom', start_date: range.from, end_date: range.to, reported_count: null } };
const classifyFailure = error => ({ message: error.message, expired: error.message === 'expired', retryable: false });

test('annual/monthly inclusive batches preserve clipped boundaries, leap days and exact adjacency', () => {
  assert.deepEqual(splitDateBatches({ from: '2023-12-17', to: '2025-01-03' }, 'year'), [
    { from: '2023-12-17', to: '2023-12-31' }, { from: '2024-01-01', to: '2024-12-31' }, { from: '2025-01-01', to: '2025-01-03' }]);
  assert.deepEqual(splitDateBatches({ from: '2024-02-28', to: '2024-03-01' }, 'month'), [
    { from: '2024-02-28', to: '2024-02-29' }, { from: '2024-03-01', to: '2024-03-01' }]);
  for (const period of ['year', 'month']) {
    const source = { from: '1999-12-31', to: '2026-10-28' };
    const batches = splitDateBatches(source, period);
    assert.equal(batches[0].from, source.from); assert.equal(batches.at(-1).to, source.to);
    for (let i = 1; i < batches.length; i++) assert.equal(Date.parse(batches[i].from) - Date.parse(batches[i - 1].to), 86400000);
  }
  assert.equal(splitDateBatches({ from: '1912-01-01', to: '2200-12-31' }, 'year').length, 289);
  assert.throws(() => splitDateBatches({ from: '1912-01-01', to: '2200-12-31' }, 'month'), /600/);
  for (const invalid of [{ from: '', to: '' }, { from: '2023-02-29', to: '2024-01-01' }, { from: '2026-01-02', to: '2026-01-01' },
    { from: '1911-12-31', to: '2024-01-01' }, { from: '2024-01-01', to: '2201-01-01' }]) assert.throws(() => splitDateBatches(invalid, 'year'));
});

test('date batching excludes source-only and unknown category selections without changing the ordinary catalog', () => {
  const result = selectedDateCategories(catalog, ['medicine', 'chronic', 'medicine', 'unknown']);
  assert.deepEqual(result.eligible.map(value => value.id), ['medicine']);
  assert.deepEqual(result.excluded.map(value => value.id), ['chronic']);
  assert.equal(catalog.length, 2);
});

test('explicit next-batch replacement releases all old selected data while preserving unselected sources', () => {
  const previous = emptyHealthData();
  previous.reports.medicine = report('medicine', [entry('STALE_SELECTED')]);
  previous.details.medicine = { old: report('medicine_detail', [entry('STALE_DETAIL')]) };
  previous.states.medicine = { status: 'failed', detail_errors: { old: 'STALE_ERROR' } };
  previous.reports.chronic = report('chronic', [entry('UNSELECTED_PRESERVED')]);
  previous.details.chronic = { retain: report('chronic_detail', [entry('UNSELECTED_DETAIL')]) };
  const next = replaceBatchCategories(previous, beginDateBatch(range, ['medicine']), ['medicine']);
  assert.equal(next.reports.chronic, previous.reports.chronic); assert.equal(next.details.chronic, previous.details.chronic);
  assert.equal(next.reports.medicine, undefined); assert.equal(next.details.medicine, undefined);
  assert.equal(next.states.medicine.status, 'unqueried'); assert.deepEqual(next.states.medicine.query, range);
  assert.equal(JSON.stringify(next).includes('STALE_'), false);
  assert.equal(previous.reports.medicine.entries.length, 1);
  assert.equal(batchEntryAllowance(previous, ['medicine']), MAX_HEALTH_DATA_ENTRIES - 2);
});

test('advance requires every returned detail, this batch laboratory, export and a separate saved acknowledgement', () => {
  const batch = beginDateBatch(range, ['medicine']);
  const plan = { ranges: [range], categories: ['medicine'], includeLaboratory: true };
  assert.equal(dateBatchStatus(batch, plan.categories, true, laboratory).acquired, false);
  batch.reports.medicine = report('medicine', [entry('list', { detail_id: 'parent' })]);
  batch.states.medicine = { status: 'ready', detail_errors: {} };
  assert.throws(() => assertBatchAdvance(batch, plan, laboratory, true, true), /明細/);
  batch.details.medicine = { parent: report('medicine_detail', [entry('child', { detail_id: 'child' })]) };
  assert.equal(dateBatchStatus(batch, plan.categories, true, laboratory).detailsPending, 1);
  batch.details.medicine.child = report('medicine_detail', []);
  assert.throws(() => assertBatchAdvance(batch, plan, { ...laboratory, official_query: { ...laboratory.official_query, start_date: '2023-01-01' } }, true, true));
  assert.throws(() => assertBatchAdvance(batch, plan, laboratory, false, true), /匯出/);
  assert.throws(() => assertBatchAdvance(batch, plan, laboratory, true, false), /保存/);
  assert.doesNotThrow(() => assertBatchAdvance(batch, plan, laboratory, true, true));
  const summary = batchSummary(batch, laboratory);
  assert.deepEqual(summary, { ...range, entries: 2, laboratory: 0, sources: 1 });
  assert.equal(JSON.stringify(summary).includes('child'), false);
});

test('canceled batch retains successes and resumes only missing nested details with immutable date range', async () => {
  const abort = new AbortController();
  let checkpoint = beginDateBatch(range, ['medicine']);
  const calls = [];
  const fetchReport = async (category, reference, query) => {
    calls.push([category, reference, query]);
    return report(reference ? `${category}_detail` : category,
      reference === '' ? [entry('one', { detail_id: 'a' }), entry('duplicate'), entry('duplicate')] : reference === 'a' ? [entry('nested', { detail_id: 'b' })] : [entry('final')]);
  };
  const options = { categories: ['medicine'], query: range, classifyFailure, fetchReport, onProgress() {}, mode: 'refresh' };
  await acquireHealthData({ ...options, snapshot: checkpoint, signal: abort.signal, onUpdate(next) {
    checkpoint = next;
    if (next.details.medicine?.a) abort.abort();
  } });
  checkpoint = stoppedDateBatch(checkpoint);
  assert.equal(checkpoint.reports.medicine.entries.length, 3);
  assert.equal(checkpoint.details.medicine.b, undefined);
  calls.length = 0;
  const resumed = await acquireHealthData({ ...options, snapshot: checkpoint, mode: 'resume', signal: new AbortController().signal,
    onUpdate(next) { checkpoint = next; } });
  assert.deepEqual(calls.map(([, reference]) => reference), ['b']);
  assert.ok(calls.every(([, , query]) => query.from === range.from && query.to === range.to));
  assert.equal(dateBatchStatus(resumed.snapshot, ['medicine'], false, null).acquired, true);
  assert.equal(resumed.snapshot.reports.medicine.entries.filter(value => value.title === 'duplicate').length, 2);
});

test('batch archive contains only current successful data plus explicit failures and never stale laboratories', async () => {
  const current = beginDateBatch(range, ['medicine']);
  current.states.medicine = { status: 'failed', error: '合成來源失敗', detail_errors: {}, query: range };
  const content = await exportDataset(missingBatchLaboratory(range, true), current, [], [catalog[0]]);
  const decoded = JSON.parse(content);
  assert.equal(decoded.schema, DATASET_SCHEMA);
  assert.equal(decoded.laboratory.records.length, 0);
  assert.equal(decoded.laboratory.complete, false);
  assert.equal(decoded.laboratory.official_query, undefined);
  assert.match(decoded.laboratory.warnings[0], /尚未成功取得/);
  assert.deepEqual(Object.keys(decoded.data.reports), []);
  const restored = await importDataset(new TextEncoder().encode(content));
  assert.equal(restored.snapshot.states.medicine.status, 'failed');
  assert.deepEqual(restored.snapshot.query, range);
});
