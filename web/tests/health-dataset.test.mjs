// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import assert from 'node:assert/strict';
import test from 'node:test';
import { exportDataset, exportDatasetPackage, importDataset, supplementReportJSON, DATASET_SCHEMA, MAX_ARCHIVE_ATTACHMENT_BYTES } from '../src/health-dataset.ts';
import { MAX_PORTABLE_REPORT_BYTES, portableReportJSON, portableReportXML } from '../src/portable-report.ts';

const encoded = value => new TextEncoder().encode(value);
const ref = 'a'.repeat(48);
const nestedRef = 'b'.repeat(48);
const attachmentRef = 'c'.repeat(48);
const actionRef = 'd'.repeat(48);
const categories = [{ id: 'medicine', label: '合成藥品來源', group: '合成測試', range_mode: 'date_search', has_details: true }];
function row(patch = {}) {
  return { date: '115/01/02', institution: '合成示範院所', title: '合成醫囑', source: 'SYNTHETIC_SOURCE',
    fields: [{ name: '來源精確值', value: '400.00000000000000000001', unit: 'mg/dL' }], ...patch };
}
function source(entries, category = 'medicine') {
  return { category, entries, coverage: '合成來源沒有獨立總數', fetched_at: '2026-10-28T00:00:00Z', complete: false };
}
function fixture() {
  const lab = { session_token: 'e'.repeat(64), records: [{ exam_date: '2026-01-02', visit_date: '2026-01-01', order_code: 'SYNTHETIC_CODE',
    order_name: '合成檢查', item_name: '合成項目', result: '>50', unit: 'mg/dL', reference_text: '[0][20]', institution: '合成示範院所',
    source: 'SYNTHETIC_SOURCE', official_flag: '來源旗標', level: 5, interpretation: 'must not trust stored grades', numeric_value: 50 }],
  fetched_at: '2026-10-28T00:00:00Z', complete: false, warnings: ['合成完整性資訊'],
  official_query: { mode: 'all_history', start_date: '1912-01-01', end_date: '2026-10-28', reported_count: null } };
  const snapshot = { reports: { medicine: source([row({ detail_id: ref }), row({ detail_id: ref })]) },
    details: { medicine: { [ref]: source([row({ detail_id: nestedRef, medicine: { code: 'SYNTHETIC', name: '合成藥品', quantity: '12.000', days: '' } })], 'medicine_detail'),
      [nestedRef]: source([row({ attachments: [{ id: attachmentRef, name: '合成報告.txt', content_type: 'text/plain;charset=utf-8' }],
        document_actions: [{ id: actionRef, kind: 'prepare_jpeg', name: '合成產製' }] })], 'medicine_detail') } },
    states: { medicine: { status: 'ready', detail_errors: {}, query: { from: '2020-01-01', to: '2026-10-28' } },
      hospital: { status: 'failed', error: '合成來源失敗', detail_errors: {} } }, query: { from: '', to: '' } };
  const files = [{ category: 'medicine', id: attachmentRef, name: '合成報告.txt', content_type: 'text/plain', bytes: encoded('合成中文附件\n原文不執行 <script>') }];
  return { lab, snapshot, files };
}

test('streaming package projection binds live attachment handles to the exact portable relationship without exporting capabilities', async () => {
  const { lab, snapshot } = fixture();
  const projection = await exportDatasetPackage(lab, snapshot, categories);
  assert.equal(projection.files.length, 1);
  assert.equal(projection.files[0].id, attachmentRef);
  assert.equal(projection.files[0].category, 'medicine');
  for (const value of [attachmentRef, actionRef, ref, nestedRef, lab.session_token]) assert.ok(!projection.content.includes(value));
  const restored = await importDataset(encoded(projection.content));
  const attachment = Object.values(restored.snapshot.details.medicine).flatMap(report => report.entries)
    .flatMap(entry => entry.attachments ?? [])[0];
  assert.equal(projection.files[0].archive_id, attachment.id);
  assert.equal(restored.attachments.length, 0);
  const importedProjection = restored.snapshot;
  await assert.rejects(exportDatasetPackage(lab, importedProjection, categories), /不屬於目前官方工作階段/);
});

