// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import type { LabRecord, ReportResponse, SessionMode } from './api';
import { categoryCoverage, filterHealthEvents, healthEvents, type DataCategory, type HealthDataSnapshot, type HealthEvent } from './health-data.ts';
import { dateLabel, formatTime, groupRecords } from './model.ts';
import { medicalDate } from './medicine-timeline.ts';
import { laboratoryCoverageStatus } from './coverage-status.ts';

export interface PrintOptions {
  scope: 'all' | 'selection';
  layout: 'records' | 'comparison';
  laboratory: boolean;
  clinical: boolean;
  from: string;
  to: string;
  category: string;
  includeUnknownDates: boolean;
}
export const DEFAULT_PRINT_OPTIONS: PrintOptions = {
  scope: 'all', layout: 'records', laboratory: true, clinical: true,
  from: '', to: '', category: '', includeUnknownDates: true,
};
export interface PrintReportInput {
  report: ReportResponse;
  snapshot: HealthDataSnapshot;
  categories: DataCategory[];
  selectedLaboratory: LabRecord[];
  mode: SessionMode;
  version: string;
  generatedAt: string;
  options: PrintOptions;
}
export interface PrintReportData {
  laboratory: LabRecord[];
  clinical: HealthEvent[];
  options: PrintOptions;
  version: string;
  generatedAt: string;
  mode: SessionMode;
  scope: string;
  laboratoryCoverage: string;
  warnings: string[];
  coverage: { label: string; summary: string }[];
}

function checkRange(options: PrintOptions) {
  for (const date of [options.from, options.to]) {
    if (date && (!/^\d{4}-\d{2}-\d{2}$/.test(date) || medicalDate(date) !== date))
      throw new Error('請使用有效的報表起訖日期。');
  }
  if (options.from && options.to && options.from > options.to) throw new Error('報表起日不能晚於迄日。');
}

export function createPrintReport(input: PrintReportInput): PrintReportData {
  const { options, report, snapshot, categories, mode } = input;
  if (options.scope === 'selection') checkRange(options);
  const laboratory = (options.laboratory ? options.scope === 'all' ? report.records : input.selectedLaboratory : [])
    .filter(record => {
      if (options.scope === 'all') return true;
      const date = medicalDate(record.exam_date);
      return date ? (!options.from || date >= options.from) && (!options.to || date <= options.to) : options.includeUnknownDates;
    }).toSorted((a, b) => b.exam_date.localeCompare(a.exam_date));
  const events = options.clinical ? healthEvents(snapshot, categories) : [];
  const clinical = options.scope === 'all' ? events : filterHealthEvents(events, {
    text: '', institution: '', category: options.category, from: options.from, to: options.to,
    includeUnknownDates: options.includeUnknownDates,
  });
  const laboratoryCoverage = laboratoryCoverageStatus(report, mode).summary;
  const coverage = categories.map(category => {
    const state = categoryCoverage(snapshot, category.id);
    const source = snapshot.reports[category.id];
    const range = source?.start_date || source?.end_date ? `來源期間：${source.start_date || '未提供'}～${source.end_date || '未提供'}。` : '';
    return { label: category.label, summary: state.queried
      ? `清單 ${state.count} 筆；已取得明細 ${state.loadedCount}/${state.detailCount}，待取得 ${state.pendingCount}。${range}${source?.coverage ?? ''}${snapshot.states[category.id]?.status === 'failed' ? '最近更新失敗，保留先前成功內容。' : ''}`
      : '尚未查詢；不代表沒有紀錄。' };
  });
  return { laboratory, clinical, options: { ...options }, version: input.version, generatedAt: input.generatedAt, mode,
    scope: options.scope === 'all' ? '全部已取得資料（不受畫面頁碼、捲動位置與顯示篩選影響）'
      : `指定分享範圍：檢驗採目前項目篩選；醫療類別 ${categories.find(category => category.id === options.category)?.label ?? '全部'}；${options.from || '不限起日'}～${options.to || '不限迄日'}；${options.includeUnknownDates ? '保留日期不明紀錄' : '排除日期不明紀錄'}`,
    laboratoryCoverage, coverage, warnings: [...report.warnings] };
}

