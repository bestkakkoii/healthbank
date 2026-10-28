// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import type { SupplementReport } from './api';
import { emptyHealthData, healthDataEntryCount, HealthDataLimitError, MAX_HEALTH_DATA_ENTRIES,
  pendingDetailReferences, validAcquisitionQuery, type HealthDataSnapshot } from './health-data.ts';

export const MAX_DETAIL_REQUESTS_PER_RUN = 50_000;

export type AcquisitionMode = 'refresh' | 'resume' | 'details';
export interface AcquisitionFailure { message: string; expired: boolean; retryable: boolean }
export interface AcquisitionProgress {
  phase: 'lists' | 'details';
  category: string;
  done: number;
  total: number;
  failed: number;
}
export interface AcquisitionOptions {
  snapshot: HealthDataSnapshot;
  categories: string[];
  query: HealthDataSnapshot['query'];
  mode: AcquisitionMode;
  signal: AbortSignal;
  fetchReport: (category: string, reference: string, query: HealthDataSnapshot['query'], signal: AbortSignal) => Promise<SupplementReport>;
  classifyFailure: (failure: unknown) => AcquisitionFailure;
  onUpdate: (snapshot: HealthDataSnapshot) => void;
  onProgress: (progress: AcquisitionProgress) => void;
  // Lower limits support constrained integrations and focused boundary tests;
  // callers cannot raise the application ceilings.
  limits?: { entries: number; detailRequests: number };
}

function aborted(signal: AbortSignal): boolean { return signal.aborted; }

async function retryDelay(signal: AbortSignal): Promise<void> {
  if (aborted(signal)) return;
  await new Promise<void>(resolve => {
    const finish = () => { clearTimeout(timer); signal.removeEventListener('abort', finish); resolve(); };
    const timer = setTimeout(finish, 500);
    signal.addEventListener('abort', finish, { once: true });
  });
}

