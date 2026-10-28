// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import assert from 'node:assert/strict';
import test from 'node:test';
import { finiteExtent, groupRecords, isExact, rulePreview, toCsv } from '../src/model.ts';

function observation(patch = {}) {
  return {
    id: 'synthetic-1', item_id: 'urine_ph', configuration_key: 'urine_ph',
    item_name: 'PH', display_name: '尿液酸鹼值', category: 'urinalysis',
    exam_date: '2026-01-20', visit_date: '2026-01-19', order_code: '06012C',
    order_name: '合成檢驗', result: '5.0', unit: '無單位', reference_low: '4.5',
    reference_high: '7.5', reference_text: '[4.5][7.5]', institution: '合成示範院所',
    source: 'SYNTHETIC', level: 1, reference_status: 'within', interpretation: '合成測試',
    numeric_value: 5, value_kind: 'numeric', rule_source: 'original',
    ...patch,
  };
}

test('CSV neutralizes formulas, including expressions beginning with a negative number', () => {
  for (const result of ['-1+2', '-1+HYPERLINK("https://example.invalid","x")', '=1+2', '+1+2', '@SUM(1)', '\t-1+2', '-1e309']) {
    const csv = toCsv([observation({ result })]);
    assert.ok(csv.includes(`,"'${result.replaceAll('"', '""')}",`), result);
  }
});

test('CSV preserves finite negative decimal and scientific results without formula-prefix changes', () => {
  for (const result of ['-1', '-1.3', '-.5', '-1e-3', '-1.2E+3']) {
    const csv = toCsv([observation({ result })]);
    assert.ok(csv.includes(`,"${result}",`), result);
    assert.ok(!csv.includes(`,"'${result}",`), result);
  }
});

test('CSV preserves commas, quotes, line breaks, source flags and same-day duplicates', () => {
  const first = observation({ result: '文字, "註記"\n第二行', official_flag: '1' });
  const second = observation({ id: 'synthetic-2', result: '6.1', numeric_value: 6.1 });
  const csv = toCsv([first, second]);
  assert.ok(csv.startsWith('\ufeff'));
  assert.ok(csv.includes('"文字, ""註記""\n第二行"'));
  assert.equal(csv.match(/\r\n"2026-01-20",/g)?.length, 2);
  assert.ok(csv.includes('"原院異常標記（原始值）"'));
  assert.equal(csv.match(/bestkakkoii llc\./g)?.length, 2);
});

test('CSV retains raw date strings alongside normalized date columns', () => {
  const csv = toCsv([observation({ raw_check_date: '115/01/20 08:30', raw_visit_date: '原始日期未明' })]);
  assert.ok(csv.includes('"檢查日期（來源原文）","就醫日期（來源原文）"'));
  assert.ok(csv.includes('\r\n"2026-01-20",'));
  assert.ok(csv.endsWith(',"115/01/20 08:30","原始日期未明"'));
});

test('grouping preserves source identities, units, every duplicate and original input order', () => {
  const records = [
    observation(),
    observation({ id: 'synthetic-2', item_id: 'venous_ph', configuration_key: 'venous_ph', item_name: 'pH', result: '7.37' }),
    observation({ id: 'synthetic-3', result: '5.5' }),
    observation({ id: 'synthetic-4', unit: 'different-unit' }),
  ];
  const before = structuredClone(records);
  const groups = groupRecords(records);
  assert.equal(groups.length, 3);
  assert.equal(groups.reduce((count, group) => count + group.records.length, 0), 4);
  assert.deepEqual(groups.find((group) => group.configurationKey === 'urine_ph' && group.unit === '無單位')?.records.map((record) => record.id), ['synthetic-1', 'synthetic-3']);
  assert.deepEqual(records, before);
  assert.deepEqual(groupRecords([]), []);
});

test('only finite exact results become chart points; ranges and bounds retain their original nature', () => {
  for (const patch of [
    { result: '≥5', numeric_value: 5 }, { result: '0–5', numeric_value: 5 },
    { result: '5', value_kind: 'range' }, { result: '5', value_kind: 'inequality' },
    { result: '5', numeric_value: null }, { result: '5', numeric_value: NaN },
    { result: '5', numeric_value: Infinity },
  ]) assert.equal(isExact(observation(patch)), false);
  assert.equal(isExact(observation({ result: '-1.2e-3', numeric_value: -0.0012 })), true);
});

test('rule previews reject malformed shapes without manufacturing clinical bands', () => {
  for (const value of [null, false, [], 'text', 5]) assert.deepEqual(rulePreview(value).bands, []);
  const band = { lower: '0', upper: '1', lower_inclusive: true, upper_inclusive: false, display_level: 1, category: 'example' };
  const preview = rulePreview({ bands: [band, null, { ...band, display_level: 6 }, { ...band, lower: 0 }], sources: ['verified', null, {}] });
  assert.deepEqual(preview.bands, [band]);
  assert.deepEqual(preview.sources, ['verified']);
});

test('chart axes accept a full 50,000-record history with two references per result', () => {
  const values = Array.from({ length: 150_000 }, (_, index) => index - 75_000);
  assert.deepEqual(finiteExtent(values), [-75_000, 74_999]);
  assert.deepEqual(finiteExtent([NaN, Infinity, -Infinity, -1.3, 2.7]), [-1.3, 2.7]);
  assert.equal(finiteExtent([]), null);
  assert.equal(finiteExtent([NaN, Infinity]), null);
});