// Every source string is escaped. The document contains no scripts, external
// assets, session tokens, official handles or browser-supplied source markup.
function escape(value: string): string {
  return value.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;').replaceAll("'", '&#39;');
}
function paragraph(value: string, className = '') { return `<p${className ? ` class="${className}"` : ''}>${escape(value)}</p>`; }
function rawDate(record: LabRecord) {
  const raw = record.raw_check_date ?? record.exam_date;
  return `${escape(raw || '檢查日期未提供')}${raw !== record.exam_date && record.exam_date ? paragraph(`對照日期 ${record.exam_date}`, 'secondary') : ''}`;
}
function result(record: LabRecord) {
  return `<strong class="result">${escape(record.result)}</strong> ${escape(record.unit)}${paragraph(record.institution || '院所未提供', 'secondary')}${record.official_flag ? paragraph(`原院旗標：${record.official_flag}`) : ''}`;
}
function labRows(records: LabRecord[]) {
  return records.map((record, index) => `<tr data-laboratory-row="${index + 1}"><td>${rawDate(record)}${paragraph(`就醫日期（非採檢日期）：${(record.raw_visit_date ?? record.visit_date) || '未提供'}`, 'secondary')}</td><td>${escape(record.display_name || record.item_name)}${paragraph(`原文：${record.item_name}`, 'secondary')}${paragraph(`醫令：${record.order_code} ${record.order_name}`, 'secondary')}</td><td>${result(record)}</td><td>${escape(record.reference_text || '原院參考值未提供')}${record.interpretation ? paragraph(`整理說明：${record.interpretation}`, 'secondary') : ''}${record.rule_source ? paragraph(`規則來源：${record.rule_source}`, 'secondary') : ''}${paragraph(`資料來源：${record.source || '未提供'}`, 'secondary')}</td></tr>`).join('');
}
function comparisonTables(records: LabRecord[]) {
  const groups = groupRecords(records);
  const sections: string[] = [];
  for (let offset = 0; offset < groups.length; offset += 4) {
    const batch = groups.slice(offset, offset + 4);
    const dates = new Set(batch.flatMap(group => group.records.map(record => record.exam_date)));
    const cells = batch.map(group => {
      const map = new Map<string, LabRecord[]>();
      for (const record of group.records) { const cell = map.get(record.exam_date); if (cell) cell.push(record); else map.set(record.exam_date, [record]); }
      return map;
    });
    sections.push(`<section class="comparison-section"><h2>檢驗日期對照 · 項目 ${offset + 1}～${offset + batch.length}</h2><table class="comparison"><thead><tr><th scope="col">檢查日期</th>${batch.map(group => `<th scope="col">${escape(group.name)}${paragraph(group.unit, 'secondary')}</th>`).join('')}</tr></thead><tbody>${[...dates].toSorted((a, b) => b.localeCompare(a)).map(date => `<tr><th scope="row">${escape(dateLabel(date))}</th>${cells.map(map => `<td>${(map.get(date) ?? []).map(record => `<div class="observation">${result(record)}${paragraph(`原日期：${(record.raw_check_date ?? record.exam_date) || '未提供'}`, 'secondary')}${paragraph(`原院參考：${record.reference_text || '未提供'}`, 'secondary')}</div>`).join('') || '—'}</td>`).join('')}</tr>`).join('')}</tbody></table></section>`);
  }
  return sections.join('');
}
function clinicalRecord(event: HealthEvent, index: number) {
  const entry = event.entry;
  const fields = [...entry.fields];
  if (entry.medicine) fields.push({ name: '官方藥品代碼', value: entry.medicine.code }, { name: '原始藥名', value: entry.medicine.name },
    { name: '開立數量（不推定服用量）', value: entry.medicine.quantity }, { name: '給藥日數（不代表實際服藥）', value: entry.medicine.days });
  return `<section class="clinical-record" data-clinical-row="${index + 1}"><h3>${index + 1}. ${escape(entry.title || event.categoryLabel)}</h3>${paragraph(`${event.categoryLabel} · ${entry.date || '日期未提供'}${entry.end_date ? `～${entry.end_date}` : ''} · ${entry.institution || '院所未提供'}`)}${paragraph(`${event.parentReference ? '已取得的明細紀錄' : '來源清單紀錄'}；來源：${entry.source || '未提供'}`, 'secondary')}<table><thead><tr><th scope="col">來源欄位</th><th scope="col">原始內容</th></tr></thead><tbody>${fields.map(field => `<tr><th scope="row">${escape(field.name)}</th><td>${escape(field.value)}${field.unit ? ` ${escape(field.unit)}` : ''}</td></tr>`).join('')}</tbody></table>${entry.attachments?.length ? paragraph(`附件索引（本報表不嵌入原始附件）：${entry.attachments.map(attachment => attachment.name).join('、')}`, 'secondary') : ''}</section>`;
}

