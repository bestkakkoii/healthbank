// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import type { DocumentAction, ReportResponse } from './api';
import type { AcquisitionFailure } from './data-acquisition';
import { categoryCoverage, emptyHealthData, type HealthDataSnapshot } from './health-data.ts';
import type { DatasetPackageFile } from './health-dataset';

export const MAX_PREPARATION_TASKS = 50_000;
export const MAX_PACKAGE_FILES = 1_000;
export const MAX_ORIGINAL_FAILURES = 3;
export const MAX_EMBEDDED_PACKAGE_DATASET = 32 * 1024 * 1024;
export const MAX_PREPARATION_POLL_ROUNDS = 6;
export const PREPARATION_POLL_DELAY_MS = 10_000;

export type PreparationState = 'queued' | 'requested' | 'indeterminate' | 'source_refreshed' | 'correlation_unavailable' | 'failed';
export interface PreparationRefresh {
  category: string;
  detail?: string;
  correlation: 'same_record' | 'unavailable';
}
export interface PreparationResult { status: 'requested'; message: string; refresh: PreparationRefresh }
export interface PreparationTask {
  category: string;
  id: string;
  name: string;
  title: string;
  parent: string;
  kind: DocumentAction['kind'];
  state: PreparationState;
  message?: string;
  refresh?: PreparationRefresh;
}
export type AcquisitionStatusState = 'ready' | 'partial' | 'failed' | 'unqueried' | 'cancelled' | 'not_requested' | 'queued' |
  'requested' | 'indeterminate' | 'source_refreshed' | 'correlation_unavailable' | 'excluded';
export type AcquisitionStatusCode = 'source_total_unknown' | 'pending_details' | 'preparation_outcome_unknown' |
  'source_did_not_provide_job_correlation' | 'poll_limit' | 'request_failed' | 'scope_excluded' | 'budget_exceeded' | 'auth_expired';
export interface AcquisitionStatusRecord {
  category: string;
  kind: 'source' | 'prepare' | 'document';
  state: AcquisitionStatusState;
  count: number;
  code?: AcquisitionStatusCode;
}
export interface AcquisitionSummary {
  scope: 'all_available' | 'selected';
  started_at: string;
  finished_at: string;
  statuses: AcquisitionStatusRecord[];
}
export interface PackagePlan { id: string; index: number; total: number; files_total: number }
export interface ExternalDataset { name: 'healthbank.dataset.v2.json'; size: number; sha256: string }
export type PackageState = 'ready' | 'streaming' | 'completed' | 'partial' | 'failed' | 'cancelled';
export interface PackageFileStatus {
  archive_id: string;
  state: 'queued' | 'streaming' | 'completed' | 'failed' | 'cancelled' | 'skipped';
  bytes: number;
  sha256?: string;
  code?: string;
}
export interface SavedOriginal extends PackageFileStatus {
  part_name: string;
  failures: number;
}
export type SavedOriginals = Record<string, SavedOriginal>;
export interface PackageReceipt {
  id: string;
  state: PackageState;
  total: number;
  completed: number;
  failed: number;
  bytes: number;
  files: PackageFileStatus[];
  expires_at: string;
  package_sha256?: string;
  package_bytes?: number;
}
export type PackageCreated = Pick<PackageReceipt, 'id' | 'state' | 'expires_at'>;
export interface PackageRequest {
  dataset?: string;
  external_dataset?: ExternalDataset;
  files: DatasetPackageFile[];
  acquisition: AcquisitionSummary;
  plan: PackagePlan;
}

// Capture only the initial source-offered actions. Later polling rotates opaque
// handles; discovering a new handle is never permission to replay preparation.
export function capturePreparations(snapshot: HealthDataSnapshot, categories: string[], previous: PreparationTask[],
  capturedReports: ReadonlySet<string>): { tasks: PreparationTask[]; captured: Set<string> } {
  const result = [...previous], captured = new Set(capturedReports);
  const seen = new Set(previous.map(task => JSON.stringify([task.category, task.id])));
  for (const category of [...new Set(categories)]) {
    if (snapshot.states[category]?.status !== 'ready') continue;
    const reports = [[undefined, snapshot.reports[category]], ...Object.entries(snapshot.details[category] ?? {})] as const;
    for (const [parent, report] of reports) {
      if (!report) continue;
      const reportKey = JSON.stringify([category, parent ?? '']);
      if (captured.has(reportKey)) continue;
      captured.add(reportKey);
      for (const entry of report.entries) for (const action of entry.document_actions ?? []) {
        const key = JSON.stringify([category, action.id]);
        if (seen.has(key)) continue;
        if (!/^[a-f0-9]{48}$/.test(action.id)) throw new Error('原始檔操作不屬於目前官方工作階段，請重新取得來源。');
        seen.add(key);
        if (result.length >= MAX_PREPARATION_TASKS) throw new Error('本輪原始檔產製操作超過 50,000 筆，請縮小所選來源。');
        result.push({ category, id: action.id, name: action.name, title: entry.title, parent: parent ?? '', kind: action.kind, state: 'queued' });
      }
    }
  }
  return { tasks: result, captured };
}

