// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { api, ApiError, isSessionInvalidation, readableError, type SupplementReport, type ReportResponse, type SessionMode, type DataFieldTerm } from './api';
import { useAnalysis } from './AnalysisContext';
import { MedicineTimeline } from './MedicineTimeline';
import { dateLabel, downloadText, isObject } from './model';
import { supplementReportJSON, UNKNOWN_DATASET_PRODUCER, type DatasetProducer } from './health-dataset';
import { AcquisitionBatches } from './AcquisitionBatches';
import { FullAcquisition } from './FullAcquisition';
import { batchEntryAllowance, replaceBatchCategories, type BatchRange } from './date-batches';
import { sourceDateLabel, sourceOriginLabel } from './source-semantics';
import { laboratoryCoverageStatus } from './coverage-status';
import { cacheProvenanceText } from './cache-provenance';
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
type DetailView = { category: string; reference: string } | null;
interface RecordReturnContext {
  detailView: DetailView;
  filter: EventFilter;
  page: number;
  scrollY: number;
  eventId: string;
  trigger: 'detail' | 'refresh';
  filtersOpen: boolean;
}
type RecordFocusPlan = { kind: 'heading' } | { kind: 'return'; context: RecordReturnContext };
function aborted(signal: AbortSignal): boolean { return signal.aborted; }

