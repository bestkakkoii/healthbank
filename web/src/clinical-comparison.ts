// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
// Source-bound scenarios, never replacements for institutional results/flags.
import type { LabRecord } from './api';
import { isExact } from './model.ts';
export type ReferenceSex = 'unknown' | 'female' | 'male';
export type ComparisonAnalyte = 'creatinine' | 'alp';
export interface ComparisonScenario { age: number | null; sex: ReferenceSex }
export interface ReferenceInterval { low: number; high: number; unit: string; source: string; method: string; scope: string }
export interface DerivedResult { value: number | null; reason: string }
export const CLINICAL_SOURCE_DATE = '2026-10-27';
export const CLINICAL_SOURCES = {
  creatinine: 'https://web.hosp.ncku.edu.tw/pathology/Pathology/Page/72',
  alp: 'https://web.hosp.ncku.edu.tw/pathology/Pathology/Page/83',
  egfr: 'https://www.niddk.nih.gov/research-funding/research-programs/kidney-clinical-research-epidemiology/laboratory/glomerular-filtration-rate-equations/adults',
} as const;

export function validAge(age: number | null): age is number {
  // Input guard, not a claim that a source was validated through age 130.
  return age !== null && Number.isInteger(age) && age >= 1 && age <= 130;
}
export function scenarioInterval(analyte: ComparisonAnalyte, scenario: ComparisonScenario): ReferenceInterval | null {
  if (!validAge(scenario.age)) return null;
  const { age, sex } = scenario;
  let low: number, high: number, scope: string;
  if (analyte === 'creatinine') {
    if (age < 15) {
      const bands = [
        [3, 0.18, 0.35], [5, 0.26, 0.42], [7, 0.29, 0.47], [9, 0.34, 0.53],
        [11, 0.33, 0.64], [13, 0.44, 0.68], [15, 0.46, 0.77],
      ] as const;
      const band = bands.find(([upper]) => age < upper);
      if (!band) return null;
      [, low, high] = band;
      scope = '成大血液肌酸酐分齡範本（1–14歲）';
    } else {
      if (sex === 'unknown') return null;
      [low, high] = sex === 'male' ? [0.67, 1.17] : [0.51, 0.95];
      scope = '成大血液肌酸酐範本（≥15歲）';
    }
    return { low, high, unit: 'mg/dL', source: CLINICAL_SOURCES.creatinine, method: 'Enzymatic method', scope };
  }
  if (age < 10) { [low, high] = [142, 335]; scope = '成大 ALP 範本（1–9歲）'; }
  else if (age < 13) { [low, high] = [129, 417]; scope = '成大 ALP 範本（10–12歲）'; }
  else {
    if (sex === 'unknown') return null;
    if (age < 15) [low, high] = sex === 'male' ? [116, 468] : [57, 254];
    else if (age < 17) [low, high] = sex === 'male' ? [82, 331] : [50, 117];
    else if (age < 19) [low, high] = sex === 'male' ? [55, 149] : [45, 87];
    else [low, high] = sex === 'male' ? [40, 129] : [35, 104];
    scope = '成大 ALP 分齡／分性別範本';
  }
  return { low, high, unit: 'U/L', source: CLINICAL_SOURCES.alp, method: 'Enzymatic colorimetric IFCC reference method', scope };
}
export function recordAnalyte(record: LabRecord): ComparisonAnalyte | null {
  const name = record.item_name.trim().toLowerCase();
  if (record.order_code === '09015C' && ['creatinine', 'creatinine(健檢)', 'creatinine（健檢）', 'crea'].includes(name)) return 'creatinine';
  if (record.order_code === '09027C' && ['alp', 'alk-p', 'alkaline phosphatase', 'alkaline phosphatase(alp)'].includes(name)) return 'alp';
  return null;
}
export function comparisonValue(record: LabRecord, analyte: ComparisonAnalyte): number | null {
  if (recordAnalyte(record) !== analyte || !isExact(record) || record.numeric_value < 0 ||
      Number(record.result.trim()) !== record.numeric_value) return null;
  const unit = record.unit.replace(/\s/g, '').replaceAll('μ', 'µ').toLowerCase();
  if (analyte === 'alp') return unit === 'u/l' ? record.numeric_value : null;
  if (unit === 'mg/dl') return record.numeric_value;
  return unit === 'µmol/l' ? record.numeric_value / 88.4 : null;
}
function decimalRatio(text: string): readonly [bigint, bigint] | null {
  if (text.length > 128) return null;
  const match = /^\+?(\d*)(?:\.(\d*))?(?:e([-+]?\d+))?$/i.exec(text.trim());
  if (!match || `${match[1] ?? ''}${match[2] ?? ''}` === '') return null;
  const fraction = match[2] ?? '';
  const exponent = Number(match[3] ?? 0) - fraction.length;
  if (!Number.isInteger(exponent) || Math.abs(exponent) > 100) return null;
  const numerator = BigInt(`${match[1] || '0'}${fraction}`);
  return exponent >= 0 ? [numerator * 10n ** BigInt(exponent), 1n] : [numerator, 10n ** BigInt(-exponent)];
}
export function compareInterval(record: LabRecord | undefined, analyte: ComparisonAnalyte, interval: ReferenceInterval | null): string {
  if (!record || comparisonValue(record, analyte) === null || interval === null) return '無法比較';
  const raw = decimalRatio(record.result), lower = decimalRatio(String(interval.low)), upper = decimalRatio(String(interval.high));
  if (!raw || !lower || !upper) return '無法比較';
  // Compare decimal rationals before formatting. µmol/L / 88.4 is exactly * 5/442.
  const converted = analyte === 'creatinine' && record.unit.replace(/\s/g, '').replaceAll('μ', 'µ').toLowerCase() === 'µmol/l';
  const numerator = raw[0] * (converted ? 5n : 1n), denominator = raw[1] * (converted ? 442n : 1n);
  return numerator * lower[1] < lower[0] * denominator ? '低於情境範本'
    : numerator * upper[1] > upper[0] * denominator ? '高於情境範本' : '位於情境範本內';
}
export function egfrCreatinine2021(value: number | null, scenario: ComparisonScenario, confirmed: boolean): DerivedResult {
  if (!confirmed) return { value: null, reason: '須確認標準化肌酸酐、穩定腎功能及公式適用性。' };
  if (!validAge(scenario.age) || scenario.age < 18 || scenario.sex === 'unknown') return { value: null, reason: '此成人公式需要≥18歲及公式採用的性別條件。' };
  if (value === null || !Number.isFinite(value) || value <= 0) return { value: null, reason: '需要可確認單位的正值、精確血液肌酸酐。' };
  const female = scenario.sex === 'female';
  const ratio = value / (female ? 0.7 : 0.9);
  const result = 142 * Math.min(ratio, 1) ** (female ? -0.241 : -0.302) *
    Math.max(ratio, 1) ** -1.2 * 0.9938 ** scenario.age * (female ? 1.012 : 1);
  if (!Number.isFinite(result)) return { value: null, reason: '公式結果超出可計算範圍。' };
  return { value: result, reason: 'CKD-EPI 2021 情境估算，mL/min/1.73m²；不覆寫原院 eGFR。' };
}
export function exactAgeOn(birthDate: string, examDate: string): number | null {
  const parse = (text: string): Date | null => {
    if (!/^\d{4}-\d{2}-\d{2}$/.test(text)) return null;
    const date = new Date(`${text}T00:00:00Z`);
    return Number.isFinite(date.getTime()) && date.toISOString().slice(0, 10) === text ? date : null;
  };
  const birth = parse(birthDate), exam = parse(examDate);
  if (birth === null || exam === null || birth > exam) return null;
  const beforeBirthday = exam.getUTCMonth() < birth.getUTCMonth() ||
    (exam.getUTCMonth() === birth.getUTCMonth() && exam.getUTCDate() < birth.getUTCDate());
  const age = exam.getUTCFullYear() - birth.getUTCFullYear() - (beforeBirthday ? 1 : 0);
  return validAge(age) ? age : null;
}
