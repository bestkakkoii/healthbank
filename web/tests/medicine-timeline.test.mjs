// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import test from 'node:test';
import assert from 'node:assert/strict';
import { medicineSeries, medicalDate, prescribedDays, prescribedDaySegments } from '../src/medicine-timeline.ts';
const report = entries => ({ category: 'medicine_detail', fetched_at: '2026-10-27', coverage: '', entries, complete: false });
const row = (code, name, date, days, amount, source = '官方處方', key = '') => ({
  key, kind: 'medicine', date, institution: '合成院所', title: '藥品醫囑', source, fields: [],
  medicine: { code, name, quantity: amount, days },
});
test('medical dates accept ROC years and reject invalid/fictitious dates', () => {
  assert.equal(medicalDate('115/10/27'), '2026-10-27');
  assert.equal(medicalDate('2026-10-27'), '2026-10-27');
  assert.equal(medicalDate('115/2/30'), '');
  assert.equal(medicalDate('  '), '');
});
test('prescription intervals only parse known day counts and do not infer adherence', () => {
  for (const input of ['', '0', '-3', '5.5', 'one', '366', '12天']) assert.equal(prescribedDays(input), null);
  assert.equal(prescribedDays('28'), 28);
});
test('same official drug code aggregates, preserving identical-day duplicates and source rows', () => {
  const groups = medicineSeries([report([
    row('A001', '示範藥甲', '115/10/27', '28', '30'),
    row('A001', '示範藥甲', '115/10/27', '14', '20'),
    row('A001', '示範藥甲', '115/10/28', '30', '30'),
    row('A002', '示範藥甲', '115/10/28', '28', '30'),
    row('', '示範藥甲', '115/10/29', '30', '30'),
  ])]);
  assert.equal(groups.length, 3);
  const matched = groups.find(group => group.code === 'A001');
  assert.equal(matched.events.length, 3);
  assert.equal(matched.events.filter(event => event.date === '2026-10-27').length, 2);
  assert.equal(prescribedDaySegments(matched).length, 1);
});
test('unknown durations cannot be fabricated into dose curves', () => {
  const [group] = medicineSeries([report([row('R', '藥', '115/10/27', '', '3'), row('R', '藥', '115/10/28', '', '')])]);
  assert.ok(group);
  assert.equal(prescribedDaySegments(group).length, 0);
  assert.ok(group.events.every(event => event.validDays === null));
});
