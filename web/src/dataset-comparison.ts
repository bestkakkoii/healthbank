// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import type { SupplementEntry } from './api';
import { datasetLaboratoryRows, type ImportedDataset } from './health-dataset.ts';
import { isObject } from './model.ts';

export interface DatasetDifference {
  category: string;
  kind: 'laboratory' | 'list' | 'detail' | 'attachment';
  title: string;
  date: string;
  before: number;
  after: number;
  content: string;
}
export interface DatasetComparison {
  beforeExportedAt: string;
  afterExportedAt: string;
  beforeCount: number;
  afterCount: number;
  addedOccurrences: number;
  previousOnlyOccurrences: number;
  unchangedOccurrences: number;
  rangeWarnings: string[];
  producerChanged: boolean;
  clinicalRulesChanged: boolean;
  differences: DatasetDifference[];
}
interface Occurrence { count: number; row: Omit<DatasetDifference, 'before' | 'after'> }

// A canonical source projection excludes row indices, live/archive handles,
// fetch times and local interpretation. It never invents a stable medical ID.
function canonicalEntry(entry: SupplementEntry) {
  return { date: entry.date, end_date: entry.end_date ?? '', institution: entry.institution, title: entry.title, source: entry.source,
    date_role: entry.date_role ?? '', end_date_role: entry.end_date_role ?? '', origin: entry.origin ?? '', contract_id: entry.contract_id ?? '',
    kind: entry.kind ?? '', fields: entry.fields.map(field => [field.name, field.value, field.unit ?? '', field.code ?? '', field.role ?? '', field.unit_basis ?? ''])
      .toSorted((a, b) => JSON.stringify(a).localeCompare(JSON.stringify(b))),
    medicine: entry.medicine ? [entry.medicine.code, entry.medicine.name, entry.medicine.quantity, entry.medicine.days] : null,
    attachments: (entry.attachments ?? []).map(file => [file.name, file.content_type, file.size ?? null]).toSorted((a, b) => JSON.stringify(a).localeCompare(JSON.stringify(b))) };
}
async function fingerprint(bytes: Uint8Array) {
  const digest = await crypto.subtle.digest('SHA-256', new Uint8Array(bytes));
  return [...new Uint8Array(digest)].map(byte => byte.toString(16).padStart(2, '0')).join('');
}
const encoder = new TextEncoder();
interface ParentEdge { parent: string; content: string }
interface AttachmentParents { edges: Set<string>; reliable: boolean }

// Each table is identified by a hash of its incoming source rows and their
// ancestry, not by an archive/live handle. Topological processing keeps a long
// or shared ancestry bounded: it hashes fixed-size parent fingerprints rather
// than expanding every possible path. Duplicate parent rows do not change the
// identity of the single detail table they share.
async function ancestryContexts(entries: SupplementEntry[], details: Record<string, { entries: SupplementEntry[] }>, canonical: (entry: SupplementEntry) => string) {
  const incoming = new Map<string, ParentEdge[]>();
  const parents = new Map<string, Set<string>>();
  const children = new Map<string, Set<string>>();
  for (const reference of Object.keys(details)) incoming.set(reference, []);
  const connect = (parent: string, entry: SupplementEntry) => {
    const reference = entry.detail_id;
    if (!reference) return;
    const edges = incoming.get(reference) ?? [];
    edges.push({ parent, content: canonical(entry) }); incoming.set(reference, edges);
    if (parent) {
      const previous = parents.get(reference) ?? new Set<string>(); previous.add(parent); parents.set(reference, previous);
      const next = children.get(parent) ?? new Set<string>(); next.add(reference); children.set(parent, next);
    }
  };
  for (const entry of entries) connect('', entry);
  for (const [reference, detail] of Object.entries(details)) for (const entry of detail.entries) connect(reference, entry);
  const remaining = new Map([...incoming.keys()].map(reference => [reference, parents.get(reference)?.size ?? 0]));
  const pending = [...incoming.keys()].filter(reference => remaining.get(reference) === 0);
  const contexts = new Map<string, string>();
  for (let index = 0; index < pending.length;) {
    // Limit in-flight hashing even for a wide graph. A batch only contains
    // tables whose dependencies were completed by an earlier batch.
    const batch = pending.slice(index, index + 32); index += batch.length;
    const values = await Promise.all(batch.map(async reference => {
      const edges = [...new Set((incoming.get(reference) ?? []).map(edge => JSON.stringify([edge.parent ? contexts.get(edge.parent) : 'root', edge.content])))].toSorted();
      return [reference, await fingerprint(encoder.encode(JSON.stringify(edges)))] as const;
    }));
    for (const [reference, context] of values) {
      contexts.set(reference, context);
      for (const child of children.get(reference) ?? []) {
        const count = (remaining.get(child) ?? 0) - 1; remaining.set(child, count);
        if (count === 0) pending.push(child);
      }
    }
  }
  // A cycle and every descendant depending on it remain unresolved. Treating
  // a handle, traversal order or a truncated ancestry as identity would claim
  // equality that the source graph cannot establish.
  return contexts;
}

