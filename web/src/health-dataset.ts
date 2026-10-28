// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import type { ReportResponse, SupplementEntry, SupplementReport, SupplementField, SourceDiagnostic, CacheProvenance, OfficialReportQuery } from './api';
import type { AcquisitionState, DataCategory, DownloadedAttachment, HealthDataSnapshot } from './health-data';
import { validAcquisitionQuery } from './health-data.ts';
import { isObject } from './model.ts';
import { parseStrictJSON } from './strict-json.ts';
import { assertPortableReportSize, portableRows, PORTABLE_REPORT_SCHEMA, LEGACY_PORTABLE_REPORT_SCHEMA } from './portable-report.ts';
import { SOURCE_DATE_ROLES, SOURCE_FIELD_ROLES, SOURCE_ORIGINS, SOURCE_DIAGNOSTIC_CODES } from './source-semantics.ts';

export const DATASET_SCHEMA = 'healthbank.dataset.v2';
export const LEGACY_DATASET_SCHEMA = 'healthbank.dataset.v1';
export interface DatasetProducer {
  application: 'Healthbank'; version: string; connector_revision: string;
  clinical_catalog_id: string; clinical_catalog_revision: string; clinical_evidence_revision: string;
}
export const UNKNOWN_DATASET_PRODUCER: DatasetProducer = { application: 'Healthbank', version: 'unknown', connector_revision: 'unknown',
  clinical_catalog_id: 'unknown', clinical_catalog_revision: 'unknown', clinical_evidence_revision: 'unknown' };
