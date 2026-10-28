// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import { useEffect, useMemo, useRef, useState } from 'react';
import { api, ApiError, readableError, type SupplementReport, type ReportResponse } from './api';
import { useAnalysis } from './AnalysisContext';
import { MedicineTimeline } from './MedicineTimeline';
import { dateLabel, downloadText } from './model';
import { acquireHealthData, type AcquisitionMode, type AcquisitionProgress } from './data-acquisition';
import {
  DEFAULT_DATA_CATEGORIES, categoryCoverage, emptyHealthData, filterHealthEvents, healthEvents,
  laboratoryEvents, validAcquisitionQuery, type ArchivedAttachment, type DataCategory, type DownloadedAttachment,
  replaceDetailReport, type EventFilter, type HealthDataSnapshot, type HealthEvent,
} from './health-data';
import './health-data.css';

const PAGE_SIZE = 50;
const MAX_EMBEDDED_ATTACHMENT = 32 * 1024 * 1024;
const INITIAL_FILTER: EventFilter = { text: '', category: '', institution: '', from: '', to: '', includeUnknownDates: true };
function aborted(signal: AbortSignal): boolean { return signal.aborted; }

function classifyFailure(failure: unknown) {
  const expired = failure instanceof ApiError && failure.status === 401;
  const retryable = failure instanceof TypeError || failure instanceof ApiError && !expired &&
    (failure.status === 502 || failure.status === 503 || failure.status === 504 ||
      failure.status === 424 && (failure.upstreamStatus === 502 || failure.upstreamStatus === 503 || failure.upstreamStatus === 504 ||
        ['NETWORK_ERROR', 'REQUEST_TIMEOUT', 'CONNECTION_FAILED'].includes(failure.code)));
  return { message: readableError(failure), expired, retryable };
}

function attachmentURL(category: string, id: string): string {
  return `/api/documents?${new URLSearchParams({ category, id })}`;
}

