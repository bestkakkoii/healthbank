// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import { useEffect, useRef, useState } from 'react';
import { api, readableError, type SupplementReport, type ReportResponse } from './api';
import { useAnalysis } from './AnalysisContext';
import { dateLabel, downloadText } from './model';
const CATEGORIES = [
  ['adult', '成人預防保健（近期）'], ['self_paid', '自費健檢清單與明細'],
  ['western', '西醫就醫與藥品醫囑'], ['chinese', '中醫就醫'], ['dental', '牙醫就醫'],
  ['recent_medicine', '近期用藥就醫摘要'], ['recent_chronic', '近期慢箋摘要'],
  ['recent_procedure', '近期手術摘要'], ['recent_hospital', '近期住院摘要'], ['imaging', '影像／病理報告索引'],
] as const;
function normalizeDate(raw: string): string {
  const match = /^(\d{2,4})[/-](\d{1,2})[/-](\d{1,2})$/.exec(raw.trim());
  if (!match) return '';
  let year = Number(match[1]); if (year < 1912) year += 1911;
  const date = `${year}-${(match[2] ?? '').padStart(2, '0')}-${(match[3] ?? '').padStart(2, '0')}`;
  return Number.isFinite(Date.parse(`${date}T00:00:00Z`)) && new Date(`${date}T00:00:00Z`).toISOString().startsWith(date) ? date : '';
}
export function SupplementaryData({ authenticated, from, to }: { authenticated: boolean; from: string; to: string }) {
  const [page, setPage] = useState(0);
  const [category, setCategory] = useState('adult'), [reports, setReports] = useState<Record<string, SupplementReport>>({});
  const [detail, setDetail] = useState<SupplementReport | null>(null), [busy, setBusy] = useState(false), [error, setError] = useState('');
  const controller = useRef<AbortController | null>(null);
  const analysis = useAnalysis();
  useEffect(() => () => controller.current?.abort(), []);
  const report = reports[category];
  async function fetchReport(reference = '') {
    controller.current?.abort(); const current = new AbortController(); controller.current = current;
    setBusy(true); setError('');
    try {
      const result = await api.supplementary(category, from, to, reference, current.signal);
      if (current.signal.aborted) return;
      setPage(0);
      if (reference) setDetail(result); else { setReports(previous => ({ ...previous, [category]: result })); setDetail(null); }
    } catch (failure) { if (!current.signal.aborted) setError(readableError(failure)); }
    finally { if (!current.signal.aborted) setBusy(false); }
  }
  const shown = detail ?? report;
  return <details className="panel analysis-tools supplementary"><summary>補充官方資料（按需取得）</summary>
    <p className="helper-text">與檢驗資料分開保留來源與範圍；不合併不確定的單位，不將就醫分類當作確診。資料僅留於目前頁面，不寫入設定資料庫。</p>
    <div className="scenario-controls"><label className="field">資料類別<select value={category} disabled={busy} onChange={event => { setCategory(event.target.value); setPage(0); setDetail(null); setError(''); }}>{CATEGORIES.map(([key, name]) => <option value={key} key={key}>{name}</option>)}</select></label>
      <button className="button secondary" type="button" disabled={!authenticated || busy} onClick={() => { void fetchReport(); }}>{busy ? '取得中…' : '取得所選類別'}</button>
      {busy && <button className="button secondary" type="button" onClick={() => { controller.current?.abort(); setBusy(false); }}>取消</button>}
      {shown && <button className="button secondary" type="button" onClick={() => downloadText(JSON.stringify(shown, null, 2), `healthbank-${shown.category}.json`, 'application/json')}>匯出此結果</button>}
      {detail && <button className="button secondary" type="button" onClick={() => { setPage(0); setDetail(null); }}>返回清單</button>}
    </div>
    {!authenticated && <p>請先完成官方登入；匯入／示範模式不存取官方資料。</p>}
    {error && <p className="error-text" role="alert">{error} 原先已取得的資料仍保留。</p>}
    {shown && <><p role="status">{shown.entries.length} 筆 · {shown.coverage}</p>{shown.start_date && <p>送出範圍：{dateLabel(shown.start_date)} — {dateLabel(shown.end_date ?? '')}</p>}
      <div className="scenario-controls"><button className="button secondary" disabled={page === 0} onClick={() => setPage(page - 1)}>上一頁</button><span>第 {page + 1}／{Math.max(1, Math.ceil(shown.entries.length / 50))} 頁 · 全部結果已保留，每頁顯示50筆</span><button className="button secondary" disabled={(page + 1) * 50 >= shown.entries.length} onClick={() => setPage(page + 1)}>下一頁</button></div>
      <div className="supplementary-list">{shown.entries.slice(page * 50, (page + 1) * 50).map((entry, index) => <details key={`${shown.fetched_at}:${page}:${index}`} className="intake-item">
        <summary>{entry.date || '未提供日期'} · {entry.title} · {entry.institution || '未提供院所'}</summary>
        <p className="helper-text">{entry.source}</p>
        <dl>{entry.fields.map((field, ordinal) => <div key={`${field.name}:${ordinal}`}><dt>{field.name}</dt><dd>{field.value}{field.unit ? ` ${field.unit}` : ''}</dd></div>)}</dl>
        {entry.detail_id && <button className="button secondary" type="button" disabled={busy} onClick={() => { void fetchReport(entry.detail_id); }}>{category === 'western' ? '讀取此就醫的藥品／醫囑明細' : '讀取此份自費健檢明細'}</button>}
        {normalizeDate(entry.date) && <button className="button secondary" type="button" onClick={() => analysis?.setSelectedDate(normalizeDate(entry.date))}>在趨勢標示此日期</button>}
      </details>)}</div></>}
  </details>;
}
export function CoverageSummary({ report }: { report: ReportResponse }) {
  const counts = new Map<string, number>();
  let earliest = '', latest = '', unknown = 0;
  for (const record of report.records) {
    if (!/^\d{4}-\d{2}-\d{2}$/.test(record.exam_date)) { unknown++; continue; }
    const year = record.exam_date.slice(0, 4); counts.set(year, (counts.get(year) ?? 0) + 1);
    if (!earliest || record.exam_date < earliest) earliest = record.exam_date;
    if (!latest || record.exam_date > latest) latest = record.exam_date;
  }
  if (!report.records.length) return null;
  return <details className="panel analysis-tools"><summary>資料範圍</summary>
    <p>已取得：{dateLabel(earliest)} — {dateLabel(latest)} · {report.records.length} 筆；{report.complete ? '本次期間有官方總數核對' : '未取得同期間的獨立官方總數，不能宣稱完整歷史'}。</p>
    <p>{[...counts.entries()].toSorted(([a], [b]) => b.localeCompare(a)).map(([year, count]) => `${year}年：${count}筆`).join('；')}{unknown ? `；日期不明：${unknown}筆` : ''}</p>
    <small>未列出的年度表示目前沒有取得該年度結果，不表示沒有做過檢查。同日多筆均保留，不以筆數推定檢查次數。</small>
  </details>;
}
