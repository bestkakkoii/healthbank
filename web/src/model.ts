// Copyright (c) 2026 bestkakkoii llc.
// SPDX-License-Identifier: MIT
import type { CatalogReference, LabRecord, Level, LevelBand } from './api';

export const COPYRIGHT_YEAR = new Intl.DateTimeFormat('en', {
  year: 'numeric', timeZone: 'Asia/Taipei',
}).format(new Date());

export function isObject(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

export const LEVELS: Record<Level, { label: string; short: string }> = {
  1: { label: '基準內', short: '基準內' },
  2: { label: '邊界／留意', short: '留意' },
  3: { label: '異常', short: '異常' },
  4: { label: '顯著異常／需評估', short: '需評估' },
  5: { label: '最高警示', short: '最高警示' },
};

export const CATEGORY_LABELS: Record<string, string> = {
  renal: '腎功能與尿蛋白',
  kidney: '腎功能與尿蛋白',
  renal_function: '腎功能與尿蛋白',
  liver: '肝功能與蛋白質',
  liver_function: '肝功能與蛋白質',
  lipid: '血脂',
  lipids: '血脂',
  glucose: '血糖',
  diabetes: '血糖',
  hematology: '血液常規',
  cbc: '血液常規',
  blood_count: '血液常規',
  electrolytes: '電解質與礦物質',
  electrolyte: '電解質與礦物質',
  urine: '尿液檢查',
  urinalysis: '尿液檢查',
  blood_gas: '血液氣體',
  cardiac: '心臟與肌肉',
  inflammation: '發炎與免疫',
  nutrition: '營養與代謝',
  coagulation: '凝血檢驗',
  other: '其他項目',
  metadata: '檢驗附註',
};

const RULE_CATEGORY_LABELS: Record<string, string> = {
  below150: '低於 150 mg/dL',
  borderline_high: '邊緣性偏高',
  high: '偏高',
  severe: '嚴重升高',
  very_high_pancreatitis_attention: '極高；需留意胰臟炎風險',
  G1: 'G1 · 正常或偏高',
  G2: 'G2 · 輕度降低',
  G3a: 'G3a · 輕度至中度降低',
  G3b: 'G3b · 中度至重度降低',
  G4: 'G4 · 重度降低',
  G5: 'G5 · 腎衰竭分類',
  A1: 'A1 · 正常至輕度增加',
  A2: 'A2 · 中度增加',
  A3: 'A3 · 重度增加',
  normal_to_mildly_increased_protein: '尿蛋白正常至輕度增加',
  moderately_increased_protein: '尿蛋白中度增加',
  severely_increased_protein: '尿蛋白重度增加',
  below_screening_threshold: '低於篩檢界值',
  impaired_fasting_glucose_range: '空腹血糖異常區間',
  prediabetes_numeric_range: '糖尿病前期數值區間',
  diabetes_diagnostic_threshold_requires_confirmation: '達糖尿病診斷界值（須確認）',
  ADA_level1_hypoglycemia: 'ADA 第 1 級低血糖',
  ADA_level2_hypoglycemia: 'ADA 第 2 級低血糖',
  vgh_abnormal_notification: '臺北榮總異常值通報範本',
  vgh_critical_threshold: '臺北榮總危急值通報範本',
};

export const ruleCategoryLabel = (value: string) => RULE_CATEGORY_LABELS[value] || value || '—';

export type RuleBand = Omit<LevelBand, 'level'> & {
  display_level: Level;
  category: string;
};

export function rulePreview(value: unknown) {
  const rule = isObject(value) ? value : {};
  const strings = (value: unknown): string[] =>
    Array.isArray(value) ? value.filter((item): item is string => typeof item === 'string') : [];
  const candidateBands: unknown[] = Array.isArray(rule['bands']) ? rule['bands'] : [];
  const bands = candidateBands.filter(
    (value): value is RuleBand => {
      if (!isObject(value)) return false;
      const band = value;
      return (
        (band['lower'] === null || typeof band['lower'] === 'string') &&
        (band['upper'] === null || typeof band['upper'] === 'string') &&
        typeof band['lower_inclusive'] === 'boolean' &&
        typeof band['upper_inclusive'] === 'boolean' &&
        typeof band['display_level'] === 'number' &&
        [1, 2, 3, 4, 5].includes(band['display_level']) &&
        typeof band['category'] === 'string'
      );
    },
  );
  return {
    kind: typeof rule['kind'] === 'string' ? rule['kind'] : '',
    unit: typeof rule['unit'] === 'string' ? rule['unit'] : '',
    description: typeof rule['description'] === 'string' ? rule['description'] : '',
    normalValues: strings(rule['normal_text_values']),
    positiveValues: strings(rule['positive_text_values']),
    reviewValues: strings(rule['review_text_values']),
    sources: strings(rule['sources']),
    bands,
  };
}

export function ruleRangeLabel(
  range: Pick<LevelBand, 'lower' | 'upper' | 'lower_inclusive' | 'upper_inclusive'>,
): string {
  if (range.lower !== null && range.upper !== null)
    return `${range.lower} ${range.lower_inclusive ? '≤' : '<'} 數值 ${range.upper_inclusive ? '≤' : '<'} ${range.upper}`;
  if (range.lower !== null) return `數值 ${range.lower_inclusive ? '≥' : '>'} ${range.lower}`;
  if (range.upper !== null) return `數值 ${range.upper_inclusive ? '≤' : '<'} ${range.upper}`;
  return '全部數值';
}

export function referenceTemplateLabel(reference: CatalogReference): string {
  if (reference.expected_values?.length) return reference.expected_values.join('／');
  if (reference.lower === null && reference.upper === null) return '未提供數值範圍';
  return ruleRangeLabel(reference);
}

export const categoryLabel = (value: string) => CATEGORY_LABELS[value] || value || '其他項目';
export const dateLabel = (date: string) =>
  date ? date.slice(0, 10).replaceAll('-', '/') : '日期未提供';
export const shortDate = (date: string) => (date ? date.slice(5, 10).replace('-', '/') : '—');
export const yearLabel = (year: string) =>
  /^\d{4}$/.test(year) ? `${year} 年 · 民國 ${Number(year) - 1911} 年` : '日期未提供';
export const formatTime = (value: string) => {
  if (!value) return '尚未取得';
  const date = new Date(value);
  return Number.isNaN(date.getTime())
    ? value
    : new Intl.DateTimeFormat('zh-TW', {
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
        timeZone: 'Asia/Taipei',
      }).format(date);
};
export const numeric = (value: string | null | undefined): number | null => {
  if (value == null || value.trim() === '') return null;
  const number = Number(value);
  return Number.isFinite(number) ? number : null;
};

export function finiteExtent(values: Iterable<number>): [number, number] | null {
  let low = Infinity, high = -Infinity;
  for (const value of values) {
    if (!Number.isFinite(value)) continue;
    low = Math.min(low, value);
    high = Math.max(high, value);
  }
  return low <= high ? [low, high] : null;
}
export function referenceLabel(
  record: Pick<LabRecord, 'reference_low' | 'reference_high' | 'reference_text'>,
): string {
  const low = record.reference_low;
  const high = record.reference_high;
  if (low != null && low !== '' && high != null && high !== '') return `${low}–${high}`;
  if (low != null && low !== '') return `≥ ${low}`;
  if (high != null && high !== '') return `≤ ${high}`;
  return record.reference_text && !record.reference_text.includes('無參考值')
    ? record.reference_text
    : '未提供';
}

export interface MeasurementGroup {
  id: string;
  configurationKey: string;
  name: string;
  abbreviation: string;
  unit: string;
  category: string;
  records: LabRecord[];
}

export function groupRecords(records: LabRecord[]): MeasurementGroup[] {
  const groups = new Map<string, MeasurementGroup>();
  for (const record of records) {
    const id = `${record.item_id || record.configuration_key || record.item_name}\u0001${record.unit}`;
    let group = groups.get(id);
    if (!group) {
      group = {
        id,
        configurationKey: record.configuration_key,
        name: record.display_name || record.item_name,
        abbreviation: record.item_name,
        unit: record.unit || '無單位',
        category: record.category || 'other',
        records: [],
      };
      groups.set(id, group);
    }
    group.records.push(record);
  }
  for (const group of groups.values())
    group.records.sort(
      (a, b) => b.exam_date.localeCompare(a.exam_date) || a.id.localeCompare(b.id),
    );
  return [...groups.values()].sort(
    (a, b) =>
      categoryLabel(a.category).localeCompare(categoryLabel(b.category), 'zh-Hant') ||
      a.name.localeCompare(b.name, 'zh-Hant'),
  );
}

export function sourceFlag(status: LabRecord['reference_status']): string {
  return status === 'high'
    ? '高於原院參考值 ↑'
    : status === 'low'
      ? '低於原院參考值 ↓'
      : status === 'within'
        ? '原院參考範圍內'
        : '無可用參考值判讀';
}

export type ExactRecord = LabRecord & { numeric_value: number };

export function isExact(record: LabRecord): record is ExactRecord {
  // The raw value is checked as well as the server kind, so a bound/range is never shown as an exact point.
  return (
    record.numeric_value !== null &&
    Number.isFinite(record.numeric_value) &&
    /^[-+]?(?:\d+(?:\.\d*)?|\.\d+)(?:e[-+]?\d+)?$/i.test(record.result.trim()) &&
    record.value_kind === 'numeric' &&
    Number(record.result.trim()) === record.numeric_value
  );
}

export function toCsv(records: LabRecord[]): string {
  const quote = (value: string | number | null | undefined) => {
    const raw = String(value ?? '');
    // A leading minus is safe only for an entire finite decimal/scientific literal.
    // Expressions such as -1+HYPERLINK(...) are spreadsheet formulas, not lab numbers.
    const trimmed = raw.trimStart();
    const negativeNumber = /^-(?:\d+(?:\.\d*)?|\.\d+)(?:e[-+]?\d+)?$/i.test(trimmed)
      && Number.isFinite(Number(trimmed));
    const safe = /^[=+@-]/.test(trimmed) && !negativeNumber ? `'${raw}` : raw;
    return `"${safe.replaceAll('"', '""')}"`;
  };
  const rows: (string | number | null | undefined)[][] = [
    [
      '檢查日期',
      '檢驗項目',
      '原始名稱',
      '結果',
      '單位',
      '原院參考值',
      '參考區間比較',
      '原院異常標記（原始值）',
      '顯示分級',
      '醫事機構',
      '就醫日期',
      '醫令代碼',
      '資料來源',
      '判讀說明',
      'Copyright',
    ],
    ...records.map((r) => [
      r.exam_date,
      r.display_name || r.item_name,
      r.item_name,
      r.result,
      r.unit,
      referenceLabel(r),
      sourceFlag(r.reference_status),
      r.official_flag ?? '',
      r.level ? `L${r.level} ${LEVELS[r.level].label}` : '未判定',
      r.institution,
      r.visit_date,
      r.order_code,
      r.source,
      r.interpretation,
      `© ${COPYRIGHT_YEAR} bestkakkoii llc.`,
    ]),
  ];
  return '\ufeff' + rows.map((row) => row.map(quote).join(',')).join('\r\n');
}

export function downloadText(text: string, filename: string, mime: string): void {
  const url = URL.createObjectURL(new Blob([text], { type: mime }));
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = filename;
  document.body.append(anchor);
  anchor.click();
  anchor.remove();
  window.setTimeout(() => URL.revokeObjectURL(url), 1000);
}
