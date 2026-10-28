// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import test from 'node:test';
import { appendAcquisitionParts, inspectAcquisitionZIP, parseAcquisitionManifest, recoveredAcquisitionDataset,
  recoveredOriginalBlob, restoreAcquisitionOriginals } from '../src/acquisition-restore.ts';
import { exportDatasetPackage, exportDataset, importDataset, MAX_ARCHIVE_ATTACHMENT_BYTES } from '../src/health-dataset.ts';
import { syntheticZIP } from './fixtures/acquisition-zip.mjs';

const encoder = new TextEncoder(), digest = value => createHash('sha256').update(value).digest('hex');
const signal = () => new AbortController().signal;
const catalog = [{ id: 'medicine', label: '合成用藥', group: '合成', range_mode: 'date_search', has_details: false }];
async function fixture(count = 3) {
  const attachments = Array.from({ length: count }, (_, index) => ({ id: (index + 1).toString(16).repeat(48), name: `合成來源提示 ${index + 1}.txt`, content_type: 'application/octet-stream' }));
  const entry = { date: '115/10/28', title: '合成醫囑', institution: '合成院所', source: 'SYNTHETIC', fields: [], attachments };
  const snapshot = { reports: { medicine: { category: 'medicine', entries: [entry, { ...entry }], coverage: '合成來源沒有獨立總數', fetched_at: '2026-10-28T00:00:00Z', complete: false } },
    details: {}, states: { medicine: { status: 'ready', detail_errors: {}, query: { from: '', to: '' } } }, query: { from: '', to: '' } };
  const report = { records: [], fetched_at: '2026-10-28T00:00:00Z', complete: false, warnings: ['合成完整性未知'] };
  const projection = await exportDatasetPackage(report, snapshot, catalog);
  return { projection, report };
}
function original(reference, content, extra = {}) {
  const bytes = Buffer.from(content), name = `合成實際檔 ${reference.archive_id}.txt`;
  return { file: { category: reference.category, archive_id: reference.archive_id, usable: true, state: 'completed', bytes: bytes.length,
    sha256: digest(bytes), path: `originals/${reference.archive_id}/${name}`, content_type: 'text/plain;charset=utf-8', ...extra }, bytes };
}
async function part(projection, originals, { planID = 'a'.repeat(32), index = 1, total = 1, filesTotal = projection.files.length,
  included = true, alterManifest, alterBytes } = {}) {
  const bytes = encoder.encode(projection.content);
  const manifest = { schema: 'healthbank.acquisition.package.v1', created_at: '2026-10-28T00:00:00Z',
    state: originals.every(item => item.file.usable) ? 'completed' : 'partial', plan: { id: planID, index, total, files_total: filesTotal },
    dataset: { name: 'healthbank.dataset.v2.json', size: bytes.length, sha256: digest(bytes), included },
    acquisition: { scope: 'all_available', started_at: '2026-10-28T00:00:00Z', finished_at: '2026-10-28T00:00:01Z',
      statuses: [{ category: 'medicine', kind: 'source', state: 'ready', count: 1, code: 'source_total_unknown' }] },
    files: originals.map(item => structuredClone(item.file)), history_completeness: 'preserved_in_dataset_and_source_statuses_not_inferred_from_transfer' };
  alterManifest?.(manifest);
  const members = [
    ...(included ? [{ name: 'healthbank.dataset.v2.json', content: bytes, method: 8 }] : []),
    { name: 'README.txt', content: '合成原始檔包，傳輸完成不代表終身資料完整。', method: 8 },
    ...originals.filter(item => item.file.path).map(item => ({ name: item.file.path, content: item.bytes, method: 0 })),
    { name: 'manifest.json', content: JSON.stringify(manifest) + '\n', method: 8 },
  ];
  let zip = syntheticZIP(members); if (alterBytes) zip = alterBytes(zip);
  const file = new File([zip], `synthetic-part-${index}.zip`);
  Object.defineProperty(file, 'arrayBuffer', { value() { throw new Error('Whole ZIP buffering is forbidden'); } });
  return inspectAcquisitionZIP(file, signal());
}

