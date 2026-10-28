// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import assert from 'node:assert/strict';
import test from 'node:test';
import { batchCollectionJSON, describeBatchDataset, matchBatchFiles, newBatchCollection,
  parseBatchCollection, restoreBatchMember, summarizeBatch, updateBatchDescriptor } from '../src/batch-collection.ts';
import { beginDateBatch, missingBatchLaboratory, splitDateBatches } from '../src/date-batches.ts';
import { exportDataset } from '../src/health-dataset.ts';

const encode = value => new TextEncoder().encode(value);
const categories = [{ id: 'medicine', label: '合成用藥來源', group: '合成', range_mode: 'date_search', has_details: false }];
async function fixture() {
  const plan = { ranges: splitDateBatches({ from: '2025-12-01', to: '2026-01-31' }, 'year'), categories: ['medicine'], includeLaboratory: false };
  let manifest = newBatchCollection(plan, 'year'); const files = [];
  for (const [index, query] of plan.ranges.entries()) {
    const snapshot = beginDateBatch(query, plan.categories);
    const row = { date: query.from, institution: '合成院所', title: `合成第 ${index + 1} 批`, source: 'SYNTHETIC', fields: [],
      attachments: [{ id: 'a'.repeat(48), name: '合成附件.txt', content_type: 'text/plain' }] };
    snapshot.reports.medicine = { category: 'medicine', entries: [row, { ...row }], coverage: '沒有獨立總數', fetched_at: '2026-10-28T00:00:00Z', complete: false };
    snapshot.states.medicine = { status: 'ready', detail_errors: {}, query };
    const content = await exportDataset(missingBatchLaboratory(query, false), snapshot, [], categories);
    const descriptor = await describeBatchDataset(content, `synthetic-batch-${index + 1}.json`, summarizeBatch(snapshot, plan, null));
    manifest = updateBatchDescriptor(manifest, index + 1, { ...descriptor, saved_acknowledged: true });
    files.push(new File([content], descriptor.name, { type: 'application/json' }));
  }
  return { plan, manifest, files };
}

test('batch collection restores separate exact snapshots and never deduplicates shared local IDs or original rows', async () => {
  const { manifest, files } = await fixture();
  const parsed = parseBatchCollection(encode(batchCollectionJSON(manifest)));
  const matches = matchBatchFiles(parsed, files);
  assert.equal(matches.unmatched, 0); assert.equal(matches.ambiguous, 0);
  const first = await restoreBatchMember(parsed, 1, matches.files[1], new AbortController().signal);
  const second = await restoreBatchMember(parsed, 2, matches.files[2], new AbortController().signal);
  assert.equal(first.snapshot.reports.medicine.entries.length, 2);
  assert.equal(second.snapshot.reports.medicine.entries.length, 2);
  assert.equal(first.snapshot.reports.medicine.entries[0].attachments[0].id, second.snapshot.reports.medicine.entries[0].attachments[0].id);
  assert.notEqual(first.snapshot.reports.medicine.entries[0].title, second.snapshot.reports.medicine.entries[0].title);
  assert.deepEqual(first.snapshot.query, parsed.batches[0].query);
  assert.deepEqual(second.snapshot.query, parsed.batches[1].query);
  assert.equal(first.snapshot.reports.medicine.complete, false);
  assert.equal(parsed.batches[0].dataset.acquisition.source_reports_without_total, 1);
  assert.equal(parsed.batches[0].dataset.acquisition.acquired, true);
  const renamed = new File([await files[0].arrayBuffer()], '使用者改名.json');
  assert.equal(matchBatchFiles(parsed, [renamed]).unmatched, 1);
  assert.deepEqual((await restoreBatchMember(parsed, 1, renamed, new AbortController().signal)).snapshot, first.snapshot);
});

test('collection validation binds range, counts, hash and byte size; unexported ranges remain explicit', async () => {
  const { plan, manifest, files } = await fixture();
  const fresh = newBatchCollection(plan, 'year'); assert.equal(fresh.batches.length, 2); assert.equal(fresh.batches[0].dataset, undefined);
  await assert.rejects(restoreBatchMember(fresh, 1, files[0], new AbortController().signal));
  for (const alter of [
    value => { value.extra = true; }, value => { value.batches.pop(); },
    value => { value.batches[1].query.from = '2026-01-02'; },
    value => { value.batches[0].dataset.acquisition.acquired = false; },
    value => { value.categories.push('medicine'); },
  ]) { const value = structuredClone(manifest); alter(value); assert.throws(() => parseBatchCollection(encode(JSON.stringify(value)))); }
  for (const alter of [
    value => { value.batches[0].dataset.sha256 = '0'.repeat(64); },
    value => { value.batches[0].dataset.size++; },
    value => { value.batches[0].dataset.acquisition.source_entries++; },
    value => { value.batches[0].dataset.exported_at = '2024-01-01T00:00:00Z'; },
  ]) {
    const value = structuredClone(manifest); alter(value);
    const parsed = parseBatchCollection(encode(JSON.stringify(value)));
    await assert.rejects(restoreBatchMember(parsed, 1, files[0], new AbortController().signal));
  }
  const duplicate = new File([await files[0].arrayBuffer()], files[0].name);
  const ambiguous = matchBatchFiles(manifest, [files[0], duplicate]);
  assert.equal(ambiguous.ambiguous, 1); assert.equal(ambiguous.files[1], undefined);
  const controller = new AbortController(); controller.abort();
  await assert.rejects(restoreBatchMember(manifest, 1, files[0], controller.signal), { name: 'AbortError' });
});
