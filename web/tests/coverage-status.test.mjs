// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import assert from 'node:assert/strict';
import test from 'node:test';
import { laboratoryCoverageStatus, laboratoryDateSummary } from '../src/coverage-status.ts';

function report(count, total = null, complete = false) {
  return { records: Array.from({ length: count }, () => ({})), fetched_at: '2026-10-28T00:00:00Z', complete,
    official_query: { mode: 'all_history', start_date: '1912-01-01', end_date: '2026-10-28', reported_count: total } };
}

test('source-count states distinguish missing, matching, and contradictory evidence without dropping rows', () => {
  const mismatch = laboratoryCoverageStatus(report(2, 1), 'official');
  assert.equal(mismatch.kind, 'mismatch');
  assert.equal(mismatch.reportedCount, 1);
  assert.equal(mismatch.returnedCount, 2);
  assert.equal(mismatch.difference, 1);
  assert.match(mismatch.summary, /筆數不一致/);
  assert.doesNotMatch(mismatch.summary, /未提供|筆數相符/);
  assert.equal(laboratoryCoverageStatus(report(1, 2), 'official').difference, -1);
  assert.equal(laboratoryCoverageStatus(report(2), 'official').kind, 'missing');
  assert.equal(laboratoryCoverageStatus(report(2, 2, true), 'official').kind, 'matched');
  assert.match(laboratoryCoverageStatus(report(2, 2, false), 'official').summary, /仍標示未完整/);
  assert.equal(laboratoryCoverageStatus(report(0, 0, true), 'official').kind, 'matched');
  assert.equal(laboratoryCoverageStatus({ records: [], fetched_at: '', complete: false }, 'official').kind, 'unqueried');
});

test('import and demo never present historical metadata as current official proof', () => {
  const imported = laboratoryCoverageStatus(report(2, 1, true), 'import');
  assert.equal(imported.kind, 'imported');
  assert.match(imported.summary, /檔案記載官方回報 1 筆、保留 2 筆（筆數不一致）/);
  assert.match(imported.summary, /本次未向官方重新核對/);
  assert.doesNotMatch(laboratoryCoverageStatus(report(2), 'import').summary, /檔案未提供/);
  assert.equal(laboratoryCoverageStatus(report(2, 2, true), 'demo').kind, 'synthetic');
  assert.equal(laboratoryCoverageStatus(report(2, 2, true), 'anonymous').kind, 'unverified');
});

test('export date summary includes unknown dates and derives scope from all supplied records', () => {
  assert.equal(laboratoryDateSummary([]), '尚無檢驗資料');
  assert.equal(laboratoryDateSummary([{ exam_date: '' }, { exam_date: '原始日期不明' }]), '2 筆日期不明');
  assert.equal(laboratoryDateSummary([{ exam_date: '2026-01-02' }, { exam_date: '2025-01-01' }, { exam_date: '' }]), '2025/01/01～2026/01/02；另有 1 筆日期不明');
  assert.equal(laboratoryDateSummary([{ exam_date: '2026-01-02' }, { exam_date: '2026-01-02' }]), '2026/01/02');
});
