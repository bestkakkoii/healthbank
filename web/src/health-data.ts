// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import type { SupplementEntry, SupplementReport, DataCategory, ReportResponse } from './api';
import { medicalDate, sourceEventDate } from './medicine-timeline.ts';

export type { SupplementEntry, SupplementReport, DataCategory } from './api';
export interface AcquisitionState {
  status: 'unqueried' | 'loading' | 'ready' | 'failed' | 'cancelled';
  error?: string;
  detail_errors: Record<string, string>;
  query?: { from: string; to: string };
}
export interface HealthDataSnapshot {
  reports: Record<string, SupplementReport>;
  details: Record<string, Record<string, SupplementReport>>;
  states: Record<string, AcquisitionState>;
  query: { from: string; to: string };
}
export interface DownloadedAttachment {
  category: string;
  id: string;
  name: string;
  content_type: string;
  bytes: Uint8Array;
}
export interface ArchivedAttachment {
  name: string;
  content_type: string;
  bytes: Uint8Array;
}
export interface HealthEvent {
  id: string;
  category: string;
  categoryLabel: string;
  date: string;
  entry: SupplementEntry;
  detailReference: string;
  parentReference: string;
  fetchedAt: string;
}
export interface EventFilter {
  text: string;
  category: string;
  institution: string;
  from: string;
  to: string;
  includeUnknownDates: boolean;
}

export const MAX_HEALTH_DATA_ENTRIES = 250_000;
export class HealthDataLimitError extends Error {}

export function healthDataEntryCount(snapshot: HealthDataSnapshot): number {
  let count = 0;
  for (const report of Object.values(snapshot.reports)) count += report.entries.length;
  for (const details of Object.values(snapshot.details)) {
    for (const report of Object.values(details)) count += report.entries.length;
  }
  return count;
}

// A renewed detail can retire its child capabilities. Keep only detail tables
// still reachable from the current list; another parent may retain a shared
// child. This removes stale descendants without merging independent records.
export function replaceDetailReport(snapshot: HealthDataSnapshot, category: string, reference: string,
  report: SupplementReport): HealthDataSnapshot {
  const combined = { ...snapshot.details[category], [reference]: report };
  const pending = (snapshot.reports[category]?.entries ?? []).flatMap(entry => entry.detail_id ? [entry.detail_id] : []);
  const reachable = new Set<string>();
  for (const id of pending) {
    if (reachable.has(id)) continue;
    reachable.add(id);
    for (const entry of combined[id]?.entries ?? []) if (entry.detail_id && !reachable.has(entry.detail_id)) pending.push(entry.detail_id);
  }
  if (!reachable.has(reference)) throw new Error('此明細已不屬於目前清單，請重新取得來源。');
  const previous = snapshot.states[category];
  const next: HealthDataSnapshot = { ...snapshot,
    details: { ...snapshot.details, [category]: Object.fromEntries(Object.entries(combined).filter(([id]) => reachable.has(id))) },
    states: { ...snapshot.states, [category]: { ...previous, status: previous?.status === 'failed' ? 'failed' : 'ready',
      detail_errors: Object.fromEntries(Object.entries(previous?.detail_errors ?? {})
        .filter(([id]) => id !== reference && reachable.has(id))) } },
  };
  if (healthDataEntryCount(next) > MAX_HEALTH_DATA_ENTRIES)
    throw new HealthDataLimitError('本次已取得紀錄達 250,000 筆容量上限，未加入不完整明細。請先保存資料包，再縮小查詢期間。');
  return next;
}

export const DEFAULT_DATA_CATEGORIES: DataCategory[] = [
  { id: 'medicine', label: '用藥紀錄', group: '用藥與就醫', range_mode: 'date_search', has_details: true },
  { id: 'chronic', label: '慢性病連續處方箋', group: '用藥與就醫', range_mode: 'source', has_details: true },
  { id: 'western', label: '西醫就醫', group: '用藥與就醫', range_mode: 'date_search', has_details: true },
  { id: 'chinese', label: '中醫就醫', group: '用藥與就醫', range_mode: 'date_search', has_details: true },
  { id: 'dental', label: '牙醫就醫', group: '用藥與就醫', range_mode: 'date_search', has_details: true },
  { id: 'procedure', label: '手術與處置', group: '用藥與就醫', range_mode: 'date_search', has_details: true },
  { id: 'hospital', label: '住院紀錄', group: '用藥與就醫', range_mode: 'date_search', has_details: true },
  { id: 'adult', label: '成人預防保健', group: '檢查與報告', range_mode: 'source' },
  { id: 'self_paid', label: '自費健檢', group: '檢查與報告', range_mode: 'source', has_details: true },
  { id: 'imaging', label: '影像與病理報告', group: '檢查與報告', range_mode: 'source', has_details: true },
  { id: 'recent_medicine', label: '近期用藥摘要', group: '官方近期摘要', range_mode: 'source' },
  { id: 'recent_chronic', label: '近期慢箋摘要', group: '官方近期摘要', range_mode: 'source' },
  { id: 'recent_procedure', label: '近期手術摘要', group: '官方近期摘要', range_mode: 'source' },
  { id: 'recent_hospital', label: '近期住院摘要', group: '官方近期摘要', range_mode: 'source' },
];