export function preparationTasks(snapshot: HealthDataSnapshot, categories: string[]): PreparationTask[] {
  return capturePreparations(snapshot, categories, [], new Set()).tasks;
}

function isAborted(signal: AbortSignal): boolean { return signal.aborted; }

// A poll can replace opaque operation and child handles. Mark that explicit
// subtree as already observed, including unqueried children, so a later resume
// cannot mistake rotated handles for permission to repeat a source mutation.
export function protectPolledPreparations(snapshot: HealthDataSnapshot, category: string, parent: string,
  captured: Set<string>): Set<string> {
  // This is the caller-owned traversal ledger, not a medical data snapshot.
  // Update it in place to avoid copying 50,000 ledger entries for every poll.
  const visited = new Set<string>(), queue = [parent];
  for (let index = 0; index < queue.length; index++) {
    const reference = queue[index];
    if (reference === undefined) continue;
    if (visited.has(reference)) continue;
    visited.add(reference); captured.add(JSON.stringify([category, reference]));
    const report = reference ? snapshot.details[category]?.[reference] : snapshot.reports[category];
    for (const entry of report?.entries ?? []) if (entry.detail_id && !visited.has(entry.detail_id)) queue.push(entry.detail_id);
  }
  return captured;
}

export async function submitPreparations(options: {
  tasks: PreparationTask[];
  signal: AbortSignal;
  submit: (task: PreparationTask, signal: AbortSignal) => Promise<PreparationResult>;
  classifyFailure: (failure: unknown) => AcquisitionFailure;
  onUpdate: (tasks: PreparationTask[], done: number) => void;
}): Promise<PreparationTask[]> {
  let tasks = options.tasks.map(task => ({ ...task }));
  for (const [index, task] of tasks.entries()) {
    if (isAborted(options.signal)) break;
    if (task.state !== 'queued') continue;
    // Mark before invoking the mutation. If it is cancelled or its response is
    // lost, a resumed plan must not automatically submit this capability again.
    tasks = tasks.map((value, ordinal) => ordinal === index ? { ...value, state: 'indeterminate' } : value);
    options.onUpdate(tasks, index);
    try {
      const result = await options.submit(task, options.signal);
      if (isAborted(options.signal)) break;
      tasks = tasks.map((value, ordinal) => ordinal === index ? { ...value,
        state: result.refresh.correlation === 'unavailable' ? 'correlation_unavailable' : 'requested',
        message: result.message, refresh: result.refresh } : value);
    } catch (failure) {
      const classified = options.classifyFailure(failure);
      tasks = tasks.map((value, ordinal) => ordinal === index ? { ...value,
        state: 'indeterminate', message: isAborted(options.signal) ? '已停止等待；官方是否收到產製要求仍未確認，不會自動重送。' : classified.message } : value);
      options.onUpdate(tasks, index + 1);
      if (classified.expired) throw failure;
      if (isAborted(options.signal)) break;
      if (failure instanceof DOMException && failure.name === 'AbortError') throw failure;
    }
    options.onUpdate(tasks, index + 1);
  }
  return tasks;
}

export function preparationRefreshTargets(tasks: PreparationTask[]): PreparationRefresh[] {
  const targets = new Map<string, PreparationRefresh>();
  for (const task of tasks) {
    if (task.state === 'queued' || task.state === 'failed') continue;
    const target = task.refresh ?? (task.kind === 'prepare_jpeg' && task.parent
      ? { category: task.category, detail: task.parent, correlation: 'same_record' as const }
      : { category: 'official_files', correlation: 'unavailable' as const });
    targets.set(JSON.stringify([target.category, target.detail ?? '']), target);
  }
  return [...targets.values()];
}

export function preparationRefreshKey(target: PreparationRefresh): string {
  return JSON.stringify([target.category, target.detail ?? '']);
}

export function nextPreparationPolls(targets: PreparationRefresh[], attempts: ReadonlyMap<string, number>): PreparationRefresh[] {
  let minimum = MAX_PREPARATION_POLL_ROUNDS;
  for (const target of targets) minimum = Math.min(minimum, attempts.get(preparationRefreshKey(target)) ?? 0);
  if (minimum >= MAX_PREPARATION_POLL_ROUNDS) return [];
  return targets.filter(target => (attempts.get(preparationRefreshKey(target)) ?? 0) === minimum);
}

export function selectedSnapshot(snapshot: HealthDataSnapshot, categories: string[]): HealthDataSnapshot {
  const selected = new Set(categories);
  const choose = <T>(values: Record<string, T>) => Object.fromEntries(Object.entries(values).filter(([category]) => selected.has(category)));
  return { ...emptyHealthData(), reports: choose(snapshot.reports), details: choose(snapshot.details), states: choose(snapshot.states), query: { ...snapshot.query } };
}