// Lists are resolved first, then every returned detail reference, one request at
// a time. Cancellation retains only completed requests; a later resume skips
// successes. One bounded retry applies only to classified transient failures.
export async function acquireHealthData(options: AcquisitionOptions): Promise<{ snapshot: HealthDataSnapshot; expired: boolean }> {
  if (!validAcquisitionQuery(options.query)) throw new Error('請填寫完整且有效的查詢起迄日期。');
  const { signal, fetchReport, classifyFailure, onUpdate, onProgress } = options;
  const categories = [...new Set(options.categories)];
  let snapshot: HealthDataSnapshot = { ...options.snapshot, query: { ...options.query } };
  let entryCount = healthDataEntryCount(snapshot);
  const entryLimit = Math.min(MAX_HEALTH_DATA_ENTRIES, options.limits?.entries ?? MAX_HEALTH_DATA_ENTRIES);
  const requestLimit = Math.min(MAX_DETAIL_REQUESTS_PER_RUN, options.limits?.detailRequests ?? MAX_DETAIL_REQUESTS_PER_RUN);
  const publish = () => { if (!aborted(signal)) onUpdate(snapshot); };
  async function fetchOne(category: string, reference: string, query: HealthDataSnapshot['query']) {
    try { return await fetchReport(category, reference, query, signal); }
    catch (failure) {
      if (aborted(signal) || !classifyFailure(failure).retryable) throw failure;
      await retryDelay(signal);
      if (aborted(signal)) throw failure;
      return fetchReport(category, reference, query, signal);
    }
  }
  const listCategories = options.mode === 'details' ? [] : categories.filter(category =>
    options.mode === 'refresh' || !snapshot.reports[category] ||
    ['failed', 'cancelled', 'loading'].includes(snapshot.states[category]?.status ?? 'unqueried'));
  let failed = 0;
  for (const [index, category] of listCategories.entries()) {
    if (aborted(signal)) return { snapshot, expired: false };
    const query = options.mode === 'resume' ? snapshot.states[category]?.query ?? options.query : options.query;
    snapshot = { ...snapshot, states: { ...snapshot.states,
      [category]: { status: 'loading', detail_errors: snapshot.states[category]?.detail_errors ?? {}, query } } };
    publish(); onProgress({ phase: 'lists', category, done: index, total: listCategories.length, failed });
    try {
      const result = await fetchOne(category, '', query);
      if (aborted(signal)) return { snapshot, expired: false };
      const previousCount = (snapshot.reports[category]?.entries.length ?? 0) +
        Object.values(snapshot.details[category] ?? {}).reduce((sum, detail) => sum + detail.entries.length, 0);
      const nextCount = entryCount - previousCount + result.entries.length;
      if (nextCount > entryLimit) throw new HealthDataLimitError(`本次已取得紀錄達 ${entryLimit.toLocaleString('zh-TW')} 筆容量上限，未加入不完整清單。請先保存資料包，再縮小查詢期間。`);
      entryCount = nextCount;
      snapshot = { ...snapshot, reports: { ...snapshot.reports, [category]: result },
        details: { ...snapshot.details, [category]: {} },
        states: { ...snapshot.states, [category]: { status: 'ready', detail_errors: {}, query } } };
    } catch (failure) {
      if (aborted(signal)) return { snapshot, expired: false };
      const error = classifyFailure(failure);
      if (error.expired) { snapshot = emptyHealthData(); publish(); return { snapshot, expired: true }; }
      failed++;
      snapshot = { ...snapshot, states: { ...snapshot.states,
        [category]: { status: 'failed', error: error.message, detail_errors: snapshot.states[category]?.detail_errors ?? {}, query } } };
      if (failure instanceof HealthDataLimitError) { publish(); return { snapshot, expired: false }; }
    }
    publish(); onProgress({ phase: 'lists', category, done: index + 1, total: listCategories.length, failed });
  }
  const queue = categories.flatMap(category => snapshot.states[category]?.status === 'failed' ? [] :
    pendingDetailReferences(snapshot, category).map(reference => ({ category, reference })));
  const queued = new Set(queue.map(task => `${task.category}:${task.reference}`));
  failed = 0;
  for (const [index, task] of queue.entries()) {
    if (aborted(signal)) return { snapshot, expired: false };
    const { category, reference } = task;
    if (index >= requestLimit) {
      snapshot = { ...snapshot, states: { ...snapshot.states, [category]: {
        ...snapshot.states[category], status: 'ready',
        detail_errors: { ...snapshot.states[category]?.detail_errors,
          [reference]: `本輪已達 ${requestLimit.toLocaleString('zh-TW')} 筆明細請求上限，尚未取得的內容仍明列；可選擇接續。` },
      } } };
      publish(); break;
    }
    onProgress({ phase: 'details', category, done: index, total: queue.length, failed });
    try {
      const query = snapshot.states[category]?.query ?? options.query;
      const result = await fetchOne(category, reference, query);
      if (aborted(signal)) return { snapshot, expired: false };
      if (entryCount + result.entries.length > entryLimit)
        throw new HealthDataLimitError(`本次已取得紀錄達 ${entryLimit.toLocaleString('zh-TW')} 筆容量上限，未加入不完整明細。請先保存資料包，再縮小查詢期間。`);
      entryCount += result.entries.length;
      const errors = Object.fromEntries(Object.entries(snapshot.states[category]?.detail_errors ?? {})
        .filter(([key]) => key !== reference));
      snapshot = { ...snapshot, details: { ...snapshot.details,
        [category]: { ...snapshot.details[category], [reference]: result } },
        states: { ...snapshot.states, [category]: { status: 'ready', detail_errors: errors, query } } };
      for (const nested of result.entries.flatMap(entry => entry.detail_id ? [entry.detail_id] : [])) {
        const key = `${category}:${nested}`;
        if (!queued.has(key) && !Object.hasOwn(snapshot.details[category] ?? {}, nested)) {
          queued.add(key); queue.push({ category, reference: nested });
        }
      }
    } catch (failure) {
      if (aborted(signal)) return { snapshot, expired: false };
      const error = classifyFailure(failure);
      if (error.expired) { snapshot = emptyHealthData(); publish(); return { snapshot, expired: true }; }
      failed++;
      snapshot = { ...snapshot, states: { ...snapshot.states, [category]: {
        status: snapshot.states[category]?.status ?? 'ready',
        detail_errors: { ...snapshot.states[category]?.detail_errors, [reference]: error.message },
        query: snapshot.states[category]?.query ?? options.query,
      } } };
      if (failure instanceof HealthDataLimitError) { publish(); return { snapshot, expired: false }; }
    }
    publish(); onProgress({ phase: 'details', category, done: index + 1, total: queue.length, failed });
  }
  return { snapshot, expired: false };
}
