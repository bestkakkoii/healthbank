// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import { ArchiveSHA256 } from './archive-integrity.ts';
import { archiveBoolean, archiveCategory, archiveChoice, archiveDigest, archiveFilename, archiveInteger, archiveJSON,
  archiveList, archiveObject, archiveReference, archiveText, archiveTime, invalidArchive } from './archive-structure.ts';
import { openArchiveZIP, readZIPEntry, visitZIPEntry, MAX_PACKAGE_MANIFEST_BYTES, MAX_PACKAGE_ORIGINAL_BYTES,
  type ArchiveZIP, type ArchiveZIPEntry } from './archive-zip.ts';
import type { DownloadedAttachment } from './health-data';
import { archiveAttachmentMIME, ArchiveAttachmentValidator, importDataset, MAX_ARCHIVE_ATTACHMENT_BYTES, MAX_ARCHIVE_ATTACHMENTS, MAX_DATASET_BYTES,
  type ImportedDataset } from './health-dataset.ts';
import type { AcquisitionSummary, ExternalDataset, PackageFileStatus, PackagePlan } from './full-acquisition';

export const MAX_RESTORE_PARTS = 1000;
export const MAX_RESTORE_MANIFEST_BYTES = 32 * 1024 ** 2;
export interface RestoredPackageFile extends PackageFileStatus {
  category: string; usable: boolean; path?: string; content_type?: string; warnings?: string[];
}
export interface AcquisitionPackageManifest {
  schema: 'healthbank.acquisition.package.v1'; created_at: string; state: 'completed' | 'partial';
  plan: PackagePlan; dataset: ExternalDataset & { included: boolean }; acquisition: AcquisitionSummary;
  files: RestoredPackageFile[]; history_completeness: 'preserved_in_dataset_and_source_statuses_not_inferred_from_transfer';
}
export interface AcquisitionRestorePart { file: File; zip: ArchiveZIP; manifest: AcquisitionPackageManifest; manifest_bytes: number; metadata_verified: boolean }
export type OriginalRestoreState = 'not_checked' | 'loaded' | 'verified_external' | 'source_unusable' | 'missing_part' | 'conflict' | 'failed';
export interface OriginalRestoreResult {
  id: string; category: string; name: string; state: OriginalRestoreState; size: number;
  sha256?: string; part?: number; path?: string; content_type?: string;
}
export interface AcquisitionRestoreSelection {
  kind: 'originals'; parts: AcquisitionRestorePart[]; external: File | null; dataset: ImportedDataset | null;
  retained: DownloadedAttachment[]; results: Record<string, OriginalRestoreResult>; checked: Record<string, 'verified' | 'failed'>;
}
export interface OriginalRestoreProgress { phase: 'dataset' | 'originals'; done: number; total: number; name: string; bytes: number; size: number }
interface OriginalCandidate { part: number; entry: ArchiveZIPEntry; file: RestoredPackageFile }
interface OriginalTarget { id: string; category: string; name: string; candidates: OriginalCandidate[]; listed: boolean }
const STATES = ['queued', 'streaming', 'completed', 'failed', 'cancelled', 'skipped'] as const;
const SUMMARY_STATES = {
  source: ['ready', 'partial', 'failed', 'unqueried', 'cancelled'],
  prepare: ['not_requested', 'queued', 'requested', 'indeterminate', 'source_refreshed', 'correlation_unavailable', 'failed', 'cancelled'],
  document: ['ready', 'excluded', 'failed', 'cancelled'],
} as const;
const SUMMARY_CODES = ['source_total_unknown', 'pending_details', 'preparation_outcome_unknown', 'source_did_not_provide_job_correlation',
  'poll_limit', 'request_failed', 'scope_excluded', 'budget_exceeded', 'auth_expired'] as const;
const key = (category: string, id: string) => `${category}:${id}`;

