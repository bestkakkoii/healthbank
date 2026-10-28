// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import assert from 'node:assert/strict';
import test from 'node:test';
import { cachedSourceSnapshot } from '../src/cache-provenance.ts';
import { laboratoryCoverageStatus } from '../src/coverage-status.ts';
import { emptyHealthData } from '../src/health-data.ts';
import { exportDataset, importDataset } from '../src/health-dataset.ts';
import { createPrintReport, DEFAULT_PRINT_OPTIONS, renderPrintReport } from '../src/print-report.ts';

const fetched = '2024-12-20T01:02:03Z', saved = '2026-10-27T01:02:03Z', restored = '2026-10-28T01:02:03Z';
const category = [{ id: 'medicine', label: '合成處方', label_en: 'Synthetic prescriptions', group: '合成', range_mode: 'date_search' }];
const labRecord = { id: 'synthetic', item_id: 'serum_alt', configuration_key: 'serum_alt', category: 'liver', display_name: '合成 ALT',
  exam_date: '2024-01-02', visit_date: '', raw_check_date: '113/01/02', raw_visit_date: '', order_code: '09026C', order_name: '合成醫令',
  item_name: 'ALT', result: '27.000000000000000001', unit: 'U/L', reference_text: '[7][52]', institution: '合成院所', source: 'SYNTHETIC_ONLY',
  reference_status: 'within', interpretation: '', rule_source: '', level: 1, numeric_value: 27, value_kind: 'number', reference_low: '7', reference_high: '52' };
const provenance = { kind: 'device_cache', saved_at: saved, restored_at: restored, source_fetched_at: fetched, source_complete: true,
  source_query: { mode: 'all_history', start_date: '1912-01-01', end_date: '2024-12-20', reported_count: 2 } };
const lab = { records: [labRecord, { ...labRecord }], complete: false, fetched_at: fetched, warnings: [], cache_provenance: provenance };
const decode = value => importDataset(new TextEncoder().encode(typeof value === 'string' ? value : JSON.stringify(value)));
function originalSnapshot() {
  const snapshot = emptyHealthData();
  const row = { date: '113/01/02', title: '合成處方', institution: '合成院所', source: 'SYNTHETIC_ONLY', fields: [], detail_id: 'a'.repeat(48) };
  snapshot.reports.medicine = { category: 'medicine', entries: [row, { ...row }], coverage: '合成原始涵蓋說明', fetched_at: fetched, complete: true };
  snapshot.details.medicine = { ['a'.repeat(48)]: { category: 'medicine_detail', entries: [], coverage: '合成明細範圍', fetched_at: '2025-01-01T00:00:00Z', complete: false } };
  snapshot.states.medicine = { status: 'ready', detail_errors: {}, query: { from: '', to: '' } };
  return snapshot;
}

test('cache reopening preserves original source timestamps, completeness and duplicates without new official proof', () => {
  const original = originalSnapshot(), before = structuredClone(original);
  const cached = cachedSourceSnapshot(original, saved, restored);
  assert.deepEqual(original, before);
  assert.equal(cached.reports.medicine.complete, false);
  assert.equal(cached.reports.medicine.cache_provenance.source_complete, true);
  assert.equal(cached.reports.medicine.fetched_at, fetched);
  assert.equal(cached.reports.medicine.entries.length, 2);
  assert.equal(cached.details.medicine['a'.repeat(48)].cache_provenance.source_fetched_at, '2025-01-01T00:00:00Z');
  assert.equal(cached.details.medicine['a'.repeat(48)].cache_provenance.source_complete, false);
  const reopened = cachedSourceSnapshot(cached, saved, '2026-10-28T03:04:05Z');
  assert.equal(reopened.reports.medicine.cache_provenance.source_complete, true);
  assert.equal(reopened.reports.medicine.cache_provenance.restored_at, '2026-10-28T03:04:05Z');
  const status = laboratoryCoverageStatus(lab, 'official');
  assert.equal(status.kind, 'cached'); assert.equal(status.reportedCount, null);
  assert.match(status.summary, /裝置快取/); assert.match(status.summary, /未重新向官方/);
  assert.doesNotMatch(status.summary, /官方未提供/);
});

test('v2 archives round-trip cache provenance and editorial category labels without identity or current completeness claims', async () => {
  const content = await exportDataset(lab, cachedSourceSnapshot(originalSnapshot(), saved, restored), [], category);
  const restoredDataset = await decode(content);
  assert.deepEqual(restoredDataset.laboratoryMetadata.cache_provenance, provenance);
  assert.equal(restoredDataset.laboratoryMetadata.complete, false);
  assert.equal(restoredDataset.laboratoryMetadata.official_query, undefined);
  assert.equal(restoredDataset.snapshot.reports.medicine.cache_provenance.source_complete, true);
  assert.equal(restoredDataset.snapshot.reports.medicine.coverage, '合成原始涵蓋說明');
  assert.equal(restoredDataset.catalog[0].label_en, 'Synthetic prescriptions');
  for (const forbidden of ['profile_key', 'global_epoch', 'profile_epoch', 'csrf_token', 'session_token']) assert.ok(!content.includes(forbidden));
});

test('cache metadata cannot smuggle credentials, imply fresh verification or retrofit modern fields into v1', async () => {
  const original = JSON.parse(await exportDataset(lab, cachedSourceSnapshot(originalSnapshot(), saved, restored), [], category));
  const cases = [
    value => { value.laboratory.cache_provenance.profile_key = 'a'.repeat(64); },
    value => { value.laboratory.cache_provenance.password = 'SYNTHETIC_FORBIDDEN'; },
    value => { value.laboratory.complete = true; },
    value => { value.laboratory.official_query = provenance.source_query; },
    value => { value.laboratory.cache_provenance.source_fetched_at = '2026-01-01T00:00:00Z'; },
    value => { value.laboratory.cache_provenance.restored_at = '2026-02-30T01:02:03Z'; },
    value => { value.data.reports.medicine.complete = true; },
    value => { value.data.reports.medicine.cache_provenance.source_query = provenance.source_query; },
    value => { value.schema = 'healthbank.dataset.v1'; value.laboratory.schema = 'healthbank.lab.v1'; delete value.producer; },
  ];
  for (const change of cases) { const value = structuredClone(original); change(value); await assert.rejects(decode(value)); }
});

test('full and selected print reports disclose historical cache provenance without leaking excluded counts', () => {
  const snapshot = cachedSourceSnapshot(originalSnapshot(), saved, restored);
  for (const scope of ['all', 'selection']) {
    const data = createPrintReport({ report: lab, snapshot, categories: category, selectedLaboratory: [labRecord], mode: 'official',
      version: 'synthetic', generatedAt: restored, options: { ...DEFAULT_PRINT_OPTIONS, scope } });
    const html = renderPrintReport(data);
    assert.match(html, /裝置快取/); assert.match(html, /2024/); assert.match(html, /未重新向官方/);
    assert.ok(data.coverage.every(item => item.summary.includes('裝置快取')));
    if (scope === 'selection') assert.doesNotMatch(data.laboratoryCoverage, /保留 2 筆|官方回報 2 筆/);
  }
});
