// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import test from 'node:test';
import assert from 'node:assert/strict';
import { createPrintReport, renderPrintReport, DEFAULT_PRINT_OPTIONS } from '../src/print-report.ts';
import { emptyHealthData } from '../src/health-data.ts';

const record = (index, extra = {}) => ({ id: `synthetic-${index}`, item_id: 'synthetic', configuration_key: 'synthetic', item_name: '合成項目', display_name: '合成項目', category: 'other', exam_date: '2026-01-01', raw_check_date: '115/01/01 08:30', visit_date: '', raw_visit_date: '', order_code: 'synthetic', order_name: '合成醫令', result: `${index}.0000000001`, unit: 'mg/dL', reference_low: null, reference_high: null, reference_text: '原院範圍', institution: '合成院所', source: '合成測試', level: null, reference_status: 'unknown', interpretation: '', numeric_value: null, value_kind: 'number', rule_source: '', ...extra });
function input(extra = {}) {
  return { report: { records: Array.from({ length: 501 }, (_, index) => record(index)), complete: false, warnings: [], fetched_at: '2026-10-28T00:00:00Z' }, snapshot: emptyHealthData(), categories: [], selectedLaboratory: [record(0)], mode: 'import', version: 'synthetic-test', generatedAt: '2026-10-28T00:00:00Z', options: { ...DEFAULT_PRINT_OPTIONS }, ...extra };
}

test('default printable report uses every actual row beyond viewport and pagination, preserving exact source text', () => {
  const source = input();
  const data = createPrintReport(source);
  const html = renderPrintReport(data);
  assert.equal(data.laboratory.length, 501);
  assert.equal((html.match(/data-laboratory-row=/g) ?? []).length, 501);
  assert.ok(html.includes('500.0000000001'));
  assert.ok(html.includes('115/01/01 08:30'));
  assert.ok(html.includes('檢驗 501 筆'));
  assert.equal(source.report.records.length, 501);
  assert.ok(!html.includes('session_token'));
});

test('explicit sharing scope restricts actual records and distinguishes unknown dates', () => {
  const selectedLaboratory = [record(1), record(2, { exam_date: '2025-01-01' }), record(3, { exam_date: '' })];
  const source = input({ selectedLaboratory, options: { ...DEFAULT_PRINT_OPTIONS, scope: 'selection', from: '2026-01-01', to: '2026-01-01', includeUnknownDates: false } });
  assert.deepEqual(createPrintReport(source).laboratory.map(row => row.id), ['synthetic-1']);
  source.options.includeUnknownDates = true;
  assert.deepEqual(createPrintReport(source).laboratory.map(row => row.id), ['synthetic-1', 'synthetic-3']);
  source.options.from = '2026-02-30';
  assert.throws(() => createPrintReport(source), /有效/);
});

test('print matrix retains all same-date duplicate observations across all column batches', () => {
  const records = Array.from({ length: 9 }, (_, index) => record(index, { item_id: `item${index}`, result: `exact-result-${index}` }));
  records.push({ ...records[0] });
  const data = createPrintReport(input({ report: { records, complete: false, warnings: [], fetched_at: '' }, options: { ...DEFAULT_PRINT_OPTIONS, layout: 'comparison' } }));
  const html = renderPrintReport(data);
  assert.equal((html.match(/class="comparison-section"/g) ?? []).length, 3);
  assert.equal((html.match(/exact-result-0/g) ?? []).length, 2);
  assert.equal((html.match(/class="observation"/g) ?? []).length, 10);
});

test('clinical report prints every source field and escapes active markup without carrying live capabilities', () => {
  const snapshot = emptyHealthData();
  const unsafe = '<script>fetch("https://example.invalid")</script><img src=x onerror=alert(1)>';
  snapshot.reports.medicine = { category: 'medicine', entries: Array.from({ length: 103 }, (_, index) => ({ date: '', title: `合成紀錄${index}`, institution: '合成院所', source: '合成', detail_id: 'secret-live-reference', fields: [{ name: '完整原文', value: unsafe + '長文字'.repeat(1000) }] })), coverage: '未取得獨立總數', complete: false, fetched_at: '' };
  const data = createPrintReport(input({ snapshot, options: { ...DEFAULT_PRINT_OPTIONS, laboratory: false }, categories: [{ id: 'medicine', label: '合成用藥', group: '合成', range_mode: 'source' }] }));
  const html = renderPrintReport(data);
  assert.equal(data.clinical.length, 103);
  assert.equal((html.match(/data-clinical-row=/g) ?? []).length, 103);
  assert.ok(html.includes('合成紀錄102'));
  assert.ok(!html.includes('<script>'));
  assert.ok(html.includes('&lt;script&gt;'));
  assert.ok(!html.includes('secret-live-reference'));
  assert.equal((html.match(/長文字/g) ?? []).length, 103000);
});

test('print completeness uses the same truthful mismatched-count model as the application', () => {
  const source = input({ mode: 'official' });
  source.report.official_query = { mode: 'all_history', start_date: '1912-01-01', end_date: '2026-10-28', reported_count: 1 };
  const data = createPrintReport(source);
  assert.match(data.laboratoryCoverage, /不一致/);
  assert.match(data.laboratoryCoverage, /501/);
  assert.doesNotMatch(data.laboratoryCoverage, /已完成筆數核對/);
});