export const MAX_DATASET_BYTES = 128 * 1024 * 1024;
export const MAX_ARCHIVE_ATTACHMENT_BYTES = 32 * 1024 * 1024;
export const MAX_ARCHIVE_ATTACHMENTS = 10_000;
const MAX_ENTRIES = 250_000;
const MAX_CATEGORIES = 200;
const MAX_FIELD_BYTES = 65_536;
const encoder = new TextEncoder();
export type PortableRow = ReturnType<typeof portableRows>[number];
export type LaboratoryMetadata = Pick<ReportResponse, 'fetched_at' | 'complete' | 'warnings' | 'official_query' | 'cache_provenance'>;
export interface ImportedDataset {
  exportedAt: string;
  producer: DatasetProducer | null;
  importedFrom: DatasetProducer | null;
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
  producer: DatasetProducer;
  imported_from?: DatasetProducer;
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
function choice<T extends string>(value: unknown, options: readonly T[]): T {
  const matched = options.find(option => option === value);
  if (matched === undefined) return invalid();
  return matched;
}
function fixedID(value: unknown, field = false): string {
  const result = text(value, field ? 64 : 128);
  if (!(field ? /^[a-z][a-z0-9_]{0,63}$/ : /^[a-z][a-z0-9_.-]{0,127}$/).test(result)) return invalid();
  return result;
}
function producer(value: unknown): DatasetProducer {
  const v = object(value, ['application', 'version', 'connector_revision', 'clinical_catalog_id', 'clinical_catalog_revision', 'clinical_evidence_revision']);
  if (v['application'] !== 'Healthbank') return invalid();
  const revision = (value: unknown) => {
    const result = text(value, 71);
    if (result !== 'unknown' && !/^sha256:[a-f0-9]{64}$/.test(result)) return invalid();
    return result;
  };
  const version = text(v['version'], 100);
  if (!/^[A-Za-z0-9._+-]{1,100}$/.test(version)) return invalid();
  return { application: 'Healthbank', version, connector_revision: fixedID(v['connector_revision']),
    clinical_catalog_id: fixedID(v['clinical_catalog_id']), clinical_catalog_revision: revision(v['clinical_catalog_revision']),
    clinical_evidence_revision: revision(v['clinical_evidence_revision']) };
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
function catalogItems(value: unknown, modern = true): DataCategory[] {
  const seen = new Set<string>();
  return list(value, MAX_CATEGORIES).map(item => {
    const v = object(item, ['id', 'label', 'group', 'range_mode', 'has_details', ...(modern ? ['label_en'] : [])]);
    const id = categoryID(v['id']);
    if (seen.has(id)) return invalid();
    seen.add(id);
    const range = v['range_mode'];
    if (range !== 'date_search' && range !== 'source') return invalid();
    return { id, label: text(v['label'], 500), group: text(v['group'], 500), range_mode: range,
      ...(v['label_en'] === undefined ? {} : { label_en: text(v['label_en'], 500) }),
      ...(v['has_details'] === undefined ? {} : { has_details: boolean(v['has_details']) }) };
  });
}
function assertCombinedCategoryLimit(catalog: DataCategory[], data: HealthDataSnapshot, files: readonly { category: string }[]): void {
  // Import adds fallback labels for reports and retained files absent from the
  // supplied catalog. Bound the resulting catalog on both paths, not just each
  // independently valid input collection.
  const ids = new Set([...catalog.map(item => item.id), ...Object.keys(data.reports), ...files.map(file => file.category)]);
  if (ids.size > MAX_CATEGORIES) return invalid();
}
function officialQuery(value: unknown): OfficialReportQuery {
  const q = object(value, ['mode', 'start_date', 'end_date', 'reported_count']);
  const mode = q['mode'];
  if (mode !== 'all_history' && mode !== 'custom') return invalid();
  const range = query({ from: q['start_date'], to: q['end_date'] });
  if (!range.from || !range.to) return invalid();
  return { mode, start_date: range.from, end_date: range.to,
    reported_count: q['reported_count'] === null ? null : integer(q['reported_count'], Number.MAX_SAFE_INTEGER) };
}
function cacheTimestamp(value: unknown, allowEmpty = false): string {
  const result = text(value, 100);
  if (allowEmpty && !result) return result;
  if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/.test(result) ||
    !validAcquisitionQuery({ from: result.slice(0, 10), to: result.slice(0, 10) }) || !Number.isFinite(Date.parse(result))) return invalid();
  return result;
}
function cacheProvenance(value: unknown): CacheProvenance {
  const v = object(value, ['kind', 'saved_at', 'restored_at', 'source_fetched_at', 'source_complete', 'source_query']);
  if (v['kind'] !== 'device_cache') return invalid();
  return { kind: 'device_cache', saved_at: cacheTimestamp(v['saved_at']), restored_at: cacheTimestamp(v['restored_at']),
    source_fetched_at: cacheTimestamp(v['source_fetched_at'], true), source_complete: boolean(v['source_complete']),
    ...(v['source_query'] === undefined ? {} : { source_query: officialQuery(v['source_query']) }) };
}
function metadata(value: unknown, modern = true): LaboratoryMetadata {
  const v = object(value, ['schema', 'records', 'fetched_at', 'complete', 'warnings', 'official_query', ...(modern ? ['cache_provenance'] : [])]);
  const result: LaboratoryMetadata = { fetched_at: text(v['fetched_at'], 100), complete: boolean(v['complete']),
    warnings: list(v['warnings'], 1000).map(warning => text(warning)) };
  if (v['official_query'] !== undefined) {
    result.official_query = officialQuery(v['official_query']);
  }
  if (v['cache_provenance'] !== undefined) {
    result.cache_provenance = cacheProvenance(v['cache_provenance']);
    if (result.complete || result.official_query || result.fetched_at !== result.cache_provenance.source_fetched_at) return invalid();
  }
  return result;
}
const LAB_FIELDS = ['check_date', 'order_code', 'order_name', 'item_name', 'result', 'unit', 'reference', 'institution', 'visit_date', 'source', 'official_flag'];
function laboratoryRows(value: unknown, modern = true): PortableRow[] {
  return list(value, 50_000).map(item => {
    const v = object(item, [...LAB_FIELDS, ...(modern ? ['upload_date', 'source_name'] : [])]);
    const row = { check_date: text(v['check_date']), order_code: text(v['order_code']), order_name: text(v['order_name']),
      item_name: text(v['item_name']), result: text(v['result']), unit: text(v['unit']), reference: text(v['reference']),
      institution: text(v['institution']), visit_date: text(v['visit_date']), source: text(v['source']), official_flag: text(v['official_flag']),
      ...(v['upload_date'] === undefined ? {} : { upload_date: text(v['upload_date']) }),
      ...(v['source_name'] === undefined ? {} : { source_name: text(v['source_name']) }) };
    if (!row.item_name.trim()) return invalid();
    return row;
  });
}
function mime(value: unknown): string {
  const result = text(value, 100).toLowerCase().split(';')[0]?.trim() ?? '';
  if (!['application/pdf', 'application/zip', 'application/octet-stream', 'application/dicom', 'text/plain', 'text/calendar', 'image/jpeg', 'image/png'].includes(result)) return invalid();
  return result;
}
function filename(value: unknown): string {
  const name = text(value, 500);
  if (!name || /[/\\]/.test(name) || name === '.' || name === '..') return invalid();
  for (const character of name) if (character.charCodeAt(0) < 32 || character.charCodeAt(0) === 127) return invalid();
  return name;
}
function sourceField(value: unknown, modern: boolean): SupplementField {
  const v = object(value, ['name', 'value', 'unit', ...(modern ? ['code', 'role', 'unit_basis'] : [])]);
  return { name: text(v['name']), value: text(v['value']), ...(v['unit'] === undefined ? {} : { unit: text(v['unit']) }),
    ...(v['code'] === undefined ? {} : { code: fixedID(v['code'], true) }),
    ...(v['role'] === undefined ? {} : { role: choice(v['role'], SOURCE_FIELD_ROLES) }),
    ...(v['unit_basis'] === undefined ? {} : { unit_basis: choice(v['unit_basis'], ['source', 'public_contract'] as const) }) };
}
function diagnostics(value: unknown): SourceDiagnostic[] {
  return list(value, 128).map(item => {
    const v = object(item, ['code', 'severity', 'section', 'count', 'message']);
    return { code: choice(v['code'], SOURCE_DIAGNOSTIC_CODES), severity: choice(v['severity'], ['info', 'warning'] as const),
      section: fixedID(v['section']), message: text(v['message'], 1000),
      ...(v['count'] === undefined ? {} : { count: integer(v['count'], 50_000) }) };
  });
}
function entry(value: unknown, modern: boolean): SupplementEntry {
  const v = object(value, ['date', 'institution', 'title', 'source', 'fields', 'detail_id', 'key', 'kind', 'end_date', 'medicine', 'attachments',
    ...(modern ? ['date_role', 'end_date_role', 'origin', 'contract_id'] : [])]);
  const result: SupplementEntry = { date: text(v['date']), institution: text(v['institution']), title: text(v['title']), source: text(v['source']),
    fields: list(v['fields'], 50_000).map(item => sourceField(item, modern)) };
  if (v['date_role'] !== undefined) result.date_role = choice(v['date_role'], SOURCE_DATE_ROLES);
  if (v['end_date_role'] !== undefined) result.end_date_role = choice(v['end_date_role'], SOURCE_DATE_ROLES);
  if (v['origin'] !== undefined) result.origin = choice(v['origin'], SOURCE_ORIGINS);
  if (v['contract_id'] !== undefined) result.contract_id = fixedID(v['contract_id']);
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
function snapshot(value: unknown, modern = true): HealthDataSnapshot {
  const v = object(value, ['reports', 'details', 'states', 'query']);
  const reports = object(v['reports']), details = object(v['details']), states = object(v['states']);
  const ids = new Set([...Object.keys(reports), ...Object.keys(details), ...Object.keys(states)]);
  if (ids.size > MAX_CATEGORIES) return invalid();
  let entries = 0;
  function report(value: unknown): SupplementReport {
    const r = object(value, ['category', 'entries', 'coverage', 'fetched_at', 'start_date', 'end_date', 'complete',
      ...(modern ? ['contract_revision', 'diagnostics', 'cache_provenance'] : [])]);
    const rows = list(r['entries'], 50_000);
    entries += rows.length;
    if (entries > MAX_ENTRIES) return invalid();
    const cached = r['cache_provenance'] === undefined ? undefined : cacheProvenance(r['cache_provenance']);
    if (cached && (boolean(r['complete']) || r['fetched_at'] !== cached.source_fetched_at || cached.source_query)) return invalid();
    return { category: text(r['category'], 150), entries: rows.map(row => entry(row, modern)), coverage: text(r['coverage']), fetched_at: text(r['fetched_at'], 100), complete: boolean(r['complete']),
      ...(cached === undefined ? {} : { cache_provenance: cached }),
      ...(r['contract_revision'] === undefined ? {} : { contract_revision: fixedID(r['contract_revision']) }),
      ...(r['diagnostics'] === undefined ? {} : { diagnostics: diagnostics(r['diagnostics']) }),
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

export const archiveAttachmentMIME = (value: unknown): string => mime(value);
export class ArchiveAttachmentValidator {
  private readonly prefix = new Uint8Array(8);
  private prefixLength = 0;
  private readonly decoder: TextDecoder | null;
  private readonly contentType: string;
  constructor(contentType: string) {
    this.contentType = mime(contentType);
    this.decoder = ['text/plain', 'text/calendar'].includes(this.contentType) ? new TextDecoder('utf-8', { fatal: true }) : null;
  }
  update(bytes: Uint8Array): void {
    const count = Math.min(this.prefix.length - this.prefixLength, bytes.length);
    this.prefix.set(bytes.subarray(0, count), this.prefixLength); this.prefixLength += count;
    // Streaming validation also handles a UTF-8 sequence split across slices.
    this.decoder?.decode(bytes, { stream: true });
  }
  finish(): void {
    this.decoder?.decode();
    const starts = (...prefix: number[]) => prefix.length <= this.prefixLength && prefix.every((byte, index) => this.prefix[index] === byte);
    if (this.contentType === 'application/pdf' && !starts(0x25, 0x50, 0x44, 0x46, 0x2d)) return invalid();
    if (this.contentType === 'application/zip' && !(starts(0x50, 0x4b, 3, 4) || starts(0x50, 0x4b, 5, 6))) return invalid();
    if (this.contentType === 'image/jpeg' && !starts(0xff, 0xd8, 0xff)) return invalid();
    if (this.contentType === 'image/png' && !starts(0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a)) return invalid();
  }
}
function validateAttachment(bytes: Uint8Array, contentType: string): void {
  const validator = new ArchiveAttachmentValidator(contentType); validator.update(bytes); validator.finish();
}

function portableSupplementReport(category: string, source: SupplementReport,
  local: (category: string, kind: string, id: string) => string,
  downloaded: Map<string, DownloadedAttachment>): SupplementReport {
  return {
    category: source.category, coverage: source.coverage, fetched_at: source.fetched_at, complete: source.complete,
    ...(source.start_date === undefined ? {} : { start_date: source.start_date }),
    ...(source.end_date === undefined ? {} : { end_date: source.end_date }),
    ...(source.contract_revision === undefined ? {} : { contract_revision: source.contract_revision }),
    ...(source.diagnostics === undefined ? {} : { diagnostics: source.diagnostics.map(item => ({ ...item })) }),
    ...(source.cache_provenance === undefined ? {} : { cache_provenance: cacheProvenance(source.cache_provenance) }),
    entries: source.entries.map(row => ({ date: row.date, institution: row.institution, title: row.title, source: row.source,
      fields: row.fields.map(field => sourceField(field, true)),
      ...(row.date_role === undefined ? {} : { date_role: row.date_role }),
      ...(row.end_date_role === undefined ? {} : { end_date_role: row.end_date_role }),
      ...(row.origin === undefined ? {} : { origin: row.origin }),
      ...(row.contract_id === undefined ? {} : { contract_id: row.contract_id }),
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

export interface DatasetPackageFile { category: string; id: string; archive_id: string }
export interface DatasetPackageProjection { content: string; files: DatasetPackageFile[] }

async function projectDataset(report: ReportResponse, data: HealthDataSnapshot, files: DownloadedAttachment[], categories: DataCategory[], bom = false,
  producedBy: DatasetProducer = UNKNOWN_DATASET_PRODUCER, importedFrom?: DatasetProducer): Promise<DatasetPackageProjection> {
  if (files.length > MAX_ARCHIVE_ATTACHMENTS) return invalid();
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
    warnings: report.warnings, ...(report.official_query === undefined ? {} : { official_query: report.official_query }),
    ...(report.cache_provenance === undefined ? {} : { cache_provenance: cacheProvenance(report.cache_provenance) }) };
  const document: DatasetDocument = { schema: DATASET_SCHEMA, exported_at: new Date().toISOString(), producer: producer(producedBy),
    ...(importedFrom === undefined ? {} : { imported_from: producer(importedFrom) }),
    laboratory: { schema: PORTABLE_REPORT_SCHEMA, records: labRecords, ...reportMetadata },
    catalog: categories, data: sanitized, attachments };
  // Validate the source-only projection before writing, preserving every row or
  // refusing the whole export. In particular, never truncate a large dataset.
  snapshot(document.data); catalogItems(document.catalog); metadata(document.laboratory);
  assertCombinedCategoryLimit(document.catalog, document.data, attachments);
  const output = `${bom ? '\uFEFF' : ''}${JSON.stringify(document)}\n`;
  if (encoder.encode(output).length > MAX_DATASET_BYTES) throw new Error('資料包超過 128 MiB；請先另存大型附件，再分次整理來源。沒有輸出截斷的檔案。');
  const packageFiles: DatasetPackageFile[] = [];
  for (const [key, archiveID] of handles) {
    const parts: unknown = JSON.parse(key);
    if (Array.isArray(parts) && parts.length === 3 && parts[1] === 'attachment' &&
      typeof parts[0] === 'string' && typeof parts[2] === 'string') {
      packageFiles.push({ category: parts[0], id: parts[2], archive_id: archiveID });
    }
  }
  return { content: output, files: packageFiles };
}

export async function exportDataset(report: ReportResponse, data: HealthDataSnapshot, files: DownloadedAttachment[], categories: DataCategory[], bom = false,
  producedBy: DatasetProducer = UNKNOWN_DATASET_PRODUCER, importedFrom?: DatasetProducer): Promise<string> {
  return (await projectDataset(report, data, files, categories, bom, producedBy, importedFrom)).content;
}

// Live handles stay in the transient package request only. The downloadable
// dataset uses the exact same local archive relationships as ordinary export.
export async function exportDatasetPackage(report: ReportResponse, data: HealthDataSnapshot, categories: DataCategory[],
  producedBy: DatasetProducer = UNKNOWN_DATASET_PRODUCER): Promise<DatasetPackageProjection> {
  const projected = await projectDataset(report, data, [], categories, false, producedBy);
  if (projected.files.some(file => !/^[a-f0-9]{48}$/.test(file.id)))
    throw new Error('部分附件不屬於目前官方工作階段；請重新取得來源，或使用原本的資料包匯出。');
  return projected;
}

export async function importDataset(bytes: Uint8Array): Promise<ImportedDataset> {
  if (bytes.length === 0 || bytes.length > MAX_DATASET_BYTES) return invalid();
  let source: string;
  try { source = new TextDecoder('utf-8', { fatal: true }).decode(bytes).replace(/^\uFEFF/, ''); }
  catch { throw new Error('資料包須為有效 UTF-8 JSON。'); }
  const raw = object(parseStrictJSON(source));
  const modern = raw['schema'] === DATASET_SCHEMA;
  if (!modern && raw['schema'] !== LEGACY_DATASET_SCHEMA) throw new Error('此檔案不是支援的 Healthbank 完整資料包 v1／v2；檢驗 JSON/XML 請使用檢驗匯入。');
  const v = object(raw, ['schema', 'exported_at', 'laboratory', 'catalog', 'data', 'attachments', ...(modern ? ['producer', 'imported_from'] : [])]);
  const exportedAt = text(v['exported_at'], 100);
  const producedBy = modern ? producer(v['producer']) : null;
  const importedFrom = modern && v['imported_from'] !== undefined ? producer(v['imported_from']) : null;
  const lab = object(v['laboratory']);
  const modernLaboratory = lab['schema'] === PORTABLE_REPORT_SCHEMA;
  if (!modernLaboratory && lab['schema'] !== LEGACY_PORTABLE_REPORT_SCHEMA || !modern && modernLaboratory) return invalid();
  const laboratoryMetadata = metadata(lab, modern);
  const laboratoryJSON = JSON.stringify({ schema: lab['schema'], records: laboratoryRows(lab['records'], modernLaboratory) });
  assertPortableReportSize(laboratoryJSON);
  const data = snapshot(v['data'], modern);
  const catalog = catalogItems(v['catalog'], modern);
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
  for (const item of list(v['attachments'], MAX_ARCHIVE_ATTACHMENTS)) {
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
  assertCombinedCategoryLimit(catalog, data, files);
  const known = new Set(catalog.map(category => category.id));
  for (const id of new Set([...Object.keys(data.reports), ...files.map(file => file.category)])) if (!known.has(id)) {
    catalog.push({ id, label: id, group: '資料包來源', range_mode: 'source', has_details: true });
    known.add(id);
  }
  return { laboratoryJSON, laboratoryMetadata, snapshot: data, attachments: files, catalog, exportedAt, producer: producedBy, importedFrom };
}

export function datasetLaboratoryRows(dataset: ImportedDataset): PortableRow[] {
  const document = object(parseStrictJSON(dataset.laboratoryJSON), ['schema', 'records']);
  return laboratoryRows(document['records'], document['schema'] === PORTABLE_REPORT_SCHEMA);
}
