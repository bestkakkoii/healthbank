// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import assert from 'node:assert/strict';
import test from 'node:test';
import { assessLipids } from '../src/lipid-analysis.ts';
const point = (code, value) => ({ id: code, order_code: code, result: String(value),
  numeric_value: value, value_kind: 'numeric', unit: 'mg/dL', exam_date: '2026-10-27',
  visit_date: '2026-10-27', institution: '合成測試院所' });
const tc = point('09001C', 250), hdl = point('09043C', 40);
test('TG >=400 never grants validity to Friedewald, and >800 blocks research estimates', () => {
  const moderate = assessLipids(tc, hdl, point('09004C', 400), true, false);
  assert.equal(moderate.nonHdl, '210');
  assert.equal(moderate.sampson, null);
  assert.match(moderate.message, /Friedewald/);
  const severe = assessLipids(tc, hdl, point('09004C', 1800), true, true);
  assert.equal(severe.nonHdl, '210');
  assert.equal(severe.sampson, null);
  assert.match(severe.message, /不估算LDL/);
  assert.equal(assessLipids(tc, hdl, { ...point('09004C', 250), visit_date: '2026-10-26' }, true, true).sampson, null);
});
