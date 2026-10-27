// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
// Source-bound scenarios, never replacements for institutional results/flags.
import type { LabRecord } from './api';
import { isExact } from './model.ts';
export type ReferenceSex = 'unknown' | 'female' | 'male';
export type ComparisonAnalyte = 'creatinine' | 'alp' | 'bun' | 'calcium' | 'uric_acid' | 'alt' | 'ck' | 'phosphorus' | 'albumin';
export interface ComparisonScenario { age: number | null; sex: ReferenceSex }
export interface AnalysisPreferences {
  mode: 'off' | 'primary' | 'comparison';
  primary: ComparisonScenario;
  comparison: ComparisonScenario;
}
export interface ReferenceInterval { low: number | null; high: number | null; unit: string; source: string; method: string; scope: string }
export interface DerivedResult { value: number | null; reason: string }
export const CLINICAL_SOURCE_DATE = '2026-10-27';
export const CLINICAL_SOURCES = {
  creatinine: 'https://web.hosp.ncku.edu.tw/pathology/Pathology/Page/72',
  alp: 'https://web.hosp.ncku.edu.tw/pathology/Pathology/Page/83',
  bun: 'https://web.hosp.ncku.edu.tw/pathology/Pathology/Page/71',
  calcium: 'https://web.hosp.ncku.edu.tw/pathology/Pathology/Page/87',
  uric_acid: 'https://web.hosp.ncku.edu.tw/pathology/Pathology/Page/74',
  alt: 'https://web.hosp.ncku.edu.tw/pathology/Pathology/Page/80',
  ck: 'https://web.hosp.ncku.edu.tw/pathology/Pathology/Page/85',
  phosphorus: 'https://web.hosp.ncku.edu.tw/pathology/Pathology/Page/88',
  albumin: 'https://web.hosp.ncku.edu.tw/pathology/Pathology/Page/104',
  egfr: 'https://www.niddk.nih.gov/research-funding/research-programs/kidney-clinical-research-epidemiology/laboratory/glomerular-filtration-rate-equations/adults',
} as const;
interface AnalyteDefinition { name: string; code: string; aliases: readonly string[]; unit: string; method: string }
export const COMPARISON_ANALYTES: Record<ComparisonAnalyte, AnalyteDefinition> = {
  creatinine: { name: '血液肌酸酐', code: '09015C', aliases: ['creatinine', 'creatinine(健檢)', 'creatinine（健檢）', 'crea'], unit: 'mg/dL', method: 'Kinetic Jaffe method（以來源頁面與實際檢驗方法為準）' },
  alp: { name: '鹼性磷酸酶 ALP', code: '09027C', aliases: ['alp', 'alk-p', 'alkaline phosphatase', 'alkaline phosphatase(alp)'], unit: 'U/L', method: 'Enzymatic colorimetric IFCC reference method' },
  bun: { name: '血中尿素氮 BUN', code: '09002C', aliases: ['bun', 'blood urea nitrogen', 'urea nitrogen'], unit: 'mg/dL', method: 'Urease/GDH kinetic method' },
  calcium: { name: '血液總鈣', code: '09011C', aliases: ['calcium', 'calcium(ca)', 'calcium (ca)', 'ca'], unit: 'mg/dL', method: 'NM-BAPTA colorimetric method' },
  uric_acid: { name: '血液尿酸', code: '09013C', aliases: ['uric acid', 'uric', 'ua'], unit: 'mg/dL', method: 'Uricase/POD enzymatic colorimetric method' },
  alt: { name: 'ALT', code: '09026C', aliases: ['alt', 'alt(gpt)', 'gpt'], unit: 'U/L', method: 'Enzymatic IFCC reference method' },
  ck: { name: '肌酸激酶 CK', code: '09032C', aliases: ['ck', 'cpk', 'creatine kinase', 'creatine kinase（ck）'], unit: 'U/L', method: 'CK/HK/G6PDH enzymatic method' },
  phosphorus: { name: '血液無機磷', code: '09012C', aliases: ['phosphorus', 'phosphorus (p)', 'phosphorus(p)', 'p'], unit: 'mg/dL', method: 'Molybdate UV method' },
  albumin: { name: '血液白蛋白', code: '09038C', aliases: ['albumin', 'alb'], unit: 'g/dL', method: 'BCG colorimetric method' },
};
export const ANALYTE_KEYS = Object.keys(COMPARISON_ANALYTES) as ComparisonAnalyte[];
export function defaultAnalysis(): AnalysisPreferences {
  return { mode: 'off', primary: { age: null, sex: 'unknown' }, comparison: { age: null, sex: 'unknown' } };
}
export function validAge(age: number | null): age is number {
  // Input bound, not a claim that every formula was validated to age 130.
  return age !== null && Number.isInteger(age) && age >= 1 && age <= 130;
}
export function scenarioInterval(analyte: ComparisonAnalyte, scenario: ComparisonScenario): ReferenceInterval | null {
  if (!validAge(scenario.age)) return null;
  const { age, sex } = scenario;
  let low: number | null = null;
  let high: number;
  let scope = '成大具名範本；來源未另列細分年齡，本功能限成人情境';
  if (analyte === 'creatinine') {
    if (age < 15) {
      const bands = [[3, 0.18, 0.35], [5, 0.26, 0.42], [7, 0.29, 0.47], [9, 0.34, 0.53], [11, 0.33, 0.64], [13, 0.44, 0.68], [15, 0.46, 0.77]] as const;
      const band = bands.find(([upper]) => age < upper);
      if (!band) return null;
      [, low, high] = band;
      scope = '成大血液肌酸酐分齡範本（1–14歲）';
    } else {
      if (sex === 'unknown') return null;
      [low, high] = sex === 'male' ? [0.67, 1.17] : [0.51, 0.95];
      scope = '成大血液肌酸酐範本（≥15歲）';
    }
  } else if (analyte === 'alp') {
    if (age < 10) [low, high] = [142, 335];
    else if (age < 13) [low, high] = [129, 417];
    else {
      if (sex === 'unknown') return null;
      if (age < 15) [low, high] = sex === 'male' ? [116, 468] : [57, 254];
      else if (age < 17) [low, high] = sex === 'male' ? [82, 331] : [50, 117];
      else if (age < 19) [low, high] = sex === 'male' ? [55, 149] : [45, 87];
      else [low, high] = sex === 'male' ? [40, 129] : [35, 104];
    }
    scope = '成大 ALP 分齡／分性別範本';
  } else if (analyte === 'bun') {
    if (age < 18) [low, high] = [5, 18];
    else if (age < 60) [low, high] = [6, 20];
    else if (age < 90) [low, high] = [8, 23];
    else return null; // The source supplies no >=90-year partition.
    scope = '成大 BUN 分齡範本（1–89歲）';
  } else if (analyte === 'calcium') {
    if (age < 2) return null; // Day-based infant interval is not rounded to years.
    if (age < 12) [low, high] = [8.8, 10.8];
    else if (age < 18) [low, high] = [8.4, 10.2];
    else if (age < 60) [low, high] = [8.6, 10.0];
    else if (age < 90) [low, high] = [8.8, 10.2];
    else [low, high] = [8.2, 9.6];
    scope = '成大總鈣分齡範本（含60–89歲與≥90歲區間）';
  } else {
    if (age < 18) return null;
    if (analyte === 'phosphorus') [low, high] = [2.5, 4.5];
    else if (analyte === 'albumin') [low, high] = [3.5, 5.2];
    else {
      if (sex === 'unknown') return null;
      if (analyte === 'uric_acid') [low, high] = sex === 'male' ? [3.4, 7.0] : [2.4, 5.7];
      else if (analyte === 'alt') high = sex === 'male' ? 50 : 35;
      else [low, high] = sex === 'male' ? [39, 308] : [26, 192];
    }
  }
  const definition = COMPARISON_ANALYTES[analyte];
  return { low, high, unit: definition.unit, source: CLINICAL_SOURCES[analyte], method: definition.method, scope };
}
export function recordAnalyte(record: LabRecord): ComparisonAnalyte | null {
  const name = record.item_name.trim().toLowerCase();
  return ANALYTE_KEYS.find(key => record.order_code === COMPARISON_ANALYTES[key].code && COMPARISON_ANALYTES[key].aliases.includes(name)) ?? null;
}
function normalizedUnit(value: string): string { return value.replace(/\s/g, '').replaceAll('μ', 'µ').toLowerCase(); }
export function comparisonValue(record: LabRecord, analyte: ComparisonAnalyte): number | null {
  if (recordAnalyte(record) !== analyte || !isExact(record) || record.numeric_value < 0 ||
      Number(record.result.trim()) !== record.numeric_value) return null;
  const unit = normalizedUnit(record.unit);
  if (unit === normalizedUnit(COMPARISON_ANALYTES[analyte].unit)) return record.numeric_value;
  return analyte === 'creatinine' && unit === 'µmol/l' ? record.numeric_value / 88.4 : null;
}
export function intervalLabel(interval: ReferenceInterval | null): string {
  if (!interval) return '尚無適用範本';
  return `${interval.low === null ? `≤ ${interval.high ?? ''}` : interval.high === null ? `≥ ${interval.low}` : `${interval.low}–${interval.high}`} ${interval.unit}`;
}
export function activeScenario(preferences: AnalysisPreferences, date: string, birthDate = ''): ComparisonScenario | null {
  if (preferences.mode === 'off') return null;
  if (preferences.mode === 'comparison') return preferences.comparison;
  return birthDate ? { ...preferences.primary, age: exactAgeOn(birthDate, date) } : preferences.primary;
}
export function referenceForRecord(record: LabRecord, preferences: AnalysisPreferences, birthDate = ''): ReferenceInterval | null {
  const analyte = recordAnalyte(record), scenario = activeScenario(preferences, record.exam_date, birthDate);
  if (!analyte || !scenario || comparisonValue(record, analyte) === null) return null;
  const interval = scenarioInterval(analyte, scenario);
  if (!interval) return null;
  if (normalizedUnit(record.unit) === normalizedUnit(interval.unit)) return interval;
  if (analyte !== 'creatinine' || normalizedUnit(record.unit) !== 'µmol/l') return null;
  return { ...interval, unit: record.unit, low: interval.low === null ? null : interval.low * 88.4, high: interval.high === null ? null : interval.high * 88.4 };
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
export function intervalStatus(record: LabRecord | undefined, analyte: ComparisonAnalyte, interval: ReferenceInterval | null): 'low' | 'high' | 'within' | 'unknown' {
  if (!record || comparisonValue(record, analyte) === null || interval === null) return 'unknown';
  const raw = decimalRatio(record.result);
  const lower = interval.low === null ? null : decimalRatio(String(interval.low));
  const upper = interval.high === null ? null : decimalRatio(String(interval.high));
  if (!raw || (interval.low !== null && !lower) || (interval.high !== null && !upper)) return 'unknown';
  const converted = analyte === 'creatinine' && normalizedUnit(record.unit) === 'µmol/l';
  const numerator = raw[0] * (converted ? 5n : 1n), denominator = raw[1] * (converted ? 442n : 1n);
  if (lower && numerator * lower[1] < lower[0] * denominator) return 'low';
  if (upper && numerator * upper[1] > upper[0] * denominator) return 'high';
  return 'within';
}
export function compareInterval(record: LabRecord | undefined, analyte: ComparisonAnalyte, interval: ReferenceInterval | null): string {
  const labels = { low: '低於情境範本', high: '高於情境範本', within: '位於情境範本內', unknown: '無法比較' };
  return labels[intervalStatus(record, analyte, interval)];
}
export function recordScenarioStatus(record: LabRecord, preferences: AnalysisPreferences, birthDate = ''): 'low' | 'high' | 'within' | 'unknown' {
  const analyte = recordAnalyte(record), scenario = activeScenario(preferences, record.exam_date, birthDate);
  return analyte && scenario ? intervalStatus(record, analyte, scenarioInterval(analyte, scenario)) : 'unknown';
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
