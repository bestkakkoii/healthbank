// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import type { LabRecord, ReportResponse, SessionMode } from './api';

export type LaboratoryCoverageKind = 'unqueried' | 'matched' | 'mismatch' | 'missing' | 'unverified' | 'imported' | 'synthetic';

export interface LaboratoryCoverageStatus {
  kind: LaboratoryCoverageKind;
  summary: string;
  returnedCount: number;
  reportedCount: number | null;
  difference: number | null;
}

/** A count match describes one submitted interval, never lifetime completeness. */
export function laboratoryCoverageStatus(
  report: Pick<ReportResponse, 'records' | 'fetched_at' | 'complete' | 'official_query'>,
  mode: SessionMode,
): LaboratoryCoverageStatus {
  const returnedCount = report.records.length;
  const suppliedCount = report.official_query?.reported_count;
  const reportedCount = typeof suppliedCount === 'number' && Number.isSafeInteger(suppliedCount) && suppliedCount >= 0
    ? suppliedCount : null;
  const difference = reportedCount === null ? null : returnedCount - reportedCount;
  const status = (kind: LaboratoryCoverageKind, summary: string): LaboratoryCoverageStatus =>
    ({ kind, summary, returnedCount, reportedCount, difference });
  if (mode === 'demo') return status('synthetic', `合成示範 ${returnedCount} 筆；不是官方個人資料，也不代表完成官方筆數核對。`);
  if (mode === 'import') {
    const historical = reportedCount === null ? `匯入資料保留 ${returnedCount} 筆` :
      `檔案記載官方回報 ${reportedCount} 筆、保留 ${returnedCount} 筆${difference === 0 ? '（筆數相同）' : '（筆數不一致）'}`;
    return status('imported', `${historical}；本次未向官方重新核對資料或身分。`);
  }
  if (!report.fetched_at && returnedCount === 0 && reportedCount === null) return status('unqueried', '尚未取得檢驗資料。');
  if (mode !== 'official') return status('unverified', `目前保留 ${returnedCount} 筆；未經本次官方身分驗證與筆數核對。`);
  if (reportedCount === null) return status('missing', `本次保留 ${returnedCount} 筆；官方未提供同期間可獨立核對的總筆數，不能確認歷史完整性。`);
  if (difference !== 0) return status('mismatch', `官方回報 ${reportedCount} 筆，實際保留 ${returnedCount} 筆；筆數不一致，所有已回傳紀錄均保留，請查看來源警告。`);
  if (!report.complete) return status('unverified', `官方回報與實際保留均為 ${returnedCount} 筆；筆數相同，但本次資料仍標示未完整，請查看來源警告。`);
  return status('matched', `官方回報與實際保留均為 ${returnedCount} 筆；本次查詢期間的筆數相符，不代表官方終身資料完整。`);
}

export function laboratoryDateSummary(records: readonly Pick<LabRecord, 'exam_date'>[]): string {
  if (records.length === 0) return '尚無檢驗資料';
  let first = '', last = '', unknown = 0;
  for (const record of records) {
    if (!/^\d{4}-\d{2}-\d{2}$/.test(record.exam_date)) { unknown++; continue; }
    if (!first || record.exam_date < first) first = record.exam_date;
    if (!last || record.exam_date > last) last = record.exam_date;
  }
  if (!first) return `${unknown} 筆日期不明`;
  const from = first.replaceAll('-', '/'), to = last.replaceAll('-', '/');
  return `${first === last ? from : `${from}～${to}`}${unknown ? `；另有 ${unknown} 筆日期不明` : ''}`;
}
