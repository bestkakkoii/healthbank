// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
// Source-level prescription events, never an assertion of actual medication intake.
import type { SupplementReport } from './api';

export type MedicineEvent = {
  id: string;
  date: string;
  sourceDate: string;
  code: string;
  name: string;
  quantity: string;
  days: string;
  validDays: number | null;
  institution: string;
  source: string;
};
export type MedicineSeries = { key: string; name: string; code: string; events: MedicineEvent[] };
export type PrescribedDayBucket = { date: string; events: MedicineEvent[]; low: MedicineEvent | null; high: MedicineEvent | null };

export function medicalDate(value: string): string {
  const match = /^(\d{1,4})[-/](\d{1,2})[-/](\d{1,2})$/.exec(value.trim());
  if (!match) return '';
  const rawYear = Number(match[1]);
  if (rawYear === 0) return '';
  // One to three digits are ROC years. Four-digit Gregorian dates must never
  // silently become ROC dates merely because they precede 1912.
  const year = (match[1] ?? '').length < 4 ? rawYear + 1911 : rawYear;
  const result = `${String(year).padStart(4, '0')}-${(match[2] ?? '').padStart(2, '0')}-${(match[3] ?? '').padStart(2, '0')}`;
  const time = Date.parse(`${result}T00:00:00Z`);
  return Number.isFinite(time) && new Date(time).toISOString().slice(0, 10) === result ? result : '';
}
export function prescribedDays(raw: string): number | null {
  const text = raw.trim();
  if (!/^\d{1,3}$/.test(text)) return null;
  const days = Number(text);
  return Number.isSafeInteger(days) && days >= 1 && days <= 365 ? days : null;
}

// Group by the exact official drug code when available; a name-only event is
// grouped only when source identity is also the same. ACT category is NOT a drug.
export function medicineSeries(reports: SupplementReport[]): MedicineSeries[] {
  const series = new Map<string, MedicineSeries>();
  for (const [reportIndex, report] of reports.entries()) {
    for (const [entryIndex, entry] of report.entries.entries()) {
      const med = entry.medicine;
      if (!med || !med.name.trim() && !med.code.trim()) continue;
      const code = med.code.trim().toUpperCase();
      const name = med.name.trim() || code;
      const key = code ? `code:${code}` : JSON.stringify(['name', name, entry.source]);
      const group = series.get(key) ?? { key, code, name, events: [] };
      const id = `${reportIndex}:${entryIndex}:${entry.key ?? ''}`;
      group.events.push({
        id,
        date: medicalDate(entry.date), sourceDate: entry.date,
        code, name, quantity: med.quantity, days: med.days, validDays: prescribedDays(med.days),
        institution: entry.institution, source: entry.source,
      });
      series.set(key, group);
    }
  }
  return [...series.values()].map(group => ({ ...group, events: group.events.toSorted((a, b) =>
    a.date.localeCompare(b.date) || a.id.localeCompare(b.id)) })).toSorted((a, b) =>
      (b.events.at(-1)?.date ?? '').localeCompare(a.events.at(-1)?.date ?? '') || a.name.localeCompare(b.name));
}

export function prescribedDayBuckets(series: MedicineSeries): PrescribedDayBucket[] {
  const buckets = new Map<string, PrescribedDayBucket>();
  for (const event of series.events) {
    if (!event.date) continue;
    const bucket: PrescribedDayBucket = buckets.get(event.date) ?? { date: event.date, events: [], low: null, high: null };
    bucket.events.push(event);
    if (event.validDays !== null) {
      if (bucket.low === null || event.validDays < (bucket.low.validDays ?? 0)) bucket.low = event;
      if (bucket.high === null || event.validDays > (bucket.high.validDays ?? 0)) bucket.high = event;
    }
    buckets.set(event.date, bucket);
  }
  return [...buckets.values()].toSorted((a, b) => a.date.localeCompare(b.date));
}

// Adjacent dates with complete day counts may be compared. Duplicate dates use
// their observed boundaries, never the arbitrary source-row order or an average.
// A date with an unknown day count breaks links instead of silently disappearing.
export function prescribedDaySegments(series: MedicineSeries): { from: MedicineEvent; to: MedicineEvent }[] {
  const dated = prescribedDayBuckets(series);
  const links: { from: MedicineEvent; to: MedicineEvent }[] = [];
  for (let i = 1; i < dated.length; i++) {
    const previous = dated[i - 1], next = dated[i];
    if (!previous?.low || !previous.high || !next?.low || !next.high ||
      previous.events.some(event => event.validDays === null) || next.events.some(event => event.validDays === null)) continue;
    links.push({ from: previous.low, to: next.low });
    if (previous.low.validDays !== previous.high.validDays || next.low.validDays !== next.high.validDays)
      links.push({ from: previous.high, to: next.high });
  }
  return links;
}