export const PRINT_REPORT_STYLE = `
@page { size: A4 portrait; margin: 14mm 12mm 17mm; @bottom-center { content: "健康映覽 · " counter(page) " / " counter(pages); font-size: 8pt; color: #475569; } }
* { box-sizing: border-box; } html { color-scheme: light; } body { margin: 0; color: #172b3a; background: #fff; font: 10pt/1.5 "Noto Sans CJK TC", "PingFang TC", "Microsoft JhengHei", sans-serif; overflow-wrap: anywhere; }
h1 { font-size: 22pt; margin: 0 0 4mm; letter-spacing: .04em; } h2 { font-size: 14pt; margin: 7mm 0 3mm; break-after: avoid; } h3 { font-size: 11pt; margin: 5mm 0 2mm; break-after: avoid; }
p { margin: 1.2mm 0; white-space: pre-wrap; } .secondary { font-size: 8.5pt; color: #475569; } .report-header { border-bottom: 2pt solid #17665b; padding-bottom: 4mm; margin-bottom: 5mm; } .scope { padding: 3mm; border: 1px solid #b8c9c5; background: #f3f8f6; } .notice { padding: 3mm; border-left: 3pt solid #8a6514; background: #fff9ed; }
table { width: 100%; border-collapse: collapse; table-layout: fixed; margin: 3mm 0 5mm; } thead { display: table-header-group; } th, td { border: .5pt solid #bac8cf; padding: 2mm; text-align: left; vertical-align: top; white-space: pre-wrap; overflow-wrap: anywhere; } th { background: #edf3f4; font-weight: 600; } thead th { font-size: 9pt; } .lab-table th:nth-child(1) { width: 19%; } .lab-table th:nth-child(2) { width: 26%; } .lab-table th:nth-child(3) { width: 22%; } .lab-table th:nth-child(4) { width: 33%; } .result { font-variant-numeric: tabular-nums; font-size: 11pt; } .observation + .observation { border-top: .5pt dashed #849aa3; margin-top: 2mm; padding-top: 2mm; } .comparison th:first-child { width: 18%; } .clinical-record th[scope="row"] { width: 30%; } .coverage-table th:first-child { width: 26%; } .report-footer { border-top: .5pt solid #bac8cf; padding-top: 3mm; margin-top: 8mm; font-size: 8.5pt; color: #475569; }
@media screen { body { max-width: 210mm; padding: 12mm; margin: 0 auto; } }
@media print { body { width: auto; max-width: none; padding: 0; } .comparison-section + .comparison-section { break-before: page; } a { color: inherit; text-decoration: none; } }
`;

export function renderPrintReport(data: PrintReportData): string {
  const mode = data.mode === 'demo' ? '合成示範資料，非真實醫療紀錄' : data.mode === 'import' ? '使用者匯入資料；未經本次官方連線驗證' : '已取得的來源紀錄';
  const lab = data.laboratory.length ? data.options.layout === 'comparison' ? comparisonTables(data.laboratory)
    : `<section><h2>檢驗原始紀錄 · ${data.laboratory.length} 筆</h2><table class="lab-table"><thead><tr><th scope="col">日期</th><th scope="col">檢驗項目</th><th scope="col">結果／單位／院所</th><th scope="col">原院參考與判讀依據</th></tr></thead><tbody>${labRows(data.laboratory)}</tbody></table></section>` : '';
  return `<!doctype html><html lang="zh-Hant-TW"><head><meta charset="utf-8"><meta name="referrer" content="no-referrer"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; font-src 'none'; img-src 'none'; base-uri 'none'; form-action 'none'"><meta name="viewport" content="width=device-width, initial-scale=1"><title>健康映覽醫療紀錄報表</title><style>${PRINT_REPORT_STYLE}</style></head><body><header class="report-header"><h1>健康映覽 · 醫療紀錄報表</h1>${paragraph(mode)}${paragraph(`製作時間：${formatTime(data.generatedAt)}（臺灣時間） · Healthbank ${data.version || '版本未提供'}`, 'secondary')}</header><section class="scope">${paragraph(data.scope)}${paragraph(`本報表實際包含：檢驗 ${data.laboratory.length} 筆、醫療來源紀錄 ${data.clinical.length} 筆。逐筆保留同日與相同數值的重複紀錄。`)}</section>${data.laboratory.length ? paragraph(data.laboratoryCoverage, 'notice') : ''}${data.warnings.map(warning => paragraph(warning, 'notice')).join('')}${lab}${data.clinical.length ? `<h2>醫療來源紀錄 · ${data.clinical.length} 筆</h2>${data.clinical.map(clinicalRecord).join('')}` : ''}${!data.laboratory.length && !data.clinical.length ? paragraph('本次報表範圍沒有已取得的紀錄。') : ''}<section><h2>來源範圍與未完成項目</h2>${paragraph('本報表由已取得資料重新排版，不代表官方完整終身歷史。清單與明細各自保留，筆數不能當作就醫次數。處方與給藥日數不代表實際服藥；整理說明不是診斷或治療建議。')}${data.coverage.length ? `<table class="coverage-table"><thead><tr><th scope="col">來源</th><th scope="col">取得狀態與限制</th></tr></thead><tbody>${data.coverage.map(item => `<tr><th scope="row">${escape(item.label)}</th><td>${escape(item.summary)}</td></tr>`).join('')}</tbody></table>` : ''}</section><footer class="report-footer">${paragraph('本檔含健康資料，由使用者自行保存與分享。Healthbank 軟體採 MIT 授權；來源醫療資料的權利不因此改變。')}${paragraph('Copyright © 2026 bestkakkoii llc.')}</footer></body></html>`;
}
