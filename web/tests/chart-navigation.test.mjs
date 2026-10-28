// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import assert from 'node:assert/strict';
import test from 'node:test';
import { nextChartPointIndex } from '../src/chart-navigation.ts';

test('roving chart navigation traverses every observation, including coincident duplicates', () => {
  const rows = [{ id: 'a', date: '2026-01-01' }, { id: 'b', date: '2026-01-01' }, { id: 'c', date: '2026-01-02' }];
  let index = 0;
  const visited = [rows[index].id];
  for (let i = 1; i < rows.length; i++) { index = nextChartPointIndex('ArrowRight', index, rows.length); visited.push(rows[index].id); }
  assert.deepEqual(visited, ['a', 'b', 'c']);
  assert.equal(nextChartPointIndex('ArrowRight', index, rows.length), index);
  assert.equal(nextChartPointIndex('Home', index, rows.length), 0);
  assert.equal(nextChartPointIndex('End', 0, 500), 499);
  assert.equal(nextChartPointIndex('ArrowLeft', 0, 500), 0);
  assert.equal(nextChartPointIndex('ArrowUp', 20, 500), 19);
  assert.equal(nextChartPointIndex('ArrowDown', 20, 500), 21);
  assert.equal(nextChartPointIndex('Tab', 20, 500), null);
  assert.equal(nextChartPointIndex('Enter', 20, 500), null);
  assert.equal(nextChartPointIndex('Home', 0, 0), null);
});
