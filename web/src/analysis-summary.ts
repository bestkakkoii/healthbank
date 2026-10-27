// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import type { LabRecord } from './api';
import { isExact } from './model.ts';

// Decimal subtraction is exact; percentage is explicitly an approximate display.
function decimalParts(text: string): { integer: bigint; scale: number } | null {
  if (text.length > 128) return null;
  const match = /^([+-]?)(\d*)(?:\.(\d*))?(?:e([+-]?\d+))?$/i.exec(text.trim());
  if (!match || `${match[2] ?? ''}${match[3] ?? ''}` === '') return null;
  const scale = (match[3]?.length ?? 0) - Number(match[4] ?? 0);
  if (!Number.isInteger(scale) || Math.abs(scale) > 100) return null;
  let integer = BigInt(`${match[2] || '0'}${match[3] ?? ''}`) * (match[1] === '-' ? -1n : 1n);
  if (scale < 0) integer *= 10n ** BigInt(-scale);
  return { integer, scale: Math.max(0, scale) };
}
export function exactDifference(current: string, previous: string): string | null {
  const a = decimalParts(current), b = decimalParts(previous);
  if (!a || !b) return null;
  const scale = Math.max(a.scale, b.scale);
  const difference = a.integer * 10n ** BigInt(scale - a.scale) - b.integer * 10n ** BigInt(scale - b.scale);
  const sign = difference < 0n ? '-' : difference > 0n ? '+' : '';
  const digits = (difference < 0n ? -difference : difference).toString().padStart(scale + 1, '0');
  const number = scale ? `${digits.slice(0, -scale)}.${digits.slice(-scale)}`.replace(/\.?0+$/, '') : digits;
  return sign + (number || '0');
}
export function recordDifference(current: LabRecord | undefined, previous: LabRecord | undefined) {
  if (!current || !previous || !isExact(current) || !isExact(previous) || current.item_id !== previous.item_id || current.unit !== previous.unit || Number(current.result.trim()) !== current.numeric_value || Number(previous.result.trim()) !== previous.numeric_value) return null;
  const difference = exactDifference(current.result, previous.result);
  if (difference === null) return null;
  const days = (Date.parse(`${current.exam_date}T00:00:00Z`) - Date.parse(`${previous.exam_date}T00:00:00Z`)) / 86400000;
  if (!Number.isInteger(days) || days < 0) return null;
  // Zero and negative denominators have no ordinary percent-change interpretation.
  const percent = previous.numeric_value > 0 ? (current.numeric_value - previous.numeric_value) / previous.numeric_value * 100 : null;
  return { difference, days, percent: percent !== null && Number.isFinite(percent) ? percent : null,
    crossInstitution: current.institution !== previous.institution };
}
export function latestDistinctPair(records: LabRecord[]): readonly [LabRecord | undefined, LabRecord | undefined] {
  const sorted = records.filter(record => /^\d{4}-\d{2}-\d{2}$/.test(record.exam_date))
    .toSorted((a, b) => b.exam_date.localeCompare(a.exam_date));
  const firstDate = sorted[0]?.exam_date;
  const secondDate = sorted.find(record => record.exam_date !== firstDate)?.exam_date;
  const first = sorted.filter(record => record.exam_date === firstDate);
  const second = sorted.filter(record => record.exam_date === secondDate);
  return [first.length === 1 ? first[0] : undefined, second.length === 1 ? second[0] : undefined];
}
export function nonHdlCholesterol(total: LabRecord | undefined, hdl: LabRecord | undefined, sameSampleConfirmed: boolean): { value: string | null; reason: string } {
  if (!sameSampleConfirmed) return { value: null, reason: '須確認兩項來自同次採檢；僅日期相同不足以證明。' };
  if (!total || !hdl || !isExact(total) || !isExact(hdl) || total.order_code !== '09001C' || hdl.order_code !== '09043C' ||
      total.exam_date === '' || total.exam_date !== hdl.exam_date || !total.institution || total.institution !== hdl.institution || total.visit_date !== hdl.visit_date ||
      total.unit !== hdl.unit || !['mg/dL', 'mmol/L'].includes(total.unit) || total.numeric_value < hdl.numeric_value || hdl.numeric_value < 0 ||
      Number(total.result.trim()) !== total.numeric_value || Number(hdl.result.trim()) !== hdl.numeric_value) {
    return { value: null, reason: '需要同日期、院所、就醫日期與單位的精確總膽固醇及HDL；不能混合不同檢體或不等式。' };
  }
  return { value: exactDifference(total.result, hdl.result)?.replace(/^\+/, '') ?? null, reason: '系統計算 non-HDL-C＝總膽固醇−HDL-C；不覆寫原院結果。' };
}
