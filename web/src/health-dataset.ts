// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import type { ReportResponse, SupplementEntry, SupplementReport } from './api';
import type { AcquisitionState, DataCategory, DownloadedAttachment, HealthDataSnapshot } from './health-data';
import { validAcquisitionQuery } from './health-data.ts';
import { isObject } from './model.ts';
import { parseStrictJSON } from './strict-json.ts';
import { assertPortableReportSize, portableRows, PORTABLE_REPORT_SCHEMA } from './portable-report.ts';

export const DATASET_SCHEMA = 'healthbank.dataset.v1';
export const MAX_DATASET_BYTES = 128 * 1024 * 1024;
export const MAX_ARCHIVE_ATTACHMENT_BYTES = 32 * 1024 * 1024;
const MAX_ENTRIES = 250_000;
const MAX_CATEGORIES = 200;
const MAX_FIELD_BYTES = 65_536;
const encoder = new TextEncoder();
type PortableRow = ReturnType<typeof portableRows>[number];
export type LaboratoryMetadata = Pick<ReportResponse, 'fetched_at' | 'complete' | 'warnings' | 'official_query'>;
export interface ImportedDataset {
  laboratoryJSON: string;
  laboratoryMetadata: LaboratoryMetadata;
  snapshot: HealthDataSnapshot;
  attachments: DownloadedAttachment[];
  catalog: DataCategory[];
}
interface StoredAttachment {
  category: string;
  id: string;
  name: string;
  content_type: string;
  size: number;
  sha256: string;
  data_base64: string;
}
interface DatasetDocument {
  schema: typeof DATASET_SCHEMA;
  exported_at: string;
  laboratory: LaboratoryMetadata & { schema: typeof PORTABLE_REPORT_SCHEMA; records: PortableRow[] };
  catalog: DataCategory[];
  data: HealthDataSnapshot;
  attachments: StoredAttachment[];
}

