// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import assert from 'node:assert/strict';
import test from 'node:test';
import { createTerminologyIndex, groupTerminology, groupTerminologySearchText, recordTerminology } from '../src/terminology-display.ts';
import { groupRecords } from '../src/model.ts';

function record(extra = {}) {
  return { id: 'synthetic-row', item_id: 'synthetic', configuration_key: 'synthetic',
    item_name: 'ACR', display_name: 'ACR', category: 'renal', unit: 'mg/g',
    result: '<12.000000000000001', exam_date: '2020-02-03', raw_check_date: '109/02/03',
    visit_date: '2020-02-01', order_code: 'SYNTHETIC_CODE', order_name: '合成醫令原文',
    value_kind: 'inequality', ...extra };
}

function item(extra = {}) {
  return { id: 'urine_acr_quantitative', configuration_key: 'urine_albumin_creatinine_ratio',
    display_name: '尿白蛋白／肌酸酐比值（定量）', display_name_en: 'Urine albumin-to-creatinine ratio (UACR)',
    terminology_note: '共用設定不代表方法等同。', terminology_sources: ['synthetic_source'],
    aliases: ['ACR', 'A/C ratio'], order_codes: ['09016C', '06012C'], builtin: true, ...extra };
}

function catalog(items = [], source_name_glossary = []) {
  return { items, sources: [{ id: 'synthetic_source', title: '合成術語來源', url: 'https://example.invalid/terms' }], source_name_glossary };
}

test('display terminology preserves channel names, records, grouping and exact source values', () => {
  const rows = [
    record({ id: 'synthetic-semi', item_id: 'urine_acr_semiquantitative', configuration_key: 'urine_albumin_creatinine_ratio', item_name: 'A/C ratio', display_name: '尿白蛋白／肌酸酐比值（半定量）' }),
    record({ id: 'synthetic-quant', item_id: 'urine_acr_quantitative', configuration_key: 'urine_albumin_creatinine_ratio', display_name: '尿白蛋白／肌酸酐比值（定量）' }),
  ];
  const before = structuredClone(rows);
  const groups = groupRecords(rows);
  const groupBefore = structuredClone(groups);
  const index = createTerminologyIndex(catalog([item()]));
  assert.equal(groups.length, 2);
  assert.equal(recordTerminology(rows[0], index).name, '尿白蛋白／肌酸酐比值（半定量）');
  assert.equal(recordTerminology(rows[1], index).englishName, 'Urine albumin-to-creatinine ratio (UACR)');
  for (const group of groups) groupTerminologySearchText(group, index);
  assert.deepEqual(rows, before);
  assert.deepEqual(groups, groupBefore);
});

test('lookup uses configuration key before exact item ID and never uses payment codes as identity', () => {
  const index = createTerminologyIndex(catalog([
    item(), item({ id: 'synthetic', configuration_key: 'different_key', display_name_en: 'Synthetic ID fallback' }),
  ]));
  assert.equal(recordTerminology(record({ configuration_key: 'urine_albumin_creatinine_ratio' }), index).englishName, 'Urine albumin-to-creatinine ratio (UACR)');
  assert.equal(recordTerminology(record({ configuration_key: '' }), index).englishName, 'Synthetic ID fallback');
  const unknown = record({ item_id: 'unknown', configuration_key: 'unknown', order_code: '09016C', item_name: 'Creatinine', display_name: 'Creatinine' });
  assert.equal(recordTerminology(unknown, index).basis, 'raw');
  assert.equal(recordTerminology(unknown, index).name, 'Creatinine');
});

test('current exact-ID wording corrects the display without rewriting an archived blood-gas record', () => {
  const raw = record({ item_id: 'venous_po2', configuration_key: 'venous_po2', display_name: '靜脈血氧分壓', item_name: 'PO2', unit: 'mmHg' });
  const index = createTerminologyIndex(catalog([item({ id: 'venous_po2', configuration_key: 'venous_po2', display_name: '血氣氧分壓', display_name_en: 'Blood-gas oxygen partial pressure (pO2)', terminology_note: '來源未確認動脈或靜脈檢體。' })]));
  assert.equal(recordTerminology(raw, index).name, '血氣氧分壓');
  assert.equal(raw.display_name, '靜脈血氧分壓');
  assert.equal(raw.item_name, 'PO2');
});

