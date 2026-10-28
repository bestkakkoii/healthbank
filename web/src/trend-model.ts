// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import type { LabRecord } from './api';
import { isExact, type ExactRecord } from './model.ts';

export type DateScale = 'elapsed' | 'ordinal';
export function dateTime(date: string): number {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(date)) return Number.NaN;
  const time = Date.parse(`${date}T00:00:00Z`);
  return Number.isFinite(time) && new Date(time).toISOString().slice(0, 10) === date ? time : Number.NaN;
}
export interface TrendBucket {
  date: string;
  records: LabRecord[];
  exact: ExactRecord[];
  low: number | null;
  high: number | null;
}
export interface TrendLink {
  from: string;
  to: string;
  fromValue: number;
  toValue: number;
  ambiguous: boolean;
}
// Bucket by date, NOT by record count. Identical values may share coordinates;
// all source rows remain selectable. No mean/median or intra-day order is invented.
export function trendGeometry(records: LabRecord[]): { buckets: TrendBucket[]; links: TrendLink[] } {
  const dates = new Map<string, LabRecord[]>();
  for (const record of records) {
    if (!Number.isFinite(dateTime(record.exam_date))) continue;
    const rows = dates.get(record.exam_date) ?? [];
    rows.push(record); dates.set(record.exam_date, rows);
  }
  const buckets = [...dates.entries()].toSorted(([a], [b]) => a.localeCompare(b)).map(([date, rows]) => {
    const exact = rows.filter(isExact);
    let low: number | null = null, high: number | null = null;
    for (const row of exact) {
      low = low === null ? row.numeric_value : Math.min(low, row.numeric_value);
      high = high === null ? row.numeric_value : Math.max(high, row.numeric_value);
    }
    return { date, records: rows, exact, low, high };
  });
  const links: TrendLink[] = [];
  for (let index = 1; index < buckets.length; index++) {
    const a = buckets[index - 1], b = buckets[index];
    if (!a || !b || a.low === null || a.high === null || b.low === null || b.high === null) continue;
    const ambiguous = a.low !== a.high || b.low !== b.high || a.exact.length !== a.records.length || b.exact.length !== b.records.length;
    links.push({ from: a.date, to: b.date, fromValue: a.low, toValue: b.low, ambiguous });
    if (a.low !== a.high || b.low !== b.high)
      links.push({ from: a.date, to: b.date, fromValue: a.high, toValue: b.high, ambiguous: true });
  }
  return { buckets, links };
}
export function createDatePosition(dates: string[], domain: readonly [number, number], mode: DateScale): (date: string) => number {
  const indices = new Map(dates.map((date, index) => [date, index]));
  return (date: string) => {
    if (mode === 'ordinal') {
      const index = indices.get(date);
      return index === undefined ? Number.NaN : dates.length < 2 ? 0.5 : index / (dates.length - 1);
    }
    const [start, end] = domain;
    return start === end ? 0.5 : (dateTime(date) - start) / (end - start);
  };
}
