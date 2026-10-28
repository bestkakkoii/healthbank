// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import test from 'node:test';
import assert from 'node:assert/strict';
import { exportDataset, importDataset, LEGACY_DATASET_SCHEMA, UNKNOWN_DATASET_PRODUCER } from '../src/health-dataset.ts';
import { compareDatasets, datasetDifferenceFields } from '../src/dataset-comparison.ts';
import { emptyHealthData, filterHealthEvents, healthEvents } from '../src/health-data.ts';
import { sourceEventInterval } from '../src/source-semantics.ts';

const category = [{ id: 'hospital', label: '合成住院', group: '測試', range_mode: 'date_search', has_details: true }];
const source = entries => ({ category: 'hospital', entries, fetched_at: '2026-10-28T00:00:00Z', complete: false, coverage: '合成資料' });
const entry = extra => ({ date: '2026-01-25', title: '合成住院', institution: '合成院所', source: 'synthetic', fields: [{ name: '原始結果', value: '1.000' }], ...extra });
const lab = { records: [{ exam_date: '2026-01-25', visit_date: '', order_code: 'SYNTHETIC', order_name: '合成醫令', item_name: '合成項目', result: '>50', unit: 'mg/dL', reference_text: '原院值', institution: '合成院所', source: 'synthetic' }], fetched_at: '2026-10-28T00:00:00Z', complete: false, warnings: [] };
const decode = content => importDataset(new TextEncoder().encode(content));
async function dataset(snapshot, producer = UNKNOWN_DATASET_PRODUCER) { return decode(await exportDataset(lab, snapshot, [], category, false, producer)); }

test('v2 preserves semantic evidence and producer hashes while v1 remains explicit provenance-unknown', async () => {
  const snapshot = emptyHealthData();
  snapshot.reports.hospital = { ...source([entry({ end_date: '2026-02-05', date_role: 'admission', end_date_role: 'discharge', origin: 'institutional', contract_id: 'hospital.list',
    fields: [{ name: '檢體', value: '原文', code: 'specimen', role: 'specimen', unit_basis: 'source' }] })]),
    contract_revision: 'nhi-spa.2026-10-28.2', diagnostics: [{ code: 'section_missing', severity: 'warning', section: 'hospital.orders', count: 1, message: '合成必要區段缺少' }] };
  const producer = { ...UNKNOWN_DATASET_PRODUCER, version: 'v1.0.3', clinical_catalog_revision: 'sha256:' + 'a'.repeat(64), clinical_evidence_revision: 'sha256:' + 'b'.repeat(64) };
  const saved = await exportDataset(lab, snapshot, [], category, false, producer);
  const restored = await decode(saved);
  assert.deepEqual(restored.producer, producer);
  assert.deepEqual(restored.snapshot.reports.hospital, snapshot.reports.hospital);
  const old = JSON.parse(await exportDataset(lab, emptyHealthData(), [], category)); old.schema = LEGACY_DATASET_SCHEMA; delete old.producer;
  const legacy = await decode(JSON.stringify(old)); assert.equal(legacy.producer, null);
  old.producer = producer; await assert.rejects(decode(JSON.stringify(old)));
  for (const patch of [value => { value.data.reports.hospital.entries[0].date_role = 'guess'; }, value => { value.data.reports.hospital.diagnostics[0].count = 50001; }, value => { value.producer.clinical_catalog_revision = 'unverified-fake-hash'; }, value => { value.data.reports.hospital.entries[0].fields[0].code = 'patient:secret'; }]) {
    const changed = JSON.parse(saved); patch(changed); await assert.rejects(decode(JSON.stringify(changed)));
  }
});

test('snapshot comparison ignores array order and retired handles while preserving duplicate counts and parent context', async () => {
  const a = emptyHealthData(); a.reports.hospital = source([entry({ detail_id: 'a'.repeat(48) }), entry({ detail_id: 'a'.repeat(48) }), entry({ title: '另一項' })]);
  a.details.hospital = { ['a'.repeat(48)]: source([entry({ title: '合成明細' })]) };
  const b = emptyHealthData(); b.reports.hospital = source([entry({ title: '另一項' }), entry({ detail_id: 'b'.repeat(48) }), entry({ detail_id: 'b'.repeat(48) })]);
  b.details.hospital = { ['b'.repeat(48)]: source([entry({ title: '合成明細' })]) };
  const before = await dataset(a), after = await dataset(b);
  const equal = await compareDatasets(before, after);
  assert.equal(equal.differences.length, 0); assert.equal(equal.unchangedOccurrences, 5);
  b.reports.hospital.entries.push(entry({ detail_id: 'b'.repeat(48) }));
  const changed = await compareDatasets(before, await dataset(b));
  assert.equal(changed.addedOccurrences, 1); assert.equal(changed.previousOnlyOccurrences, 0);
  assert.equal(changed.differences.length, 1); assert.equal(changed.differences[0].before, 2); assert.equal(changed.differences[0].after, 3);
  assert.ok(datasetDifferenceFields(changed.differences[0]).some(field => field.value === '1.000'));
});