test('standalone source exports preserve rows and local relationships without live document capabilities', () => {
  const original = source([row({ key: 'synthetic-source-key', detail_id: ref,
    attachments: [{ id: attachmentRef, name: '合成報告.txt', content_type: 'text/plain;charset=utf-8' }],
    document_actions: [{ id: actionRef, kind: 'prepare_export', name: '合成產製' }] }), row({ detail_id: ref })]);
  const before = structuredClone(original);
  const output = supplementReportJSON(original);
  for (const secret of [ref, attachmentRef, actionRef, 'document_actions', 'synthetic-source-key']) assert.ok(!output.includes(secret), secret);
  const parsed = JSON.parse(output);
  assert.equal(parsed.entries.length, 2);
  assert.equal(parsed.entries[0].detail_id, parsed.entries[1].detail_id);
  assert.notEqual(parsed.entries[0].detail_id, parsed.entries[0].attachments[0].id);
  assert.match(parsed.entries[0].attachments[0].id, /^archive_\d+$/);
  assert.equal(parsed.entries[0].fields[0].value, '400.00000000000000000001');
  assert.equal(parsed.coverage, original.coverage);
  assert.equal(parsed.complete, false);
  assert.deepEqual(original, before);
});

test('complete dataset preserves duplicates, nested source relationships, exact fields and UTF-8 attachments', async () => {
  const { lab, snapshot, files } = fixture();
  const previous = structuredClone({ lab, snapshot, files });
  const exported = await exportDataset(lab, snapshot, files, categories, true);
  assert.ok(exported.startsWith('\ufeff'));
  assert.ok(exported.includes(DATASET_SCHEMA));
  for (const secret of [ref, nestedRef, attachmentRef, actionRef, lab.session_token, 'session_token', 'document_actions', 'numeric_value', 'interpretation']) assert.ok(!exported.includes(secret), secret);
  const result = await importDataset(encoded(exported));
  assert.equal(result.snapshot.reports.medicine.entries.length, 2);
  const first = result.snapshot.reports.medicine.entries[0];
  assert.match(first.detail_id, /^archive_\d+$/);
  const detail = result.snapshot.details.medicine[first.detail_id];
  assert.equal(detail.entries[0].medicine.quantity, '12.000');
  assert.equal(detail.entries[0].medicine.days, '');
  assert.equal(detail.entries[0].fields[0].value, '400.00000000000000000001');
  const nested = result.snapshot.details.medicine[detail.entries[0].detail_id];
  assert.equal(nested.entries[0].attachments[0].id, result.attachments[0].id);
  assert.deepEqual(result.attachments[0].bytes, files[0].bytes);
  assert.equal(JSON.parse(result.laboratoryJSON).records[0].result, '>50');
  assert.equal(JSON.parse(result.laboratoryJSON).records[0].official_flag, '來源旗標');
  assert.deepEqual(result.laboratoryMetadata, { fetched_at: lab.fetched_at, complete: lab.complete, warnings: lab.warnings, official_query: lab.official_query });
  assert.deepEqual(result.snapshot.states.medicine.query, snapshot.states.medicine.query);
  assert.equal(result.snapshot.states.hospital.status, 'failed');
  assert.deepEqual({ lab, snapshot, files }, previous);
});

test('suggested refill calendars and PNG originals retain bytes without implying calendar import or medication adherence', async () => {
  const { lab, snapshot } = fixture();
  const calendar = encoded('BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nSUMMARY:合成建議領藥日期（不是已領藥）\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n');
  const png = new Uint8Array([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0]);
  const files = [{ category: 'medicine', id: attachmentRef, name: '合成建議領藥日期.ics', content_type: 'text/calendar;charset=utf-8', bytes: calendar },
    { category: 'medicine', id: 'e'.repeat(48), name: '合成原圖.png', content_type: 'image/png', bytes: png }];
  snapshot.details.medicine[nestedRef].entries[0].attachments = files.map(file => ({ id: file.id, name: file.name, content_type: file.content_type }));
  const restored = await importDataset(encoded(await exportDataset(lab, snapshot, files, categories)));
  assert.deepEqual(restored.attachments.map(file => file.content_type), ['text/calendar', 'image/png']);
  assert.deepEqual(restored.attachments.map(file => file.bytes), [calendar, png]);
  for (const [index, bytes] of [[0, new Uint8Array([0xff])], [1, encoded('not a PNG')]]) {
    const invalidFiles = files.map((file, ordinal) => ordinal === index ? { ...file, bytes } : file);
    await assert.rejects(exportDataset(lab, snapshot, invalidFiles, categories));
  }
});

