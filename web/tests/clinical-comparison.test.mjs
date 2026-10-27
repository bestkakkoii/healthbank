// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import assert from 'node:assert/strict';
import test from 'node:test';
import { scenarioInterval, egfrCreatinine2021, recordAnalyte, comparisonValue, compareInterval, exactAgeOn } from '../src/clinical-comparison.ts';
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
test('age is evaluated on each examination date, never from inferred birthdays', () => {
  assert.equal(exactAgeOn('1956-10-28', '2026-10-27'), 69);
  assert.equal(exactAgeOn('1956-10-28', '2026-10-28'), 70);
  assert.equal(exactAgeOn('1956-10-28', '2025-10-28'), 69);
  assert.equal(exactAgeOn('2000-02-29', '2023-03-01'), 23);
  for (const [birth, exam] of [['2020-02-30','2026-01-01'],['1956-10-28',''],['2027-01-01','2026-01-01']]) assert.equal(exactAgeOn(birth, exam), null);
});