async function occurrences(dataset: ImportedDataset, side: 'before' | 'after'): Promise<{ rows: Map<string, Occurrence>; unreliableCategories: Set<string> }> {
  const map = new Map<string, Occurrence>();
  const unreliableCategories = new Set<string>();
  const attachmentParents = new Map<string, AttachmentParents>();
  const downloaded = new Set(dataset.attachments.map(file => JSON.stringify([file.category, file.id])));
  const canonicalRows = new WeakMap<SupplementEntry, string>();
  const canonical = (entry: SupplementEntry) => {
    let value = canonicalRows.get(entry);
    if (value === undefined) { value = JSON.stringify(canonicalEntry(entry)); canonicalRows.set(entry, value); }
    return value;
  };
  const add = (row: Omit<DatasetDifference, 'before' | 'after'>, context = '') => {
    const key = JSON.stringify([row.category, row.kind, context, row.content]);
    const prior = map.get(key);
    if (prior) prior.count++; else map.set(key, { count: 1, row });
  };
  for (const row of datasetLaboratoryRows(dataset)) add({ category: 'laboratory', kind: 'laboratory', title: row.item_name, date: row.check_date, content: JSON.stringify(row) });
  for (const [category, report] of Object.entries(dataset.snapshot.reports)) {
    const details = dataset.snapshot.details[category] ?? {};
    const contexts = await ancestryContexts(report.entries, details, canonical);
    const attach = (entry: SupplementEntry, context: string | undefined) => {
      for (const file of entry.attachments ?? []) {
        const key = JSON.stringify([category, file.id]);
        if (!downloaded.has(key)) continue;
        const parents = attachmentParents.get(key) ?? { edges: new Set<string>(), reliable: true };
        if (context === undefined) parents.reliable = false;
        else parents.edges.add(JSON.stringify([context, canonical(entry)]));
        attachmentParents.set(key, parents);
      }
    };
    for (const entry of report.entries) {
      add({ category, kind: 'list', title: entry.title, date: entry.date, content: canonical(entry) });
      attach(entry, 'root');
    }
    for (const [reference, detail] of Object.entries(details)) {
      const context = contexts.get(reference);
      if (context === undefined) unreliableCategories.add(category);
      for (const entry of detail.entries) {
        add({ category, kind: 'detail', title: entry.title, date: entry.date, content: canonical(entry) }, context ?? `unresolved:${side}`);
        attach(entry, context);
      }
    }
  }
  for (const file of dataset.attachments) {
    const content = JSON.stringify({ name: file.name, content_type: file.content_type, size: file.bytes.length, sha256: await fingerprint(file.bytes) });
    const parents = attachmentParents.get(JSON.stringify([file.category, file.id]));
    let context = 'unlinked';
    if (parents && !parents.reliable) {
      unreliableCategories.add(file.category); context = `unresolved:${side}`;
    } else if (parents) context = await fingerprint(encoder.encode(JSON.stringify([...parents.edges].toSorted())));
    add({ category: file.category, kind: 'attachment', title: file.name, date: '', content }, context);
  }
  return { rows: map, unreliableCategories };
}
function ranges(dataset: ImportedDataset) {
  const result = new Map<string, string>();
  const query = dataset.laboratoryMetadata.official_query;
  result.set('laboratory', query ? JSON.stringify([query.start_date, query.end_date]) : 'unknown');
  for (const [category, source] of Object.entries(dataset.snapshot.reports)) {
    result.set(category, JSON.stringify([source.start_date ?? '', source.end_date ?? '', dataset.snapshot.states[category]?.query ?? null]));
  }
  return result;
}

