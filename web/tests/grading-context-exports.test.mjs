// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import test from 'node:test';
import assert from 'node:assert/strict';
import { rulePreview, toCsv } from '../src/model.ts';
import { portableReportJSON } from '../src/portable-report.ts';
import { createPrintReport, renderPrintReport, DEFAULT_PRINT_OPTIONS } from '../src/print-report.ts';
import { emptyHealthData } from '../src/health-data.ts';

const context = (note, kind = 'profile') => ({ kind, rule_id: 'synthetic_rule', label: '合成條件範本（未確認）', note });
const record = (index, contexts) => ({ id: `synthetic-context-${index}`, item_id: 'synthetic', configuration_key: 'synthetic',
  item_name: '合成原名', display_name: '合成項目', category: 'other', exam_date: '2024-03-07', visit_date: '2024-03-06',
  order_code: 'SYNTHETIC_ONLY', order_name: '合成醫令', result: '37.000000000000000001', numeric_value: 37,
  unit: 'mg/dL', reference_low: '1', reference_high: '10', reference_text: '[1][10]', institution: '合成院所',
  source: 'SYNTHETIC_ONLY', level: 4, reference_status: 'high', official_flag: '1', value_kind: 'numeric',
  interpretation: '相同合成說明', rule_source: 'synthetic_rule', grading_contexts: contexts });

test('rule previews retain applicability and valid condition strings without inventing missing metadata', () => {
  const value = { scope_label: '合成適用範圍', applicability_note: '須另核對條件', conditions: ['合成條件 A', null, 7, '合成條件 B'] };
  assert.equal(rulePreview(value).scopeLabel, value.scope_label);
  assert.equal(rulePreview(value).applicabilityNote, value.applicability_note);
  assert.deepEqual(rulePreview(value).conditions, ['合成條件 A', '合成條件 B']);
  assert.deepEqual(rulePreview({ conditions: 'unknown', scope_label: 3 }).conditions, []);
  assert.equal(rulePreview({}).scopeLabel, '');
});

test('derived CSV retains all qualification notes and original flags while raw portable rows remain recomputable', () => {
  const rows = [record(1, [context('合成條件第一行\n方法與病程未確認。'), context('合成獨立通報條件；不等於原院危急值。', 'independent_alert')])];
  const before = structuredClone(rows), csv = toCsv(rows);
  assert.match(csv, /分級依據與適用限制/);
  assert.ok(csv.includes('合成條件第一行\n方法與病程未確認。'));
  assert.ok(csv.includes('合成獨立通報條件；不等於原院危急值。'));
  assert.ok(csv.includes('37.000000000000000001'));
  assert.ok(csv.includes('"原院異常標記（原始值）"'));
  const raw = JSON.parse(portableReportJSON(rows));
  assert.equal(raw.schema, 'healthbank.lab.v2');
  assert.equal(raw.records[0].official_flag, '1');
  assert.equal(Object.hasOwn(raw.records[0], 'grading_contexts'), false);
  assert.equal(Object.hasOwn(raw.records[0], 'level'), false);
  assert.deepEqual(rows, before);
});

test('both print layouts preserve full qualifications and index different conditions separately', () => {
  const rows = [record(1, [context('合成條件 A：年齡未確認。')]), record(2, [context('合成條件 B：方法未確認。')]),
    record(3, [context('合成條件 A：年齡未確認。')])];
  const input = { report: { records: rows, fetched_at: '', complete: false, warnings: [] }, snapshot: emptyHealthData(),
    categories: [], selectedLaboratory: rows, mode: 'import', version: 'synthetic-test', generatedAt: '2026-10-28T00:00:00Z' };
  for (const layout of ['records', 'comparison']) {
    const html = renderPrintReport(createPrintReport({ ...input, options: { ...DEFAULT_PRINT_OPTIONS, layout } }));
    assert.ok(html.includes('合成條件 A：年齡未確認。'));
    assert.ok(html.includes('合成條件 B：方法未確認。'));
    assert.equal((html.match(/id="interpretation-/g) ?? []).length, 2);
    assert.equal((html.match(/href="#interpretation-1"/g) ?? []).length, 2);
    assert.equal((html.match(/37\.000000000000000001/g) ?? []).length, 3);
    assert.equal((html.match(/原院旗標：1/g) ?? []).length, 3);
  }
  const selected = renderPrintReport(createPrintReport({ ...input, selectedLaboratory: [rows[0]], options: { ...DEFAULT_PRINT_OPTIONS, scope: 'selection' } }));
  assert.ok(!selected.includes('合成條件 B'), 'Qualifications from excluded observations must not leak into selected reports.');
});

test('untrusted display metadata remains inert in HTML and formula-safe in CSV', () => {
  const rows = [record(1, [{ ...context('<script>synthetic()</script>'), label: '=SYNTHETIC_FORMULA()' }])];
  assert.ok(toCsv(rows).includes('"\'=SYNTHETIC_FORMULA()：'));
  const html = renderPrintReport(createPrintReport({ report: { records: rows, fetched_at: '', complete: false, warnings: [] },
    snapshot: emptyHealthData(), categories: [], selectedLaboratory: rows, mode: 'import', version: 'synthetic-test',
    generatedAt: '2026-10-28T00:00:00Z', options: { ...DEFAULT_PRINT_OPTIONS } }));
  assert.ok(html.includes('&lt;script&gt;synthetic()&lt;/script&gt;'));
  assert.ok(!html.includes('<script>'));
});