function invalid(): never { throw new Error('資料包格式、欄位、識別或容量不符合 Healthbank 規格；原有資料未被取代。'); }
function object(value: unknown, allowed?: readonly string[]): Record<string, unknown> {
  if (!isObject(value)) return invalid();
  if (Object.keys(value).some(key => ['__proto__', 'constructor', 'prototype'].includes(key) || allowed && !allowed.includes(key))) return invalid();
  return value;
}
function text(value: unknown, max = MAX_FIELD_BYTES): string {
  // JSON may contain escaped lone UTF-16 surrogates. Reject them rather than
  // allowing UTF-8 encoding or the Go importer to replace the original value.
  if (typeof value !== 'string' || value.length > max || /[\uD800-\uDFFF]/u.test(value) || encoder.encode(value).length > max) return invalid();
  return value;
}
function boolean(value: unknown): boolean { if (typeof value !== 'boolean') return invalid(); return value; }
function list(value: unknown, max: number): unknown[] { if (!Array.isArray(value) || value.length > max) return invalid(); return value; }
function integer(value: unknown, max: number): number {
  if (typeof value !== 'number' || !Number.isSafeInteger(value) || value < 0 || value > max) return invalid();
  return value;
}
function categoryID(value: unknown): string {
  const id = text(value, 100);
  if (!/^[a-z][a-z0-9_]*$/.test(id) || ['constructor', 'prototype'].includes(id)) return invalid();
  return id;
}
function reference(value: unknown): string {
  const id = text(value, 128);
  // Archive handles are local record relationships, never official capabilities.
  if (!/^archive_[1-9][0-9]*$/.test(id)) return invalid();
  return id;
}
function query(value: unknown): HealthDataSnapshot['query'] {
  const v = object(value, ['from', 'to']);
  const result = { from: text(v['from'], 10), to: text(v['to'], 10) };
  if (!validAcquisitionQuery(result)) return invalid();
  return result;
}
function catalogItems(value: unknown): DataCategory[] {
  const seen = new Set<string>();
  return list(value, MAX_CATEGORIES).map(item => {
    const v = object(item, ['id', 'label', 'group', 'range_mode', 'has_details']);
    const id = categoryID(v['id']);
    if (seen.has(id)) return invalid();
    seen.add(id);
    const range = v['range_mode'];
    if (range !== 'date_search' && range !== 'source') return invalid();
    return { id, label: text(v['label'], 500), group: text(v['group'], 500), range_mode: range,
      ...(v['has_details'] === undefined ? {} : { has_details: boolean(v['has_details']) }) };
  });
}
function metadata(value: unknown): LaboratoryMetadata {
  const v = object(value, ['schema', 'records', 'fetched_at', 'complete', 'warnings', 'official_query']);
  const result: LaboratoryMetadata = { fetched_at: text(v['fetched_at'], 100), complete: boolean(v['complete']),
    warnings: list(v['warnings'], 1000).map(warning => text(warning)) };
  if (v['official_query'] !== undefined) {
    const q = object(v['official_query'], ['mode', 'start_date', 'end_date', 'reported_count']);
    const mode = q['mode'];
    if (mode !== 'all_history' && mode !== 'custom') return invalid();
    const range = query({ from: q['start_date'], to: q['end_date'] });
    if (!range.from || !range.to) return invalid();
    result.official_query = { mode, start_date: range.from, end_date: range.to,
      reported_count: q['reported_count'] === null ? null : integer(q['reported_count'], Number.MAX_SAFE_INTEGER) };
  }
  return result;
}
const LAB_FIELDS = ['check_date', 'order_code', 'order_name', 'item_name', 'result', 'unit', 'reference', 'institution', 'visit_date', 'source', 'official_flag'];
function laboratoryRows(value: unknown): PortableRow[] {
  return list(value, 50_000).map(item => {
    const v = object(item, LAB_FIELDS);
    const row = { check_date: text(v['check_date']), order_code: text(v['order_code']), order_name: text(v['order_name']),
      item_name: text(v['item_name']), result: text(v['result']), unit: text(v['unit']), reference: text(v['reference']),
      institution: text(v['institution']), visit_date: text(v['visit_date']), source: text(v['source']), official_flag: text(v['official_flag']) };
    if (!row.item_name.trim()) return invalid();
    return row;
  });
}
function mime(value: unknown): string {
  const result = text(value, 100).toLowerCase().split(';')[0]?.trim() ?? '';
  if (!['application/pdf', 'application/zip', 'application/octet-stream', 'application/dicom', 'text/plain', 'image/jpeg'].includes(result)) return invalid();
  return result;
}
function filename(value: unknown): string {
  const name = text(value, 500);
  if (!name || /[/\\]/.test(name) || name === '.' || name === '..') return invalid();
  for (const character of name) if (character.charCodeAt(0) < 32 || character.charCodeAt(0) === 127) return invalid();
  return name;
}
function entry(value: unknown): SupplementEntry {
  const v = object(value, ['date', 'institution', 'title', 'source', 'fields', 'detail_id', 'key', 'kind', 'end_date', 'medicine', 'attachments']);
  const result: SupplementEntry = { date: text(v['date']), institution: text(v['institution']), title: text(v['title']), source: text(v['source']),
    fields: list(v['fields'], 50_000).map(item => {
      const field = object(item, ['name', 'value', 'unit']);
      return { name: text(field['name']), value: text(field['value']), ...(field['unit'] === undefined ? {} : { unit: text(field['unit']) }) };
    }) };
  for (const key of ['key', 'kind', 'end_date'] as const) if (v[key] !== undefined) result[key] = text(v[key]);
  if (v['detail_id'] !== undefined) result.detail_id = reference(v['detail_id']);
  if (v['medicine'] !== undefined) {
    const medicine = object(v['medicine'], ['code', 'name', 'quantity', 'days']);
    result.medicine = { code: text(medicine['code']), name: text(medicine['name']), quantity: text(medicine['quantity']), days: text(medicine['days']) };
  }
  if (v['attachments'] !== undefined) result.attachments = list(v['attachments'], 500).map(item => {
    const file = object(item, ['id', 'name', 'content_type', 'size']);
    return { id: reference(file['id']), name: filename(file['name']), content_type: mime(file['content_type']),
      ...(file['size'] === undefined ? {} : { size: integer(file['size'], 500 * 1024 * 1024) }) };
  });
  return result;
}
function snapshot(value: unknown): HealthDataSnapshot {
  const v = object(value, ['reports', 'details', 'states', 'query']);
  const reports = object(v['reports']), details = object(v['details']), states = object(v['states']);
  const ids = new Set([...Object.keys(reports), ...Object.keys(details), ...Object.keys(states)]);
  if (ids.size > MAX_CATEGORIES) return invalid();
  let entries = 0;
  function report(value: unknown): SupplementReport {
    const r = object(value, ['category', 'entries', 'coverage', 'fetched_at', 'start_date', 'end_date', 'complete']);
    const rows = list(r['entries'], 50_000);
    entries += rows.length;
    if (entries > MAX_ENTRIES) return invalid();
    return { category: text(r['category'], 150), entries: rows.map(entry), coverage: text(r['coverage']), fetched_at: text(r['fetched_at'], 100), complete: boolean(r['complete']),
      ...(r['start_date'] === undefined ? {} : { start_date: text(r['start_date'], 100) }),
      ...(r['end_date'] === undefined ? {} : { end_date: text(r['end_date'], 100) }) };
  }
  const result: HealthDataSnapshot = { reports: {}, details: {}, states: {}, query: query(v['query']) };
  for (const key of ids) {
    const id = categoryID(key);
    if (reports[id] !== undefined) {
      const source = report(reports[id]);
      if (source.category !== id) return invalid();
      result.reports[id] = source;
    }
    if (details[id] !== undefined) {
      const records = object(details[id]);
      if (!result.reports[id] || Object.keys(records).length > 50_000) return invalid();
      result.details[id] = Object.fromEntries(Object.entries(records).map(([key, value]) => [reference(key), report(value)]));
      if (Object.values(result.details[id]).some(source => source.category !== id && source.category !== `${id}_detail`)) return invalid();
    }
    if (states[id] !== undefined) {
      const state = object(states[id], ['status', 'error', 'detail_errors', 'query']);
      const status = state['status'];
      if (status !== 'unqueried' && status !== 'loading' && status !== 'ready' && status !== 'failed' && status !== 'cancelled') return invalid();
      const errors = object(state['detail_errors']);
      if (Object.keys(errors).length > 50_000) return invalid();
      const parsed: AcquisitionState = { status: status === 'loading' ? 'cancelled' : status,
        detail_errors: Object.fromEntries(Object.entries(errors).map(([key, value]) => [reference(key), text(value)])) };
      if (state['error'] !== undefined) parsed.error = text(state['error']);
      if (state['query'] !== undefined) parsed.query = query(state['query']);
      result.states[id] = parsed;
    }
    // Reject unlinked detail tables; they must be reachable from a list or from
    // another returned detail, not fabricated provenance based on matching dates.
    const loaded = result.details[id] ?? {};
    const reachable = new Set<string>();
    const queue = result.reports[id]?.entries.flatMap(row => row.detail_id ? [row.detail_id] : []) ?? [];
    for (const ref of queue) {
      if (reachable.has(ref)) continue;
      reachable.add(ref);
      for (const row of loaded[ref]?.entries ?? []) if (row.detail_id) queue.push(row.detail_id);
    }
    if (Object.keys(loaded).some(ref => !reachable.has(ref))) return invalid();
  }
  return result;
}