export async function compareDatasets(before: ImportedDataset, after: ImportedDataset): Promise<DatasetComparison> {
  const previousRows = await occurrences(before, 'before'), currentRows = await occurrences(after, 'after');
  const left = previousRows.rows, right = currentRows.rows;
  const differences: DatasetDifference[] = [];
  let beforeCount = 0, afterCount = 0, addedOccurrences = 0, previousOnlyOccurrences = 0, unchangedOccurrences = 0;
  for (const key of new Set([...left.keys(), ...right.keys()])) {
    const previous = left.get(key), current = right.get(key);
    const oldCount = previous?.count ?? 0, newCount = current?.count ?? 0;
    beforeCount += oldCount; afterCount += newCount;
    unchangedOccurrences += Math.min(oldCount, newCount);
    addedOccurrences += Math.max(0, newCount - oldCount);
    previousOnlyOccurrences += Math.max(0, oldCount - newCount);
    const representative = current ?? previous;
    if (oldCount !== newCount && representative) differences.push({ ...representative.row, before: oldCount, after: newCount });
  }
  const leftRanges = ranges(before), rightRanges = ranges(after);
  const labels = new Map([...before.catalog, ...after.catalog].map(category => [category.id, category.label]));
  const rangeWarnings: string[] = [];
  for (const category of new Set([...previousRows.unreliableCategories, ...currentRows.unreliableCategories]))
    rangeWarnings.push(`${labels.get(category) ?? category}：部分明細／附件的來源關係含有循環或無法可靠對照；相關內容分列為前次／本次，未判定為相同紀錄。`);
  for (const category of new Set([...leftRanges.keys(), ...rightRanges.keys()])) {
    if (!leftRanges.has(category) || !rightRanges.has(category)) rangeWarnings.push(`${labels.get(category) ?? category}：兩份檔案收錄的來源不同。`);
    else if (leftRanges.get(category) !== rightRanges.get(category)) rangeWarnings.push(`${labels.get(category) ?? category}：查詢期間或範圍資訊不同，差異不能解讀為官方新增或刪除。`);
  }
  if (!before.producer || !after.producer) rangeWarnings.push('至少一份為舊版資料包，沒有產生程式與判讀版本資訊。');
  const rules = (dataset: ImportedDataset) => dataset.producer ? JSON.stringify([dataset.producer.clinical_catalog_id, dataset.producer.clinical_catalog_revision, dataset.producer.clinical_evidence_revision]) : 'unknown';
  return { beforeExportedAt: before.exportedAt, afterExportedAt: after.exportedAt, beforeCount, afterCount, addedOccurrences, previousOnlyOccurrences, unchangedOccurrences,
    rangeWarnings, producerChanged: JSON.stringify(before.producer) !== JSON.stringify(after.producer), clinicalRulesChanged: rules(before) !== rules(after),
    differences: differences.toSorted((a, b) => a.category.localeCompare(b.category) || b.date.localeCompare(a.date) || a.title.localeCompare(b.title)) };
}

export function datasetDifferenceFields(row: DatasetDifference): { name: string; value: string }[] {
  const value: unknown = JSON.parse(row.content);
  if (!isObject(value)) return [];
  const names: Record<string, string> = row.kind === 'laboratory'
    ? { check_date: '檢查日期（原文）', order_code: '醫令代碼', order_name: '醫令名稱', item_name: '檢驗項目', result: '原始結果', unit: '原始單位', reference: '原院參考值', institution: '院所', visit_date: '就醫日期（原文）', source: '來源', official_flag: '原院旗標' }
    : row.kind === 'attachment' ? { name: '附件名稱', content_type: '附件類型', size: '位元組數', sha256: '附件 SHA-256' }
      : { date: '來源日期', end_date: '來源結束日期', date_role: '來源日期用途', end_date_role: '來源結束日期用途', origin: '來源紀錄性質', contract_id: '來源契約識別',
        institution: '院所', title: '紀錄標題', source: '來源', kind: '來源紀錄類型' };
  const fields = Object.entries(names).map(([key, name]) => ({ name, value: typeof value[key] === 'string' || typeof value[key] === 'number' ? String(value[key]) : '' }));
  const rawFields: unknown = value['fields'];
  if (Array.isArray(rawFields)) for (const field of rawFields) {
    if (Array.isArray(field) && typeof field[0] === 'string' && typeof field[1] === 'string') {
      fields.push({ name: field[0], value: `${field[1]}${typeof field[2] === 'string' && field[2] ? ` ${field[2]}` : ''}` });
      for (const [index, label] of [[3, '欄位識別'], [4, '欄位用途'], [5, '單位依據']] as const)
        if (typeof field[index] === 'string' && field[index]) fields.push({ name: `${field[0]}：${label}`, value: field[index] });
    }
  }
  return fields;
}