test('snapshot comparison preserves every explicit entry and field semantic difference', async () => {
  const snapshot = emptyHealthData();
  snapshot.reports.hospital = source([entry({ date_role: 'admission', end_date_role: 'discharge', origin: 'institutional', contract_id: 'hospital.list',
    end_date: '2026-02-05', fields: [{ name: '原始結果', value: '1.000', unit: 'mg/dL', code: 'result', role: 'result', unit_basis: 'source' }] })]);
  const before = await dataset(snapshot);
  const patches = [row => { row.date_role = 'recorded'; }, row => { row.end_date_role = 'uploaded'; }, row => { row.origin = 'self_entered'; },
    row => { row.contract_id = 'hospital.detail'; }, row => { row.fields[0].code = 'quantity'; }, row => { row.fields[0].role = 'quantity'; },
    row => { row.fields[0].unit_basis = 'public_contract'; }];
  for (const patch of patches) {
    const changed = structuredClone(snapshot); patch(changed.reports.hospital.entries[0]);
    const result = await compareDatasets(before, await dataset(changed));
    assert.equal(result.addedOccurrences, 1); assert.equal(result.previousOnlyOccurrences, 1);
    assert.equal(result.unchangedOccurrences, 1); assert.equal(result.differences.length, 2);
    assert.ok(result.differences.flatMap(datasetDifferenceFields).some(field => field.name.includes('用途')));
  }
});

test('equal attachment bytes moved between distinct source records remain visible differences', async () => {
  const a = 'a'.repeat(48), b = 'b'.repeat(48);
  const attachment = id => ({ id, name: '合成附件.txt', content_type: 'text/plain', size: 1 });
  const snapshot = emptyHealthData();
  snapshot.reports.hospital = source([entry({ title: '合成住院甲', attachments: [attachment(a)] }), entry({ title: '合成住院乙', attachments: [attachment(b)] })]);
  const files = [{ category: 'hospital', id: a, name: '合成附件.txt', content_type: 'text/plain', bytes: new TextEncoder().encode('A') },
    { category: 'hospital', id: b, name: '合成附件.txt', content_type: 'text/plain', bytes: new TextEncoder().encode('B') }];
  const swapped = structuredClone(snapshot); swapped.reports.hospital.entries[0].attachments = [attachment(b)]; swapped.reports.hospital.entries[1].attachments = [attachment(a)];
  const before = await decode(await exportDataset(lab, snapshot, files, category));
  const after = await decode(await exportDataset(lab, swapped, files, category));
  const result = await compareDatasets(before, after);
  assert.equal(result.addedOccurrences, 2); assert.equal(result.previousOnlyOccurrences, 2); assert.equal(result.unchangedOccurrences, 3);
  assert.equal(result.differences.length, 4); assert.ok(result.differences.every(row => row.kind === 'attachment'));
  const firstFile = result.differences.filter(row => row.content === result.differences[0].content);
  assert.equal(firstFile.length, 2);
  assert.deepEqual(new Set(firstFile.map(row => row.relationship.parents[0].title)), new Set(['合成住院甲', '合成住院乙']));
  assert.notEqual(firstFile[0].relationship.fingerprint, firstFile[1].relationship.fingerprint);
  assert.ok(firstFile.every(row => datasetDifferenceFields(row).some(field => field.name === '直接所屬紀錄 1' && /合成住院[甲乙]/.test(field.value))));
  const exportedDiff = JSON.stringify(result);
  assert.ok(exportedDiff.includes('relationship') && exportedDiff.includes('sha256:'));
  assert.ok(!exportedDiff.includes(a) && !exportedDiff.includes(b) && !/archive_\d+/.test(exportedDiff));
});

