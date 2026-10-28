// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import { useEffect, useRef, useState } from 'react';
import { api, readableError, type ReportResponse } from './api';
import { acquireHealthData, MAX_DETAIL_REQUESTS_PER_RUN, type AcquisitionFailure, type AcquisitionProgress } from './data-acquisition';
import { emptyHealthData, healthDataEntryCount, type DataCategory, type HealthDataSnapshot } from './health-data';
import { exportDataset, type DatasetProducer } from './health-dataset';
import { downloadText } from './model';
import { assertBatchAdvance, batchLaboratoryMatches, batchSummary, beginDateBatch, dateBatchStatus,
  missingBatchLaboratory, selectedDateCategories, splitDateBatches, stoppedDateBatch,
  type BatchPeriod, type BatchRange, type DateBatchPlan, type DateBatchSummary } from './date-batches';

interface BatchState { plan: DateBatchPlan; index: number; summaries: DateBatchSummary[] }

export function AcquisitionBatches({ catalog, selectedCategories, includeLaboratory, onIncludeLaboratoryChange,
  entryAllowanceFor, producer, disabled, fetchLaboratory, classifyFailure, onPublish, onBegin, onActiveChange, onBusyChange, onSessionExpired }: {
  catalog: DataCategory[];
  selectedCategories: string[];
  includeLaboratory: boolean;
  onIncludeLaboratoryChange: (value: boolean) => void;
  entryAllowanceFor: (categories: string[]) => number;
  producer: DatasetProducer;
  disabled: boolean;
  fetchLaboratory: ((signal: AbortSignal, range?: BatchRange) => Promise<ReportResponse>) | undefined;
  classifyFailure: (failure: unknown) => AcquisitionFailure;
  onPublish: (batch: HealthDataSnapshot, categories: string[]) => void;
  onBegin: (categories: string[], laboratory: boolean) => void;
  onActiveChange: (active: boolean) => void;
  onBusyChange: (busy: boolean) => void;
  onSessionExpired: () => void;
}) {
  const [from, setFrom] = useState('');
  const [to, setTo] = useState('');
  const [period, setPeriod] = useState<BatchPeriod>('year');
  const [state, setState] = useState<BatchState | null>(null);
  const [batch, setBatch] = useState<HealthDataSnapshot>(emptyHealthData);
  const batchRef = useRef(batch);
  const [laboratory, setLaboratory] = useState<ReportResponse | null>(null);
  const laboratoryRef = useRef<ReportResponse | null>(null);
  const [busy, setBusy] = useState(false);
  const [saving, setSaving] = useState(false);
  const [exported, setExported] = useState(false);
  const [savedAcknowledged, setSavedAcknowledged] = useState(false);
  const [error, setError] = useState('');
  const [message, setMessage] = useState('');
  const [progress, setProgress] = useState<AcquisitionProgress | null>(null);
  const controller = useRef<AbortController | null>(null);
  const lifecycle = useRef(0);
  const callbacks = useRef({ onActiveChange, onBusyChange });
  useEffect(() => { callbacks.current = { onActiveChange, onBusyChange }; }, [onActiveChange, onBusyChange]);
  useEffect(() => () => {
    lifecycle.current += 1; controller.current?.abort(); controller.current = null;
    callbacks.current.onBusyChange(false); callbacks.current.onActiveChange(false);
  }, []);
  const selection = selectedDateCategories(catalog, selectedCategories);
  const status = state ? dateBatchStatus(batch, state.plan.categories, state.plan.includeLaboratory, laboratory) : null;
  const currentRange = state?.plan.ranges[state.index];
  const finalBatch = state !== null && state.index === state.plan.ranges.length - 1;

  function publish(next: HealthDataSnapshot, plan: DateBatchPlan) {
    onPublish(next, plan.categories);
    batchRef.current = next; setBatch(next);
    setExported(false); setSavedAcknowledged(false);
  }

  async function run(currentState: BatchState, initial: HealthDataSnapshot, fresh: boolean) {
    if (controller.current) return;
    const range = currentState.plan.ranges[currentState.index];
    if (!range) return;
    const entryAllowance = entryAllowanceFor(currentState.plan.categories);
    const current = new AbortController(); controller.current = current;
    setBusy(true); onBusyChange(true); setError(''); setMessage(''); setProgress(null);
    setExported(false); setSavedAcknowledged(false);
    try {
      if (currentState.plan.includeLaboratory && fetchLaboratory && !batchLaboratoryMatches(laboratoryRef.current, range)) {
        setMessage('正在取得本批檢驗結果…');
        try {
          const result = await fetchLaboratory(current.signal, range);
          current.signal.throwIfAborted();
          if (controller.current !== current) return;
          if (!batchLaboratoryMatches(result, range)) throw new Error('檢驗回應的查詢期間與本批不一致，未混入本批資料包。');
          laboratoryRef.current = result; setLaboratory(result);
        } catch (failure) {
          if (current.signal.aborted || controller.current !== current) return;
          if (classifyFailure(failure).expired) { onSessionExpired(); return; }
          setError(`本批檢驗尚未取得：${readableError(failure)}`);
        }
      }
      current.signal.throwIfAborted();
      if (controller.current !== current) return;
      const result = await acquireHealthData({ snapshot: initial, categories: currentState.plan.categories,
        query: range, mode: fresh ? 'refresh' : 'resume', signal: current.signal,
        fetchReport: (category, reference, query, signal) => api.supplementary(category, query.from, query.to, reference, signal),
        classifyFailure, onUpdate: next => { if (controller.current === current) publish(next, currentState.plan); },
        onProgress: next => { if (controller.current === current) setProgress(next); },
        limits: { entries: entryAllowance, detailRequests: MAX_DETAIL_REQUESTS_PER_RUN },
      });
      if (current.signal.aborted || controller.current !== current) return;
      if (result.expired) { onSessionExpired(); return; }
      publish(result.snapshot, currentState.plan);
      const complete = dateBatchStatus(result.snapshot, currentState.plan.categories, currentState.plan.includeLaboratory, laboratoryRef.current).acquired;
      setMessage(complete ? '本批可讀取的清單與明細已處理，可匯出本批資料包。來源歷史完整性仍依各項說明。'
        : '本批仍有未完成項目；成功內容已保留，請接續取得。也可先保存部分結果，結束計畫後改用更小期間。');
    } catch (failure) {
      if (!current.signal.aborted && controller.current === current) setError(readableError(failure));
    } finally {
      if (controller.current === current) {
        if (current.signal.aborted) {
          publish(stoppedDateBatch(batchRef.current), currentState.plan);
          setMessage('已停止本批取得；成功清單與明細仍保留，可接續本批。');
        }
        controller.current = null; setBusy(false); onBusyChange(false);
      }
    }
  }

  function start() {
    if (disabled || busy || saving || controller.current) return;
    try {
      const plan: DateBatchPlan = { ranges: splitDateBatches({ from, to }, period),
        categories: selection.eligible.map(item => item.id), includeLaboratory: Boolean(fetchLaboratory && includeLaboratory) };
      if (plan.categories.length === 0 && !plan.includeLaboratory) throw new Error('請選擇至少一個支援日期搜尋的來源，或一併取得檢驗結果。');
      const first = plan.ranges[0]; if (!first) return;
      const next = beginDateBatch(first, plan.categories);
      const nextState = { plan, index: 0, summaries: [] };
      onBegin(plan.categories, plan.includeLaboratory);
      laboratoryRef.current = null; setLaboratory(null);
      publish(next, plan); setState(nextState); onActiveChange(true);
      void run(nextState, next, true);
    } catch (failure) { setError(readableError(failure)); }
  }

  async function save() {
    if (!state || busy || saving || disabled) return;
    const generation = lifecycle.current;
    setSaving(true); setError(''); setMessage('');
    try {
      const content = await exportDataset(laboratory ?? missingBatchLaboratory(batch.query, state.plan.includeLaboratory),
        batch, [], catalog.filter(item => state.plan.categories.includes(item.id)), false, producer);
      if (generation !== lifecycle.current) return;
      downloadText(content, `healthbank-batch-${batch.query.from}_${batch.query.to}.json`, 'application/json;charset=utf-8');
      setExported(true); setSavedAcknowledged(false);
      setMessage('已啟動本批資料包下載。請確認檔案已保存後勾選；瀏覽器無法代您確認磁碟上的檔案。');
    } catch (failure) { if (generation === lifecycle.current) setError(readableError(failure)); }
    finally { if (generation === lifecycle.current) setSaving(false); }
  }

  function advance() {
    if (!state || disabled || busy || saving || controller.current) return;
    try {
      assertBatchAdvance(batch, state.plan, laboratory, exported, savedAcknowledged);
      if (finalBatch) { finish(); return; }
      const range = state.plan.ranges[state.index + 1]; if (!range) return;
      const nextState = { ...state, index: state.index + 1, summaries: [...state.summaries, batchSummary(batch, laboratory)] };
      const next = beginDateBatch(range, state.plan.categories);
      onBegin(state.plan.categories, state.plan.includeLaboratory);
      laboratoryRef.current = null; setLaboratory(null);
      publish(next, state.plan); setState(nextState);
      void run(nextState, next, true);
    } catch (failure) { setError(readableError(failure)); }
  }

  function finish() {
    if (busy || saving || controller.current) return;
    lifecycle.current += 1; setState(null); setBatch(emptyHealthData()); batchRef.current = emptyHealthData();
    laboratoryRef.current = null; setLaboratory(null); setProgress(null); setExported(false); setSavedAcknowledged(false);
    onActiveChange(false); setMessage('分批計畫已結束；目前這批已取得資料仍留在工作區，先前批次由您下載的檔案保存。');
  }

  return <details className="health-data-management health-data-batches" open={state !== null || undefined}>
    <summary>按年／按月分批取得與保存</summary>
    <p className="helper-text">每批處理所選來源的清單及明細，只保留目前這批與先前批次的筆數摘要。來源沒有獨立總數時，完成本批仍不代表終身資料完整；重複原始紀錄不合併。</p>
    {!state ? <form onSubmit={event => { event.preventDefault(); start(); }}>
      <div className="scenario-controls">
        <label className="field">分批起日<input type="date" required min="1912-01-01" max="2200-12-31" value={from} disabled={disabled} onChange={event => setFrom(event.target.value)} /></label>
        <label className="field">分批迄日<input type="date" required min={from || '1912-01-01'} max="2200-12-31" value={to} disabled={disabled} onChange={event => setTo(event.target.value)} /></label>
        <label className="field">每批期間<select value={period} disabled={disabled} onChange={event => setPeriod(event.target.value === 'month' ? 'month' : 'year')}><option value="year">按年</option><option value="month">按月</option></select></label>
      </div>
      {fetchLaboratory && <label className="checkbox-label"><input type="checkbox" checked={includeLaboratory} disabled={disabled} onChange={event => onIncludeLaboratoryChange(event.target.checked)} />本計畫一併取得檢驗結果</label>}
      <p>目前納入 {selection.eligible.length} 個已選且支援日期搜尋的來源{fetchLaboratory && includeLaboratory ? '，以及檢驗結果' : ''}。可在「選擇來源與查詢期間」調整來源。</p>
      {selection.excluded.length > 0 && <details><summary>{selection.excluded.length} 個已選來源不支援日期搜尋，不納入分批</summary><p>{selection.excluded.map(item => item.label).join('、')}。這些來源仍可用原本的全部資料操作取得。</p></details>}
      <p className="helper-text">開始第一批或下一批會取代所選來源在工作區的資料，未選來源保留。請先保存需要留下的既有資料。每次最多 600 批，附件不會自動下載。</p>
      <button type="submit" className="button secondary" disabled={disabled}>開始第一批</button>
    </form> : <>
      <h4>第 {state.index + 1}／{state.plan.ranges.length} 批：{currentRange?.from} 至 {currentRange?.to}</h4>
      <p role="status">本批已保留 {healthDataEntryCount(batch)} 筆來源紀錄、{laboratory?.records.length ?? 0} 筆檢驗。{status && !status.acquired ? `待取得：${status.listsPending} 個清單、${status.detailsPending} 筆明細${status.laboratoryPending ? '及檢驗結果' : ''}。` : '本批可讀取項目已處理。'}</p>
      {progress && <p role="status">{progress.phase === 'lists' ? '來源清單' : '逐筆明細'} {progress.done}／{progress.total} · {catalog.find(item => item.id === progress.category)?.label ?? progress.category}</p>}
      <div className="health-data-actions">
        {busy ? <button type="button" className="button secondary" onClick={() => controller.current?.abort()}>停止本批取得</button>
          : <button type="button" className="button secondary" disabled={disabled || saving || status?.acquired} onClick={() => { void run(state, batchRef.current, false); }}>接續本批未完成項目</button>}
        <button type="button" className="button secondary" disabled={disabled || busy || saving} onClick={() => { void save(); }}>{saving ? '產生資料包中…' : '匯出本批資料包 v2'}</button>
        <button type="button" className="button quiet" disabled={busy || saving} onClick={finish}>結束分批計畫</button>
      </div>
      <label className="checkbox-label"><input type="checkbox" checked={savedAcknowledged} disabled={!exported || busy || saving} onChange={event => setSavedAcknowledged(event.target.checked)} />我已確認本批資料包已保存</label>
      <button type="button" className="button primary" disabled={disabled || busy || saving || !status?.acquired || !exported || !savedAcknowledged} onClick={advance}>{finalBatch ? '完成分批計畫' : '開始下一批並釋放這批資料'}</button>
      <p className="helper-text">資料包只包含本批成功取得的清單、明細與本批檢驗；未取得項目保留狀態。附件須另外保存，本流程不自動備份檔案。若未完成項目無法取得，可保存部分結果後結束計畫，改用較小期間。</p>
      {state.summaries.length > 0 && <details><summary>已確認保存的批次摘要（{state.summaries.length}）</summary><ul>{state.summaries.map(summary => <li key={summary.from}>{summary.from} 至 {summary.to}：{summary.sources} 個來源、{summary.entries} 筆來源紀錄、{summary.laboratory} 筆檢驗。</li>)}</ul></details>}
    </>}
    {message && <p role="status">{message}</p>}
    {error && <p className="error-text" role="alert">{error}</p>}
  </details>;
}