test('export retains missing details and nonembedded attachment metadata without fabricating completeness', async () => {
  const { lab, snapshot } = fixture();
  delete snapshot.details.medicine[nestedRef];
  snapshot.states.medicine.detail_errors[nestedRef] = '合成明細未取得';
  snapshot.states.medicine.status = 'loading';
  snapshot.reports.medicine.entries.push(row({ attachments: [{ id: attachmentRef, name: '大型原始檔.zip', content_type: 'application/zip', size: 100_000_000 }] }));
  const result = await importDataset(encoded(await exportDataset(lab, snapshot, [], categories)));
  assert.equal(Object.keys(result.snapshot.details.medicine).length, 1);
  assert.equal(result.attachments.length, 0);
  assert.equal(result.snapshot.reports.medicine.complete, false);
  assert.equal(result.snapshot.states.medicine.status, 'cancelled');
  assert.equal(Object.keys(result.snapshot.states.medicine.detail_errors).length, 1);
  assert.equal(result.snapshot.reports.medicine.entries[2].attachments[0].size, 100_000_000);
});

test('archive import rejects unknown credentials, unsafe keys, live references, unlinked details and duplicate JSON keys', async () => {
  const { lab, snapshot, files } = fixture();
  const original = JSON.parse(await exportDataset(lab, snapshot, files, categories));
  const cases = [
    value => { value.jwt_token = 'synthetic-forbidden'; },
    value => { value.data.reports.medicine.entries[0].detail_id = ref; },
    value => { value.data.reports.medicine.entries[0].document_actions = []; },
    value => { value.data.details.medicine.archive_9999 = source([]); },
    value => { value.laboratory.records[0].patient_id = 'synthetic-forbidden'; },
    value => { value.catalog[0].id = 'constructor'; },
    value => { value.data.query = { from: '2026-02-30', to: '2026-03-01' }; },
  ];
  for (const mutate of cases) {
    const changed = structuredClone(original); mutate(changed);
    await assert.rejects(importDataset(encoded(JSON.stringify(changed))));
  }
  const valid = JSON.stringify(original);
  await assert.rejects(importDataset(encoded(valid.replace('"complete":false', '"complete":true,"complete":false'))));
  await assert.rejects(importDataset(encoded(valid.replace('"schema":', '"__proto__":{},"schema":'))));
  await assert.rejects(importDataset(encoded(valid.replace('"complete":false', '"compl\\u0065te":true,"complete":false'))));
  await assert.rejects(importDataset(encoded(`${valid} {}`)));
});

test('attachment digests, content types, names, category associations and bounds are validated before restoration', async () => {
  const { lab, snapshot, files } = fixture();
  const original = JSON.parse(await exportDataset(lab, snapshot, files, categories));
  for (const mutate of [
    value => { value.attachments[0].sha256 = '0'.repeat(64); },
    value => { value.attachments[0].data_base64 = '!!!!'; },
    value => { value.attachments[0].name = '../report.txt'; },
    value => { value.attachments[0].content_type = 'text/html'; },
    value => { value.attachments[0].category = 'hospital'; },
    value => { value.attachments[0].size = MAX_ARCHIVE_ATTACHMENT_BYTES + 1; },
    value => { value.attachments.push(value.attachments[0]); },
  ]) {
    const changed = structuredClone(original); mutate(changed);
    await assert.rejects(importDataset(encoded(JSON.stringify(changed))));
  }
  await assert.rejects(exportDataset(lab, snapshot, [{ ...files[0], content_type: 'application/pdf' }], categories));
});

test('supplement-only dataset and missing catalog categories remain inspectable without creating identity or laboratory observations', async () => {
  const { lab, snapshot, files } = fixture();
  lab.records = [];
  const result = await importDataset(encoded(await exportDataset(lab, snapshot, files, [])));
  assert.equal(JSON.parse(result.laboratoryJSON).records.length, 0);
  assert.equal(result.catalog[0].id, 'medicine');
  assert.equal(result.catalog[0].group, '資料包來源');
  assert.equal(result.snapshot.reports.medicine.entries.length, 2);
  assert.ok(!('authenticated' in result));
});

test('previously downloaded originals remain in the archive after source references refresh', async () => {
  const { lab, snapshot, files } = fixture();
  snapshot.details = {};
  snapshot.reports = { medicine: source([]) };
  const result = await importDataset(encoded(await exportDataset(lab, snapshot, files, categories)));
  assert.equal(result.attachments.length, 1);
  assert.deepEqual(result.attachments[0].bytes, files[0].bytes);
  assert.equal(result.snapshot.reports.medicine.entries.length, 0);
});

