// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
// Browser-only data. Never pass these objects to the settings or report APIs.
export type IntakeCategory = 'medicine' | 'herbal' | 'supplement' | 'other';
export interface IntakeRecord { id: string; name: string; category: IntakeCategory; amount: string; start: string; end: string; note: string }
export const INTAKE_CATEGORIES: Record<IntakeCategory, string> = { medicine: '西藥', herbal: '中藥', supplement: '補充品', other: '其他' };
const keys = ['id', 'name', 'category', 'amount', 'start', 'end', 'note'];
function dateValid(value: string): boolean {
  if (!value) return true;
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return false;
  const date = new Date(`${value}T00:00:00Z`);
  return Number.isFinite(date.getTime()) && date.toISOString().slice(0, 10) === value;
}
export function parseIntakeRecords(text: string): IntakeRecord[] {
  if (text.length > 256_000) throw new Error('個人紀錄檔案過大（最多256,000字元）。');
  const value: unknown = JSON.parse(text);
  if (!Array.isArray(value) || value.length > 200) throw new Error('個人紀錄須為最多200項的清單。');
  const result: IntakeRecord[] = [], ids = new Set<string>();
  for (const entry of value as unknown[]) {
    if (typeof entry !== 'object' || entry === null || Array.isArray(entry)) throw new Error('個人紀錄格式不正確。');
    const row = entry as Record<string, unknown>;
    if (Object.keys(row).some(key => !keys.includes(key)) || keys.some(key => typeof row[key] !== 'string')) throw new Error('個人紀錄包含未支援欄位。');
    const id = row['id'], name = row['name'], category = row['category'], amount = row['amount'], start = row['start'], end = row['end'], note = row['note'];
    if (typeof id !== 'string' || typeof name !== 'string' || typeof amount !== 'string' || typeof start !== 'string' || typeof end !== 'string' || typeof note !== 'string' ||
        (category !== 'medicine' && category !== 'herbal' && category !== 'supplement' && category !== 'other')) throw new Error('個人紀錄欄位型別不正確。');
    if (!/^[a-f0-9-]{36}$/i.test(id) || ids.has(id) || !name.trim() || name.length > 128 || amount.length > 128 || note.length > 512 ||
        !dateValid(start) || !dateValid(end) || (start && end && end < start)) throw new Error('品項名稱、日期、長度或識別碼不正確。');
    ids.add(id); result.push({ id, name, category, amount, start, end, note });
  }
  return result;
}
export function intakeStorageKey(profileKey: string): string | null {
  return /^[a-f0-9]{64}$/.test(profileKey) ? `healthbank.intake.v1.${profileKey}` : null;
}
export function intakeOnDate(records: IntakeRecord[], date: string): IntakeRecord[] {
  if (!date || !dateValid(date)) return [];
  return records.filter(record => record.start !== '' && record.start <= date && (record.end === '' || record.end >= date));
}
