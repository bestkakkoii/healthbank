// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import type { SourceDateRole, SupplementEntry } from './api';
import { sourceEventDate } from './medicine-timeline.ts';

export const SOURCE_DATE_ROLES = ['examination', 'specimen_collection', 'report', 'encounter', 'admission', 'discharge', 'claim_start', 'claim_end', 'prescription', 'dispensing', 'recorded', 'uploaded', 'valid_from', 'valid_to', 'period_start', 'period_end', 'unknown'] as const;
export const SOURCE_FIELD_ROLES = ['result', 'unit', 'reference', 'method', 'specimen', 'date', 'medication_code', 'medication_name', 'prescribed_days', 'quantity'] as const;
export const SOURCE_ORIGINS = ['institutional', 'self_entered', 'device', 'official_summary', 'mixed', 'unknown'] as const;
export const SOURCE_DIAGNOSTIC_CODES = ['section_missing', 'section_optional_absent', 'section_null', 'section_empty', 'known_result_missing'] as const;
export const SOURCE_DATE_LABELS: Record<SourceDateRole, string> = {
  examination: '檢查日期', specimen_collection: '採檢日期', report: '報告日期', encounter: '就醫日期', admission: '入院日期', discharge: '出院日期',
  claim_start: '申報起日', claim_end: '申報迄日', prescription: '處方日期', dispensing: '來源確認的調劑日期', recorded: '紀錄日期', uploaded: '上傳日期',
  valid_from: '有效起日', valid_to: '有效迄日', period_start: '來源期間起日', period_end: '來源期間迄日', unknown: '來源日期（意義未確認）',
};

export function sourceDateLabel(role: SourceDateRole | undefined): string { return SOURCE_DATE_LABELS[role ?? 'unknown']; }

// Only explicitly typed, compatible endpoints define a span. A prescription's
// days/quantity and a claim/encounter pair never manufacture a treatment interval.
export function sourceEventInterval(entry: SupplementEntry): { start: string; end: string; isInterval: boolean } {
  const start = sourceEventDate(entry.date);
  const compatible = entry.date_role === 'admission' && entry.end_date_role === 'discharge' ||
    entry.date_role === 'claim_start' && entry.end_date_role === 'claim_end' ||
    entry.date_role === 'valid_from' && entry.end_date_role === 'valid_to' ||
    entry.date_role === 'period_start' && entry.end_date_role === 'period_end';
  const end = compatible && entry.end_date ? sourceEventDate(entry.end_date) : '';
  return start && end && end >= start ? { start, end, isInterval: true } : { start, end: start, isInterval: false };
}
