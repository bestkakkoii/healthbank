// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import type { CacheProvenance, SupplementReport } from './api';
import type { HealthDataSnapshot } from './health-data';
import { formatTime } from './model.ts';

export function cachedSourceSnapshot(snapshot: HealthDataSnapshot, savedAt: string, restoredAt = new Date().toISOString()): HealthDataSnapshot {
  const restored = (report: SupplementReport): SupplementReport => ({ ...report, complete: false,
    cache_provenance: { kind: 'device_cache', saved_at: savedAt, restored_at: restoredAt,
      source_fetched_at: report.fetched_at, source_complete: report.cache_provenance?.source_complete ?? report.complete } });
  return { ...snapshot, reports: Object.fromEntries(Object.entries(snapshot.reports).map(([id, report]) => [id, restored(report)])),
    details: Object.fromEntries(Object.entries(snapshot.details).map(([id, values]) => [id,
      Object.fromEntries(Object.entries(values).map(([reference, report]) => [reference, restored(report)]))])) };
}

export function cacheProvenanceText(provenance: CacheProvenance): string {
  return `裝置快取；原始取得時間：${provenance.source_fetched_at ? formatTime(provenance.source_fetched_at) : '未提供'}。` +
    '本次僅重新判讀／顯示已保存內容，未重新向官方拉取或核對完整性。';
}
