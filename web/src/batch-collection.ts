// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import type { ReportResponse } from './api';
import { ArchiveSHA256 } from './archive-integrity.ts';
import { archiveBoolean, archiveCategory, archiveChoice, archiveDigest, archiveFilename, archiveInteger, archiveJSON,
  archiveList, archiveObject, archiveText, archiveTime, invalidArchive } from './archive-structure.ts';
import { dateBatchStatus, MAX_DATE_BATCHES, splitDateBatches, type BatchPeriod, type BatchRange, type DateBatchPlan } from './date-batches.ts';
import { healthDataEntryCount, validAcquisitionQuery, type HealthDataSnapshot } from './health-data.ts';
import { datasetLaboratoryRows, importDataset, MAX_DATASET_BYTES, type ImportedDataset } from './health-dataset.ts';

export const BATCH_COLLECTION_SCHEMA = 'healthbank.batch.collection.v1';
export const MAX_BATCH_COLLECTION_BYTES = 2 * 1024 * 1024;
export interface BatchAcquisitionSummary {
  source_entries: number;
  laboratory_records: number;
  source_reports: number;
  lists_pending: number;
  details_pending: number;
  laboratory_pending: boolean;
  acquired: boolean;
  source_reports_without_total: number;
  laboratory_complete: boolean;
}
export interface BatchDatasetDescriptor {
  name: string;
  size: number;
  sha256: string;
  exported_at: string;
  saved_acknowledged: boolean;
  acquisition: BatchAcquisitionSummary;
}
export interface BatchCollectionEntry { index: number; query: BatchRange; dataset?: BatchDatasetDescriptor }
export interface BatchCollectionManifest {
  schema: typeof BATCH_COLLECTION_SCHEMA;
  collection_id: string;
  created_at: string;
  query: BatchRange;
  period: BatchPeriod;
  categories: string[];
  include_laboratory: boolean;
  batches: BatchCollectionEntry[];
}
export interface BatchCollectionSelection {
  kind: 'batches';
  manifest: BatchCollectionManifest;
  files: Record<number, File>;
  current_index: number | null;
}

function range(value: unknown): BatchRange {
  const input = archiveObject(value, ['from', 'to']);
  const result = { from: archiveText(input['from'], 10), to: archiveText(input['to'], 10) };
  if (!result.from || !validAcquisitionQuery(result)) return invalidArchive();
  return result;
}
function sameRange(first: BatchRange, second: BatchRange): boolean { return first.from === second.from && first.to === second.to; }

export function newBatchCollection(plan: DateBatchPlan, period: BatchPeriod): BatchCollectionManifest {
  const first = plan.ranges[0], last = plan.ranges.at(-1);
  if (!first || !last) return invalidArchive();
  const result: BatchCollectionManifest = { schema: BATCH_COLLECTION_SCHEMA,
    collection_id: [...crypto.getRandomValues(new Uint8Array(16))].map(value => value.toString(16).padStart(2, '0')).join(''),
    created_at: new Date().toISOString(), query: { from: first.from, to: last.to }, period,
    categories: [...plan.categories], include_laboratory: plan.includeLaboratory,
    batches: plan.ranges.map((query, index) => ({ index: index + 1, query: { ...query } })) };
  return parseBatchCollection(new TextEncoder().encode(JSON.stringify(result)));
}

export function summarizeBatch(snapshot: HealthDataSnapshot, plan: DateBatchPlan, laboratory: ReportResponse | null): BatchAcquisitionSummary {
  const status = dateBatchStatus(snapshot, plan.categories, plan.includeLaboratory, laboratory);
  return { source_entries: healthDataEntryCount(snapshot), laboratory_records: laboratory?.records.length ?? 0,
    source_reports: Object.keys(snapshot.reports).length, lists_pending: status.listsPending, details_pending: status.detailsPending,
    laboratory_pending: status.laboratoryPending, acquired: status.acquired,
    source_reports_without_total: Object.values(snapshot.reports).filter(report => !report.complete).length,
    laboratory_complete: laboratory?.complete ?? false };
}

export async function describeBatchDataset(content: string, name: string, acquisition: BatchAcquisitionSummary): Promise<BatchDatasetDescriptor> {
  const bytes = new TextEncoder().encode(content);
  const dataset = await importDataset(bytes), hash = new ArchiveSHA256(); hash.update(bytes);
  return { name: archiveFilename(name), size: bytes.length, sha256: hash.digest(), exported_at: dataset.exportedAt,
    saved_acknowledged: false, acquisition };
}

export function updateBatchDescriptor(manifest: BatchCollectionManifest, index: number, dataset: BatchDatasetDescriptor): BatchCollectionManifest {
  if (!manifest.batches.some(batch => batch.index === index)) return invalidArchive();
  return { ...manifest, batches: manifest.batches.map(batch => batch.index === index ? { ...batch, dataset } : batch) };
}

export function batchCollectionJSON(manifest: BatchCollectionManifest): string {
  const output = JSON.stringify(manifest, null, 2) + '\n';
  parseBatchCollection(new TextEncoder().encode(output)); return output;
}

