// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import test from 'node:test';
import assert from 'node:assert/strict';
import { medicineSeries, medicalDate, prescribedDays, prescribedDayBuckets, prescribedDaySegments } from '../src/medicine-timeline.ts';
const report = entries => ({ category: 'medicine_detail', fetched_at: '2026-10-27', coverage: '', entries, complete: false });
const row = (code, name, date, days, amount, source = '官方處方', key = '') => ({
  key, kind: 'medicine', date, institution: '合成院所', title: '藥品醫囑', source, fields: [],
  medicine: { code, name, quantity: amount, days },
});
test('medical dates accept ROC years and reject invalid/fictitious dates', () => {
  assert.equal(medicalDate('115/10/27'), '2026-10-27');
  assert.equal(medicalDate('2026-10-27'), '2026-10-27');
  assert.equal(medicalDate('1/1/1'), '1912-01-01');
  assert.equal(medicalDate('1911-12-31'), '1911-12-31');
  assert.equal(medicalDate('0000/1/1'), '');
  assert.equal(medicalDate('0/1/1'), '');
  assert.equal(medicalDate('113/2/29'), '2024-02-29');
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
  assert.deepEqual(prescribedDaySegments(matched).map(segment => [segment.from.validDays, segment.to.validDays]), [[14, 30], [28, 30]]);
  assert.deepEqual(prescribedDayBuckets(matched).map(bucket => [bucket.date, bucket.low.validDays, bucket.high.validDays]), [
    ['2026-10-27', 14, 28], ['2026-10-28', 30, 30],
  ]);
});
test('unknown durations cannot be fabricated into dose curves', () => {
  const [group] = medicineSeries([report([row('R', '藥', '115/10/27', '', '3'), row('R', '藥', '115/10/28', '', '')])]);
  assert.ok(group);
  assert.equal(prescribedDaySegments(group).length, 0);
  assert.ok(group.events.every(event => event.validDays === null));
});
test('unknown-duration dates break comparisons and same-day ordering never changes boundary links', () => {
  const rows = [row('R', '藥', '115/10/20', '7', ''), row('R', '藥', '115/10/20', '21', ''),
    row('R', '藥', '115/10/21', '14', ''), row('R', '藥', '115/10/22', '', ''),
    row('R', '藥', '115/10/23', '28', ''), row('R', '藥', '115/10/24', '30', '')];
  const segments = events => prescribedDaySegments(medicineSeries([report(events)])[0])
    .map(link => [link.from.date, link.from.validDays, link.to.date, link.to.validDays]);
  assert.deepEqual(segments(rows), [
    ['2026-10-20', 7, '2026-10-21', 14], ['2026-10-20', 21, '2026-10-21', 14],
    ['2026-10-23', 28, '2026-10-24', 30],
  ]);
  assert.deepEqual(segments(rows), segments(rows.toReversed()));
  const incompleteDay = [...rows, row('R', '藥', '115/10/21', '', '')];
  assert.deepEqual(segments(incompleteDay), [['2026-10-23', 28, '2026-10-24', 30]]);
});
test('name-only medicine records require identical names and source and preserve every original row', () => {
  const groups = medicineSeries([report([
    row('', 'Drug A', '115/1/1', '7', '', '處方'), row('', 'Drug A', '115/1/2', '7', '', '處方'),
    row('', 'drug a', '115/1/2', '7', '', '處方'), row('', 'Drug A', '115/1/2', '7', '', '領藥'),
  ])]);
  assert.equal(groups.length, 3);
  assert.equal(groups.reduce((total, group) => total + group.events.length, 0), 4);
});
