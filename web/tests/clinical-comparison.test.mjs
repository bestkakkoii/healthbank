// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import assert from 'node:assert/strict';
import test from 'node:test';
import { scenarioInterval, egfrCreatinine2021, estimatedGFRLabel, recordAnalyte, comparisonInput, comparisonValue, compareInterval, exactAgeOn } from '../src/clinical-comparison.ts';
import { canonicalUnit } from '../src/unit-normalization.ts';
function record(patch = {}) { return { id: 'synthetic', item_name: 'Creatinine', item_id: 'serum_creatinine', order_code: '09015C',
  result: '1', unit: 'mg/dL', numeric_value: 1, value_kind: 'numeric', official_flag: 'H', ...patch }; }
const male = age => ({ age, sex: 'male' });
const female = age => ({ age, sex: 'female' });
test('source age boundaries and sex-specific constants do not become invented elderly bands', () => {
  assert.deepEqual([scenarioInterval('alp', male(18)).low, scenarioInterval('alp', male(18)).high], [55, 149]);
  assert.deepEqual([scenarioInterval('alp', female(18)).low, scenarioInterval('alp', female(18)).high], [45, 87]);
  assert.equal(scenarioInterval('alp', male(19)).low, 40);
  assert.equal(scenarioInterval('alp', female(19)).high, 104);
  assert.deepEqual(scenarioInterval('alp', male(60)), scenarioInterval('alp', male(80)));
  assert.equal(scenarioInterval('creatinine', male(14)).high, 0.77);
  assert.equal(scenarioInterval('creatinine', male(15)).high, 1.17);
  assert.equal(scenarioInterval('creatinine', female(15)).low, 0.51);
  for (const age of [null, NaN, Infinity, 0, -1, 12.5, 131]) assert.equal(scenarioInterval('alp', male(age)), null);
  assert.equal(scenarioInterval('alp', { age: 70, sex: 'unknown' }), null);
  assert.notEqual(scenarioInterval('alp', { age: 9, sex: 'unknown' }), null);
});
test('specimen/code, units, exactness and raw numeric consistency gate calculations', () => {
  assert.equal(recordAnalyte(record()), 'creatinine');
  for (const patch of [{ order_code: '09016C' }, { item_name: 'eGFR' }, { unit: '無單位' },
    { result: '≥1' }, { value_kind: 'inequality' }, { numeric_value: Infinity }, { result: '2', numeric_value: 1 },
    { result: '-1', numeric_value: -1 }]) assert.equal(comparisonValue(record(patch), 'creatinine'), null);
  assert.equal(comparisonValue(record({ result: '88.4', numeric_value: 88.4, unit: 'µmol/L' }), 'creatinine'), 1);
});
test('unit prefix case stays significant while explicit source spelling aliases remain usable', () => {
  for (const unit of ['Mg/dL', 'MG/DL', 'Umol/L', 'UMOL/L'])
    assert.equal(comparisonValue(record({ unit }), 'creatinine'), null);
  assert.equal(comparisonValue(record({ unit: 'mg/dl' }), 'creatinine'), 1);
  for (const unit of ['µmol/L', 'μmol/l', 'umol/L']) {
    const decision = comparisonInput(record({ unit, result: '88.4', numeric_value: 88.4 }), 'creatinine');
    assert.equal(decision.value, 1);
    assert.equal(decision.sourceUnit, unit);
    assert.equal(decision.conversion, 'creatinine_umol_to_mg');
  }
  assert.notEqual(canonicalUnit('ML/min/1.73m2'), canonicalUnit('mL/min/1.73m2'));
  assert.notEqual(canonicalUnit('u/L'), canonicalUnit('U/L'));
  assert.equal(canonicalUnit('mL / min / 1.73m²'), 'mL/min/1.73m2');
});
test('derived eGFR rounds only its presentation and keeps full calculation precision', () => {
  const derived = egfrCreatinine2021(1.25, male(70), true);
  assert.ok(Math.abs(derived.value - 61.946505014981526) < 1e-11);
  assert.equal(estimatedGFRLabel(derived.value), '62');
  assert.equal(estimatedGFRLabel(null), '未計算');
  assert.equal(estimatedGFRLabel(Infinity), '未計算');
  assert.equal(estimatedGFRLabel(-1), '未計算');
  assert.equal(derived.value, egfrCreatinine2021(1.25, male(70), true).value);
});
test('unit-normalized interval comparison is exact at and immediately outside decimal endpoints', () => {
  const interval = scenarioInterval('creatinine', male(70));
  const value = (raw) => record({ unit: 'µmol/L', result: raw, numeric_value: Number(raw) });
  assert.equal(compareInterval(value('59.228'), 'creatinine', interval), '位於情境範本內');
  assert.equal(compareInterval(value('59.22799999999999'), 'creatinine', interval), '低於情境範本');
  assert.equal(compareInterval(value('103.428'), 'creatinine', interval), '位於情境範本內');
  assert.equal(compareInterval(value('103.42800000000001'), 'creatinine', interval), '高於情境範本');
  assert.equal(compareInterval(undefined, 'creatinine', interval), '無法比較');
});
test('adult CKD-EPI gating and equation anchor retain original observations', () => {
  const original = Object.freeze(record());
  assert.equal(egfrCreatinine2021(1, male(18), true).value, 142 * (1 / 0.9) ** -1.2 * 0.9938 ** 18);
  assert.equal(egfrCreatinine2021(0.7, female(40), true).value, 142 * 0.9938 ** 40 * 1.012);
  assert.equal(egfrCreatinine2021(1, male(17), true).value, null);
  assert.equal(egfrCreatinine2021(1, male(70), false).value, null);
  for (const value of [null, 0, -1, NaN, Infinity]) assert.equal(egfrCreatinine2021(value, male(70), true).value, null);
  assert.ok(egfrCreatinine2021(1, male(70), true).value > egfrCreatinine2021(1, male(80), true).value);
  comparisonValue(original, 'creatinine'); assert.equal(original.official_flag, 'H');
});