export function parseAcquisitionManifest(bytes: Uint8Array): AcquisitionPackageManifest {
  const value = archiveObject(archiveJSON(bytes, MAX_PACKAGE_MANIFEST_BYTES),
    ['schema', 'created_at', 'state', 'plan', 'dataset', 'acquisition', 'files', 'history_completeness']);
  if (value['schema'] !== 'healthbank.acquisition.package.v1' ||
    value['history_completeness'] !== 'preserved_in_dataset_and_source_statuses_not_inferred_from_transfer') return invalidArchive();
  const plan = archiveObject(value['plan'], ['id', 'index', 'total', 'files_total']), id = archiveText(plan['id'], 32);
  if (!/^[a-f0-9]{32}$/.test(id)) return invalidArchive();
  const total = archiveInteger(plan['total'], 1000, 1), count = archiveInteger(plan['files_total'], 50_000);
  const descriptor = archiveObject(value['dataset'], ['name', 'size', 'sha256', 'included']);
  if (descriptor['name'] !== 'healthbank.dataset.v2.json') return invalidArchive();
  const dataset = { name: 'healthbank.dataset.v2.json' as const, size: archiveInteger(descriptor['size'], MAX_DATASET_BYTES, 1),
    sha256: archiveDigest(descriptor['sha256']), included: archiveBoolean(descriptor['included']) };
  const source = archiveObject(value['acquisition'], ['scope', 'started_at', 'finished_at', 'statuses']);
  const started = archiveTime(source['started_at']), finished = archiveTime(source['finished_at']);
  if (Date.parse(finished) < Date.parse(started)) return invalidArchive();
  const statuses: AcquisitionSummary['statuses'] = archiveList(source['statuses'], 4096).map(item => {
    const status = archiveObject(item, ['category', 'kind', 'state', 'count', 'code']);
    const kind = archiveChoice(status['kind'], ['source', 'prepare', 'document']);
    const state = archiveChoice(status['state'], [...SUMMARY_STATES.source, ...SUMMARY_STATES.prepare, ...SUMMARY_STATES.document]);
    if (!(SUMMARY_STATES[kind] as readonly string[]).includes(state)) return invalidArchive();
    return { category: archiveCategory(status['category']), kind, state, count: status['count'] === undefined ? 0 : archiveInteger(status['count'], 250_000),
      ...(status['code'] === undefined ? {} : { code: archiveChoice(status['code'], SUMMARY_CODES) }) };
  });
  const ids = new Set<string>(), paths = new Set<string>(); let originalBytes = 0;
  const files = archiveList(value['files'], Math.min(count, 1000)).map(item => {
    const file = archiveObject(item, ['category', 'usable', 'archive_id', 'state', 'bytes', 'sha256', 'code', 'path', 'content_type', 'warnings']);
    const id = archiveReference(file['archive_id']), category = archiveCategory(file['category']), state = archiveChoice(file['state'], STATES);
    const usable = archiveBoolean(file['usable']), size = archiveInteger(file['bytes'], MAX_PACKAGE_ORIGINAL_BYTES);
    if (ids.has(id) || usable !== (state === 'completed') || state === 'queued' || state === 'streaming') return invalidArchive();
    ids.add(id); originalBytes += size; if (originalBytes > 8 * 1024 ** 3) return invalidArchive();
    const path = file['path'] === undefined ? undefined : archiveText(file['path'], 500);
    if (path !== undefined && (!path.startsWith(`originals/${id}/`) || paths.has(path))) return invalidArchive();
    if (path !== undefined) { archiveFilename(path.slice(`originals/${id}/`.length)); paths.add(path); }
    if (usable && (!path || file['sha256'] === undefined || file['content_type'] === undefined) || !path && size !== 0) return invalidArchive();
    const code = file['code'] === undefined ? undefined : archiveText(file['code'], 100);
    if (code !== undefined && !/^[A-Za-z0-9_]{1,100}$/.test(code)) return invalidArchive();
    return { archive_id: id, category, usable, state, bytes: size,
      ...(file['sha256'] === undefined ? {} : { sha256: archiveDigest(file['sha256']) }), ...(code === undefined ? {} : { code }),
      ...(path === undefined ? {} : { path }), ...(file['content_type'] === undefined ? {} : { content_type: archiveAttachmentMIME(file['content_type']) }),
      ...(file['warnings'] === undefined ? {} : { warnings: archiveList(file['warnings'], 10).map(value => archiveChoice(value, ['official_temporary_cleanup_unconfirmed'])) }) };
  });
  const state = archiveChoice(value['state'], ['completed', 'partial']);
  if ((state === 'completed') !== files.every(file => file.usable)) return invalidArchive();
  return { schema: 'healthbank.acquisition.package.v1', created_at: archiveTime(value['created_at']), state,
    plan: { id, index: archiveInteger(plan['index'], total, 1), total, files_total: count }, dataset,
    acquisition: { scope: archiveChoice(source['scope'], ['all_available', 'selected']), started_at: started, finished_at: finished, statuses },
    files, history_completeness: 'preserved_in_dataset_and_source_statuses_not_inferred_from_transfer' };
}

