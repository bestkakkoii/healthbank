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

export function medicalDate(value: string): string {
  const match = /^(\d{2,4})[-/](\d{1,2})[-/](\d{1,2})$/.exec(value.trim());
  if (!match) return '';
  const rawYear = Number(match[1]);
  const year = rawYear < 1912 ? rawYear + 1911 : rawYear;
  const result = `${year}-${(match[2] ?? '').padStart(2, '0')}-${(match[3] ?? '').padStart(2, '0')}`;
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
      const key = code ? `code:${code}` : `name:${name.toLocaleLowerCase('zh-TW')}:source:${entry.source}`;
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

// Do not connect two occurrences from the same date, or any event whose
// prescribed-days field cannot be parsed. Even a valid days count is not adherence.
export function prescribedDaySegments(series: MedicineSeries): { from: MedicineEvent; to: MedicineEvent }[] {
  const dated = series.events.filter(event => event.date !== '' && event.validDays !== null);
  const links: { from: MedicineEvent; to: MedicineEvent }[] = [];
  for (let i = 1; i < dated.length; i++) {
    const previous = dated[i - 1], next = dated[i];
    if (previous && next && previous.date !== next.date) links.push({ from: previous, to: next });
  }
  return links;
}