test('CKD-EPI reproduces independent high-precision vectors across both sex and creatinine branches', () => {
  // NIDDK 2021 creatinine equation, rechecked 2026-10-28. These synthetic
  // reference values were computed separately with Python Decimal, precision 70;
  // they do not call the JavaScript implementation or repeat its expression.
  const vectors = [
    [0.3, 18, 'female', '157.5916220089958337172092859272225932208467'],
    [0.7, 40, 'female', '112.0543067987974983147735102768754065592619'],
    [1.1, 70, 'female', '54.05610523366437414339315470494423666567039'],
    [4, 90, 'female', '10.13967026250502136565290667272541475774211'],
    [0.5, 18, 'male', '151.6220989088429452098054920086539272133474'],
    [0.9, 40, 'male', '110.7255996035548402319896346609440776277291'],
    [1.25, 70, 'male', '61.94650501498152614672265669345069030124808'],
    [5, 90, 'male', '10.36393468564113393310495760837769867692391'],
  ];
  for (const [creatinine, age, sex, expected] of vectors) {
    const actual = egfrCreatinine2021(creatinine, { age, sex }, true).value;
    assert.notEqual(actual, null);
    assert.ok(Math.abs(actual - Number(expected)) < 1e-11);
  }
});
test('age is evaluated on each examination date, never from inferred birthdays', () => {
  assert.equal(exactAgeOn('1956-10-28', '2026-10-27'), 69);
  assert.equal(exactAgeOn('1956-10-28', '2026-10-28'), 70);
  assert.equal(exactAgeOn('1956-10-28', '2025-10-28'), 69);
  assert.equal(exactAgeOn('2000-02-29', '2023-03-01'), 23);
  for (const [birth, exam] of [['2020-02-30','2026-01-01'],['1956-10-28',''],['2027-01-01','2026-01-01']]) assert.equal(exactAgeOn(birth, exam), null);
});
test('additional source partitions and one-sided limits are exact and bounded', () => {
 assert.equal(scenarioInterval('bun', male(17)).high, 18);
 assert.equal(scenarioInterval('bun', male(18)).high, 20);
 assert.equal(scenarioInterval('bun', male(59)).low, 6);
 assert.equal(scenarioInterval('bun', male(60)).low, 8);
 assert.equal(scenarioInterval('bun', male(89)).high, 23);
 assert.equal(scenarioInterval('bun', male(90)), null);
 assert.equal(scenarioInterval('calcium', male(1)), null);
 assert.equal(scenarioInterval('calcium', male(2)).low, 8.8);
 assert.equal(scenarioInterval('calcium', male(12)).low, 8.4);
 assert.equal(scenarioInterval('calcium', male(18)).high, 10);
 assert.equal(scenarioInterval('calcium', male(60)).high, 10.2);
 assert.equal(scenarioInterval('calcium', male(90)).high, 9.6);
 assert.equal(scenarioInterval('alt', male(70)).low, null);
 assert.equal(scenarioInterval('alt', female(70)).high, 35);
 assert.equal(scenarioInterval('ck', female(70)).high, 192);
 assert.equal(scenarioInterval('uric_acid', female(70)).high, 5.7);
 assert.equal(scenarioInterval('albumin', male(17)), null);
 assert.equal(recordAnalyte(record({order_code:'06012C',item_name:'Albumin'})), null);
 assert.equal(compareInterval(record({order_code:'09026C',item_name:'ALT',result:'0',numeric_value:0,unit:'U/L'}),'alt',scenarioInterval('alt',male(70))), '位於情境範本內');
});