function base64(bytes: Uint8Array): string {
  const chunks: string[] = [];
  for (let start = 0; start < bytes.length; start += 8192) chunks.push(String.fromCharCode(...bytes.subarray(start, start + 8192)));
  return btoa(chunks.join(''));
}
function unbase64(value: string, expected: number): Uint8Array {
  // Avoid repeated capture groups and iterable materialization for multi-MiB
  // files: both can exhaust the stack/heap before the explicit byte ceiling.
  if (value.length !== 4 * Math.ceil(expected / 3) || !/^[A-Za-z0-9+/]*={0,2}$/.test(value)) return invalid();
  const decoded = atob(value);
  if (decoded.length !== expected) return invalid();
  const bytes = new Uint8Array(expected);
  for (let index = 0; index < expected; index++) bytes[index] = decoded.charCodeAt(index);
  return bytes;
}
async function sha256(bytes: Uint8Array): Promise<string> {
  const digest = await crypto.subtle.digest('SHA-256', new Uint8Array(bytes));
  return [...new Uint8Array(digest)].map(byte => byte.toString(16).padStart(2, '0')).join('');
}

function validateAttachment(bytes: Uint8Array, contentType: string): void {
  const starts = (...prefix: number[]) => prefix.every((byte, index) => bytes[index] === byte);
  if (contentType === 'application/pdf' && !starts(0x25, 0x50, 0x44, 0x46, 0x2d)) return invalid();
  if (contentType === 'application/zip' && !(starts(0x50, 0x4b, 3, 4) || starts(0x50, 0x4b, 5, 6))) return invalid();
  if (contentType === 'image/jpeg' && !starts(0xff, 0xd8, 0xff)) return invalid();
  if (contentType === 'text/plain') {
    try { new TextDecoder('utf-8', { fatal: true }).decode(bytes); }
    catch { return invalid(); }
  }
}

