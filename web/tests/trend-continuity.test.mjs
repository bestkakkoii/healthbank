// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import assert from 'node:assert/strict';
import test from 'node:test';
import { dateTime, trendGeometry, createDatePosition } from '../src/trend-model.ts';
const make = (date, amount, identity) => ({ id: identity, exam_date: date,
  result: String(amount), numeric_value: amount, value_kind: 'numeric' });
test('first measurement dates connect even when same-date source duplicates occur', () => {
  const data = [make('2023-01-01', 80, 'first'), make('2023-01-01', 120, 'second'),
    make('2023-02-02', 100, 'third'), make('2023-03-03', 115, 'fourth')];
  const shape = trendGeometry(data);
  assert.equal(shape.buckets.length, 3);
  assert.deepEqual(shape.buckets.map(x => x.records.length), [2, 1, 1]);
  assert.equal(shape.buckets[0].low, 80);
  assert.equal(shape.buckets[0].high, 120);
  assert.equal(shape.links.length, 3);
  assert.equal(shape.links[0].ambiguous, true);
  assert.equal(shape.links.at(-1).to, '2023-03-03');
});
test('inequalities are not turned into fabricated numeric points', () => {
  const bounded = { ...make('2023-01-03', 150, 'inequality'), result: '>150', value_kind: 'lower_bound' };
  const shape = trendGeometry([make('2023-01-01', 90, 'low'), bounded, make('2023-01-04', 91, 'high')]);
  assert.equal(shape.buckets[1].exact.length, 0);
  assert.equal(shape.links.length, 0);
});
test('elapsed/ordinal modes preserve dates, while ordinal spreads dense early observations', () => {
  const dates = ['2022-01-01', '2022-01-02', '2026-12-01'];
  const first = dateTime(dates[0]), last = dateTime(dates[2]);
  assert.ok(Number.isFinite(first));
  assert.ok(Number.isNaN(dateTime('2026-02-30')));
  const time = createDatePosition(dates, [first, last], 'elapsed');
  const ordinal = createDatePosition(dates, [first, last], 'ordinal');
  assert.ok(time(dates[1]) < 0.01);
  assert.equal(ordinal(dates[1]), 0.5);
  assert.equal(time(dates[2]), 1);
  // A synchronized date without a source observation has no invented ordinal
  // coordinate; callers must leave its cursor hidden in that mode.
  assert.ok(Number.isNaN(ordinal('2023-01-01')));
  assert.ok(Number.isFinite(time('2023-01-01')));
});