export function emptyHealthData(): HealthDataSnapshot {
  return { reports: {}, details: {}, states: {}, query: { from: '', to: '' } };
}

export function validAcquisitionQuery(query: HealthDataSnapshot['query']): boolean {
  if (!query.from && !query.to) return true;
  return /^\d{4}-\d{2}-\d{2}$/.test(query.from) && /^\d{4}-\d{2}-\d{2}$/.test(query.to) &&
    medicalDate(query.from) === query.from && medicalDate(query.to) === query.to &&
    query.from >= '1912-01-01' && query.to <= '2200-12-31' && query.from <= query.to;
}

export function pendingDetailReferences(snapshot: HealthDataSnapshot, category: string): string[] {
  const loaded = snapshot.details[category] ?? {};
  const entries = [...snapshot.reports[category]?.entries ?? [], ...Object.values(loaded).flatMap(report => report.entries)];
  return [...new Set(entries
    .flatMap(entry => entry.detail_id && !Object.hasOwn(loaded, entry.detail_id) ? [entry.detail_id] : []))];
}

export function categoryCoverage(snapshot: HealthDataSnapshot, category: string) {
  const report = snapshot.reports[category];
  const loaded = snapshot.details[category] ?? {};
  const entries = [...report?.entries ?? [], ...Object.values(loaded).flatMap(detail => detail.entries)];
  const references = new Set(entries.flatMap(entry => entry.detail_id ? [entry.detail_id] : []));
  const loadedCount = [...references].filter(reference => Object.hasOwn(loaded, reference)).length;
  const failedCount = Object.keys(snapshot.states[category]?.detail_errors ?? {})
    .filter(reference => references.has(reference) && !Object.hasOwn(loaded, reference)).length;
  return { count: report?.entries.length ?? 0, queried: report !== undefined,
    detailCount: references.size, loadedCount, failedCount, pendingCount: references.size - loadedCount };
}

// Lists and their detail sections are different source records. A parent link is
// explicit only when supplied by a returned opaque detail reference. Similar
// dates, titles or institutions never merge two independent source records.
export function healthEvents(snapshot: HealthDataSnapshot, categories: DataCategory[]): HealthEvent[] {
  const labels = new Map(categories.map(category => [category.id, category.label]));
  const events: HealthEvent[] = [];
  for (const [category, report] of Object.entries(snapshot.reports)) {
    for (const [index, entry] of report.entries.entries()) {
      events.push({ id: JSON.stringify([category, 'list', index]), category,
        categoryLabel: labels.get(category) ?? category, date: sourceEventDate(entry.date), entry,
        detailReference: entry.detail_id ?? '', parentReference: '', fetchedAt: report.fetched_at });
    }
    for (const [reference, detail] of Object.entries(snapshot.details[category] ?? {})) {
      for (const [index, entry] of detail.entries.entries()) {
        events.push({ id: JSON.stringify([category, reference, index]), category,
          categoryLabel: labels.get(category) ?? category, date: sourceEventDate(entry.date), entry,
          detailReference: entry.detail_id ?? '', parentReference: reference, fetchedAt: detail.fetched_at });
      }
    }
  }
  return events.toSorted((a, b) => b.date.localeCompare(a.date) || a.id.localeCompare(b.id));
}

export function laboratoryEvents(report: ReportResponse): HealthEvent[] {
  return report.records.map((record, index) => ({
    id: JSON.stringify(['laboratory', index, record.id]), category: 'laboratory', categoryLabel: '檢驗結果',
    date: sourceEventDate(record.exam_date), detailReference: '', parentReference: '', fetchedAt: report.fetched_at,
    entry: {
      date: record.exam_date, institution: record.institution, title: record.display_name || record.item_name,
      source: record.source,
      fields: [
        { name: '檢驗項目（原文）', value: record.item_name },
        { name: '原始結果', value: record.result, unit: record.unit },
        { name: '原院參考值（原文）', value: record.reference_text },
        { name: '原院參考下限', value: record.reference_low ?? '' },
        { name: '原院參考上限', value: record.reference_high ?? '' },
        { name: '原院旗標（原文）', value: record.official_flag ?? '' },
        { name: '醫令代碼', value: record.order_code },
        { name: '醫令名稱（原文）', value: record.order_name },
        { name: '就醫日期（非採檢日期）', value: record.visit_date },
      ].filter(field => field.value !== ''),
    },
  }));
}

export function filterHealthEvents(events: HealthEvent[], filter: EventFilter): HealthEvent[] {
  const words = filter.text.trim().toLocaleLowerCase('zh-TW').split(/\s+/).filter(Boolean);
  return events.filter(event => {
    if (filter.category && event.category !== filter.category) return false;
    if (filter.institution && event.entry.institution !== filter.institution) return false;
    if (!event.date) {
      if (!filter.includeUnknownDates) return false;
    } else if (filter.from && event.date < filter.from || filter.to && event.date > filter.to) return false;
    if (words.length === 0) return true;
    const entry = event.entry;
    const text = [event.categoryLabel, entry.date, entry.end_date, entry.institution, entry.title, entry.source,
      entry.medicine?.code, entry.medicine?.name, ...entry.fields.flatMap(field => [field.name, field.value, field.unit])]
      .filter(Boolean).join('\n').toLocaleLowerCase('zh-TW');
    return words.every(word => text.includes(word));
  });
}