test('original ZIP restoration preserves exact parent data and merges parts/retries only through identical dataset bytes and archive IDs', async () => {
  const { projection, report } = await fixture();
  const first = original(projection.files[0], '合成中文原始檔一\n'), second = original(projection.files[1], '合成中文原始檔二\n'), third = original(projection.files[2], '合成中文原始檔三\n');
  const failedSecond = { bytes: Buffer.from('合成截斷'), file: { ...second.file, usable: false, state: 'failed', code: 'DOCUMENT_INCOMPLETE',
    bytes: Buffer.byteLength('合成截斷'), sha256: digest('合成截斷') } };
  let selection = appendAcquisitionParts(null, [await part(projection, [first, failedSecond], { total: 2 })]);
  selection = await restoreAcquisitionOriginals(selection, signal(), () => {});
  assert.deepEqual(Object.values(selection.results).map(file => file.state), ['loaded', 'source_unusable', 'missing_part']);
  assert.equal(selection.retained.length, 1); assert.equal(selection.dataset.snapshot.reports.medicine.complete, false);
  selection = appendAcquisitionParts(selection, [await part(projection, [second], { planID: 'b'.repeat(32), filesTotal: 1 }),
    await part(projection, [third], { index: 2, total: 2 })]);
  selection = await restoreAcquisitionOriginals(selection, signal(), () => {});
  assert.deepEqual(Object.values(selection.results).map(file => file.state), ['loaded', 'loaded', 'loaded']);
  const restored = recoveredAcquisitionDataset(selection);
  assert.equal(restored.snapshot.reports.medicine.entries.length, 2); assert.equal(restored.attachments.length, 3);
  assert.equal(restored.snapshot.reports.medicine.entries[0].attachments[0].name, first.file.path.split('/').at(-1));
  assert.equal(restored.snapshot.reports.medicine.entries[0].attachments[0].content_type, 'text/plain');
  const portableAgain = await importDataset(encoder.encode(await exportDataset(report, restored.snapshot, restored.attachments, restored.catalog)));
  assert.equal(portableAgain.attachments.length, 3); assert.equal(portableAgain.snapshot.reports.medicine.entries.length, 2);
  assert.equal(await recoveredOriginalBlob(selection, Object.values(selection.results)[0]).text(), '合成中文原始檔一\n');
  assert.throws(() => appendAcquisitionParts(selection, [selection.parts[0]]), /重複/);
});

test('capacity limits retain a bounded attachment set while verified large originals remain recoverable as Blob slices', async () => {
  const { projection } = await fixture(2);
  const first = original(projection.files[0], '合成小檔'), large = original(projection.files[1], Buffer.alloc(MAX_ARCHIVE_ATTACHMENT_BYTES + 1, 65));
  let selection = appendAcquisitionParts(null, [await part(projection, [first, large])]); let checkpoints = 0;
  selection = await restoreAcquisitionOriginals(selection, signal(), () => { checkpoints++; });
  assert.ok(checkpoints >= 3); assert.equal(selection.retained.length, 1);
  const result = Object.values(selection.results)[1]; assert.equal(result.state, 'verified_external');
  const blob = recoveredOriginalBlob(selection, result); assert.equal(blob.size, MAX_ARCHIVE_ATTACHMENT_BYTES + 1);
  assert.equal(await blob.slice(0, 3).text(), 'AAA');
  assert.ok(selection.retained.reduce((sum, file) => sum + file.bytes.length, 0) <= MAX_ARCHIVE_ATTACHMENT_BYTES);
});

