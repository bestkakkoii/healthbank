// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import { useEffect, useRef, useState } from 'react';
import { api, readableError, type SupplementReport, type ReportResponse } from './api';
import { useAnalysis } from './AnalysisContext';
import { MedicineTimeline } from './MedicineTimeline';
import { medicalDate } from './medicine-timeline';
import { dateLabel, downloadText } from './model';

const CATEGORIES = [
  ['medicine', '用藥紀錄／處方明細'], ['chronic', '慢性病連續處方箋／領藥資料'],
  ['western', '西醫就醫與醫囑'], ['chinese', '中醫就醫與醫囑'], ['dental', '牙醫就醫與醫囑'],
  ['procedure', '手術與處置紀錄'], ['hospital', '住院與住院醫囑'],
  ['adult', '成人預防保健（近期）'], ['self_paid', '自費健檢清單與明細'],
  ['imaging', '影像／病理報告索引'],
  ['recent_medicine', '首頁近期用藥摘要（非完整紀錄）'],
  ['recent_chronic', '首頁近期慢箋摘要（非完整紀錄）'],
  ['recent_procedure', '首頁近期手術摘要（非完整紀錄）'],
  ['recent_hospital', '首頁近期住院摘要（非完整紀錄）'],
] as const;
const MEDICAL = new Set(['medicine', 'chronic', 'western', 'chinese', 'dental', 'procedure', 'hospital']);
const BATCH_SIZE = 24;