function downloadBytes(bytes: Uint8Array, name: string, type: string) {
  const url = URL.createObjectURL(new Blob([new Uint8Array(bytes)], { type }));
  const link = document.createElement('a');
  link.href = url; link.download = name; link.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

export function SupplementaryData({ authenticated, onSessionExpired, initialDataset, onDatasetChange,
  catalog = DEFAULT_DATA_CATEGORIES, onAttachmentDownloaded, archivedAttachments = {}, onFetchLaboratory, onBusyChange,
  imported = false, disabled = false, laboratory,
}: {
  authenticated: boolean;
  onSessionExpired: () => void;
  initialDataset?: HealthDataSnapshot;
  onDatasetChange?: (snapshot: HealthDataSnapshot) => void;
  catalog?: DataCategory[];
  onAttachmentDownloaded?: (attachment: DownloadedAttachment) => void;
  archivedAttachments?: Record<string, ArchivedAttachment>;
  onFetchLaboratory?: (signal: AbortSignal) => Promise<void>;
  onBusyChange?: (value: boolean) => void;
  imported?: boolean;
  disabled?: boolean;
  laboratory?: ReportResponse;
}) {
  const [snapshot, setSnapshot] = useState<HealthDataSnapshot>(() => initialDataset ?? emptyHealthData());
  const snapshotRef = useRef(snapshot);
  const [busy, setBusy] = useState(false);
  const [progress, setProgress] = useState<AcquisitionProgress | null>(null);
  const [message, setMessage] = useState('');
  const [error, setError] = useState('');
  const [labFailed, setLabFailed] = useState(false);
  const [includeLaboratory, setIncludeLaboratory] = useState(true);
  const [excluded, setExcluded] = useState<Set<string>>(() => new Set());
  const [fromDraft, setFromDraft] = useState('');
  const [toDraft, setToDraft] = useState('');
  const [filter, setFilter] = useState<EventFilter>(INITIAL_FILTER);
  const [page, setPage] = useState(0);
  const [detailView, setDetailView] = useState<{ category: string; reference: string } | null>(null);
  const controller = useRef<AbortController | null>(null);
  const analysis = useAnalysis();
  useEffect(() => () => controller.current?.abort(), []);

  const events = useMemo(() => [...healthEvents(snapshot, catalog), ...laboratory ? laboratoryEvents(laboratory) : []]
    .toSorted((a, b) => b.date.localeCompare(a.date) || a.id.localeCompare(b.id)), [snapshot, catalog, laboratory]);
  const displayCatalog: DataCategory[] = laboratory ? [{ id: 'laboratory', label: '檢驗結果', group: '檢驗', range_mode: 'date_search' }, ...catalog] : catalog;
  const filtered = useMemo(() => filterHealthEvents(detailView ? events.filter(event =>
    event.category === detailView.category && event.parentReference === detailView.reference) : events, filter), [events, detailView, filter]);
  const institutions = useMemo(() => [...new Set(events.map(event => event.entry.institution).filter(Boolean))].toSorted(), [events]);
  const allReports = useMemo(() => [...Object.values(snapshot.reports),
    ...Object.values(snapshot.details).flatMap(value => Object.values(value))], [snapshot]);
  const groups = [...new Set(catalog.map(category => category.group))];
  const pageCount = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
  const currentPage = Math.min(page, pageCount - 1);
  const selectedCategories = catalog.filter(category => !excluded.has(category.id)).map(category => category.id);
  const queried = Object.keys(snapshot.reports).length;
  const failures = Object.values(snapshot.states).filter(state => state.status === 'failed').length;
  const pending = catalog.reduce((count, category) => count + categoryCoverage(snapshot, category.id).pendingCount, 0);
  const acquisitionDisabled = !authenticated || busy || disabled;

  function markBusy(value: boolean) { setBusy(value); onBusyChange?.(value); }
  function publish(next: HealthDataSnapshot) {
    snapshotRef.current = next; setSnapshot(next); onDatasetChange?.(next);
  }
  function expire() {
    publish(emptyHealthData()); setDetailView(null); setProgress(null);
    setError('官方登入階段已結束，本次資料已清除。請重新登入後取得。'); onSessionExpired();
  }
  function cancel() {
    controller.current?.abort(); controller.current = null;
    const current = snapshotRef.current;
    publish({ ...current, states: Object.fromEntries(Object.entries(current.states).map(([key, state]) =>
      [key, state.status === 'loading' ? { ...state, status: 'cancelled' } : state])) });
    markBusy(false); setMessage('已停止取得；完成的清單與明細仍保留，可接續未完成項目。');
  }
  async function run(mode: AcquisitionMode, categories: string[], query: HealthDataSnapshot['query'], withLaboratory = false) {
    if (acquisitionDisabled) return;
    if (!validAcquisitionQuery(query)) { setError('請填寫完整的有效日期；起日不得晚於迄日。'); return; }
    if (categories.length === 0 && !withLaboratory) { setError('請至少選取一個資料類別。'); return; }
    controller.current?.abort();
    const current = new AbortController(); controller.current = current;
    markBusy(true); setError(''); setMessage(''); setProgress(null);
    try {
      if (withLaboratory && onFetchLaboratory) {
        setMessage('正在取得檢驗結果…');
        try { await onFetchLaboratory(current.signal); setLabFailed(false); }
        catch (failure) {
          if (aborted(current.signal)) return;
          if (classifyFailure(failure).expired) { expire(); return; }
          setLabFailed(true); setError(`檢驗結果尚未更新：${readableError(failure)}。其他類別仍繼續取得。`);
        }
      }
      if (aborted(current.signal)) return;
      setMessage('');
      const result = await acquireHealthData({ snapshot: snapshotRef.current, categories, query, mode,
        signal: current.signal, fetchReport: (category, reference, range, signal) =>
          api.supplementary(category, range.from, range.to, reference, signal),
        classifyFailure, onUpdate: next => { if (controller.current === current) publish(next); },
        onProgress: next => { if (controller.current === current) setProgress(next); },
      });
      if (aborted(current.signal) || controller.current !== current) return;
      if (result.expired) { expire(); return; }
      const failedLists = categories.filter(category => result.snapshot.states[category]?.status === 'failed').length;
      const missingDetails = categories.reduce((count, category) => count + categoryCoverage(result.snapshot, category).pendingCount, 0);
      setMessage(failedLists || missingDetails
        ? `本輪取得已結束：${failedLists} 個類別未成功更新、${missingDetails} 筆明細未取得；成功內容已保留，可接續重試。`
        : '本輪可讀取的清單及明細已逐筆處理。來源未提供獨立總數的類別，仍不能宣稱完整終身歷史。');
    } catch (failure) { if (!aborted(current.signal)) setError(readableError(failure)); }
    finally { if (controller.current === current) { markBusy(false); controller.current = null; } }
  }
  async function openDetail(category: string, reference: string, refresh = false) {
    if (!refresh && Object.hasOwn(snapshotRef.current.details[category] ?? {}, reference)) {
      setDetailView({ category, reference }); setFilter(INITIAL_FILTER); setPage(0); return;
    }
    if (acquisitionDisabled || !/^[a-f0-9]{48}$/.test(reference)) return;
    const current = new AbortController(); controller.current = current;
    markBusy(true); setError(''); setMessage('正在取得所選紀錄明細…');
    try {
      const result = await api.supplementary(category, '', '', reference, current.signal);
      if (aborted(current.signal) || controller.current !== current) return;
      publish(replaceDetailReport(snapshotRef.current, category, reference, result));
      setDetailView({ category, reference }); setFilter(INITIAL_FILTER); setPage(0); setMessage('已取得明細。');
    } catch (failure) {
      if (!aborted(current.signal)) {
        if (classifyFailure(failure).expired) expire(); else setError(readableError(failure));
      }
    } finally { if (controller.current === current) { controller.current = null; markBusy(false); } }
  }
  async function downloadAttachment(event: HealthEvent, attachment: NonNullable<HealthEvent['entry']['attachments']>[number]) {
    const archived = archivedAttachments[`${event.category}:${attachment.id}`];
    if (archived) { downloadBytes(archived.bytes, archived.name, archived.content_type); return; }
    if (acquisitionDisabled) return;
    const current = new AbortController(); controller.current = current; markBusy(true); setError('');
    setMessage('正在讀取附件；完成後會下載並加入本次可攜資料。');
    try {
      const response = await fetch(attachmentURL(event.category, attachment.id), { credentials: 'same-origin', cache: 'no-store', signal: current.signal });
      if (response.status === 401) { expire(); return; }
      if (!response.ok || !response.body) throw new Error(`附件未完成（HTTP ${response.status}），可稍後重試。`);
      const size = Number(response.headers.get('content-length'));
      if (size > MAX_EMBEDDED_ATTACHMENT) { await response.body.cancel(); throw new Error('此附件超過資料包的 32 MiB 單檔上限。請使用「另存原始檔」直接下載。'); }
      const reader = response.body.getReader();
      const chunks: Uint8Array[] = []; let length = 0;
      try {
        for (;;) {
          const part = await reader.read();
          if (part.done) break;
          length += part.value.byteLength;
          if (length > MAX_EMBEDDED_ATTACHMENT) { await reader.cancel(); throw new Error('此附件超過資料包的 32 MiB 單檔上限。請使用「另存原始檔」直接下載。'); }
          chunks.push(part.value);
        }
      } finally { reader.releaseLock(); }
      if (aborted(current.signal) || controller.current !== current) return;
      const bytes = new Uint8Array(length); let offset = 0;
      for (const chunk of chunks) { bytes.set(chunk, offset); offset += chunk.byteLength; }
      const contentType = response.headers.get('content-type')?.split(';')[0] ?? attachment.content_type;
      const sourceName = /(?:^|;)\s*filename="?([A-Za-z0-9._-]{1,160})"?(?:;|$)/i.exec(response.headers.get('content-disposition') ?? '')?.[1];
      const name = sourceName ?? attachment.name;
      onAttachmentDownloaded?.({ category: event.category, id: attachment.id, name, content_type: contentType, bytes });
      downloadBytes(bytes, name, contentType);
      const warning = response.headers.get('X-Healthbank-Document-Warning')?.includes('official_temporary_cleanup_unconfirmed');
      setMessage(`附件已讀取並啟動下載；可在完整資料包中一併保存。${warning ? ' 官方暫存檔清除結果尚未確認，原始檔的保留仍由官方管理。' : ''}`);
    } catch (failure) { if (!aborted(current.signal)) setError(readableError(failure)); }
    finally { if (controller.current === current) { controller.current = null; markBusy(false); } }
  }
  async function prepareDocument(category: string, id: string) {
    if (acquisitionDisabled) return;
    const current = new AbortController(); controller.current = current; markBusy(true); setError('');
    try {
      const result = await api.prepareDocument(category, id, current.signal);
      if (aborted(current.signal) || controller.current !== current) return;
      setMessage(`${result.message} 請稍後在取得狀態中重新取得此來源，或取得「已產製原始檔」清單。`);
    } catch (failure) {
      if (!aborted(current.signal)) { if (classifyFailure(failure).expired) expire(); else setError(readableError(failure)); }
    } finally { if (controller.current === current) { controller.current = null; markBusy(false); } }
  }
  function updateFilter(patch: Partial<EventFilter>) { setFilter(previous => ({ ...previous, ...patch })); setPage(0); }
  function exportSource(report: SupplementReport) {
    downloadText(`${JSON.stringify(report, null, 2)}\n`, `healthbank-${report.category}.json`, 'application/json;charset=utf-8');
  }

  return <section className="panel analysis-tools supplementary health-data-workspace" aria-label="醫療資料工作區">
    <header><h2>我的醫療資料</h2><p>一次取得各類可用資料，再依日期、院所與內容查看。清單、明細和原始附件各自保留來源。</p></header>
    <div className="health-data-actions">
      <button className="button primary" type="button" disabled={acquisitionDisabled} onClick={() => { void run('refresh', selectedCategories, { from: '', to: '' }, includeLaboratory); }}>取得全部可提供資料</button>
      <button className="button secondary" type="button" disabled={acquisitionDisabled || queried === 0 && !labFailed} onClick={() => { void run('resume', selectedCategories, snapshot.query, includeLaboratory && labFailed); }}>接續未完成項目</button>
      {busy && <button className="button secondary" type="button" onClick={cancel}>停止取得</button>}
    </div>
    <p className="helper-text">預設要求全部可查年度及類型；沒有日期搜尋的來源，保留官方實際回傳範圍。只查近期的來源會個別標示。列表和明細逐筆取得，附件由你選擇下載。</p>
    {!authenticated && <p className="health-data-notice">請完成官方登入以取得新資料。已匯入資料包可直接在這裡查看，不會代替身分驗證。</p>}
    {imported && <p className="health-data-notice">目前紀錄來自匯入資料包。來源、範圍與完整性文字沿用檔案記載，未經本次官方連線驗證。</p>}
    <details className="health-data-options"><summary>選擇來源與查詢期間</summary>
      {onFetchLaboratory && <label className="checkbox-label"><input type="checkbox" checked={includeLaboratory} disabled={busy} onChange={event => setIncludeLaboratory(event.target.checked)} />一併取得全部檢驗結果</label>}
      <div className="health-data-actions"><button className="button secondary" type="button" disabled={busy} onClick={() => setExcluded(new Set())}>選取所有來源</button><button className="button secondary" type="button" disabled={busy} onClick={() => setExcluded(new Set(catalog.map(category => category.id)))}>清除來源選取</button></div>
      {groups.map(group => <fieldset className="health-data-category-group" key={group}><legend>{group}</legend><div className="health-data-category-grid">
        {catalog.filter(category => category.group === group).map(category => <label className="checkbox-label" key={category.id}><input type="checkbox" disabled={busy} checked={!excluded.has(category.id)} onChange={event => setExcluded(previous => { const next = new Set(previous); if (event.target.checked) next.delete(category.id); else next.add(category.id); return next; })} /><span>{category.label}<small>{category.range_mode === 'date_search' ? '支援日期搜尋' : '官方可回傳範圍'}</small></span></label>)}
      </div></fieldset>)}
      <form className="health-data-custom-range" onSubmit={event => { event.preventDefault(); void run('refresh', selectedCategories, { from: fromDraft, to: toDraft }, false); }}>
        <h3>另外指定取得期間</h3><p className="helper-text">只套用支援日期搜尋的來源。修改欄位不會清除資料或改變顯示篩選；送出後才查詢。檢驗結果的指定期間請在檢驗頁操作。</p>
        <div className="scenario-controls"><label className="field">取得起日<input type="date" required min="1912-01-01" max="2200-12-31" value={fromDraft} disabled={busy} onChange={event => setFromDraft(event.target.value)} /></label><label className="field">取得迄日<input type="date" required min={fromDraft || '1912-01-01'} max="2200-12-31" value={toDraft} disabled={busy} onChange={event => setToDraft(event.target.value)} /></label><button type="submit" className="button secondary" disabled={acquisitionDisabled}>取得指定期間</button></div>
      </form>
    </details>
    {progress && <div className="health-data-progress" role="status"><span>{progress.phase === 'lists' ? '來源清單' : '逐筆明細'}：{progress.done}／{progress.total} · {catalog.find(category => category.id === progress.category)?.label ?? progress.category}{progress.failed ? ` · ${progress.failed} 筆待重試` : ''}</span><progress max={Math.max(1, progress.total)} value={progress.done} aria-label={progress.phase === 'lists' ? '清單取得進度' : '明細取得進度'} /></div>}
    {message && <p role="status">{message}</p>}
    {error && <p className="error-text" role="alert">{error}</p>}

    <details className="health-data-coverage"><summary>取得狀態：{queried}／{catalog.length} 個來源已有資料回應 · {pending} 筆明細待取得{failures ? ` · ${failures} 個來源失敗` : ''}</summary>
      {laboratory && <p>檢驗結果：{laboratory.records.length} 筆 · {laboratory.fetched_at ? `取得時間 ${laboratory.fetched_at}` : '尚未取得'} · {laboratory.complete && !imported ? '本次查詢有官方總數核對' : '未經本次獨立總數核對'}。相同日期的檢驗與就醫紀錄不會自動視為同次採檢。</p>}
      <div className="health-data-coverage-grid">{catalog.map(category => {
        const report = snapshot.reports[category.id], coverage = categoryCoverage(snapshot, category.id), state = snapshot.states[category.id];
        const label = state?.status === 'loading' ? '正在取得' : state?.status === 'failed' ? '更新失敗' : state?.status === 'cancelled' ? '已停止' : report ? report.entries.length ? '已取得' : '已回應 0 筆' : '尚未查詢';
        return <article key={category.id} className="health-data-source"><h3>{category.label}</h3><strong>{label}</strong>
          {report && <><p>{coverage.count} 筆清單 · 明細 {coverage.loadedCount}／{coverage.detailCount}{coverage.failedCount ? `（${coverage.failedCount}筆失敗）` : ''}</p><p className="helper-text">{report.coverage}</p>
            {report.start_date && <p>送出期間：{report.start_date} — {report.end_date || '未提供'}</p>}
            <p className="helper-text">{report.complete ? imported ? '檔案記載曾有來源總數核對；本次未重新驗證' : '來源已提供可核對總數' : '未取得獨立總數，歷史完整性未確認'} · 取得時間 {report.fetched_at}</p></>}
          {state?.error && <p className="error-text">{state.error}{report ? '；保留上次成功內容。' : ''}</p>}
          {coverage.failedCount > 0 && <details><summary>明細失敗原因</summary>{Object.entries(state?.detail_errors ?? {}).map(([reference, reason], index) => <p className="error-text" key={reference}>第 {index + 1} 筆：{reason}</p>)}</details>}
          <div className="health-data-actions"><button className="button secondary" type="button" disabled={acquisitionDisabled} onClick={() => { void run('refresh', [category.id], { from: '', to: '' }); }}>{report ? '重新取得此來源' : '取得此來源'}</button>
            {report && <button className="button quiet" type="button" onClick={() => { setDetailView(null); updateFilter({ ...INITIAL_FILTER, category: category.id }); }}>查看紀錄</button>}
            {coverage.pendingCount > 0 && <button className="button secondary" type="button" disabled={acquisitionDisabled} onClick={() => { void run('resume', [category.id], snapshot.query); }}>接續此來源明細</button>}
            {report && <button className="button quiet" type="button" onClick={() => exportSource(report)}>匯出此清單</button>}
          </div></article>;
      })}</div>
      <p className="helper-text">「尚未查詢」、「已回應 0 筆」與「更新失敗」意義不同；沒有取得紀錄不能解讀為沒有就醫。相同紀錄可能出現在不同來源，均保留，不把來源筆數當成就醫次數。</p>
    </details>

    <details className="health-data-medication"><summary>同藥品處方時間比較</summary><MedicineTimeline reports={allReports} onDate={date => analysis?.setSelectedDate(date)} /><p className="helper-text">官方處方、給藥日數、續領事件與實際服用是不同資訊。未知日數不推算停藥日期，未知劑量不畫每日服用曲線。</p></details>

    <section className="health-data-events" aria-label="所有醫療紀錄">
      <h3>{detailView ? '所選紀錄的明細' : '跨來源醫療紀錄'}</h3>
      {detailView && <div className="health-data-actions"><button className="button secondary" type="button" onClick={() => { setDetailView(null); setFilter(INITIAL_FILTER); setPage(0); }}>返回全部紀錄</button><button className="button secondary" type="button" disabled={acquisitionDisabled} onClick={() => { const category = detailView.category; setDetailView(null); setFilter(INITIAL_FILTER); setPage(0); void run('refresh', [category], { from: '', to: '' }); }}>重新取得此來源與明細</button></div>}
      <div className="scenario-controls"><label className="field">搜尋紀錄<input type="search" value={filter.text} placeholder="藥品、檢查、原文或院所" onChange={event => updateFilter({ text: event.target.value })} /></label>
        <label className="field">顯示類別<select value={filter.category} onChange={event => updateFilter({ category: event.target.value })}><option value="">所有已取得類別</option>{displayCatalog.map(category => <option value={category.id} key={category.id}>{category.label}</option>)}</select></label>
        <label className="field">顯示院所<select value={filter.institution} onChange={event => updateFilter({ institution: event.target.value })}><option value="">所有院所</option>{institutions.map(institution => <option key={institution} value={institution}>{institution}</option>)}</select></label>
      </div>
      <div className="scenario-controls"><label className="field">顯示起日<input type="date" value={filter.from} onChange={event => updateFilter({ from: event.target.value })} /></label><label className="field">顯示迄日<input type="date" value={filter.to} onChange={event => updateFilter({ to: event.target.value })} /></label>
        <label className="checkbox-label"><input type="checkbox" checked={filter.includeUnknownDates} onChange={event => updateFilter({ includeUnknownDates: event.target.checked })} />保留日期不明紀錄</label><button className="button secondary" type="button" onClick={() => updateFilter(INITIAL_FILTER)}>清除顯示篩選</button></div>
      <p className="helper-text">顯示篩選只改變目前畫面，不會重新查詢或刪除已取得資料；完整資料包保留所有已取得內容。</p>
      <p role="status">顯示 {filtered.length}／{events.length} 筆來源紀錄 · 每筆原始資料獨立保留</p>
      <div className="health-data-pagination"><button className="button secondary" type="button" disabled={currentPage === 0} onClick={() => setPage(currentPage - 1)}>上一頁</button><span>第 {currentPage + 1}／{pageCount} 頁 · 每頁最多{PAGE_SIZE}筆</span><button className="button secondary" type="button" disabled={currentPage + 1 >= pageCount} onClick={() => setPage(currentPage + 1)}>下一頁</button></div>
      <div className="health-data-event-list">{filtered.slice(currentPage * PAGE_SIZE, (currentPage + 1) * PAGE_SIZE).map(event => {
        const entry = event.entry;
        const hasDetail = event.detailReference && Object.hasOwn(snapshot.details[event.category] ?? {}, event.detailReference);
        return <details key={event.id} className="health-data-event"><summary><span>{event.date ? dateLabel(event.date) : entry.date || '未提供日期'}</span><strong>{entry.title}</strong><span>{entry.institution || '未提供院所'}</span><small>{event.categoryLabel} · {event.parentReference ? '明細紀錄' : '來源清單'}</small></summary>
          <p className="helper-text">{entry.source}{entry.end_date ? ` · 來源迄日 ${entry.end_date}` : ''}{event.date && event.date !== entry.date ? ` · 原始日期 ${entry.date}` : ''}</p>
          {event.parentReference && <p className="helper-text">這筆內容由所選來源紀錄的明細取得；相同日期的其他資料不會自動被當作同次就醫。</p>}
          {entry.medicine && <p>藥品：{entry.medicine.name || entry.medicine.code} · 代碼 {entry.medicine.code || '未提供'} · 給藥日數 {entry.medicine.days || '未提供'} · 原始數量 {entry.medicine.quantity || '未提供'}（單位未確認）</p>}
          <dl>{entry.fields.map((field, ordinal) => <div key={`${ordinal}:${field.name}`}><dt>{field.name}</dt><dd>{field.value}{field.unit ? ` ${field.unit}` : ''}</dd></div>)}</dl>
          <div className="health-data-actions">{event.detailReference && <button className="button secondary" type="button" disabled={busy || !hasDetail && (acquisitionDisabled || !/^[a-f0-9]{48}$/.test(event.detailReference))} onClick={() => { void openDetail(event.category, event.detailReference); }}>{hasDetail ? '查看已取得明細' : '取得此筆明細'}</button>}
            {hasDetail && /^[a-f0-9]{48}$/.test(event.detailReference) && <button className="button quiet" type="button" disabled={acquisitionDisabled} onClick={() => { void openDetail(event.category, event.detailReference, true); }}>更新這筆明細</button>}
            {event.date && <button className="button quiet" type="button" onClick={() => analysis?.setSelectedDate(event.date)}>在檢驗趨勢標示日期</button>}
          </div>
          {entry.attachments && entry.attachments.length > 0 && <div className="health-data-attachments"><h4>原始附件</h4>{entry.attachments.map(attachment => <div key={attachment.id}><span>{attachment.name}</span><div className="health-data-actions">
            {archivedAttachments[`${event.category}:${attachment.id}`] ? <button className="button secondary" type="button" onClick={() => { void downloadAttachment(event, attachment); }}>下載資料包內附件</button> : authenticated && /^[a-f0-9]{48}$/.test(attachment.id) ? <>
              <a className="button secondary" href={attachmentURL(event.category, attachment.id)} download={attachment.name}>另存原始檔</a>
              {onAttachmentDownloaded && (!attachment.size || attachment.size <= MAX_EMBEDDED_ATTACHMENT) && <button className="button secondary" type="button" disabled={acquisitionDisabled} onClick={() => { void downloadAttachment(event, attachment); }}>下載並加入資料包</button>}
            </> : <span className="helper-text">資料包未內含檔案；原工作階段的下載連結已失效。</span>}
          </div></div>)}<p className="helper-text">另存原始檔由瀏覽器管理；只有選擇加入資料包且成功讀取的附件，才能隨資料包再次開啟。</p></div>}
          {entry.document_actions && entry.document_actions.length > 0 && <div className="health-data-actions">{entry.document_actions.map(action => <button className="button secondary" type="button" key={action.id} disabled={acquisitionDisabled || !/^[a-f0-9]{48}$/.test(action.id)} onClick={() => { void prepareDocument(event.category, action.id); }}>{action.name}</button>)}</div>}
        </details>;
      })}</div>
      {filtered.length === 0 && <p>{events.length ? '目前顯示條件沒有符合的紀錄，可清除篩選查看。' : '尚未取得可顯示的紀錄。請登入取得，或匯入先前下載的完整資料包。'}</p>}
    </section>
  </section>;
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