export async function inspectAcquisitionZIP(file: File, signal: AbortSignal): Promise<AcquisitionRestorePart> {
  const zip = await openArchiveZIP(file, signal), entry = zip.entries.get('manifest.json'); if (!entry) return invalidArchive();
  const { content } = await readZIPEntry(zip, entry, MAX_PACKAGE_MANIFEST_BYTES, signal), manifest = parseAcquisitionManifest(content);
  const dataset = zip.entries.get(manifest.dataset.name);
  if (Boolean(dataset) !== manifest.dataset.included || dataset && dataset.size !== manifest.dataset.size) return invalidArchive();
  const expected = new Set(['README.txt', 'manifest.json', ...(manifest.dataset.included ? [manifest.dataset.name] : [])]);
  for (const file of manifest.files) {
    if (!file.path) continue;
    expected.add(file.path); const original = zip.entries.get(file.path);
    if (!original || original.size !== file.bytes || original.method !== 0) return invalidArchive();
  }
  if (zip.entries.size !== expected.size || [...zip.entries.keys()].some(name => !expected.has(name))) return invalidArchive();
  const readme = zip.entries.get('README.txt'); if (!readme) return invalidArchive();
  await visitZIPEntry(zip, readme, signal);
  return { file, zip, manifest, manifest_bytes: content.length, metadata_verified: false };
}

export function appendAcquisitionParts(previous: AcquisitionRestoreSelection | null, additions: AcquisitionRestorePart[]): AcquisitionRestoreSelection {
  const parts = [...previous?.parts ?? [], ...additions];
  if (!parts.length || parts.length > MAX_RESTORE_PARTS || parts.reduce((sum, part) => sum + part.manifest_bytes, 0) > MAX_RESTORE_MANIFEST_BYTES ||
    parts.reduce((sum, part) => sum + part.manifest.files.length, 0) > 150_000) return invalidArchive('所選 ZIP 索引合計超過本次還原容量；請分組開啟，原始檔案仍由您保存。');
  const first = parts[0]; if (!first) return invalidArchive();
  const seen = new Set<string>(), plans = new Map<string, PackagePlan>();
  for (const part of parts) {
    const manifest = part.manifest, existing = plans.get(manifest.plan.id), identity = `${manifest.plan.id}:${manifest.plan.index}`;
    if (seen.has(identity)) return invalidArchive('已選過相同計畫的這個 ZIP 部分；請勿重複加入同一部分。');
    if (manifest.dataset.sha256 !== first.manifest.dataset.sha256 || manifest.dataset.size !== first.manifest.dataset.size ||
      existing && (manifest.plan.total !== existing.total || manifest.plan.files_total !== existing.files_total))
      return invalidArchive('這些 ZIP 不屬於完全相同的資料 JSON；請分別還原，不能依檔名或相似醫療紀錄合併。');
    seen.add(identity); plans.set(manifest.plan.id, manifest.plan);
  }
  return { kind: 'originals', parts, external: previous?.external ?? null, dataset: previous?.dataset ?? null,
    retained: previous?.retained ?? [], results: previous?.results ?? {}, checked: previous?.checked ?? {} };
}

