// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import assert from 'node:assert/strict';
import test from 'node:test';
import { assessLipids } from '../src/lipid-analysis.ts';
const point = (code, value) => ({ id: code, order_code: code, result: String(value),
  item_name: { '09001C': 'Cholesterol', '09043C': 'HDL-Cholesterol', '09004C': 'Triglyceride' }[code],
  numeric_value: value, value_kind: 'numeric', unit: 'mg/dL', exam_date: '2026-10-27',
  visit_date: '2026-10-27', institution: '合成測試院所' });
const tc = point('09001C', 250), hdl = point('09043C', 40);
const rawTG = raw => ({ ...point('09004C', Number(raw)), result: raw });
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

test('Sampson coefficients reproduce independently calculated decimal reference values', () => {
  // JAMA Cardiology 2020 equation 2; reference vectors evaluated at 60-digit decimal precision.
  for (const [tg, expected] of [[50, '202.4860514242905968456477025277878589'], [400, '137.0350152601296866465416480783770503'], [800, '80.86737144518802530097944920451625038']]) {
    const result = assessLipids(Object.freeze(tc), Object.freeze(hdl), Object.freeze(point('09004C', tg)), true, true);
    assert.equal(result.nonHdl, '210');
    assert.notEqual(result.sampson, null);
    assert.ok(Math.abs(result.sampson - Number(expected)) < 1e-10);
    assert.match(result.message, /非直接量測/);
  }
});

test('TG applicability checks exact source decimals before floating-point rounding', () => {
  for (const raw of ['800.00000000000000001', '8.0000000000000000001e2', '801']) {
    const result = assessLipids(tc, hdl, rawTG(raw), true, true);
    assert.equal(result.nonHdl, '210');
    assert.equal(result.sampson, null);
    assert.match(result.message, /超出/);
  }
  for (const raw of ['799.99999999999999999', '800', '8e2']) {
    assert.notEqual(assessLipids(tc, hdl, rawTG(raw), true, true).sampson, null);
  }
  const justBelow400 = assessLipids(tc, hdl, rawTG('399.99999999999999999'), true, true);
  assert.doesNotMatch(justBelow400.message, /不適用Friedewald/);
  for (const raw of ['400', '400.00000000000000001']) {
    assert.match(assessLipids(tc, hdl, rawTG(raw), true, true).message, /不適用Friedewald/);
  }
});

test('LDL estimates require explicit opt-in and compatible exact TG from the confirmed collection', () => {
  assert.equal(assessLipids(tc, hdl, point('09004C', 400), false, true).nonHdl, null);
  assert.equal(assessLipids(tc, hdl, point('09004C', 400), true, false).sampson, null);
  assert.equal(assessLipids(tc, hdl, undefined, true, true).nonHdl, '210');
  for (const patch of [
    { unit: 'mmol/L' }, { unit: '' }, { order_code: '09001C' }, { item_name: 'Unrelated synthetic measurement' },
    { exam_date: '2026-10-26' }, { visit_date: '2026-10-26' }, { institution: 'OTHER SYNTHETIC' },
    { result: '>400', value_kind: 'inequality' }, { result: '400-500', value_kind: 'range' },
    { result: '-1', numeric_value: -1 }, { result: '1e-101', numeric_value: 1e-101 },
    { numeric_value: 1 }, { numeric_value: null }, { result: 'Infinity', numeric_value: Infinity },
  ]) {
    const result = assessLipids(tc, hdl, { ...point('09004C', 400), ...patch }, true, true);
    assert.equal(result.nonHdl, '210');
    assert.equal(result.sampson, null);
    assert.match(result.message, /TG須為/);
  }
});

test('lipid identity requires both original source name and code', () => {
  assert.equal(assessLipids({ ...tc, item_name: 'Unrelated synthetic measurement' }, hdl, point('09004C', 400), true, true).nonHdl, null);
  assert.equal(assessLipids(tc, { ...hdl, item_name: 'Unrelated synthetic measurement' }, point('09004C', 400), true, true).nonHdl, null);
});

test('known or suspected type III excludes Sampson without discarding original non-HDL inputs', () => {
  for (const tg of [100, 400, 800]) {
    const result = assessLipids(tc, hdl, point('09004C', tg), true, true, true);
    assert.equal(result.nonHdl, '210');
    assert.equal(result.sampson, null);
    assert.match(result.message, /原始研究排除第三型/);
  }
});

test('negative or above-non-HDL formula outputs are rejected without clipping', () => {
  for (const [total, highDensity, tg] of [[80, 79, 200], [200, 40, 0]]) {
    const result = assessLipids(point('09001C', total), point('09043C', highDensity), point('09004C', tg), true, true);
    assert.equal(result.nonHdl, String(total - highDensity));
    assert.equal(result.sampson, null);
    assert.match(result.message, /不合理估值/);
  }
});