function portableSupplementReport(category: string, source: SupplementReport,
  local: (category: string, kind: string, id: string) => string,
  downloaded: Map<string, DownloadedAttachment>): SupplementReport {
  return {
    category: source.category, coverage: source.coverage, fetched_at: source.fetched_at, complete: source.complete,
    ...(source.start_date === undefined ? {} : { start_date: source.start_date }),
    ...(source.end_date === undefined ? {} : { end_date: source.end_date }),
    entries: source.entries.map(row => ({ date: row.date, institution: row.institution, title: row.title, source: row.source,
      fields: row.fields.map(field => ({ name: field.name, value: field.value, ...(field.unit === undefined ? {} : { unit: field.unit }) })),
      ...(row.kind === undefined ? {} : { kind: row.kind }), ...(row.end_date === undefined ? {} : { end_date: row.end_date }),
      ...(row.medicine === undefined ? {} : { medicine: { ...row.medicine } }),
      ...(row.detail_id ? { detail_id: local(category, 'detail', row.detail_id) } : {}),
      ...(row.attachments === undefined ? {} : { attachments: row.attachments.map(file => {
        // The source list supplies a display hint. A completed download has the
        // inspected response filename/type and exact byte size; use these for
        // both sides of the archive relationship so our own export round-trips.
        const actual = downloaded.get(JSON.stringify([category, file.id]));
        const size = actual?.bytes.length ?? file.size;
        return { id: local(category, 'attachment', file.id), name: actual?.name ?? file.name,
          content_type: mime(actual?.content_type ?? file.content_type),
          ...(size === undefined ? {} : { size }) };
      }) }),
    })),
  };
}

// Standalone list downloads share the whole-dataset projection: local archive
// relationships preserve source structure without exporting live capabilities.
export function supplementReportJSON(report: SupplementReport): string {
  const handles = new Map<string, string>();
  const local = (category: string, kind: string, id: string) => {
    const key = JSON.stringify([category, kind, id]);
    let value = handles.get(key);
    if (!value) { value = `archive_${handles.size + 1}`; handles.set(key, value); }
    return value;
  };
  const output = `${JSON.stringify(portableSupplementReport(report.category, report, local, new Map()), null, 2)}\n`;
  if (encoder.encode(output).length > MAX_DATASET_BYTES) throw new Error('來源清單超過 128 MiB，未輸出截斷內容。');
  return output;
}

export async function exportDataset(report: ReportResponse, data: HealthDataSnapshot, files: DownloadedAttachment[], categories: DataCategory[], bom = false): Promise<string> {
  if (files.length > 10_000) return invalid();
  const labRecords = laboratoryRows(portableRows(report.records));
  // This is the exact body sent to the laboratory importer on restore. Check
  // the same bound on both paths before producing an un-restorable archive.
  assertPortableReportSize(JSON.stringify({ schema: PORTABLE_REPORT_SCHEMA, records: labRecords }));
  const handles = new Map<string, string>();
  const downloaded = new Map(files.map(file => [JSON.stringify([file.category, file.id]), file]));
  const local = (category: string, kind: string, id: string) => {
    const key = JSON.stringify([category, kind, id]);
    let handle = handles.get(key);
    if (!handle) { handle = `archive_${handles.size + 1}`; handles.set(key, handle); }
    return handle;
  };

  const sanitized: HealthDataSnapshot = {
    reports: Object.fromEntries(Object.entries(data.reports).map(([category, value]) => [category, portableSupplementReport(category, value, local, downloaded)])),
    details: Object.fromEntries(Object.entries(data.details).map(([category, values]) => [category,
      Object.fromEntries(Object.entries(values).map(([id, value]) => [local(category, 'detail', id), portableSupplementReport(category, value, local, downloaded)]))])),
    states: Object.fromEntries(Object.entries(data.states).map(([category, state]) => [category, {
      status: state.status === 'loading' ? 'cancelled' : state.status,
      ...(state.error === undefined ? {} : { error: state.error }),
      ...(state.query === undefined ? {} : { query: { ...state.query } }),
      detail_errors: Object.fromEntries(Object.entries(state.detail_errors).map(([key, value]) => [local(category, 'detail', key), value])),
    }])), query: { ...data.query },
  };
  const attachments: StoredAttachment[] = [];
  let total = 0;
  const unique = new Set<string>();
  for (const file of files) {
    total += file.bytes.length;
    const id = local(file.category, 'attachment', file.id);
    if (unique.has(id) || total > MAX_ARCHIVE_ATTACHMENT_BYTES) return invalid();
    unique.add(id);
    const contentType = mime(file.content_type);
    validateAttachment(file.bytes, contentType);
    attachments.push({ category: categoryID(file.category), id, name: filename(file.name), content_type: contentType,
      size: file.bytes.length, sha256: await sha256(file.bytes), data_base64: base64(file.bytes) });
  }
  const reportMetadata: LaboratoryMetadata = { fetched_at: report.fetched_at, complete: report.complete,
    warnings: report.warnings, ...(report.official_query === undefined ? {} : { official_query: report.official_query }) };
  const document: DatasetDocument = { schema: DATASET_SCHEMA, exported_at: new Date().toISOString(),
    laboratory: { schema: PORTABLE_REPORT_SCHEMA, records: labRecords, ...reportMetadata },
    catalog: categories, data: sanitized, attachments };
  // Validate the source-only projection before writing, preserving every row or
  // refusing the whole export. In particular, never truncate a large dataset.
  snapshot(document.data); catalogItems(document.catalog); metadata(document.laboratory);
  const output = `${bom ? '\uFEFF' : ''}${JSON.stringify(document)}\n`;
  if (encoder.encode(output).length > MAX_DATASET_BYTES) throw new Error('資料包超過 128 MiB；請先另存大型附件，再分次整理來源。沒有輸出截斷的檔案。');
  return output;
}