async function loadDataset(selection: AcquisitionRestoreSelection, signal: AbortSignal): Promise<ImportedDataset> {
  const descriptor = selection.parts[0]?.manifest.dataset; if (!descriptor) return invalidArchive();
  const included = selection.parts.find(part => part.manifest.dataset.included);
  let bytes: Uint8Array, digest: string;
  if (included) {
    const entry = included.zip.entries.get(descriptor.name); if (!entry) return invalidArchive();
    const result = await readZIPEntry(included.zip, entry, MAX_DATASET_BYTES, signal); bytes = result.content; digest = result.sha256;
  } else {
    const external = selection.external;
    if (!external || external.size !== descriptor.size) return invalidArchive('這些 ZIP 另存了完整資料 JSON；請選擇符合 manifest 的原始 JSON。');
    signal.throwIfAborted(); bytes = new Uint8Array(await external.arrayBuffer()); signal.throwIfAborted();
    const hash = new ArchiveSHA256(); hash.update(bytes); digest = hash.digest();
  }
  if (bytes.length !== descriptor.size || digest !== descriptor.sha256) return invalidArchive('完整資料 JSON 的長度或 SHA-256 不符，未套用 ZIP 關聯。');
  const dataset = await importDataset(bytes); signal.throwIfAborted(); return dataset;
}

function originalTargets(selection: AcquisitionRestoreSelection, dataset: ImportedDataset): OriginalTarget[] {
  const targets = new Map<string, OriginalTarget>(), ids = new Map<string, string>();
  const reports = [...Object.entries(dataset.snapshot.reports), ...Object.entries(dataset.snapshot.details)
    .flatMap(([category, details]) => Object.values(details).map(report => [category, report] as const))];
  for (const [category, report] of reports) for (const row of report.entries) for (const attachment of row.attachments ?? []) {
    const identity = key(category, attachment.id), other = ids.get(attachment.id);
    if (other !== undefined && other !== identity) return invalidArchive();
    ids.set(attachment.id, identity);
    if (!targets.has(identity)) targets.set(identity, { id: attachment.id, category, name: attachment.name, candidates: [], listed: false });
  }
  if (targets.size > 50_000) return invalidArchive();
  const byPlan = new Map<string, Set<string>>();
  for (const [partIndex, part] of selection.parts.entries()) for (const file of part.manifest.files) {
    const target = targets.get(key(file.category, file.archive_id)); if (!target) return invalidArchive('ZIP 附件無法對應到資料 JSON 中的原始關聯，未建立推測配對。');
    const planIDs = byPlan.get(part.manifest.plan.id) ?? new Set<string>();
    if (planIDs.has(file.archive_id)) return invalidArchive();
    planIDs.add(file.archive_id); byPlan.set(part.manifest.plan.id, planIDs);
    target.listed = true;
    if (file.usable && file.path) {
      const entry = part.zip.entries.get(file.path); if (!entry) return invalidArchive();
      target.candidates.push({ part: partIndex, entry, file });
    }
  }
  for (const part of selection.parts) if ((byPlan.get(part.manifest.plan.id)?.size ?? 0) > part.manifest.plan.files_total) return invalidArchive();
  return [...targets.values()];
}