export function parseBatchCollection(bytes: Uint8Array): BatchCollectionManifest {
  const input = archiveObject(archiveJSON(bytes, MAX_BATCH_COLLECTION_BYTES),
    ['schema', 'collection_id', 'created_at', 'query', 'period', 'categories', 'include_laboratory', 'batches']);
  if (input['schema'] !== BATCH_COLLECTION_SCHEMA) return invalidArchive('這不是支援的分批保存索引。請選擇分批計畫匯出的 collection JSON。');
  const id = archiveText(input['collection_id'], 32); if (!/^[a-f0-9]{32}$/.test(id)) return invalidArchive();
  const query = range(input['query']), period = archiveChoice(input['period'], ['year', 'month']);
  const ranges = splitDateBatches(query, period);
  const categories = archiveList(input['categories'], 128).map(archiveCategory);
  const includeLaboratory = archiveBoolean(input['include_laboratory']);
  if (new Set(categories).size !== categories.length || !categories.length && !includeLaboratory) return invalidArchive();
  const batches = archiveList(input['batches'], MAX_DATE_BATCHES).map((item, ordinal) => {
    const batch = archiveObject(item, ['index', 'query', 'dataset']);
    const index = archiveInteger(batch['index'], MAX_DATE_BATCHES, 1), query = range(batch['query']);
    const expectedRange = ranges[ordinal];
    if (index !== ordinal + 1 || !expectedRange || !sameRange(query, expectedRange)) return invalidArchive();
    if (batch['dataset'] === undefined) return { index, query };
    const file = archiveObject(batch['dataset'], ['name', 'size', 'sha256', 'exported_at', 'saved_acknowledged', 'acquisition']);
    const acquisition = archiveObject(file['acquisition'], ['source_entries', 'laboratory_records', 'source_reports', 'lists_pending',
      'details_pending', 'laboratory_pending', 'acquired', 'source_reports_without_total', 'laboratory_complete']);
    const sourceReports = archiveInteger(acquisition['source_reports'], categories.length);
    const summary: BatchAcquisitionSummary = {
      source_entries: archiveInteger(acquisition['source_entries'], 250_000), laboratory_records: archiveInteger(acquisition['laboratory_records'], 250_000),
      source_reports: sourceReports, lists_pending: archiveInteger(acquisition['lists_pending'], categories.length),
      details_pending: archiveInteger(acquisition['details_pending'], 250_000), laboratory_pending: archiveBoolean(acquisition['laboratory_pending']),
      acquired: archiveBoolean(acquisition['acquired']), source_reports_without_total: archiveInteger(acquisition['source_reports_without_total'], sourceReports),
      laboratory_complete: archiveBoolean(acquisition['laboratory_complete']),
    };
    if (summary.acquired !== (!summary.lists_pending && !summary.details_pending && !summary.laboratory_pending) ||
      !includeLaboratory && (summary.laboratory_records || summary.laboratory_pending || summary.laboratory_complete)) return invalidArchive();
    const dataset: BatchDatasetDescriptor = { name: archiveFilename(file['name']), size: archiveInteger(file['size'], MAX_DATASET_BYTES, 1),
      sha256: archiveDigest(file['sha256']), exported_at: archiveTime(file['exported_at']), saved_acknowledged: archiveBoolean(file['saved_acknowledged']),
      acquisition: summary };
    return { index, query, dataset };
  });
  if (batches.length !== ranges.length) return invalidArchive();
  return { schema: BATCH_COLLECTION_SCHEMA, collection_id: id, created_at: archiveTime(input['created_at']), query, period,
    categories, include_laboratory: includeLaboratory, batches };
}

export async function restoreBatchMember(manifest: BatchCollectionManifest, index: number, file: File, signal: AbortSignal): Promise<ImportedDataset> {
  const batch = manifest.batches[index - 1], descriptor = batch?.dataset;
  if (!batch || batch.index !== index || !descriptor || file.size !== descriptor.size || file.size > MAX_DATASET_BYTES)
    return invalidArchive('這批資料包未提供或檔案長度不符；請選擇對應的原始 JSON。');
  signal.throwIfAborted(); const bytes = new Uint8Array(await file.arrayBuffer()); signal.throwIfAborted();
  const hash = new ArchiveSHA256(); hash.update(bytes);
  if (hash.digest() !== descriptor.sha256) return invalidArchive('這批資料包的 SHA-256 與索引不符；沒有載入錯誤或變更過的檔案。');
  const dataset = await importDataset(bytes); signal.throwIfAborted();
  if (!sameRange(dataset.snapshot.query, batch.query) || dataset.exportedAt !== descriptor.exported_at ||
    [...Object.keys(dataset.snapshot.reports), ...Object.keys(dataset.snapshot.details), ...Object.keys(dataset.snapshot.states)]
      .some(category => !manifest.categories.includes(category))) return invalidArchive();
  for (const state of Object.values(dataset.snapshot.states)) if (state.query && !sameRange(state.query, batch.query)) return invalidArchive();
  const rows = datasetLaboratoryRows(dataset);
  const laboratory = dataset.laboratoryMetadata.official_query
    ? { ...dataset.laboratoryMetadata, records: [] } : null;
  const summary = summarizeBatch(dataset.snapshot, { categories: manifest.categories, ranges: [batch.query], includeLaboratory: manifest.include_laboratory }, laboratory);
  summary.laboratory_records = rows.length;
  if (Object.entries(descriptor.acquisition).some(([key, value]) => summary[key as keyof BatchAcquisitionSummary] !== value)) return invalidArchive();
  return dataset;
}

export function matchBatchFiles(manifest: BatchCollectionManifest, files: File[], previous: Record<number, File> = {}): {
  files: Record<number, File>; unmatched: number; ambiguous: number;
} {
  if (files.length > MAX_DATE_BATCHES) return invalidArchive('一次最多選擇 600 個分批 JSON。');
  const assigned = { ...previous }, used = new Set<File>(); let ambiguous = 0;
  for (const batch of manifest.batches) {
    if (!batch.dataset) continue;
    const candidates = files.filter(file => file.name === batch.dataset?.name && file.size === batch.dataset.size);
    if (candidates.length === 1 && candidates[0]) { assigned[batch.index] = candidates[0]; used.add(candidates[0]); }
    else if (candidates.length > 1) ambiguous++;
  }
  return { files: assigned, unmatched: files.filter(file => !used.has(file)).length, ambiguous };
}