export async function importDataset(bytes: Uint8Array): Promise<ImportedDataset> {
  if (bytes.length === 0 || bytes.length > MAX_DATASET_BYTES) return invalid();
  let source: string;
  try { source = new TextDecoder('utf-8', { fatal: true }).decode(bytes).replace(/^\uFEFF/, ''); }
  catch { throw new Error('資料包須為有效 UTF-8 JSON。'); }
  const v = object(parseStrictJSON(source), ['schema', 'exported_at', 'laboratory', 'catalog', 'data', 'attachments']);
  if (v['schema'] !== DATASET_SCHEMA) throw new Error('此檔案不是 Healthbank 完整資料包 healthbank.dataset.v1；檢驗 JSON/XML 請使用檢驗匯入。');
  text(v['exported_at'], 100);
  const lab = object(v['laboratory']);
  if (lab['schema'] !== PORTABLE_REPORT_SCHEMA) return invalid();
  const laboratoryMetadata = metadata(lab);
  const laboratoryJSON = JSON.stringify({ schema: PORTABLE_REPORT_SCHEMA, records: laboratoryRows(lab['records']) });
  assertPortableReportSize(laboratoryJSON);
  const data = snapshot(v['data']);
  const catalog = catalogItems(v['catalog']);
  const files: DownloadedAttachment[] = [];
  const references = new Map<string, { category: string; name: string; content_type: string }>();
  for (const [category, report] of Object.entries(data.reports)) {
    for (const row of [...report.entries, ...Object.values(data.details[category] ?? {}).flatMap(detail => detail.entries)]) {
      for (const file of row.attachments ?? []) {
        const previous = references.get(file.id);
        if (previous && (previous.category !== category || previous.name !== file.name || previous.content_type !== file.content_type)) return invalid();
        references.set(file.id, { category, name: file.name, content_type: file.content_type });
      }
    }
  }
  let total = 0;
  const seen = new Set<string>();
  for (const item of list(v['attachments'], 10_000)) {
    const file = object(item, ['category', 'id', 'name', 'content_type', 'size', 'sha256', 'data_base64']);
    const id = reference(file['id']), category = categoryID(file['category']);
    const name = filename(file['name']), contentType = mime(file['content_type']);
    const size = integer(file['size'], MAX_ARCHIVE_ATTACHMENT_BYTES);
    total += size;
    const expected = references.get(id);
    if (seen.has(id) || total > MAX_ARCHIVE_ATTACHMENT_BYTES || expected && (expected.category !== category || expected.name !== name || expected.content_type !== contentType)) return invalid();
    seen.add(id);
    const digest = text(file['sha256'], 64);
    if (!/^[a-f0-9]{64}$/.test(digest)) return invalid();
    const decoded = unbase64(text(file['data_base64'], MAX_DATASET_BYTES), size);
    if (await sha256(decoded) !== digest) throw new Error('資料包附件 SHA-256 不符；沒有匯入損壞的附件或部分資料。');
    validateAttachment(decoded, contentType);
    files.push({ category, id, name, content_type: contentType, bytes: decoded });
  }
  const known = new Set(catalog.map(category => category.id));
  for (const id of new Set([...Object.keys(data.reports), ...files.map(file => file.category)])) if (!known.has(id)) {
    catalog.push({ id, label: id, group: '資料包來源', range_mode: 'source', has_details: true });
    known.add(id);
  }
  return { laboratoryJSON, laboratoryMetadata, snapshot: data, attachments: files, catalog };
}