export async function restoreAcquisitionOriginals(initial: AcquisitionRestoreSelection, signal: AbortSignal,
  onCheckpoint: (selection: AcquisitionRestoreSelection) => void, onProgress?: (progress: OriginalRestoreProgress) => void): Promise<AcquisitionRestoreSelection> {
  onProgress?.({ phase: 'dataset', done: 0, total: 1, name: 'healthbank.dataset.v2.json', bytes: 0, size: initial.parts[0]?.manifest.dataset.size ?? 0 });
  const dataset = initial.dataset ?? await loadDataset(initial, signal), targets = originalTargets(initial, dataset);
  let current = { ...initial, dataset, retained: initial.dataset ? [...initial.retained] : [...dataset.attachments], results: { ...initial.results }, checked: { ...initial.checked } };
  // Every selected part's embedded copy must bind to the exact same dataset;
  // a healthy copy in another part cannot conceal a damaged JSON member.
  for (const [index, part] of current.parts.entries()) {
    signal.throwIfAborted(); if (part.metadata_verified) continue;
    if (part.manifest.dataset.included) {
      const entry = part.zip.entries.get(part.manifest.dataset.name); if (!entry) return invalidArchive();
      const verification = await visitZIPEntry(part.zip, entry, signal, undefined,
        (bytes, size) => onProgress?.({ phase: 'dataset', done: index, total: current.parts.length, name: part.file.name, bytes, size }));
      if (verification.sha256 !== part.manifest.dataset.sha256) return invalidArchive('其中一個 ZIP 的完整資料 JSON 校驗不符，未忽略損壞部分。');
    }
    current.parts = current.parts.map((value, ordinal) => ordinal === index ? { ...value, metadata_verified: true } : value); onCheckpoint(current);
  }
  let retainedBytes = current.retained.reduce((sum, file) => sum + file.bytes.length, 0);
  const retained = new Map(current.retained.map(file => [key(file.category, file.id), file]));
  for (const target of targets) if (!current.results[key(target.category, target.id)])
    current.results[key(target.category, target.id)] = { id: target.id, category: target.category, name: target.name,
      state: 'not_checked', size: target.candidates[0]?.file.bytes ?? 0 };
  onCheckpoint(current);
  for (const [ordinal, target] of targets.entries()) {
    signal.throwIfAborted(); const identity = key(target.category, target.id), candidate = target.candidates[0];
    const basic = { id: target.id, category: target.category, name: target.name, size: candidate?.file.bytes ?? 0 };
    const conflict = candidate !== undefined && target.candidates.some(item => item.file.sha256 !== candidate.file.sha256 ||
      item.file.bytes !== candidate.file.bytes || item.file.content_type !== candidate.file.content_type);
    let result: OriginalRestoreResult;
    if (conflict) {
      const previous = retained.get(identity);
      if (previous) { retainedBytes -= previous.bytes.length; retained.delete(identity); current.retained = current.retained.filter(file => key(file.category, file.id) !== identity); }
      result = { ...basic, state: 'conflict' };
    } else if (!candidate) result = { ...basic, state: target.listed ? 'source_unusable' : 'missing_part' };
    else {
      const saved = current.results[identity];
      let verifiedResult = saved && ['loaded', 'verified_external'].includes(saved.state) && saved.sha256 === candidate.file.sha256 ? saved : null;
      let embeddedConflict = false;
      for (const copy of target.candidates) {
        signal.throwIfAborted(); const copyKey = `${copy.part}:${copy.entry.name}`;
        if (current.checked[copyKey] === 'verified') continue;
        const part = current.parts[copy.part], type = copy.file.content_type, expectedHash = copy.file.sha256;
        if (!part || !type || !expectedHash) return invalidArchive();
        const filename = archiveFilename(copy.entry.name.split('/').at(-1)), existing = retained.get(identity);
        const fits = !verifiedResult && !existing && retained.size < MAX_ARCHIVE_ATTACHMENTS && retainedBytes + copy.entry.size <= MAX_ARCHIVE_ATTACHMENT_BYTES;
        const bytes = fits ? new Uint8Array(copy.entry.size) : null, validator = new ArchiveAttachmentValidator(type);
        try {
          const verified = await visitZIPEntry(part.zip, copy.entry, signal, (chunk, offset) => { validator.update(chunk); bytes?.set(chunk, offset); },
            (bytes, size) => onProgress?.({ phase: 'originals', done: ordinal, total: targets.length, name: filename, bytes, size }));
          validator.finish(); if (verified.sha256 !== expectedHash) return invalidArchive('原始檔 SHA-256 不符。');
          if (existing && !verifiedResult) {
            const hash = new ArchiveSHA256(); hash.update(existing.bytes);
            if (hash.digest() !== verified.sha256) { embeddedConflict = true; return invalidArchive('JSON 與 ZIP 對同一原始檔提供不同內容。'); }
          }
          if (bytes) {
            const file = { category: target.category, id: target.id, name: filename, content_type: type, bytes };
            retainedBytes += bytes.length; current.retained = [...current.retained, file]; retained.set(identity, file);
          }
          verifiedResult ??= { ...basic, name: filename, state: bytes || existing ? 'loaded' : 'verified_external', sha256: verified.sha256,
            part: copy.part, path: copy.entry.name, content_type: type };
          current.checked[copyKey] = 'verified';
          current.results[identity] = verifiedResult; current = { ...current }; onCheckpoint(current);
        } catch (error) {
          if (signal.aborted) throw error;
          current.checked[copyKey] = 'failed';
        }
      }
      if (!verifiedResult || embeddedConflict) {
        const previous = retained.get(identity);
        if (previous) { retainedBytes -= previous.bytes.length; retained.delete(identity); current.retained = current.retained.filter(file => key(file.category, file.id) !== identity); }
        result = { ...basic, state: embeddedConflict ? 'conflict' : 'failed' };
      } else result = verifiedResult;
    }
    // This is the operation-owned verification ledger, not source medical data.
    // Avoid copying up to 50,000 prior results for each checked original.
    current.results[identity] = result; current = { ...current }; onCheckpoint(current);
    onProgress?.({ phase: 'originals', done: ordinal + 1, total: targets.length, name: result.name, bytes: result.size, size: result.size });
  }
  signal.throwIfAborted(); return current;
}

