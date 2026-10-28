// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import type { LabRecord } from './api';
export const PORTABLE_REPORT_SCHEMA = 'healthbank.lab.v2';
export const LEGACY_PORTABLE_REPORT_SCHEMA = 'healthbank.lab.v1';
export const MAX_PORTABLE_REPORT_BYTES = 32 * 1024 * 1024;
export function assertPortableReportSize(source: string): void {
  if (new TextEncoder().encode(source).length > MAX_PORTABLE_REPORT_BYTES) throw new Error('檢驗內容超過可攜匯入上限 32 MiB；請分次整理檢驗資料。沒有輸出截斷的檔案，目前資料未變更。');
}
// Original source fields only: grades, calculated values and identity data are
// deliberately excluded. Import recalculates interpretation under current rules.
export function portableRows(records: LabRecord[]) {
  return records.map(record => ({ check_date: record.raw_check_date ?? record.exam_date, order_code: record.order_code,
    order_name: record.order_name, item_name: record.item_name, result: record.result,
    unit: record.unit, reference: record.reference_text, institution: record.institution,
    visit_date: record.raw_visit_date ?? record.visit_date, source: record.source, official_flag: record.official_flag ?? '',
    ...(record.raw_upload_date === undefined && record.upload_date === undefined ? {} : { upload_date: record.raw_upload_date ?? record.upload_date ?? '' }),
    ...(record.source_name === undefined ? {} : { source_name: record.source_name }) }));
}
export function portableReportJSON(records: LabRecord[], textEditorMode = false): string {
  // BOM is opt-in for downloaded files only, never an API JSON response.
  const output = (textEditorMode ? '\uFEFF' : '') + JSON.stringify({ schema: PORTABLE_REPORT_SCHEMA, records: portableRows(records) }, null, 2) + '\n';
  assertPortableReportSize(output);
  return output;
}
function xmlEscape(value: string): string {
  for (const character of value) {
    const point = character.codePointAt(0);
    if (point === undefined || !((point >= 0x20 && point <= 0xD7FF) || (point >= 0xE000 && point <= 0xFFFD) || (point >= 0x10000 && point <= 0x10FFFF) || point === 9 || point === 10 || point === 13)) throw new Error('原始資料含 XML 不支援字元；請改用 JSON 匯出，原文未被刪改。');
  }
  return value.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;').replaceAll('"', '&quot;').replaceAll("'", '&apos;');
}
export function portableReportXML(records: LabRecord[]): string {
  const output = `<?xml version="1.0" encoding="UTF-8"?>\n<healthbank_report schema="${PORTABLE_REPORT_SCHEMA}">\n${portableRows(records).map(row => `  <record>${Object.entries(row).map(([key, value]) => `<${key}>${xmlEscape(value)}</${key}>`).join('')}</record>`).join('\n')}\n</healthbank_report>\n`;
  assertPortableReportSize(output);
  return output;
}