export function SupplementaryData({ authenticated, from, to }: {
  authenticated: boolean; from: string; to: string;
}) {
  const [page, setPage] = useState(0);
  const [category, setCategory] = useState('medicine');
  const [reports, setReports] = useState<Record<string, SupplementReport>>({});
  const [details, setDetails] = useState<Record<string, Record<string, SupplementReport>>>({});
  const [detail, setDetail] = useState<SupplementReport | null>(null);
  const [busy, setBusy] = useState(false);
  const [progress, setProgress] = useState('');
  const [error, setError] = useState('');
  const [textEditorMode, setTextEditorMode] = useState(false);
  const controller = useRef<AbortController | null>(null);
  const analysis = useAnalysis();
  useEffect(() => () => controller.current?.abort(), []);
  const report = reports[category];
  const loaded = details[category] ?? {};
  const allDetails = Object.values(details).flatMap(byCategory => Object.values(byCategory));
  const pending = report?.entries.filter(entry => entry.detail_id && !loaded[entry.detail_id]) ?? [];
  const canLoadDetails = MEDICAL.has(category) || category === 'self_paid';

  function exportResult(value: SupplementReport) {
    const data = `${textEditorMode ? '\uFEFF' : ''}${JSON.stringify(value, null, 2)}\n`;
    downloadText(data, `healthbank-${value.category}.json`, 'application/json;charset=utf-8');
  }
  async function fetchReport(reference = '') {
    controller.current?.abort();
    const current = new AbortController(); controller.current = current;
    setBusy(true); setProgress(''); setError('');
    try {
      const result = await api.supplementary(category, from, to, reference, current.signal);
      if (current.signal.aborted) return;
      setPage(0);
      if (reference) {
        setDetail(result);
        setDetails(previous => ({ ...previous, [category]: { ...previous[category], [reference]: result } }));
      } else {
        setReports(previous => ({ ...previous, [category]: result }));
        // Official detail tokens are regenerated on every list query. Do not
        // accumulate stale records or infer record completeness from duplicates.
        setDetails(previous => ({ ...previous, [category]: {} }));
        setDetail(null);
      }
    } catch (failure) {
      if (!current.signal.aborted) setError(readableError(failure));
    } finally {
      if (!current.signal.aborted) setBusy(false);
    }
  }
  async function fetchDetailBatch() {
    if (!report || pending.length === 0) return;
    controller.current?.abort();
    const current = new AbortController(); controller.current = current;
    setBusy(true); setError('');
    const batch = pending.slice(0, BATCH_SIZE);
    let done = 0;
    try {
      for (const entry of batch) {
        const reference = entry.detail_id;
        if (!reference || current.signal.aborted) return;
        const result = await api.supplementary(category, from, to, reference, current.signal);
        if (controller.current !== current) return;
        setDetails(previous => ({ ...previous, [category]: { ...previous[category], [reference]: result } }));
        done++; setProgress(`本次已取得 ${done}／${batch.length} 筆明細；其餘可續批取得。`);
      }
    } catch (failure) {
      if (!current.signal.aborted) setError(`僅取得 ${done} 筆明細，已保留成功內容。${readableError(failure)}`);
    } finally {
      if (!current.signal.aborted) setBusy(false);
    }
  }
  const shown = detail ?? report;
  return <details className="panel analysis-tools supplementary"><summary>用藥、慢箋、就醫、手術／住院與補充官方資料</summary>
    <p className="helper-text">查詢及明細需官方登入。處方、開立日數、領藥或疾病分類並非實際服用／確診證明；資訊只保留在當前瀏覽器記憶體，不寫入本站資料庫或跨裝置設定。</p>
    <div className="scenario-controls">
      <label className="field">資料類別<select value={category} disabled={busy} onChange={event => { setCategory(event.target.value); setPage(0); setDetail(null); setError(''); setProgress(''); }}>{CATEGORIES.map(([key, name]) => <option value={key} key={key}>{name}</option>)}</select></label>
      <button className="button secondary" type="button" disabled={!authenticated || busy} onClick={() => { void fetchReport(); }}>{busy ? '取得中…' : '取得所選類別清單'}</button>
      {busy && <button className="button secondary" type="button" onClick={() => { controller.current?.abort(); controller.current = null; setBusy(false); setProgress('已取消，本次已取得的明細仍保留。'); }}>取消</button>}
      {shown && <button className="button secondary" type="button" onClick={() => exportResult(shown)}>匯出本頁 JSON（UTF-8）</button>}
      {detail && <button className="button secondary" type="button" onClick={() => { setPage(0); setDetail(null); }}>返回清單</button>}
      {shown && <label className="checkbox-label small"><input type="checkbox" checked={textEditorMode} onChange={event => setTextEditorMode(event.target.checked)} />文字編輯器需要時加 UTF-8 BOM</label>}
    </div>
    {report && !detail && canLoadDetails && pending.length > 0 && <div className="scenario-controls">
      <button className="button secondary" type="button" disabled={busy} onClick={() => { void fetchDetailBatch(); }}>分批取得明細（每批最多{BATCH_SIZE}筆）</button>
      <span className="helper-text">尚有 {pending.length} 筆可取得明細；逐筆取回、可取消，失敗不宣稱完整。</span>
    </div>}
    {report && canLoadDetails && pending.length === 0 && report.entries.some(entry => entry.detail_id) && <p className="helper-text">此清單目前可取得的明細已逐筆載入；是否涵蓋官方完整歷史，仍取決於官方回傳範圍。</p>}
    {progress && <p role="status">{progress}</p>}
    {!authenticated && <p>請先完成官方登入；匯入／示範模式不存取官方資料。</p>}
    {error && <p className="error-text" role="alert">{error} 原先已取得的資料仍保留。</p>}
    {allDetails.length > 0 && <MedicineTimeline reports={allDetails} onDate={date => analysis?.setSelectedDate(date)} />}
    {shown && <><p role="status">{shown.entries.length} 筆 · {shown.coverage}</p>{shown.start_date && <p>送出範圍：{dateLabel(shown.start_date)} — {dateLabel(shown.end_date ?? '')}</p>}
      <div className="scenario-controls"><button className="button secondary" disabled={page === 0} onClick={() => setPage(page - 1)}>上一頁</button><span>第 {page + 1}／{Math.max(1, Math.ceil(shown.entries.length / 50))} 頁 · 全部結果已保留，每頁顯示50筆</span><button className="button secondary" disabled={(page + 1) * 50 >= shown.entries.length} onClick={() => setPage(page + 1)}>下一頁</button></div>
      <div className="supplementary-list">{shown.entries.slice(page * 50, (page + 1) * 50).map((entry, index) => <details key={`${shown.fetched_at}:${page}:${entry.key || index}`} className="intake-item">
        <summary>{entry.date || '未提供日期'} · {entry.title} · {entry.institution || '未提供院所'}</summary>
        <p className="helper-text">{entry.source}{entry.end_date ? ` · 迄日 ${entry.end_date}` : ''}</p>
        {entry.medicine && <p className="helper-text">藥品：{entry.medicine.name || entry.medicine.code} · 代碼{entry.medicine.code || '不明'} · 給藥日數{entry.medicine.days || '未提供'} · 原始數量{entry.medicine.quantity || '未提供'}（單位未知）</p>}
        <dl>{entry.fields.map((field, ordinal) => <div key={`${field.name}:${ordinal}`}><dt>{field.name}</dt><dd>{field.value}{field.unit ? ` ${field.unit}` : ''}</dd></div>)}</dl>
        {entry.detail_id && <button className="button secondary" type="button" disabled={busy} onClick={() => { void fetchReport(entry.detail_id ?? ''); }}>取得此筆官方明細</button>}
        {medicalDate(entry.date) && <button className="button secondary" type="button" onClick={() => analysis?.setSelectedDate(medicalDate(entry.date))}>在趨勢標示此日期</button>}
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