export function recoveredAcquisitionDataset(selection: AcquisitionRestoreSelection): ImportedDataset {
  if (!selection.dataset) return invalidArchive();
  const files = new Map(selection.retained.map(file => [key(file.category, file.id), file]));
  const updateReport = (category: string, report: ImportedDataset['snapshot']['reports'][string]) => ({ ...report,
    entries: report.entries.map(row => ({ ...row, ...(row.attachments === undefined ? {} : { attachments: row.attachments.map(attachment => {
      const file = files.get(key(category, attachment.id));
      return file ? { ...attachment, name: file.name, content_type: file.content_type, size: file.bytes.length } : attachment;
    }) }) })) });
  const snapshot = selection.dataset.snapshot;
  return { ...selection.dataset, attachments: selection.retained, snapshot: { ...snapshot,
    reports: Object.fromEntries(Object.entries(snapshot.reports).map(([category, report]) => [category, updateReport(category, report)])),
    details: Object.fromEntries(Object.entries(snapshot.details).map(([category, details]) => [category,
      Object.fromEntries(Object.entries(details).map(([id, report]) => [id, updateReport(category, report)]))])) } };
}

export function recoveredOriginalBlob(selection: AcquisitionRestoreSelection, result: OriginalRestoreResult): Blob {
  if (!['loaded', 'verified_external'].includes(result.state) || result.part === undefined || !result.path || !result.content_type) return invalidArchive();
  const part = selection.parts[result.part], entry = part?.zip.entries.get(result.path);
  if (!part || !entry || entry.method !== 0 || entry.size !== result.size) return invalidArchive();
  return part.file.slice(entry.data_offset, entry.data_offset + entry.size, result.content_type);
}
