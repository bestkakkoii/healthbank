// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import { useEffect, useRef, useState } from 'react';
import { api, ApiError, assertPackageReferences, readableError, type ReportResponse } from './api';
import { acquireHealthData, type AcquisitionFailure, type AcquisitionProgress } from './data-acquisition';
import { categoryCoverage, replaceDetailReport, type DataCategory, type HealthDataSnapshot } from './health-data';
import { exportDatasetPackage, type DatasetPackageProjection, type DatasetProducer } from './health-dataset';
import { downloadText } from './model';
import { acquisitionSummary, capturePreparations, datasetDigest, MAX_EMBEDDED_PACKAGE_DATASET, MAX_ORIGINAL_FAILURES, MAX_PREPARATION_POLL_ROUNDS,
  nextPreparationPolls, packageFileGroups, preparationRefreshKey, preparationRefreshTargets, PREPARATION_POLL_DELAY_MS, protectPolledPreparations, recordSavedOriginals,
  remainingOriginals, retryableOriginals, selectedSnapshot, submitPreparations, waitForAcquisition,
  type AcquisitionSummary, type ExternalDataset, type PackageCreated, type PackageReceipt, type PackageRequest,
  type PreparationTask, type SavedOriginals } from './full-acquisition';

interface Plan {
  id: string;
  categories: string[];
  includeLaboratory: boolean;
  requestPreparation: boolean;
  scope: AcquisitionSummary['scope'];
  startedAt: string;
}
interface Bundle {
  projection: DatasetPackageProjection;
  dataset: ExternalDataset;
  acquisition: AcquisitionSummary;
  groups: DatasetPackageProjection['files'][];
}
interface SavedPart {
  plan_id: string;
  index: number;
  name: string;
  sha256: string;
  size: number;
  state: PackageReceipt['state'];
  completed: number;
  failed: number;
  total: number;
  dataset_included: boolean;
  saved_acknowledged: true;
}
const PREPARATION_LABELS = {
  queued: '尚未送出', requested: '官方已受理，產製完成尚未確認', indeterminate: '送達結果未明，不自動重送',
  source_refreshed: '已更新同來源狀態，依實際可用檔案收錄', correlation_unavailable: '官方未提供可核對的工作關聯', failed: '未完成',
};

function planID(): string { return [...crypto.getRandomValues(new Uint8Array(16))].map(value => value.toString(16).padStart(2, '0')).join(''); }
function missingLaboratory(included: boolean): ReportResponse {
  return { records: [], fetched_at: '', complete: false, warnings: [included
    ? '本輪檢驗未取得；本包不混入先前期間或登入階段的檢驗，零筆不是沒有檢查。'
    : '本輪未選擇檢驗結果；資料包不包含工作區其他檢驗。'] };
}
function partName(plan: string, index: number): string { return `healthbank-acquisition-${plan}-part${index}.zip`; }