test('nested detail and attachment ancestry preserves distinct grandparents', async () => {
  const [a, b, c, d, fileID] = ['a', 'b', 'c', 'd', 'e'].map(value => value.repeat(48));
  const snapshot = emptyHealthData();
  snapshot.reports.hospital = source([entry({ title: '合成住院甲', detail_id: a }), entry({ title: '合成住院乙', detail_id: b })]);
  snapshot.details.hospital = {
    [a]: source([entry({ title: '相同中間列', detail_id: c })]), [b]: source([entry({ title: '相同中間列', detail_id: d })]),
    [c]: source([entry({ title: '合成明細甲', attachments: [{ id: fileID, name: '合成附件.txt', content_type: 'text/plain' }] })]),
    [d]: source([entry({ title: '合成明細乙' })]),
  };
  const files = [{ category: 'hospital', id: fileID, name: '合成附件.txt', content_type: 'text/plain', bytes: new TextEncoder().encode('synthetic') }];
  const swapped = structuredClone(snapshot); swapped.details.hospital[a].entries[0].detail_id = d; swapped.details.hospital[b].entries[0].detail_id = c;
  const before = await decode(await exportDataset(lab, snapshot, files, category));
  const after = await decode(await exportDataset(lab, swapped, files, category));
  const result = await compareDatasets(before, after);
  assert.equal(result.addedOccurrences, 3); assert.equal(result.previousOnlyOccurrences, 3); assert.equal(result.unchangedOccurrences, 5);
  assert.equal(result.differences.filter(row => row.kind === 'detail').length, 4);
  assert.equal(result.differences.filter(row => row.kind === 'attachment').length, 2);
  const movedLeaf = result.differences.filter(row => row.kind === 'detail' && row.title === '合成明細甲');
  assert.deepEqual(movedLeaf.map(row => row.relationship.parents[0].title), ['相同中間列', '相同中間列']);
  assert.deepEqual(new Set(movedLeaf.map(row => row.relationship.roots[0].title)), new Set(['合成住院甲', '合成住院乙']));
  assert.ok(movedLeaf.every(row => datasetDifferenceFields(row).some(field => field.name === '最上層來源紀錄 1' && /合成住院[甲乙]/.test(field.value))));
});

test('shared ancestry stays bounded and duplicate parent rows do not change child identity', { timeout: 5000 }, async () => {
  const snapshot = emptyHealthData();
  const reference = (level, branch) => `synthetic_${level}_${branch}`;
  snapshot.reports.hospital = source([entry({ title: '合成入口甲', detail_id: reference(0, 0) }), entry({ title: '合成入口乙', detail_id: reference(0, 1) })]);
  snapshot.details.hospital = {};
  // Only 48 tables, but expanding each possible root-to-leaf path would grow
  // exponentially. Shared parents are source links, not duplicated rows.
  for (let level = 0; level < 24; level++) for (let branch = 0; branch < 2; branch++) {
    snapshot.details.hospital[reference(level, branch)] = source(level === 23 ? [entry({ title: `合成末端${branch}` })]
      : [entry({ title: '合成分支甲', detail_id: reference(level + 1, 0) }), entry({ title: '合成分支乙', detail_id: reference(level + 1, 1) })]);
  }
  const before = await dataset(snapshot);
  const reordered = structuredClone(snapshot); reordered.reports.hospital.entries.reverse();
  reordered.details.hospital = Object.fromEntries(Object.entries(reordered.details.hospital).reverse().map(([key, value]) => [key, { ...value, entries: value.entries.toReversed() }]));
  const equal = await compareDatasets(before, await dataset(reordered));
  assert.equal(equal.differences.length, 0); assert.deepEqual(equal.rangeWarnings, []);
  reordered.reports.hospital.entries.push(structuredClone(reordered.reports.hospital.entries[0]));
  const duplicated = await compareDatasets(before, await dataset(reordered));
  assert.equal(duplicated.addedOccurrences, 1); assert.equal(duplicated.previousOnlyOccurrences, 0);
  assert.equal(duplicated.differences.length, 1); assert.equal(duplicated.differences[0].kind, 'list');
});