test('actual download filename, inspected MIME and byte size replace source hints consistently in exported references', async () => {
  const { lab, snapshot, files } = fixture();
  files[0].name = 'actual-content-disposition.pdf';
  files[0].content_type = 'application/pdf';
  files[0].bytes = encoded('%PDF-1.7\nSYNTHETIC_NOT_A_PATIENT_DOCUMENT');
  const result = await importDataset(encoded(await exportDataset(lab, snapshot, files, categories)));
  const downloaded = result.attachments[0];
  const links = Object.values(result.snapshot.details.medicine).flatMap(detail => detail.entries.flatMap(entry => entry.attachments ?? []));
  assert.equal(links[0].name, downloaded.name);
  assert.equal(links[0].content_type, 'application/pdf');
  assert.equal(links[0].size, downloaded.bytes.length);
  assert.equal(downloaded.name, 'actual-content-disposition.pdf');
});

test('invalid UTF-8, excessive structure and oversized scalar fields are rejected', async () => {
  await assert.rejects(importDataset(new Uint8Array([0x7b, 0xff, 0x7d])), /UTF-8/);
  await assert.rejects(importDataset(encoded('['.repeat(30) + '0' + ']'.repeat(30))));
  const { lab, snapshot } = fixture();
  snapshot.reports.medicine.entries[0].title = '合成\ud800';
  await assert.rejects(exportDataset(lab, snapshot, [], categories));
  snapshot.reports.medicine.entries[0].title = '中'.repeat(22_000);
  await assert.rejects(exportDataset(lab, snapshot, [], categories));
});

test('multi-MiB attachment restoration stays bounded without recursive regular expression or iterable expansion', async () => {
  const { lab, snapshot, files } = fixture();
  files[0].bytes = new Uint8Array(4 * 1024 * 1024);
  files[0].bytes.set(encoded('%PDF-1.7\nSYNTHETIC_LARGE_DOCUMENT'));
  files[0].content_type = 'application/pdf'; files[0].name = 'synthetic-large.pdf';
  const imported = await importDataset(encoded(await exportDataset(lab, snapshot, files, categories)));
  assert.deepEqual(imported.attachments[0].bytes, files[0].bytes);
});

test('large laboratory datasets use the same 32 MiB portable bound before export and restoration', async () => {
  const { lab, snapshot } = fixture();
  const record = { ...lab.records[0], reference_text: '合成'.repeat(10_000) };
  lab.records = Array.from({ length: 216 }, () => ({ ...record }));
  const output = await exportDataset(lab, snapshot, [], categories);
  const imported = await importDataset(encoded(output));
  assert.ok(encoded(imported.laboratoryJSON).length > 12 * 1024 * 1024);
  assert.ok(encoded(imported.laboratoryJSON).length < MAX_PORTABLE_REPORT_BYTES);
  assert.equal(JSON.parse(imported.laboratoryJSON).records.length, 216);
  assert.equal(JSON.parse(imported.laboratoryJSON).records[215].reference, record.reference_text);
  assert.ok(encoded(portableReportJSON(lab.records)).length > 12 * 1024 * 1024);
  assert.ok(encoded(portableReportXML(lab.records)).length > 12 * 1024 * 1024);

  lab.records = Array.from({ length: 560 }, () => ({ ...record }));
  await assert.rejects(exportDataset(lab, snapshot, [], categories), /32 MiB/);
  assert.throws(() => portableReportJSON(lab.records), /32 MiB/);
  assert.throws(() => portableReportXML(lab.records), /32 MiB/);
  const modified = JSON.parse(output);
  modified.laboratory.records = Array.from({ length: 560 }, () => ({ ...modified.laboratory.records[0] }));
  await assert.rejects(importDataset(encoded(JSON.stringify(modified))), /32 MiB/);
});

test('a source e-prescription with 1,000 refill pairs preserves all 2,008 fields', async () => {
  const { lab, snapshot } = fixture();
  const fields = Array.from({ length: 2008 }, (_, index) => ({ name: `合成來源欄位 ${index}`, value: `保留原值 ${index}` }));
  snapshot.reports.medicine.entries[0].fields = fields;
  const imported = await importDataset(encoded(await exportDataset(lab, snapshot, [], categories)));
  assert.deepEqual(imported.snapshot.reports.medicine.entries[0].fields, fields);
});