export function FullAcquisition({ catalog, selectedCategories, includeLaboratory, producer, disabled, getSnapshot,
  fetchLaboratory, classifyFailure, onPublish, onBegin, onActiveChange, onBusyChange, onSessionExpired }: {
  catalog: DataCategory[];
  selectedCategories: string[];
  includeLaboratory: boolean;
  producer: DatasetProducer;
  disabled: boolean;
  getSnapshot: () => HealthDataSnapshot;
  fetchLaboratory: ((signal: AbortSignal) => Promise<ReportResponse>) | undefined;
  classifyFailure: (failure: unknown) => AcquisitionFailure;
  onPublish: (snapshot: HealthDataSnapshot) => void;
  onBegin: ((categories: string[], includeLaboratory: boolean) => void) | undefined;
  onActiveChange: (value: boolean) => void;
  onBusyChange: (value: boolean) => void;
  onSessionExpired: () => void;
}) {
  const [plan, setPlan] = useState<Plan | null>(null);
  const planRef = useRef<Plan | null>(null);
  const [requestPreparation, setRequestPreparation] = useState(true);
  const [tasks, setTasks] = useState<PreparationTask[]>([]);
  const tasksRef = useRef<PreparationTask[]>([]);
  const captured = useRef(new Set<string>());
  const polledRoots = useRef(new Map<string, { category: string; parent: string }>());
  const pollAttempts = useRef(new Map<string, number>());
  const laboratory = useRef<ReportResponse | null>(null);
  const [progress, setProgress] = useState<AcquisitionProgress | null>(null);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState('');
  const [error, setError] = useState('');
  const [bundle, setBundle] = useState<Bundle | null>(null);
  const bundleRef = useRef<Bundle | null>(null);
  const [externalRequired, setExternalRequired] = useState(false);
  const [externalDownloaded, setExternalDownloaded] = useState(false);
  const [externalSaved, setExternalSaved] = useState(false);
  const [part, setPart] = useState(1);
  const [packageInfo, setPackageInfo] = useState<PackageCreated | null>(null);
  const packageRef = useRef<PackageCreated | null>(null);
  const [receipt, setReceipt] = useState<PackageReceipt | null>(null);
  const [savedAcknowledged, setSavedAcknowledged] = useState(false);
  const [savedParts, setSavedParts] = useState<SavedPart[]>([]);
  const collectionID = useRef('');
  const [savedOriginals, setSavedOriginals] = useState<SavedOriginals>({});
  const [taskPage, setTaskPage] = useState(0);
  const controller = useRef<AbortController | null>(null);
  const lifecycle = useRef(0);
  const callbacks = useRef({ onBusyChange, onActiveChange });
  useEffect(() => { callbacks.current = { onBusyChange, onActiveChange }; }, [onBusyChange, onActiveChange]);
  useEffect(() => () => {
    lifecycle.current += 1; controller.current?.abort(); controller.current = null;
    callbacks.current.onBusyChange(false); callbacks.current.onActiveChange(false);
  }, []);

  function markBusy(value: boolean) { setBusy(value); onBusyChange(value); }
  function valid(current: AbortController, generation: number): boolean {
    return lifecycle.current === generation && controller.current === current && !current.signal.aborted;
  }
  function publishTasks(next: PreparationTask[]) { tasksRef.current = next; setTasks(next); }
  function rememberPackage(next: PackageCreated | null) { packageRef.current = next; setPackageInfo(next); }
  function protectPolling(snapshot: HealthDataSnapshot, category?: string, parent = '') {
    if (category) {
      polledRoots.current.set(`${category}:${parent}`, { category, parent });
      captured.current = protectPolledPreparations(snapshot, category, parent, captured.current);
      return;
    }
    for (const root of polledRoots.current.values())
      captured.current = protectPolledPreparations(snapshot, root.category, root.parent, captured.current);
  }

  async function preparePart(nextBundle: Bundle, currentPlan: Plan, index: number, external: boolean,
    current: AbortController, generation: number) {
    const files = nextBundle.groups[index - 1]; if (!files) return;
    const input: PackageRequest = { ...(external ? { external_dataset: nextBundle.dataset } : { dataset: nextBundle.projection.content }),
      files, acquisition: nextBundle.acquisition,
      plan: { id: currentPlan.id, index, total: nextBundle.groups.length,
        files_total: nextBundle.groups.reduce((sum, group) => sum + group.length, 0) } };
    setMessage(`正在準備第 ${index}／${nextBundle.groups.length} 個原始檔包…`);
    try {
      const created = await api.createAcquisitionPackage(input, current.signal);
      if (!valid(current, generation)) return;
      rememberPackage(created); setPart(index); setReceipt(created.state === 'ready' ? null : created); setSavedAcknowledged(false);
      setMessage('原始檔包已準備。按下下載後會逐檔串流取得，請等待瀏覽器完成下載，再確認已保存。');
    } catch (failure) {
      if (!valid(current, generation)) return;
      if (!external && failure instanceof ApiError && failure.status === 503 && failure.code === 'package_retained_capacity') {
        setExternalRequired(true); setExternalDownloaded(false); setExternalSaved(false);
        setMessage('目前伺服器可用記憶體不足以保留資料包。請先將同一份完整 JSON 保存到裝置；原始檔包將以 SHA-256 明確連結它。');
        return;
      }
      if (classifyFailure(failure).expired) throw failure;
      if (failure instanceof ApiError && failure.code === 'package_plan_conflict') throw failure;
      // A lost creation response is reconciled by the stable plan/part key.
      // Neither lookup nor an explicit idempotent retry starts file transfers.
      try {
        const observed = await api.acquisitionPackageStatus({ plan: currentPlan.id, part: index }, current.signal);
        if (!valid(current, generation)) return;
        assertPackageReferences(observed, files);
        rememberPackage(observed);
        setReceipt(observed); setPart(index); setSavedAcknowledged(false);
        setMessage('已核對先前的原始檔包狀態；沒有重新啟動下載。');
      } catch (lookupFailure) {
        if (classifyFailure(lookupFailure).expired) throw lookupFailure;
        throw failure;
      }
    }
  }

  async function makeBundle(currentPlan: Plan, snapshot: HealthDataSnapshot, pollingLimited: boolean,
    current: AbortController, generation: number) {
    const selected = selectedSnapshot(snapshot, currentPlan.categories);
    const projection = await exportDatasetPackage(laboratory.current ?? missingLaboratory(currentPlan.includeLaboratory), selected,
      catalog.filter(category => currentPlan.categories.includes(category.id)), producer);
    const digest = await datasetDigest(projection.content);
    if (!valid(current, generation)) return;
    const next: Bundle = { projection, dataset: digest, groups: packageFileGroups(projection.files),
      acquisition: acquisitionSummary({ snapshot: selected, categories: currentPlan.categories, laboratory: laboratory.current,
        includeLaboratory: currentPlan.includeLaboratory, tasks: tasksRef.current, requestPreparation: currentPlan.requestPreparation,
        scope: currentPlan.scope, startedAt: currentPlan.startedAt, finishedAt: new Date().toISOString(), pollingLimited }) };
    bundleRef.current = next; setBundle(next); setExternalDownloaded(false); setExternalSaved(false);
    const external = digest.size > MAX_EMBEDDED_PACKAGE_DATASET;
    setExternalRequired(external);
    if (external) setMessage('完整資料 JSON 超過 32 MiB。請先下載並確認保存，原始檔包會用其 SHA-256 連結同一份完整資料，不拆散紀錄關聯。');
    else await preparePart(next, currentPlan, 1, false, current, generation);
  }

  async function acquire(currentPlan: Plan, resume: boolean) {
    if (controller.current || disabled || savedParts.length > 0 || packageRef.current) return;
    const previousBundle = bundleRef.current;
    const current = new AbortController(), generation = lifecycle.current;
    controller.current = current; markBusy(true); setError(''); setProgress(null); rememberPackage(null); setReceipt(null);
    setSavedAcknowledged(false);
    const deadline = setTimeout(() => current.abort(new DOMException('本輪等待已達 20 分鐘，可接續尚未完成項目。', 'TimeoutError')), 20 * 60_000);
    let snapshot = getSnapshot();
    if (!resume) {
      const otherCategories = [...new Set([...Object.keys(snapshot.reports), ...Object.keys(snapshot.details), ...Object.keys(snapshot.states)])]
        .filter(category => !currentPlan.categories.includes(category));
      snapshot = selectedSnapshot(snapshot, otherCategories); onPublish(snapshot);
    }
    try {
      if (resume && previousBundle) {
        // Re-reading sources changes exported_at and may change capabilities.
        // Retire any ambiguously created package before choosing a fresh plan;
        // a status lookup for an old body cannot validate a new dataset hash.
        try {
          const previous = await api.acquisitionPackageStatus({ plan: currentPlan.id, part: 1 }, current.signal);
          if (!valid(current, generation)) return;
          await api.cancelAcquisitionPackage(previous.id, current.signal);
        } catch (failure) { if (!(failure instanceof ApiError && failure.code === 'package_unavailable')) throw failure; }
        if (!valid(current, generation)) return;
        currentPlan = { ...currentPlan, id: planID() }; planRef.current = currentPlan; setPlan(currentPlan);
        pollAttempts.current.clear();
      }
      bundleRef.current = null; setBundle(null);
      if (currentPlan.includeLaboratory && !laboratory.current && fetchLaboratory) {
        setMessage('正在取得全部可查期間的檢驗結果…');
        try { laboratory.current = await fetchLaboratory(current.signal); }
        catch (failure) {
          if (!valid(current, generation) || failure instanceof DOMException && failure.name === 'AbortError') return;
          if (classifyFailure(failure).expired) throw failure;
          setError(`本輪檢驗尚未取得：${readableError(failure)}。其他來源仍繼續；資料包不沿用舊檢驗。`);
        }
      }
      if (!valid(current, generation)) return;
      setMessage('正在取得所選來源的全部可查清單及明細…');
      const result = await acquireHealthData({ snapshot, categories: currentPlan.categories, query: { from: '', to: '' }, mode: resume ? 'resume' : 'refresh',
        signal: current.signal, fetchReport: (category, reference, query, signal) => api.supplementary(category, query.from, query.to, reference, signal),
        classifyFailure, onUpdate: next => { if (valid(current, generation)) { snapshot = next; onPublish(next); } },
        onProgress: next => { if (valid(current, generation)) setProgress(next); } });
      if (!valid(current, generation)) return;
      if (result.expired) { onSessionExpired(); return; }
      snapshot = result.snapshot;
      protectPolling(snapshot);
      const initial = capturePreparations(snapshot, currentPlan.categories, tasksRef.current, captured.current);
      publishTasks(initial.tasks); captured.current = initial.captured;
      if (currentPlan.requestPreparation) {
        setProgress(null); setMessage('正在依本次要求，逐項送出官方提供的原始檔產製操作…');
        const submitted = await submitPreparations({ tasks: tasksRef.current, signal: current.signal,
          submit: (task, signal) => api.prepareDocument(task.category, task.id, signal), classifyFailure,
          onUpdate: next => { if (lifecycle.current === generation && controller.current === current) publishTasks(next); } });
        if (!valid(current, generation)) return;
        publishTasks(submitted);
      }
      const targets = currentPlan.requestPreparation ? preparationRefreshTargets(tasksRef.current) : [];
      if (targets.some(target => target.category === 'official_files') && !currentPlan.categories.includes('official_files')) {
        currentPlan = { ...currentPlan, categories: [...currentPlan.categories, 'official_files'] };
        planRef.current = currentPlan; setPlan(currentPlan);
      }
      for (let round = 0; round < (targets.length ? MAX_PREPARATION_POLL_ROUNDS : 0); round++) {
        const pending = nextPreparationPolls(targets, pollAttempts.current);
        if (!pending.length) break;
        if (round > 0) await waitForAcquisition(current.signal, PREPARATION_POLL_DELAY_MS);
        if (!valid(current, generation)) return;
        for (const target of pending) {
          if (!valid(current, generation)) return;
          const key = preparationRefreshKey(target), attempt = (pollAttempts.current.get(key) ?? 0) + 1;
          setMessage(`更新官方產製狀態 ${attempt}／${MAX_PREPARATION_POLL_ROUNDS}。官方未提供工作關聯的要求仍標示未確認，不按時間或檔名猜測。`);
          try {
            if (target.detail) {
              const range = snapshot.states[target.category]?.query ?? snapshot.query;
              const detail = await api.supplementary(target.category, range.from, range.to, target.detail, current.signal);
              if (!valid(current, generation)) return;
              snapshot = replaceDetailReport(snapshot, target.category, target.detail, detail); onPublish(snapshot);
              protectPolling(snapshot, target.category, target.detail);
              publishTasks(tasksRef.current.map(task => task.state === 'requested' && task.refresh?.category === target.category && task.refresh.detail === target.detail
                ? { ...task, state: 'source_refreshed' } : task));
            } else {
              const refreshed = await acquireHealthData({ snapshot, categories: [target.category], query: { from: '', to: '' }, mode: 'refresh',
                signal: current.signal, fetchReport: (category, reference, query, signal) => api.supplementary(category, query.from, query.to, reference, signal),
                classifyFailure, onUpdate: next => { if (valid(current, generation)) { snapshot = next; onPublish(next); } },
                onProgress: next => { if (valid(current, generation)) setProgress(next); } });
              if (!valid(current, generation)) return;
              if (refreshed.expired) { onSessionExpired(); return; }
              snapshot = refreshed.snapshot;
              protectPolling(snapshot, target.category);
            }
          } catch (failure) {
            if (!valid(current, generation)) return;
            if (classifyFailure(failure).expired) throw failure;
            setError(`部分產製狀態尚未更新：${readableError(failure)}。保留先前成功內容，不自動重送產製。`);
          }
          pollAttempts.current.set(key, attempt);
        }
      }
      if (!valid(current, generation)) return;
      setProgress(null);
      await makeBundle(currentPlan, snapshot, targets.length > 0, current, generation);
    } catch (failure) {
      if (lifecycle.current !== generation || controller.current !== current) return;
      if (classifyFailure(failure).expired) { onSessionExpired(); return; }
      if (!current.signal.aborted) setError(readableError(failure));
    } finally {
      clearTimeout(deadline);
      if (lifecycle.current === generation && controller.current === current) {
        if (current.signal.aborted) {
          onPublish({ ...snapshot, states: Object.fromEntries(Object.entries(snapshot.states).map(([category, state]) =>
            [category, state.status === 'loading' ? { ...state, status: 'cancelled' } : state])) });
          setMessage('本輪已停止；成功來源仍保留。已送出或結果不明的產製不會自動重送，可接續未完成項目。');
        }
        controller.current = null; markBusy(false);
      }
    }
  }

  function start() {
    if (disabled || controller.current) return;
    if (selectedCategories.length === 0 && (!includeLaboratory || !fetchLaboratory)) { setError('請至少選擇一個資料來源或檢驗結果。'); return; }
    const next: Plan = { id: planID(), categories: [...selectedCategories], includeLaboratory: Boolean(includeLaboratory && fetchLaboratory),
      requestPreparation, scope: selectedCategories.length === catalog.length && includeLaboratory ? 'all_available' : 'selected', startedAt: new Date().toISOString() };
    planRef.current = next; setPlan(next); collectionID.current = next.id; captured.current = new Set(); polledRoots.current.clear(); pollAttempts.current.clear(); laboratory.current = null;
    publishTasks([]); setPart(1); setSavedParts([]); setSavedOriginals({}); onBegin?.(next.categories, next.includeLaboratory);
    onActiveChange(true); void acquire(next, false);
  }

  async function prepareCurrentPart(index = part) {
    const currentPlan = planRef.current, currentBundle = bundleRef.current;
    if (!currentPlan || !currentBundle || disabled || controller.current || externalRequired && !externalSaved) return;
    const current = new AbortController(), generation = lifecycle.current; controller.current = current; markBusy(true); setError('');
    const deadline = setTimeout(() => current.abort(), 60_000);
    try { await preparePart(currentBundle, currentPlan, index, externalRequired, current, generation); }
    catch (failure) { if (valid(current, generation)) { if (classifyFailure(failure).expired) onSessionExpired(); else setError(readableError(failure)); } }
    finally { clearTimeout(deadline); if (controller.current === current) { controller.current = null; markBusy(false); } }
  }

  async function watchDownload() {
    const currentPackage = packageRef.current;
    if (!currentPackage || controller.current) return;
    const current = new AbortController(), generation = lifecycle.current; controller.current = current; markBusy(true); setError(''); setSavedAcknowledged(false);
    const started = Date.now();
    try {
      while (Date.now() - started <= 31 * 60_000) {
        await waitForAcquisition(current.signal, 2000);
        const observed = await api.acquisitionPackageStatus({ id: currentPackage.id }, current.signal);
        if (!valid(current, generation)) return;
        assertPackageReferences(observed, bundleRef.current?.groups[part - 1] ?? []);
        setReceipt(observed);
        if (observed.state === 'ready') {
          setMessage('尚未觀察到下載開始；若瀏覽器阻擋下載，請再次點選下載連結。');
          if (Date.now() - started >= 10_000) break;
          continue;
        }
        if (observed.state === 'streaming') { setMessage(`原始檔串流中：完成 ${observed.completed}／${observed.total}，失敗 ${observed.failed}。`); continue; }
        setMessage(observed.package_sha256
          ? observed.state === 'completed' ? '伺服器已完成原始檔包傳輸；請確認瀏覽器檔案已保存，再勾選確認。'
            : '原始檔包已結束，但包含未成功檔案。manifest.json 逐檔列出結果；請確認保存後再處理下一部分。'
          : '這次傳輸沒有形成可核對的完整 ZIP，不能當作已保存；可重新下載尚未保存的原始檔，不會重送產製。');
        break;
      }
    } catch (failure) {
      if (valid(current, generation)) { if (classifyFailure(failure).expired) onSessionExpired(); else setError(`下載狀態未確認：${readableError(failure)}。請勿將未確認檔案當作完整備份。`); }
    } finally { if (controller.current === current) { controller.current = null; markBusy(false); } }
  }

  async function stop() {
    const generation = lifecycle.current;
    controller.current?.abort();
    const currentPackage = packageRef.current;
    if (currentPackage) {
      const current = new AbortController(); controller.current = current; markBusy(true);
      const deadline = setTimeout(() => current.abort(), 15_000);
      try {
        try { await api.cancelAcquisitionPackage(currentPackage.id, current.signal); }
        catch (failure) { if (!(failure instanceof ApiError && failure.code === 'package_unavailable')) throw failure; }
        if (valid(current, generation)) {
          rememberPackage({ ...currentPackage, state: 'cancelled' });
          setReceipt(previous => {
            if (!previous) return null;
            const cancelled = { ...previous, state: 'cancelled' as const }; delete cancelled.package_sha256; return cancelled;
          });
          setMessage('已要求停止原始檔包。未完成傳輸不能當作完整檔案；可重新下載尚未保存的原始檔。');
        }
      } catch (failure) {
        if (valid(current, generation)) { if (classifyFailure(failure).expired) onSessionExpired(); else setError(`停止結果未確認：${readableError(failure)}`); }
      } finally { clearTimeout(deadline); if (controller.current === current) { controller.current = null; markBusy(false); } }
    }
  }

  async function confirmPart() {
    if (!plan || !bundle || controller.current || !receipt?.package_sha256 || receipt.package_bytes === undefined || !savedAcknowledged || externalRequired && !externalSaved) return;
    const current = new AbortController(), generation = lifecycle.current; controller.current = current; markBusy(true); setError('');
    const deadline = setTimeout(() => current.abort(), 60_000);
    try {
      try { await api.cancelAcquisitionPackage(receipt.id, current.signal); }
      catch (failure) { if (classifyFailure(failure).expired) throw failure; }
      if (!valid(current, generation)) return;
      const name = partName(plan.id, part);
      setSavedParts([...savedParts.filter(item => item.plan_id !== plan.id || item.index !== part), {
        plan_id: plan.id, index: part, name, sha256: receipt.package_sha256, size: receipt.package_bytes,
        state: receipt.state, completed: receipt.completed, failed: receipt.failed, total: receipt.total,
        dataset_included: !externalRequired, saved_acknowledged: true,
      }]);
      const originals = recordSavedOriginals(savedOriginals, receipt.files, name);
      setSavedOriginals(originals);
      rememberPackage(null); setReceipt(null); setSavedAcknowledged(false);
      if (part < bundle.groups.length) { setPart(part + 1); await preparePart(bundle, plan, part + 1, externalRequired, current, generation); }
      else {
        const outstanding = remainingOriginals(bundle.projection.files, originals).length;
        setMessage(outstanding ? `本輪各部分已確認保存，仍有 ${outstanding} 個原始檔未成功取得。可只重試這些檔案；已保存成功的附件與官方產製都不會重做。`
          : '目前已知的可下載原始檔均已確認保存。請連同整組索引保留；來源範圍未知及產製未確認狀態仍有效。');
      }
    } catch (failure) {
      if (valid(current, generation)) { if (classifyFailure(failure).expired) onSessionExpired(); else setError(readableError(failure)); }
    } finally { clearTimeout(deadline); if (controller.current === current) { controller.current = null; markBusy(false); } }
  }

  async function retryOriginalFiles() {
    const currentPlan = planRef.current, currentBundle = bundleRef.current;
    if (!currentPlan || !currentBundle || disabled || controller.current || receipt?.package_sha256) return;
    const files = retryableOriginals(currentBundle.projection.files, savedOriginals);
    if (!files.length && currentBundle.projection.files.length) return;
    const current = new AbortController(), generation = lifecycle.current;
    controller.current = current; markBusy(true); setError('');
    const deadline = setTimeout(() => current.abort(), 60_000);
    try {
      if (packageRef.current) {
        try { await api.cancelAcquisitionPackage(packageRef.current.id, current.signal); }
        catch (failure) {
          if (!(failure instanceof ApiError && failure.code === 'package_unavailable')) throw failure;
        }
        if (!valid(current, generation)) return;
      }
      const nextPlan = { ...currentPlan, id: planID() };
      const nextBundle = { ...currentBundle, groups: packageFileGroups(files) };
      planRef.current = nextPlan; setPlan(nextPlan); bundleRef.current = nextBundle; setBundle(nextBundle);
      rememberPackage(null); setReceipt(null); setPart(1); setSavedAcknowledged(false);
      // The original projection, archive IDs and dataset byte hash stay fixed.
      // This path performs package requests only; it never prepares a source.
      if (!externalRequired || externalSaved) await preparePart(nextBundle, nextPlan, 1, externalRequired, current, generation);
    } catch (failure) {
      if (valid(current, generation)) { if (classifyFailure(failure).expired) onSessionExpired(); else setError(readableError(failure)); }
    } finally { clearTimeout(deadline); if (controller.current === current) { controller.current = null; markBusy(false); } }
  }

  async function releaseForSourceResume() {
    if (!packageRef.current || savedParts.length || controller.current || disabled) return;
    const current = new AbortController(), generation = lifecycle.current;
    controller.current = current; markBusy(true); setError('');
    const deadline = setTimeout(() => current.abort(), 15_000);
    try {
      try { await api.cancelAcquisitionPackage(packageRef.current.id, current.signal); }
      catch (failure) { if (!(failure instanceof ApiError && failure.code === 'package_unavailable')) throw failure; }
      if (!valid(current, generation)) return;
      rememberPackage(null); setReceipt(null); setSavedAcknowledged(false);
      const currentPlan = planRef.current;
      if (currentPlan) { const next = { ...currentPlan, id: planID() }; planRef.current = next; setPlan(next); }
      setMessage('目前下載包已解除。可接續未取得來源及更新產製狀態；既有產製要求不會自動重送。');
    } catch (failure) {
      if (valid(current, generation)) { if (classifyFailure(failure).expired) onSessionExpired(); else setError(readableError(failure)); }
    } finally { clearTimeout(deadline); if (controller.current === current) { controller.current = null; markBusy(false); } }
  }

  function saveIndex() {
    if (!plan || !bundle) return;
    const currentParts = savedParts.filter(item => item.plan_id === plan.id);
    const content = { schema: 'healthbank.acquisition.collection.v1', collection_id: collectionID.current, exported_at: new Date().toISOString(),
      dataset: { ...bundle.dataset, included_in_every_part: savedParts.every(item => item.dataset_included),
        external_saved_acknowledged: externalRequired && externalSaved },
      acquisition: bundle.acquisition, current_plan: { id: plan.id, expected_parts: bundle.groups.length }, parts: savedParts,
      original_files_total: bundle.projection.files.length,
      originals: bundle.projection.files.map(file => ({ archive_id: file.archive_id, category: file.category,
        ...(savedOriginals[file.archive_id] ?? { state: 'queued', bytes: 0, failures: 0 }) })),
      all_parts_saved_acknowledged: currentParts.length === bundle.groups.length,
      all_known_originals_saved: remainingOriginals(bundle.projection.files, savedOriginals).length === 0 && currentParts.length === bundle.groups.length,
      all_required_files_saved_acknowledged: remainingOriginals(bundle.projection.files, savedOriginals).length === 0 &&
        currentParts.length === bundle.groups.length && (!externalRequired || externalSaved),
      limitation: '保存確認由使用者提供；官方未提供獨立總數或產製工作關聯的項目仍未確認。外部資料 JSON 需與全部 ZIP 一起保存。' };
    downloadText(`${JSON.stringify(content, null, 2)}\n`, `healthbank-acquisition-${collectionID.current}-index.json`, 'application/json;charset=utf-8');
  }

  async function finish() {
    if (controller.current) return;
    const current = new AbortController(), generation = lifecycle.current; controller.current = current; markBusy(true);
    const deadline = setTimeout(() => current.abort(), 15_000);
    try {
      if (packageRef.current) await api.cancelAcquisitionPackage(packageRef.current.id, current.signal);
    } catch (failure) {
      if (generation !== lifecycle.current) return;
      if (classifyFailure(failure).expired) { onSessionExpired(); return; }
    } finally {
      clearTimeout(deadline);
      if (generation === lifecycle.current && controller.current === current) {
        lifecycle.current += 1; controller.current = null; planRef.current = null; setPlan(null); bundleRef.current = null; setBundle(null);
        publishTasks([]); captured.current.clear(); polledRoots.current.clear(); pollAttempts.current.clear(); laboratory.current = null;
        rememberPackage(null); setReceipt(null); setSavedParts([]); setSavedOriginals({}); collectionID.current = '';
        markBusy(false); onActiveChange(false); setMessage('完整取得計畫已結束。已取得來源留在工作區；下載檔案由您保存。');
      }
    }
  }

  const remaining = plan ? plan.categories.reduce((sum, category) => sum + categoryCoverage(getSnapshot(), category).pendingCount, 0) : 0;
  const taskPages = Math.max(1, Math.ceil(tasks.length / 50)), visibleTaskPage = Math.min(taskPage, taskPages - 1);
  const currentSavedCount = plan ? savedParts.filter(item => item.plan_id === plan.id).length : 0;
  const allSaved = bundle !== null && currentSavedCount === bundle.groups.length;
  const outstandingOriginals = bundle ? remainingOriginals(bundle.projection.files, savedOriginals).length : 0;
  const retryableCount = bundle ? retryableOriginals(bundle.projection.files, savedOriginals).length : 0;
  const downloadDisabled = busy || disabled || !packageInfo || packageInfo.state !== 'ready' || externalRequired && !externalSaved ||
    receipt?.state !== undefined && receipt.state !== 'ready';
  return <section className="health-data-full-acquisition" aria-label="完整取得與原始檔保存" aria-busy={busy}>
    <h3>完整取得與原始檔保存</h3>
    {!plan ? <>
      <p>取得所選來源的全部可查清單、巢狀明細及檢驗，將可下載的原始檔集中成 ZIP。來源的實際期間、未取得及失敗項目會保留在索引中。</p>
      <label className="checkbox-label"><input type="checkbox" checked={requestPreparation} disabled={disabled} onChange={event => setRequestPreparation(event.target.checked)} />一併要求官方產製尚未就緒的 JPEG、DICOM 與原始匯出檔</label>
      <p className="helper-text">按下開始即要求本次所選的產製操作，每項送出一次；結果不明時保留待確認狀態。可展開來源選項調整範圍。</p>
      <button type="button" className="button primary" disabled={disabled} onClick={start}>完整取得並保存原始檔</button>
    </> : <>
      <p>{plan.categories.length} 個來源{plan.includeLaboratory ? '及檢驗結果' : ''} · {tasks.length} 項來源產製操作 · {remaining} 筆明細待取得。</p>
      {progress && <p role="status">{progress.phase === 'lists' ? '來源清單' : '逐筆明細'} {progress.done}／{progress.total} · {catalog.find(item => item.id === progress.category)?.label ?? progress.category}</p>}
      <div className="health-data-actions">
        {busy ? <button type="button" className="button secondary" onClick={() => { void stop(); }}>停止本輪取得／傳輸</button>
          : <>{!packageInfo && !savedParts.length && <button type="button" className="button secondary" disabled={disabled} onClick={() => { void acquire(plan, true); }}>接續未完成項目與更新產製狀態</button>}
            <button type="button" className="button quiet" onClick={() => { void finish(); }}>結束完整取得計畫</button></>}
      </div>
      {tasks.length > 0 && <details><summary>逐項產製狀態（{tasks.length}）</summary>
        <p className="helper-text">已受理與檔案可下載不同。更新來源後只收錄實際可讀取檔案，不以新增檔名、時間或筆數推斷哪一項產製已完成。產製只送出初次取得的操作；更新後的操作仍可在來源紀錄查看，不會當成新要求重送。</p>
        <ol start={visibleTaskPage * 50 + 1}>{tasks.slice(visibleTaskPage * 50, (visibleTaskPage + 1) * 50).map(task => <li key={`${task.category}:${task.id}`}>
          {catalog.find(item => item.id === task.category)?.label ?? task.category} · {task.title} · {task.name}：{plan.requestPreparation ? PREPARATION_LABELS[task.state] : '本次未要求產製'}{task.message ? `；${task.message}` : ''}
        </li>)}</ol>
        {taskPages > 1 && <nav className="health-data-pagination" aria-label="產製狀態分頁"><button type="button" className="button secondary" disabled={visibleTaskPage === 0} onClick={() => setTaskPage(visibleTaskPage - 1)}>上一頁</button><span>{visibleTaskPage + 1}／{taskPages}</span><button type="button" className="button secondary" disabled={visibleTaskPage + 1 >= taskPages} onClick={() => setTaskPage(visibleTaskPage + 1)}>下一頁</button></nav>}
      </details>}
      {bundle && <div className="health-data-package">
        <h4>保存原始檔包 · {bundle.projection.files.length} 個已知附件 · 本輪 {bundle.groups.length} 部分</h4>
        <details><summary>檔案內容與取得限制</summary><p className="helper-text">每部分最多 1,000 個附件、8 GiB 原始內容及 30 分鐘；超過或失敗均寫入 manifest.json。ZIP 內含資料 JSON、逐檔 SHA-256 與實際結果。原始附件逐檔串流下載。</p></details>
        {externalRequired && <>
          <p>完整資料 JSON 將獨立保存（{Math.ceil(bundle.dataset.size / 1024 / 1024)} MiB），每個 ZIP 明確記錄其 SHA-256，不會宣稱已內含。</p>
          <button type="button" className="button secondary" disabled={busy} onClick={() => { downloadText(bundle.projection.content, bundle.dataset.name, 'application/json;charset=utf-8'); setExternalDownloaded(true); }}>下載完整資料 JSON</button>
          <label className="checkbox-label"><input type="checkbox" disabled={!externalDownloaded || busy} checked={externalSaved} onChange={event => setExternalSaved(event.target.checked)} />我已確認完整資料 JSON 已保存，會與原始檔包一起保留</label>
        </>}
        {!packageInfo && !allSaved && <button type="button" className="button secondary" disabled={disabled || busy || externalRequired && !externalSaved} onClick={() => { void prepareCurrentPart(); }}>準備第 {part} 部分原始檔包</button>}
        {packageInfo && <>
          <a className={`button primary${downloadDisabled ? ' disabled' : ''}`} href={`/api/acquisition/packages/download?${new URLSearchParams({ id: packageInfo.id })}`}
            download={partName(plan.id, part)} aria-disabled={downloadDisabled}
            onClick={event => { if (controller.current || downloadDisabled) event.preventDefault(); else void watchDownload(); }}>下載第 {part}／{bundle.groups.length} 部分 ZIP</a>
          <p className="helper-text">原始下載由瀏覽器管理。伺服器傳輸完成不等於裝置已保存；請查看瀏覽器的下載結果後確認。資料包還原可使用 ZIP 中的 dataset.v2.json，或另存的完整資料 JSON。</p>
          {receipt && <p role="status">檔案結果：{receipt.completed} 成功、{receipt.failed} 失敗／共 {receipt.total}；{Math.ceil(receipt.bytes / 1024 / 1024)} MiB 已傳輸。</p>}
          {!busy && !receipt?.package_sha256 && <button type="button" className="button secondary" onClick={() => { void watchDownload(); }}>重新核對下載狀態</button>}
          <label className="checkbox-label"><input type="checkbox" checked={savedAcknowledged} disabled={busy || !receipt?.package_sha256} onChange={event => setSavedAcknowledged(event.target.checked)} />我已確認第 {part} 部分 ZIP 已保存</label>
          <button type="button" className="button secondary" disabled={busy || !receipt?.package_sha256 || !savedAcknowledged || externalRequired && !externalSaved} onClick={() => { void confirmPart(); }}>{part < bundle.groups.length ? '確認保存並準備下一部分' : '確認最後一部分已保存'}</button>
          {!savedParts.length && <button type="button" className="button quiet" disabled={busy} onClick={() => { void releaseForSourceResume(); }}>解除目前下載包以接續來源</button>}
        </>}
        {(allSaved && outstandingOriginals > 0 || packageInfo?.state === 'cancelled' || !receipt?.package_sha256 && (receipt?.state === 'failed' || packageInfo && error)) && <>
          <p>尚有 {outstandingOriginals} 個已知原始檔未成功保存；只重試未完成項目，已保存成功的檔案不再下載。每個檔案失敗 {MAX_ORIGINAL_FAILURES} 次後，本計畫保留失敗狀態並停止重試；容量／時間上限跳過的項目可接續。</p>
          <button type="button" className="button secondary" disabled={busy || disabled || !retryableCount && bundle.projection.files.length > 0} onClick={() => { void retryOriginalFiles(); }}>重新下載未保存的原始檔（不重送產製）</button>
        </>}
        {savedParts.length > 0 && <button type="button" className="button secondary" disabled={busy} onClick={saveIndex}>下載整組檔案索引與校驗碼（已確認 {savedParts.length} 個 ZIP）</button>}
      </div>}
    </>}
    {message && <p role="status">{message}</p>}
    {error && <p className="error-text" role="alert">{error}</p>}
  </section>;
}