test('cyclic ancestry terminates and never labels dependent details or attachments unchanged', { timeout: 5000 }, async () => {
  const [a, b, fileID] = ['a', 'b', 'c'].map(value => value.repeat(48));
  const snapshot = emptyHealthData(); snapshot.reports.hospital = source([entry({ detail_id: a })]);
  snapshot.details.hospital = { [a]: source([entry({ title: '合成循環甲', detail_id: b })]),
    [b]: source([entry({ title: '合成循環乙', detail_id: a, attachments: [{ id: fileID, name: '合成附件.txt', content_type: 'text/plain' }] })]) };
  const files = [{ category: 'hospital', id: fileID, name: '合成附件.txt', content_type: 'text/plain', bytes: new TextEncoder().encode('synthetic') }];
  const saved = await exportDataset(lab, snapshot, files, category);
  const result = await compareDatasets(await decode(saved), await decode(saved));
  assert.equal(result.unchangedOccurrences, 2); assert.equal(result.addedOccurrences, 3); assert.equal(result.previousOnlyOccurrences, 3);
  assert.equal(result.rangeWarnings.length, 1); assert.match(result.rangeWarnings[0], /循環.*未判定為相同/);
  assert.ok(result.differences.every(row => row.kind === 'detail' || row.kind === 'attachment'));
  assert.ok(result.differences.every(row => row.relationship.status === 'unresolved' && row.relationship.fingerprint === undefined));
  assert.ok(result.differences.every(row => datasetDifferenceFields(row).some(field => field.name === '所屬來源關係' && /未判定為相同/.test(field.value))));
});

test('relationship summaries cap readable text and parents without changing complete ancestry identity', async () => {
  const reference = 'f'.repeat(48), snapshot = emptyHealthData();
  snapshot.reports.hospital = source(Array.from({ length: 6 }, (_, index) => entry({ detail_id: reference,
    title: index === 0 ? `000${'🧪'.repeat(200)}` : `合成上層${index}`, institution: `合成院所${index}` })));
  snapshot.details.hospital = { [reference]: source([entry({ title: '合成末端', fields: [{ name: '原始結果', value: 'A' }] })]) };
  const changed = structuredClone(snapshot); changed.details.hospital[reference].entries[0].fields[0].value = 'B';
  const result = await compareDatasets(await dataset(snapshot), await dataset(changed));
  assert.equal(result.differences.length, 2);
  const relationships = result.differences.map(row => row.relationship);
  assert.ok(relationships.every(value => value.status === 'resolved' && value.summary_truncated && value.parents.length === 4 && value.roots.length === 4));
  assert.equal(relationships[0].fingerprint, relationships[1].fingerprint);
  for (const value of relationships) for (const summary of [...value.parents, ...value.roots]) {
    assert.ok(summary.title.length <= 161);
    assert.ok(!/[\uD800-\uDFFF]/u.test(summary.title));
  }
  assert.ok(result.differences.every(row => datasetDifferenceFields(row).some(field => field.name === '關係摘要顯示上限' && /部分摘要已省略/.test(field.value))));
  changed.reports.hospital.entries[5].institution = '另一個未列於摘要的合成院所';
  const differentParent = await compareDatasets(await dataset(snapshot), await dataset(changed));
  const leaf = differentParent.differences.filter(row => row.kind === 'detail');
  assert.equal(leaf.length, 2); assert.notEqual(leaf[0].relationship.fingerprint, leaf[1].relationship.fingerprint);
});

test('snapshot comparison discloses different ranges and rule versions without manufacturing changed clinical events', async () => {
  const a = emptyHealthData(); a.reports.hospital = { ...source([entry()]), start_date: '2026-01-01', end_date: '2026-02-01' };
  const b = structuredClone(a); b.reports.hospital.start_date = '2025-01-01';
  const result = await compareDatasets(await dataset(a), await dataset(b, { ...UNKNOWN_DATASET_PRODUCER, clinical_catalog_revision: 'sha256:' + 'c'.repeat(64) }));
  assert.equal(result.differences.length, 0); assert.equal(result.clinicalRulesChanged, true);
  assert.match(result.rangeWarnings.join(''), /查詢期間/);
});

test('date filtering includes only verified compatible overlapping intervals and never infers a medication stop date', () => {
  const snapshot = emptyHealthData();
  snapshot.reports.hospital = source([entry({ date_role: 'admission', end_date_role: 'discharge', end_date: '2026-02-05' }),
    entry({ title: '無日期語意', end_date: '2026-02-05' }), entry({ title: '不同日期角色', date_role: 'encounter', end_date_role: 'claim_end', end_date: '2026-02-05' }),
    entry({ title: '僅處方日數', date_role: 'prescription', medicine: { code: 'synthetic', name: '合成', quantity: '28', days: '28' } })]);
  const events = healthEvents(snapshot, category);
  const selected = filterHealthEvents(events, { text: '', category: '', institution: '', from: '2026-02-01', to: '2026-02-03', includeUnknownDates: false });
  assert.equal(selected.length, 1); assert.equal(selected[0].entry.date_role, 'admission');
  assert.equal(sourceEventInterval(entry({ date_role: 'admission', end_date_role: 'discharge', end_date: '2026-01-01' })).isInterval, false);
});
