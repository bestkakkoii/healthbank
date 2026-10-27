// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import type { LabRecord } from './api';
export const PORTABLE_REPORT_SCHEMA = 'healthbank.lab.v1';
// Original source fields only: grades, calculated values and identity data are
// deliberately excluded. Import recalculates interpretation under current rules.
export function portableRows(records: LabRecord[]) {
  return records.map(record => ({ check_date: record.exam_date, order_code: record.order_code,
    order_name: record.order_name, item_name: record.item_name, result: record.result,
    unit: record.unit, reference: record.reference_text, institution: record.institution,
    visit_date: record.visit_date, source: record.source, official_flag: record.official_flag ?? '' }));
}
export function portableReportJSON(records: LabRecord[]): string {
  return JSON.stringify({ schema: PORTABLE_REPORT_SCHEMA, records: portableRows(records) }, null, 2);
}
function xmlEscape(value: string): string {
  for (const character of value) {
    const point = character.codePointAt(0);
    if (point === undefined || !((point >= 0x20 && point <= 0xD7FF) || (point >= 0xE000 && point <= 0xFFFD) || (point >= 0x10000 && point <= 0x10FFFF) || point === 9 || point === 10 || point === 13)) throw new Error('原始資料含 XML 不支援字元；請改用 JSON 匯出，原文未被刪改。');
  }
  return value.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;').replaceAll('"', '&quot;').replaceAll("'", '&apos;');
}
export function portableReportXML(records: LabRecord[]): string {
  return `<?xml version="1.0" encoding="UTF-8"?>\n<healthbank_report schema="${PORTABLE_REPORT_SCHEMA}">\n${portableRows(records).map(row => `  <record>${Object.entries(row).map(([key, value]) => `<${key}>${xmlEscape(value)}</${key}>`).join('')}</record>`).join('\n')}\n</healthbank_report>\n`;
}