test('ZIP restore rejects dataset/category conflicts and external missing JSON; hash failures never become loaded originals', async () => {
  const { projection } = await fixture(1), file = original(projection.files[0], '合成原始檔');
  const external = await part(projection, [file], { included: false });
  let selected = appendAcquisitionParts(null, [external]);
  await assert.rejects(restoreAcquisitionOriginals(selected, signal(), () => {}), /完整資料 JSON/);
  selected.external = new File([projection.content], '使用者改名.json');
  assert.equal((await restoreAcquisitionOriginals(selected, signal(), () => {})).retained.length, 1);
  selected.external = new File([projection.content.replace('合成醫囑', '合成差異')], 'healthbank.dataset.v2.json');
  await assert.rejects(restoreAcquisitionOriginals(selected, signal(), () => {}), /SHA-256|完整資料 JSON/);
  const wrongBinding = await part(projection, [file], { alterManifest(value) { value.files[0].category = 'hospital'; } });
  await assert.rejects(restoreAcquisitionOriginals(appendAcquisitionParts(null, [wrongBinding]), signal(), () => {}), /關聯/);
  const wrongHash = await part(projection, [file], { alterManifest(value) { value.files[0].sha256 = '0'.repeat(64); } });
  const failed = await restoreAcquisitionOriginals(appendAcquisitionParts(null, [wrongHash]), signal(), () => {});
  assert.equal(failed.retained.length, 0); assert.equal(Object.values(failed.results)[0].state, 'failed');
  const different = await part({ ...projection, content: projection.content + ' ' }, [file], { planID: 'b'.repeat(32) });
  assert.throws(() => appendAcquisitionParts(appendAcquisitionParts(null, [external]), [different]), /相同的資料 JSON/);
  const manifest = structuredClone(external.manifest); manifest.files[0].usable = false;
  assert.throws(() => parseAcquisitionManifest(encoder.encode(JSON.stringify(manifest))));
});

test('cancellation retains verified checkpoints and conflicting retry bytes never silently replace one original', async () => {
  const { projection } = await fixture(2), first = original(projection.files[0], '合成小檔'), second = original(projection.files[1], Buffer.alloc(2 * 1024 ** 2, 65));
  let selection = appendAcquisitionParts(null, [await part(projection, [first, second])]); const controller = new AbortController();
  await assert.rejects(restoreAcquisitionOriginals(selection, controller.signal, next => { selection = next; }, progress => {
    if (progress.name === second.file.path.split('/').at(-1) && progress.bytes > 0) controller.abort();
  }), { name: 'AbortError' });
  assert.equal(selection.retained.length, 1); assert.deepEqual(Object.values(selection.results).map(file => file.state), ['loaded', 'not_checked']);
  selection = await restoreAcquisitionOriginals(selection, signal(), () => {}); assert.equal(selection.retained.length, 2);
  selection = appendAcquisitionParts(selection, [await part(projection, [original(projection.files[0], '同一識別卻不同內容')], { planID: 'b'.repeat(32), filesTotal: 1 })]);
  selection = await restoreAcquisitionOriginals(selection, signal(), () => {});
  assert.equal(Object.values(selection.results)[0].state, 'conflict'); assert.equal(selection.retained.length, 1);
});

test('a corrupt duplicate cannot conceal a healthy retry, and the failed copy remains explicit', async () => {
  const { projection } = await fixture(1), content = 'SYNTHETIC_ORIGINAL_UNIQUE_BYTES', file = original(projection.files[0], content);
  const corrupted = await part(projection, [file], { alterBytes(bytes) {
    const position = bytes.indexOf(Buffer.from(content)); assert.ok(position >= 0); bytes[position] ^= 1; return bytes;
  } });
  const retry = await part(projection, [file], { planID: 'b'.repeat(32) });
  const restored = await restoreAcquisitionOriginals(appendAcquisitionParts(null, [corrupted, retry]), signal(), () => {});
  assert.equal(restored.retained.length, 1); assert.equal(Object.values(restored.results)[0].state, 'loaded');
  assert.deepEqual(Object.values(restored.checked), ['failed', 'verified']);
  assert.equal(await recoveredOriginalBlob(restored, Object.values(restored.results)[0]).text(), content);
});
