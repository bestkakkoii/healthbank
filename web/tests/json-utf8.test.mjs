// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import assert from 'node:assert/strict';
import test from 'node:test';
import { portableReportJSON } from '../src/portable-report.ts';
const original = [{ exam_date: '2026-10-27', order_code: '09004C', order_name: '三酸甘油脂',
  item_name: '三酸甘油酯', result: '1800', unit: 'mg/dL', reference_text: '<150',
  institution: '合成測試院所', visit_date: '2026-10-27', source: '測試來源', official_flag: 'H' }];
test('portable JSON uses unambiguous UTF-8 bytes and round-trips Chinese', () => {
  for (const editor of [false, true]) {
    const text = portableReportJSON(original, editor);
    const bytes = new TextEncoder().encode(text);
    assert.equal(bytes[0], editor ? 0xef : 0x7b);
    const decoded = new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(bytes).replace(/^\uFEFF/, '');
    const data = JSON.parse(decoded);
    assert.equal(data.records[0].item_name, '三酸甘油酯');
    assert.equal(data.records[0].institution, '合成測試院所');
    assert.equal(data.schema, 'healthbank.lab.v1');
  }
});