test('metadata and missing-only rows use only exact reviewed public glossary names', () => {
  const index = createTerminologyIndex(catalog([], [
    { source_names: ['pH'], display_name: '酸鹼值（檢體未註明）', display_name_en: 'pH (specimen unspecified)', terminology_sources: ['synthetic_source'], terminology_basis: 'primary_source_terms' },
    { source_names: ['是否人工閱片'], display_name: '人工閱片狀態', display_name_en: 'Manual slide review status', terminology_note: '來源狀態欄位直譯。', terminology_basis: 'literal_source_translation' },
    { source_names: ['Unattributed'], display_name: '不得猜測', display_name_en: 'Unattributed label', terminology_sources: ['missing_source'], terminology_basis: 'primary_source_terms' },
    { source_names: ['Unexplained'], display_name: '不得猜測', display_name_en: 'Unexplained literal label', terminology_basis: 'literal_source_translation' },
  ]));
  assert.equal(recordTerminology(record({ item_name: 'pH', value_kind: 'missing' }), index).englishName, 'pH (specimen unspecified)');
  assert.equal(recordTerminology(record({ item_name: 'PH', display_name: 'PH', value_kind: 'missing' }), index).basis, 'raw');
  const metadata = record({ item_name: '是否人工閱片', display_name: '是否人工閱片', category: 'metadata', value_kind: 'metadata', configuration_key: '' });
  assert.equal(recordTerminology(metadata, index).englishName, 'Manual slide review status');
  for (const item_name of ['Unattributed', 'Unexplained', 'Unknown'])
    assert.equal(recordTerminology(record({ item_name, display_name: item_name }), index).basis, 'raw');
});

test('search includes every source spelling, order name, code, English term and catalog alias', () => {
  const index = createTerminologyIndex(catalog([item({ id: 'synthetic', configuration_key: 'synthetic', display_name: '合成對照', display_name_en: 'Synthetic English name', aliases: ['Synthetic alternate spelling'], order_codes: ['SYNTHETIC_DECLARED_CODE'], builtin: false })]));
  const rows = [record({ item_name: 'FirstRaw', order_name: 'First synthetic order', order_code: 'SOURCE_A' }),
    record({ id: 'second-synthetic', item_name: 'SecondRaw', order_name: 'Second synthetic order', order_code: 'SOURCE_B' })];
  const group = groupRecords(rows)[0];
  const display = groupTerminology(group, index);
  const text = groupTerminologySearchText(group, index);
  assert.deepEqual(new Set(display.originalNames), new Set(['FirstRaw', 'SecondRaw']));
  for (const expected of ['firstraw', 'secondraw', 'first synthetic order', 'second synthetic order', 'source_a', 'source_b', 'synthetic english name', 'synthetic alternate spelling', 'synthetic_declared_code', 'mg/g'])
    assert.ok(text.includes(expected), `missing search term ${expected}`);
  assert.equal(group.id, 'synthetic\u0001mg/g');
  assert.equal(group.records.length, 2);
});

test('unobserved settings get names without fabricated original records', () => {
  const index = createTerminologyIndex(catalog([item()]));
  const group = { id: 'settings-only', configurationKey: 'urine_albumin_creatinine_ratio', name: '尿白蛋白／肌酸酐比值', abbreviation: 'ACR', unit: 'mg/g', category: 'renal', records: [] };
  assert.equal(groupTerminology(group, index).englishName, 'Urine albumin-to-creatinine ratio (UACR)');
  assert.deepEqual(groupTerminology(group, index).originalNames, []);
  assert.deepEqual(group.records, []);
});