export function acquisitionSummary(options: {
  snapshot: HealthDataSnapshot; categories: string[]; laboratory: ReportResponse | null;
  includeLaboratory: boolean; tasks: PreparationTask[]; requestPreparation: boolean;
  scope: AcquisitionSummary['scope']; startedAt: string; finishedAt: string; pollingLimited: boolean;
}): AcquisitionSummary {
  const grouped = new Map<string, AcquisitionStatusRecord>();
  const add = (record: AcquisitionStatusRecord) => {
    const key = JSON.stringify([record.category, record.kind, record.state, record.code ?? '']);
    const previous = grouped.get(key);
    grouped.set(key, { ...record, count: (previous?.count ?? 0) + record.count });
  };
  for (const category of [...new Set(options.categories)]) {
    const source = options.snapshot.reports[category], state = options.snapshot.states[category];
    const coverage = categoryCoverage(options.snapshot, category);
    if (!source || state?.status !== 'ready') add({ category, kind: 'source', state: state?.status === 'cancelled' ? 'cancelled' : source ? 'partial' : state?.status === 'failed' ? 'failed' : 'unqueried', count: 1,
      ...(state?.status === 'failed' ? { code: 'request_failed' as const } : {}) });
    else add({ category, kind: 'source', state: coverage.pendingCount ? 'partial' : 'ready', count: 1,
      ...(coverage.pendingCount ? { code: 'pending_details' as const } : !source.complete ? { code: 'source_total_unknown' as const } : {}) });
  }
  add({ category: 'laboratory', kind: 'source', state: options.includeLaboratory ? options.laboratory ? 'ready' : 'failed' : 'excluded', count: 1,
    ...(!options.includeLaboratory ? { code: 'scope_excluded' as const } : !options.laboratory ? { code: 'request_failed' as const }
      : !options.laboratory.complete ? { code: 'source_total_unknown' as const } : {}) });
  for (const task of options.tasks) add({ category: task.category, kind: 'prepare',
    state: options.requestPreparation ? task.state : 'not_requested', count: 1,
    ...(task.state === 'indeterminate' ? { code: 'preparation_outcome_unknown' as const }
      : task.state === 'correlation_unavailable' ? { code: 'source_did_not_provide_job_correlation' as const }
        : options.pollingLimited && (task.state === 'requested' || task.state === 'source_refreshed') ? { code: 'poll_limit' as const } : {}) });
  return { scope: options.scope, started_at: options.startedAt, finished_at: options.finishedAt, statuses: [...grouped.values()] };
}

export function packageFileGroups(files: DatasetPackageFile[]): DatasetPackageFile[][] {
  const groups: DatasetPackageFile[][] = [];
  for (let offset = 0; offset < files.length; offset += MAX_PACKAGE_FILES) groups.push(files.slice(offset, offset + MAX_PACKAGE_FILES));
  return groups.length ? groups : [[]];
}

export function recordSavedOriginals(previous: SavedOriginals, files: PackageFileStatus[], partName: string): SavedOriginals {
  const next = { ...previous };
  for (const file of files) {
    const prior = previous[file.archive_id];
    if (prior?.state === 'completed') continue;
    next[file.archive_id] = { ...file, part_name: partName,
      failures: (prior?.failures ?? 0) + (file.state === 'failed' ? 1 : 0) };
  }
  return next;
}

export function remainingOriginals(files: DatasetPackageFile[], saved: SavedOriginals): DatasetPackageFile[] {
  return files.filter(file => saved[file.archive_id]?.state !== 'completed');
}

export function retryableOriginals(files: DatasetPackageFile[], saved: SavedOriginals): DatasetPackageFile[] {
  return remainingOriginals(files, saved).filter(file => (saved[file.archive_id]?.failures ?? 0) < MAX_ORIGINAL_FAILURES);
}

export async function waitForAcquisition(signal: AbortSignal, milliseconds: number): Promise<void> {
  signal.throwIfAborted();
  await new Promise<void>((resolve, reject) => {
    const finish = () => { clearTimeout(timer); signal.removeEventListener('abort', cancel); resolve(); };
    const cancel = () => {
      clearTimeout(timer); signal.removeEventListener('abort', cancel);
      const reason: unknown = signal.reason;
      reject(reason instanceof Error ? reason : new DOMException('取得已停止。', 'AbortError'));
    };
    const timer = setTimeout(finish, milliseconds);
    signal.addEventListener('abort', cancel, { once: true });
  });
}

export async function datasetDigest(content: string): Promise<ExternalDataset> {
  const bytes = new TextEncoder().encode(content);
  const digest = new Uint8Array(await crypto.subtle.digest('SHA-256', bytes));
  return { name: 'healthbank.dataset.v2.json', size: bytes.length, sha256: [...digest].map(value => value.toString(16).padStart(2, '0')).join('') };
}