function classifyFailure(failure: unknown) {
  const expired = isSessionInvalidation(failure);
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
  catalog = DEFAULT_DATA_CATEGORIES, fieldTerms = [], onAttachmentDownloaded, archivedAttachments = {}, onFetchLaboratory, onBusyChange,
  datasetProducer = UNKNOWN_DATASET_PRODUCER, onBatchActiveChange, onBeginBatch,
  imported = false, cached = false, disabled = false, laboratory, onOpenTrends,
  sessionMode = authenticated ? 'official' : imported ? 'import' : 'anonymous',
}: {
  authenticated: boolean;
  onSessionExpired: () => void;
  initialDataset?: HealthDataSnapshot;
  onDatasetChange?: (snapshot: HealthDataSnapshot) => void;
  catalog?: DataCategory[];
  fieldTerms?: DataFieldTerm[];
  onAttachmentDownloaded?: (attachment: DownloadedAttachment) => void;
  archivedAttachments?: Record<string, ArchivedAttachment>;
  onFetchLaboratory?: (signal: AbortSignal, query?: BatchRange) => Promise<ReportResponse>;
  onBusyChange?: (value: boolean) => void;
  datasetProducer?: DatasetProducer;
  onBatchActiveChange?: (active: boolean) => void;
  onBeginBatch?: (categories: string[], includeLaboratory: boolean) => void;
  imported?: boolean;
  cached?: boolean;
  disabled?: boolean;
  laboratory?: ReportResponse;
  sessionMode?: SessionMode;
  onOpenTrends?: (date: string) => void;
}) {
  const [snapshot, setSnapshot] = useState<HealthDataSnapshot>(() => initialDataset ?? emptyHealthData());
  const snapshotRef = useRef(snapshot);
  const [busy, setBusy] = useState(false);
  const [batchActive, setBatchActive] = useState(false);
  const [fullActive, setFullActive] = useState(false);
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
  const [detailView, setDetailView] = useState<DetailView>(null);
  const [markedEvent, setMarkedEvent] = useState<{ id: string; date: string } | null>(null);
  const controller = useRef<AbortController | null>(null);
  const returnHistory = useRef<RecordReturnContext[]>([]);
  const pendingFocus = useRef<RecordFocusPlan | null>(null);
  const recordsHeading = useRef<HTMLHeadingElement>(null);
  const recordElements = useRef(new Map<string, HTMLDetailsElement>());
  const filterOptions = useRef<HTMLDetailsElement>(null);
  const management = useRef<HTMLDetailsElement>(null);
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
  const fieldEnglish = useMemo(() => new Map(fieldTerms.map(term => [term.label, term.label_en])), [fieldTerms]);
  const pageCount = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
  const currentPage = Math.min(page, pageCount - 1);
  const selectedCategories = catalog.filter(category => !excluded.has(category.id)).map(category => category.id);
  const allSourcesSelected = selectedCategories.length === catalog.length && (!onFetchLaboratory || includeLaboratory);
  const queried = Object.keys(snapshot.reports).length;
  const failures = Object.values(snapshot.states).filter(state => state.status === 'failed').length;
  const sourceWarnings = Object.values(snapshot.reports).filter(report => report.diagnostics?.some(diagnostic => diagnostic.severity === 'warning')).length;
  const pending = catalog.reduce((count, category) => count + categoryCoverage(snapshot, category.id).pendingCount, 0);
  const acquisitionDisabled = !authenticated || busy || disabled || batchActive || fullActive;
  const activeFilterCount = Number(Boolean(filter.category)) + Number(Boolean(filter.institution)) + Number(Boolean(filter.from || filter.to)) + Number(!filter.includeUnknownDates);
  const laboratoryCoverage = laboratory ? laboratoryCoverageStatus(laboratory, sessionMode) : null;
  const selectedDetail = detailView ? snapshot.details[detailView.category]?.[detailView.reference] : undefined;

  useLayoutEffect(() => {
    const plan = pendingFocus.current;
    if (!plan) return;
    pendingFocus.current = null;
    // The workspace stays mounted while another main view is active. A late
    // detail response must not move focus or scroll in that other view.
    const heading = recordsHeading.current;
    if (!heading?.getClientRects().length) return;
    if (plan.kind === 'return') {
      if (filterOptions.current) filterOptions.current.open = plan.context.filtersOpen;
      const record = recordElements.current.get(plan.context.eventId);
      if (record) {
        record.open = true;
        const target = record.querySelector<HTMLElement>(`[data-detail-trigger="${plan.context.trigger}"]`) ?? record.querySelector('summary');
        target?.focus({ preventScroll: true });
        window.scrollTo({ top: plan.context.scrollY, behavior: 'auto' });
        if (target) {
          const bounds = target.getBoundingClientRect();
          if (bounds.top < 0 || bounds.bottom > window.innerHeight) target.scrollIntoView({ block: 'center', behavior: 'auto' });
        }
        return;
      }
    }
    heading.focus({ preventScroll: true });
    heading.scrollIntoView({ block: 'start', behavior: 'auto' });
  }, [detailView, filter, currentPage]);

  function markBusy(value: boolean) { setBusy(value); onBusyChange?.(value); }
  function publish(next: HealthDataSnapshot) {
    snapshotRef.current = next; setSnapshot(next); onDatasetChange?.(next);
  }
  function expire() {
    returnHistory.current = []; pendingFocus.current = null; setMarkedEvent(null);
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
          if (aborted(current.signal) || controller.current !== current) return;
          // A parent generation change can precede React's unmount cleanup.
          if (failure instanceof DOMException && failure.name === 'AbortError') return;
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
  async function openDetail(event: HealthEvent, refresh = false) {
    const category = event.category, reference = event.detailReference;
    if (!reference) return;
    const origin: RecordReturnContext = { detailView, filter: { ...filter }, page: currentPage,
      scrollY: window.scrollY, eventId: event.id, trigger: refresh ? 'refresh' : 'detail', filtersOpen: filterOptions.current?.open ?? false };
    const enter = () => {
      returnHistory.current.push(origin);
      pendingFocus.current = { kind: 'heading' };
      if (filterOptions.current) filterOptions.current.open = false;
      setDetailView({ category, reference }); setFilter(INITIAL_FILTER); setPage(0);
    };
    if (!refresh && Object.hasOwn(snapshotRef.current.details[category] ?? {}, reference)) {
      enter(); return;
    }
    if (acquisitionDisabled || !/^[a-f0-9]{48}$/.test(reference)) return;
    const current = new AbortController(); controller.current = current;
    markBusy(true); setError(''); setMessage('正在取得所選紀錄明細…');
    try {
      const result = await api.supplementary(category, '', '', reference, current.signal);
      if (aborted(current.signal) || controller.current !== current) return;
      publish(replaceDetailReport(snapshotRef.current, category, reference, result));
      enter(); setMessage('已取得明細。');
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
      const response = await api.document(event.category, attachment.id, current.signal);
      if (response.status === 401) { expire(); return; }
      if (!response.ok && response.headers.get('content-type')?.includes('application/json')) {
        const payload: unknown = await response.json();
        if (isObject(payload) && isObject(payload['error']) && typeof payload['error']['code'] === 'string' &&
          isSessionInvalidation(new ApiError('', payload['error']['code'], response.status))) {
          expire(); return;
        }
      }
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
  function returnToRecord() {
    const context = returnHistory.current.pop();
    pendingFocus.current = context ? { kind: 'return', context } : { kind: 'heading' };
    setDetailView(context?.detailView ?? null);
    setFilter(context?.filter ?? INITIAL_FILTER);
    setPage(context?.page ?? 0);
  }
  function showCategory(category: string) {
    returnHistory.current = []; pendingFocus.current = { kind: 'heading' };
    if (management.current) management.current.open = false;
    setDetailView(null); setFilter({ ...INITIAL_FILTER, category }); setPage(0);
  }
  function changePage(next: number) { pendingFocus.current = { kind: 'heading' }; setPage(next); }
  function pagination(position: 'top' | 'bottom') {
    if (pageCount < 2) return null;
    return <nav className="health-data-pagination" aria-label={`醫療紀錄分頁（${position === 'top' ? '上方' : '下方'}）`}>
      <button className="button secondary" type="button" disabled={currentPage === 0} onClick={() => changePage(currentPage - 1)}>上一頁</button>
      <span>第 {currentPage + 1}／{pageCount} 頁 · 每頁最多 {PAGE_SIZE} 筆</span>
      <button className="button secondary" type="button" disabled={currentPage + 1 >= pageCount} onClick={() => changePage(currentPage + 1)}>下一頁</button>
    </nav>;
  }
  function markEventDate(event: HealthEvent) {
    analysis?.setSelectedDate(event.date);
    setMarkedEvent({ id: event.id, date: event.date });
  }
  function exportSource(report: SupplementReport) {
    try {
      downloadText(supplementReportJSON(report), `healthbank-${report.category}.json`, 'application/json;charset=utf-8');
    } catch (failure) { setError(readableError(failure)); }
  }

  return <section className="panel analysis-tools supplementary health-data-workspace" aria-label="醫療資料工作區">
    <header><h2>查找醫療紀錄</h2></header>
    {authenticated && <FullAcquisition catalog={catalog} selectedCategories={selectedCategories} includeLaboratory={includeLaboratory}
      producer={datasetProducer} disabled={disabled || batchActive || busy && !fullActive} getSnapshot={() => snapshotRef.current}
      fetchLaboratory={onFetchLaboratory} classifyFailure={classifyFailure} onPublish={publish} onBegin={onBeginBatch} onBusyChange={markBusy} onSessionExpired={expire}
      onActiveChange={active => {
        setFullActive(active); onBatchActiveChange?.(active);
        if (active) { setProgress(null); setMessage(''); setError(''); }
      }} />}
    {authenticated && <div className="health-data-actions">
      <button className="button secondary" type="button" disabled={acquisitionDisabled} onClick={() => { void run('refresh', selectedCategories, { from: '', to: '' }, includeLaboratory); }}>{allSourcesSelected ? '取得全部清單與明細' : '取得所選來源的全部年度清單與明細'}</button>
      {(queried > 0 || labFailed) && <button className="button secondary" type="button" disabled={acquisitionDisabled} onClick={() => { void run('resume', selectedCategories, snapshot.query, includeLaboratory && labFailed); }}>接續未完成項目</button>}
      {busy && !batchActive && !fullActive && <button className="button secondary" type="button" onClick={cancel}>停止取得</button>}
    </div>}
    {cached ? <p className="health-data-notice">裝置快取：可查看已保存的清單、明細與附件；未保存的明細或原始檔不會假裝存在。請先清除舊資料並重新取得，再使用官方連線補取。</p> : imported ? <p className="health-data-notice">匯入資料；來源與完整性記載未經本次官方核對。</p> : !authenticated && sessionMode !== 'demo' && <p className="helper-text">可查看已取得紀錄；取得新資料需要官方登入。</p>}
    {progress && <div className="health-data-progress" role="status"><span>{progress.phase === 'lists' ? '來源清單' : '逐筆明細'}：{progress.done}／{progress.total} · {catalog.find(category => category.id === progress.category)?.label ?? progress.category}{progress.failed ? ` · ${progress.failed} 筆待重試` : ''}</span><progress max={Math.max(1, progress.total)} value={progress.done} aria-label={progress.phase === 'lists' ? '清單取得進度' : '明細取得進度'} /></div>}
    {message && <p role="status">{message}</p>}
    {error && <p className="error-text" role="alert">{error}</p>}
    <details className="health-data-management" ref={management}><summary>來源狀態與查詢範圍 · {queried}／{catalog.length} 個來源已回應{pending ? ` · ${pending} 筆明細待取得` : ''}{failures ? ` · ${failures} 個來源失敗` : ''}{sourceWarnings ? ` · ${sourceWarnings} 個清單回應需確認` : ''}</summary>
    <p className="helper-text">預設要求全部可查年度及類型；沒有日期搜尋的來源，保留官方實際回傳範圍。只查近期的來源會個別標示。「完整取得」可集中保存原始檔；下方仍可逐項取得與下載。</p>
    <details className="health-data-options"><summary>選擇來源與查詢期間</summary>
      {onFetchLaboratory && <label className="checkbox-label"><input type="checkbox" checked={includeLaboratory} disabled={busy || batchActive || fullActive} onChange={event => setIncludeLaboratory(event.target.checked)} />一併取得全部檢驗結果</label>}
      <div className="health-data-actions"><button className="button secondary" type="button" disabled={busy || batchActive || fullActive} onClick={() => setExcluded(new Set())}>選取所有來源</button><button className="button secondary" type="button" disabled={busy || batchActive || fullActive} onClick={() => setExcluded(new Set(catalog.map(category => category.id)))}>清除來源選取</button></div>
      {groups.map(group => <fieldset className="health-data-category-group" key={group}><legend>{group}</legend><div className="health-data-category-grid">
        {catalog.filter(category => category.group === group).map(category => <label className="checkbox-label" key={category.id}><input type="checkbox" disabled={busy || batchActive || fullActive} checked={!excluded.has(category.id)} onChange={event => setExcluded(previous => { const next = new Set(previous); if (event.target.checked) next.delete(category.id); else next.add(category.id); return next; })} /><span>{category.label}{category.label_en && <span className="terminology-english" lang="en">{category.label_en}</span>}<small>{category.range_mode === 'date_search' ? '支援日期搜尋' : '官方可回傳範圍'}</small></span></label>)}
      </div></fieldset>)}
      <form className="health-data-custom-range" onSubmit={event => { event.preventDefault(); void run('refresh', selectedCategories, { from: fromDraft, to: toDraft }, false); }}>
        <h3>另外指定取得期間</h3><p className="helper-text">只套用支援日期搜尋的來源。修改欄位不會清除資料或改變顯示篩選；送出後才查詢。檢驗結果的指定期間請在檢驗頁操作。</p>
        <div className="scenario-controls"><label className="field">取得起日<input type="date" required min="1912-01-01" max="2200-12-31" value={fromDraft} disabled={busy} onChange={event => setFromDraft(event.target.value)} /></label><label className="field">取得迄日<input type="date" required min={fromDraft || '1912-01-01'} max="2200-12-31" value={toDraft} disabled={busy} onChange={event => setToDraft(event.target.value)} /></label><button type="submit" className="button secondary" disabled={acquisitionDisabled}>取得指定期間</button></div>
      </form>
    </details>
    <details className="health-data-coverage"><summary>逐來源清單、明細與重試</summary>
      {laboratory && laboratoryCoverage && <p className={laboratoryCoverage.kind === 'mismatch' ? 'error-text' : undefined}>{laboratoryCoverage.summary} {laboratory.fetched_at ? `取得時間 ${laboratory.fetched_at}` : ''}</p>}
      <div className="health-data-coverage-grid">{catalog.map(category => {
        const report = snapshot.reports[category.id], coverage = categoryCoverage(snapshot, category.id), state = snapshot.states[category.id];
        const hasSourceWarning = report?.diagnostics?.some(diagnostic => diagnostic.severity === 'warning');
        const label = state?.status === 'loading' ? '正在取得' : state?.status === 'failed' ? '更新失敗' : state?.status === 'cancelled' ? '已停止' : report ? hasSourceWarning ? '來源回應需確認' : report.entries.length ? '已取得' : '已回應 0 筆' : '尚未查詢';
        return <article key={category.id} className="health-data-source"><h3>{category.label}{category.label_en && <span className="terminology-english" lang="en">{category.label_en}</span>}</h3><strong>{label}</strong>
          {report && <><p>已保留 {coverage.count} 筆清單 · 明細 {coverage.loadedCount}／{coverage.detailCount}{coverage.failedCount ? `（${coverage.failedCount}筆失敗）` : ''}</p><p className="helper-text">{report.coverage}</p>
            {report.cache_provenance && <p className="helper-text">{cacheProvenanceText(report.cache_provenance)}</p>}
            {report.start_date && <p>送出期間：{report.start_date} — {report.end_date || '未提供'}</p>}
            <p className="helper-text">{report.complete ? imported ? '檔案記載曾有來源總數核對；本次未重新驗證' : '來源已提供可核對總數' : '歷史完整性未確認，請一併查看來源範圍與警告'} · 取得時間 {report.fetched_at}</p>
            {report.diagnostics && report.diagnostics.length > 0 && <ul className="health-data-source-diagnostics" aria-label="來源回應結構說明">{report.diagnostics.map((diagnostic, index) => <li key={`${diagnostic.code}:${diagnostic.section}:${index}`} className={diagnostic.severity === 'warning' ? 'error-text' : 'helper-text'}>{diagnostic.severity === 'warning' ? '來源提醒：' : ''}{diagnostic.message}</li>)}</ul>}
          </>}
          {state?.error && <p className="error-text">{state.error}{report ? '；保留上次成功內容。' : ''}</p>}
          {coverage.failedCount > 0 && <details><summary>明細失敗原因</summary>{Object.entries(state?.detail_errors ?? {}).map(([reference, reason], index) => <p className="error-text" key={reference}>第 {index + 1} 筆：{reason}</p>)}</details>}
          <div className="health-data-actions"><button className="button secondary" type="button" disabled={acquisitionDisabled} onClick={() => { void run('refresh', [category.id], { from: '', to: '' }); }}>{report ? '重新取得此來源' : '取得此來源'}</button>
            {report && <button className="button quiet" type="button" onClick={() => showCategory(category.id)}>查看紀錄</button>}
            {coverage.pendingCount > 0 && <button className="button secondary" type="button" disabled={acquisitionDisabled} onClick={() => { void run('resume', [category.id], snapshot.query); }}>接續此來源明細</button>}
            {report && <button className="button quiet" type="button" onClick={() => exportSource(report)}>匯出此清單</button>}
          </div></article>;
      })}</div>
      <p className="helper-text">「尚未查詢」、「已回應 0 筆」與「更新失敗」意義不同；沒有取得紀錄不能解讀為沒有就醫。相同紀錄可能出現在不同來源，均保留，不把來源筆數當成就醫次數。</p>
    </details>
    </details>

    {authenticated && <AcquisitionBatches catalog={catalog} selectedCategories={selectedCategories} includeLaboratory={includeLaboratory}
      onIncludeLaboratoryChange={setIncludeLaboratory} entryAllowanceFor={categories => batchEntryAllowance(snapshotRef.current, categories)} producer={datasetProducer}
      disabled={disabled || fullActive || busy && !batchActive} fetchLaboratory={onFetchLaboratory} classifyFailure={classifyFailure}
      onPublish={(next, categories) => publish(replaceBatchCategories(snapshotRef.current, next, categories))}
      onBegin={(categories, includeLabs) => {
        returnHistory.current = []; pendingFocus.current = null; setDetailView(null); setPage(0); setMarkedEvent(null);
        onBeginBatch?.(categories, includeLabs);
      }}
      onActiveChange={active => { setBatchActive(active); onBatchActiveChange?.(active); }}
      onBusyChange={markBusy} onSessionExpired={expire} />}

    <section className="health-data-events" aria-label="所有醫療紀錄">
      <h3 ref={recordsHeading} tabIndex={-1}>{detailView ? '所選紀錄的明細' : '跨來源醫療紀錄'}</h3>
      {detailView && <div className="health-data-actions"><button className="button secondary" type="button" onClick={returnToRecord}>返回上一層紀錄</button><button className="button secondary" type="button" disabled={acquisitionDisabled} onClick={() => { const category = detailView.category; showCategory(category); void run('refresh', [category], { from: '', to: '' }); }}>重新取得此來源與明細</button></div>}
      {selectedDetail?.diagnostics && selectedDetail.diagnostics.length > 0 && <ul className="health-data-source-diagnostics" aria-label="所選明細的來源回應說明">{selectedDetail.diagnostics.map((diagnostic, index) => <li key={`${diagnostic.code}:${diagnostic.section}:${index}`} className={diagnostic.severity === 'warning' ? 'error-text' : 'helper-text'}>{diagnostic.severity === 'warning' ? '來源提醒：' : ''}{diagnostic.message}</li>)}</ul>}
      <label className="field health-data-search">搜尋紀錄<input type="search" value={filter.text} placeholder="藥品、檢查、原文或院所" onChange={event => updateFilter({ text: event.target.value })} /></label>
      <details className="health-data-display-options" ref={filterOptions}><summary>類別、院所與日期篩選{activeFilterCount ? `（已套用 ${activeFilterCount} 項）` : ''}</summary>
      <div className="scenario-controls">
        <label className="field">顯示類別<select value={filter.category} onChange={event => updateFilter({ category: event.target.value })}><option value="">所有已取得類別</option>{displayCatalog.map(category => <option value={category.id} key={category.id}>{category.label}</option>)}</select></label>
        <label className="field">顯示院所<select value={filter.institution} onChange={event => updateFilter({ institution: event.target.value })}><option value="">所有院所</option>{institutions.map(institution => <option key={institution} value={institution}>{institution}</option>)}</select></label>
      </div>
      <div className="scenario-controls"><label className="field">顯示起日<input type="date" value={filter.from} onChange={event => updateFilter({ from: event.target.value })} /></label><label className="field">顯示迄日<input type="date" value={filter.to} onChange={event => updateFilter({ to: event.target.value })} /></label>
        <label className="checkbox-label"><input type="checkbox" checked={filter.includeUnknownDates} onChange={event => updateFilter({ includeUnknownDates: event.target.checked })} />保留日期不明紀錄</label></div>
      <p className="helper-text">顯示篩選只改變目前畫面，不會重新查詢或刪除已取得資料；完整資料包保留所有已取得內容。</p>
      </details>
      {(filter.text || activeFilterCount > 0) && <button className="button quiet" type="button" onClick={() => updateFilter(INITIAL_FILTER)}>清除顯示篩選</button>}
      <p role="status">顯示 {filtered.length}／{events.length} 筆來源紀錄 · 第 {currentPage + 1}／{pageCount} 頁</p>
      <details className="health-data-medication"><summary>同藥品處方時間比較</summary><MedicineTimeline reports={allReports} onDate={date => analysis?.setSelectedDate(date)} onOpenTrends={onOpenTrends} /><p className="helper-text">官方處方、給藥日數、續領事件與實際服用是不同資訊。未知日數不推算停藥日期，未知劑量不畫每日服用曲線。</p></details>
      {pagination('top')}
      <div className="health-data-event-list">{filtered.slice(currentPage * PAGE_SIZE, (currentPage + 1) * PAGE_SIZE).map(event => {
        const entry = event.entry;
        const hasDetail = event.detailReference && Object.hasOwn(snapshot.details[event.category] ?? {}, event.detailReference);
        return <details key={event.id} className="health-data-event" ref={element => { if (element) recordElements.current.set(event.id, element); else recordElements.current.delete(event.id); }}><summary><span>{sourceDateLabel(event.category === 'laboratory' ? 'examination' : entry.date_role)}：{event.date ? dateLabel(event.date) : entry.date || '未提供日期'}</span><strong>{entry.title}</strong><span>{entry.institution || '未提供院所'}</span><small>{event.categoryLabel} · {event.parentReference ? '明細紀錄' : '來源清單'}{event.categoryLabelEn && <span className="terminology-english" lang="en">{event.categoryLabelEn}</span>}</small></summary>
          <p className="helper-text">{entry.source} · {sourceOriginLabel(entry.origin)}{entry.end_date ? ` · ${sourceDateLabel(entry.end_date_role)} ${entry.end_date}` : ''}{event.date && event.date !== entry.date ? ` · 原始日期 ${entry.date}` : ''}</p>
          {event.parentReference && <p className="helper-text">這筆內容由所選來源紀錄的明細取得；相同日期的其他資料不會自動被當作同次就醫。</p>}
          {entry.medicine && <p>藥品：{entry.medicine.name || entry.medicine.code} · 代碼 {entry.medicine.code || '未提供'} · 給藥日數 {entry.medicine.days || '未提供'} · 原始數量 {entry.medicine.quantity || '未提供'}（單位未確認）</p>}
          <dl>{entry.fields.map((field, ordinal) => <div key={`${ordinal}:${field.name}`}><dt>{field.name}{fieldEnglish.get(field.name) && <span className="terminology-english" lang="en">{fieldEnglish.get(field.name)}</span>}</dt><dd>{field.value}{field.unit ? ` ${field.unit}` : ''}</dd></div>)}</dl>
          <div className="health-data-actions">{event.detailReference && <button className="button secondary" type="button" data-detail-trigger="detail" disabled={busy || !hasDetail && (acquisitionDisabled || !/^[a-f0-9]{48}$/.test(event.detailReference))} onClick={() => { void openDetail(event); }}>{hasDetail ? '查看已取得明細' : '取得此筆明細'}</button>}
            {hasDetail && /^[a-f0-9]{48}$/.test(event.detailReference) && <button className="button quiet" type="button" data-detail-trigger="refresh" disabled={acquisitionDisabled} onClick={() => { void openDetail(event, true); }}>更新這筆明細</button>}
            {event.date && <button className="button quiet" type="button" onClick={() => markEventDate(event)}>在檢驗趨勢標示日期</button>}
          </div>
          {markedEvent?.id === event.id && <div className="health-data-date-feedback"><p role="status">已標示 {dateLabel(markedEvent.date)}。{laboratory?.records.some(record => record.exam_date === markedEvent.date) ? '可在趨勢查看當日檢驗；同日不代表同次採檢。' : '目前未取得當日檢驗結果。'}</p>{onOpenTrends && <button className="button secondary" type="button" onClick={() => { analysis?.setSelectedDate(markedEvent.date); onOpenTrends(markedEvent.date); }}>前往檢驗趨勢</button>}</div>}
          {entry.attachments && entry.attachments.length > 0 && <div className="health-data-attachments"><h4>原始附件</h4>{entry.attachments.map(attachment => <div key={attachment.id}><span>{attachment.name}</span><div className="health-data-actions">
            {archivedAttachments[`${event.category}:${attachment.id}`] ? <button className="button secondary" type="button" onClick={() => { void downloadAttachment(event, attachment); }}>下載資料包內附件</button> : authenticated && /^[a-f0-9]{48}$/.test(attachment.id) ? <>
              <a className="button secondary" href={attachmentURL(event.category, attachment.id)} download={attachment.name}>另存原始檔</a>
              {onAttachmentDownloaded && (!attachment.size || attachment.size <= MAX_EMBEDDED_ATTACHMENT) && <button className="button secondary" type="button" disabled={acquisitionDisabled} onClick={() => { void downloadAttachment(event, attachment); }}>下載並加入資料包</button>}
            </> : <span className="helper-text">資料包未內含檔案；原工作階段的下載連結已失效。</span>}
          </div></div>)}<p className="helper-text">另存原始檔由瀏覽器管理；只有選擇加入資料包且成功讀取的附件，才能隨資料包再次開啟。</p></div>}
          {entry.document_actions && entry.document_actions.length > 0 && <div className="health-data-actions">{entry.document_actions.map(action => <button className="button secondary" type="button" key={action.id} disabled={acquisitionDisabled || !/^[a-f0-9]{48}$/.test(action.id)} onClick={() => { void prepareDocument(event.category, action.id); }}>{action.name}</button>)}</div>}
        </details>;
      })}</div>
      {pagination('bottom')}
      {filtered.length === 0 && <p>{events.length ? '目前顯示條件沒有符合的紀錄，可清除篩選查看。' : '尚未取得可顯示的紀錄。請登入取得，或匯入先前下載的完整資料包。'}</p>}
    </section>
  </section>;
}

export function CoverageSummary({ report, mode = 'anonymous' }: { report: ReportResponse; mode?: SessionMode }) {
  const coverage = laboratoryCoverageStatus(report, mode);
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
    <p>已取得：{dateLabel(earliest)} — {dateLabel(latest)} · {report.records.length} 筆。</p>
    <p className={coverage.kind === 'mismatch' ? 'error-text' : undefined}>{coverage.summary}</p>
    <p>{[...counts.entries()].toSorted(([a], [b]) => b.localeCompare(a)).map(([year, count]) => `${year}年：${count}筆`).join('；')}{unknown ? `；日期不明：${unknown}筆` : ''}</p>
    <small>未列出的年度表示目前沒有取得該年度結果，不表示沒有做過檢查。同日多筆均保留，不以筆數推定檢查次數。</small>
  </details>;
}
