// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import type { ReportResponse } from './api';
import { categoryCoverage, emptyHealthData, healthDataEntryCount, MAX_HEALTH_DATA_ENTRIES, validAcquisitionQuery,
  type DataCategory, type HealthDataSnapshot } from './health-data.ts';

export type BatchPeriod = 'year' | 'month';
export type BatchRange = HealthDataSnapshot['query'];
export const MAX_DATE_BATCHES = 600;
export interface DateBatchPlan { ranges: BatchRange[]; categories: string[]; includeLaboratory: boolean }
export interface DateBatchSummary { from: string; to: string; entries: number; laboratory: number; sources: number }

function dateText(date: Date): string { return date.toISOString().slice(0, 10); }

// Each inclusive interval ends immediately before the next begins. UTC calendar
// arithmetic avoids host timezone/DST changes and preserves leap-day boundaries.
export function splitDateBatches(range: BatchRange, period: BatchPeriod): BatchRange[] {
  if (!range.from || !range.to || !validAcquisitionQuery(range)) throw new Error('分批取得須填寫有效的起迄日期；起日不得晚於迄日。');
  const ranges: BatchRange[] = [];
  let from = range.from;
  while (from <= range.to) {
    const start = new Date(`${from}T00:00:00Z`);
    const next = period === 'year' ? new Date(Date.UTC(start.getUTCFullYear() + 1, 0, 1))
      : new Date(Date.UTC(start.getUTCFullYear(), start.getUTCMonth() + 1, 1));
    const end = dateText(new Date(next.getTime() - 86_400_000));
    const to = end < range.to ? end : range.to;
    ranges.push({ from, to });
    if (ranges.length > MAX_DATE_BATCHES) throw new Error('一次計畫最多 600 批；請改用按年，或縮小本次期間。');
    if (to === range.to) break;
    from = dateText(next);
  }
  return ranges;
}

export function selectedDateCategories(catalog: DataCategory[], selected: string[]) {
  const ids = new Set(selected);
  return { eligible: catalog.filter(item => ids.has(item.id) && item.range_mode === 'date_search'),
    excluded: catalog.filter(item => ids.has(item.id) && item.range_mode !== 'date_search') };
}

export function beginDateBatch(range: BatchRange, categories: string[]): HealthDataSnapshot {
  if (!range.from || !validAcquisitionQuery(range)) throw new Error('分批期間不正確。');
  return { ...emptyHealthData(), query: { ...range }, states: Object.fromEntries(categories.map(category =>
    [category, { status: 'unqueried', detail_errors: {}, query: { ...range } }])) };
}

// Only the explicitly selected categories are replaced. No old list, detail,
// failure state or lab response is carried from one batch into the next archive.
export function replaceBatchCategories(workspace: HealthDataSnapshot, batch: HealthDataSnapshot, categories: string[]): HealthDataSnapshot {
  const selected = new Set(categories);
  const keep = <T>(values: Record<string, T>) => Object.fromEntries(Object.entries(values).filter(([key]) => !selected.has(key)));
  const choose = <T>(values: Record<string, T>) => Object.fromEntries(Object.entries(values).filter(([key]) => selected.has(key)));
  const next = { reports: { ...keep(workspace.reports), ...choose(batch.reports) },
    details: { ...keep(workspace.details), ...choose(batch.details) },
    states: { ...keep(workspace.states), ...choose(batch.states) }, query: { ...batch.query } };
  if (healthDataEntryCount(next) > MAX_HEALTH_DATA_ENTRIES) throw new Error('未選來源與本批資料合計超過 250,000 筆；請先保存目前資料，再縮小本次來源或期間。');
  return next;
}

export function batchEntryAllowance(workspace: HealthDataSnapshot, categories: string[]): number {
  const remaining = replaceBatchCategories(workspace, emptyHealthData(), categories);
  return Math.max(0, MAX_HEALTH_DATA_ENTRIES - healthDataEntryCount(remaining));
}

export function stoppedDateBatch(batch: HealthDataSnapshot): HealthDataSnapshot {
  return { ...batch, states: Object.fromEntries(Object.entries(batch.states).map(([category, state]) =>
    [category, state.status === 'loading' ? { ...state, status: 'cancelled' } : state])) };
}

export function batchLaboratoryMatches(report: ReportResponse | null, range: BatchRange): boolean {
  return report !== null && report.official_query?.mode === 'custom' &&
    report.official_query.start_date === range.from && report.official_query.end_date === range.to;
}

export function dateBatchStatus(batch: HealthDataSnapshot, categories: string[], includeLaboratory: boolean, laboratory: ReportResponse | null) {
  const listsPending = categories.filter(category => !batch.reports[category] || batch.states[category]?.status !== 'ready').length;
  const detailsPending = categories.reduce((count, category) => count + categoryCoverage(batch, category).pendingCount, 0);
  const laboratoryPending = includeLaboratory && !batchLaboratoryMatches(laboratory, batch.query);
  return { listsPending, detailsPending, laboratoryPending, acquired: listsPending === 0 && detailsPending === 0 && !laboratoryPending };
}

export function assertBatchAdvance(batch: HealthDataSnapshot, plan: DateBatchPlan, laboratory: ReportResponse | null,
  exported: boolean, savedAcknowledged: boolean): void {
  if (!dateBatchStatus(batch, plan.categories, plan.includeLaboratory, laboratory).acquired)
    throw new Error('本批仍有清單、明細或檢驗待取得；請先接續，或保存部分結果後結束計畫並縮小期間。');
  if (!exported || !savedAcknowledged) throw new Error('請先匯出本批資料包，並確認檔案已保存，才能開始下一批。');
}

export function batchSummary(batch: HealthDataSnapshot, laboratory: ReportResponse | null): DateBatchSummary {
  return { ...batch.query, entries: healthDataEntryCount(batch), laboratory: laboratory?.records.length ?? 0,
    sources: Object.keys(batch.reports).length };
}

export function missingBatchLaboratory(range: BatchRange, requested: boolean): ReportResponse {
  return { records: [], fetched_at: '', complete: false, warnings: [requested
    ? `本批 ${range.from} 至 ${range.to} 的檢驗結果尚未成功取得；零筆不是沒有檢查，檔案未包含上一批檢驗。`
    : '本批未選擇檢驗結果；此資料包不包含其他期間或先前取得的檢驗。'] };
}
