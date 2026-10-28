// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import { useMemo, useRef, useState } from 'react';
import type { LabRecord, ReportResponse, SessionMode } from './api';
import type { DataCategory, HealthDataSnapshot } from './health-data';
import { createPrintReport, DEFAULT_PRINT_OPTIONS, renderPrintReport, type PrintOptions } from './print-report';
import { downloadText } from './model';
import { Alert, Modal } from './ui';

export function PrintReport({ report, snapshot, categories, selectedLaboratory, mode, version, onClose }: {
  report: ReportResponse; snapshot: HealthDataSnapshot; categories: DataCategory[];
  selectedLaboratory: LabRecord[]; mode: SessionMode; version: string; onClose: () => void;
}) {
  const [options, setOptions] = useState<PrintOptions>(DEFAULT_PRINT_OPTIONS);
  const [generatedAt] = useState(() => new Date().toISOString());
  const [loadedDocument, setLoadedDocument] = useState('');
  const [printError, setPrintError] = useState('');
  const frame = useRef<HTMLIFrameElement>(null);
  const prepared = useMemo(() => {
    try {
      const data = createPrintReport({ report, snapshot, categories, selectedLaboratory, mode, version, generatedAt, options });
      return { data, html: renderPrintReport(data), error: '' };
    } catch { return { data: null, html: '', error: '報表日期或範圍不正確，請確認後重新產生。' }; }
  }, [report, snapshot, categories, selectedLaboratory, mode, version, generatedAt, options]);
  const count = (prepared.data?.laboratory.length ?? 0) + (prepared.data?.clinical.length ?? 0);
  function print() {
    const target = frame.current?.contentWindow;
    if (!target || !prepared.html || loadedDocument !== prepared.html) return;
    try { setPrintError(''); target.focus(); target.print(); }
    catch { setPrintError('瀏覽器未能開啟列印。可下載專用報表，再以瀏覽器開啟並列印或另存 PDF。'); }
  }
  return <Modal title="製作列印／PDF 報表" subtitle="由實際資料重新製作完整表格，先確認範圍與筆數，再列印或另存 PDF。" onClose={onClose} wide>
    <div className="report-options">
      <label>報表範圍<select value={options.scope} onChange={event => setOptions({ ...options, scope: event.target.value === 'selection' ? 'selection' : 'all' })}>
        <option value="all">全部已取得資料</option><option value="selection">指定分享範圍</option>
      </select></label>
      <label>檢驗排版<select value={options.layout} onChange={event => setOptions({ ...options, layout: event.target.value === 'comparison' ? 'comparison' : 'records' })}>
        <option value="records">逐筆原始結果表</option><option value="comparison">日期對照表（每段最多四項）</option>
      </select></label>
      <label className="checkbox-label"><input type="checkbox" checked={options.laboratory} onChange={event => setOptions({ ...options, laboratory: event.target.checked })} />包含檢驗結果</label>
      <label className="checkbox-label"><input type="checkbox" checked={options.clinical} onChange={event => setOptions({ ...options, clinical: event.target.checked })} />包含醫療來源紀錄</label>
    </div>
    {options.scope === 'selection' && <fieldset className="report-options"><legend>分享範圍</legend>
      <p>檢驗採目前畫面的項目篩選，共 {selectedLaboratory.length} 筆候選；再依下列日期選取。醫療紀錄依下列類別與日期選取，涵蓋所有頁碼。</p>
      <label>報表起日<input type="date" value={options.from} onChange={event => setOptions({ ...options, from: event.target.value })} /></label>
      <label>報表迄日<input type="date" value={options.to} onChange={event => setOptions({ ...options, to: event.target.value })} /></label>
      <label>報表醫療類別<select value={options.category} onChange={event => setOptions({ ...options, category: event.target.value })}><option value="">全部已取得類別</option>{categories.map(category => <option key={category.id} value={category.id}>{category.label}</option>)}</select></label>
      <label className="checkbox-label"><input type="checkbox" checked={options.includeUnknownDates} onChange={event => setOptions({ ...options, includeUnknownDates: event.target.checked })} />保留日期不明紀錄</label>
    </fieldset>}
    <p role="status">報表包含檢驗 {prepared.data?.laboratory.length ?? 0} 筆、醫療來源紀錄 {prepared.data?.clinical.length ?? 0} 筆。{loadedDocument !== prepared.html && prepared.html ? '正在準備專用報表…' : ''}</p>
    {prepared.error && <Alert tone="error">{prepared.error}</Alert>}{printError && <Alert tone="error">{printError}</Alert>}
    <div className="button-row"><button type="button" className="button primary" disabled={!count || !prepared.html || loadedDocument !== prepared.html} onClick={print}>列印／另存 PDF</button>
      <button type="button" className="button secondary" disabled={!count || !prepared.html} onClick={() => downloadText(prepared.html, `健康映覽-專用報表-${generatedAt.slice(0, 10)}.html`, 'text/html;charset=utf-8')}>下載專用報表 HTML</button></div>
    <p className="small">PDF 請在瀏覽器列印視窗選擇「另存為 PDF」。專用報表已包含全部選取資料，可自動跨頁；原始附件另行下載。本機輸出含健康資料，請確認範圍後分享。</p>
    {prepared.html && <iframe ref={frame} className="report-preview" title="專用報表完整預覽" sandbox="allow-same-origin allow-modals" srcDoc={prepared.html} onLoad={() => setLoadedDocument(prepared.html)} />}
  </Modal>;
}
