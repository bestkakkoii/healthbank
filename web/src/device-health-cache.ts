// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
// A consented device cache, not a credential store. Every medical payload is an
// already validated portable v2 document without live official capabilities.
import type { Session } from './api';
import { ArchiveSHA256 } from './archive-integrity.ts';
import { importDataset, MAX_DATASET_BYTES, type ImportedDataset } from './health-dataset.ts';
import { isObject } from './model.ts';

export const DEVICE_CACHE_DATABASE = 'healthbank.device-health-cache.v1';
export const MAX_DEVICE_CACHE_PROFILES = 8;
export const MAX_DEVICE_CACHE_BYTES = 256 * 1024 ** 2;
const CHANNEL = 'healthbank.device-health-cache.v1';
const STORES = ['control', 'profiles', 'payloads'];
export interface DeviceCacheLease { profile_key: string; global_epoch: string; profile_epoch: string; revision: string }
export interface DeviceCacheMetadata {
  size: number; sha256: string; saved_at: string; source_fetched_at: string; records: number; sources: number; version: string;
}
export interface DeviceCacheState { enabled: boolean; lease: DeviceCacheLease | null; metadata: DeviceCacheMetadata | null }
export interface DeviceCacheLoaded { state: DeviceCacheState; dataset: ImportedDataset | null }
export type DeviceCacheEvent = { kind: 'profile'; profile_key: string } | { kind: 'all' };
interface ProfileRecord { profile_key: string; epoch: string; revision: string; metadata: DeviceCacheMetadata | null }
interface PayloadRecord { profile_key: string; epoch: string; revision: string; global_epoch: string; content: Blob }
export class DeviceCacheError extends Error {
  constructor(readonly code: 'unavailable' | 'invalid' | 'stale' | 'capacity' | 'quota', message: string) { super(message); this.name = 'DeviceCacheError'; }
}
let connection: Promise<IDBDatabase> | null = null;

export function verifiedCacheProfile(session: Session): string | null {
  return session.mode === 'official' && session.authenticated && /^[a-f0-9]{64}$/.test(session.profile_key ?? '') ? session.profile_key ?? null : null;
}
function profileKey(value: string): void { if (!/^[a-f0-9]{64}$/.test(value)) throw new DeviceCacheError('invalid', '尚未取得本次官方驗證身分，不能存取個人健康快取。'); }
function token(): string { return [...crypto.getRandomValues(new Uint8Array(16))].map(value => value.toString(16).padStart(2, '0')).join(''); }
function validToken(value: unknown): value is string { return typeof value === 'string' && /^[a-f0-9]{32}$/.test(value); }
function invalid(): never { throw new DeviceCacheError('invalid', '本機快取的格式或校驗不符，未顯示其中資料；可清除此快取後重新取得。'); }
function databaseError(error: DOMException | null): DeviceCacheError {
  return error?.name === 'QuotaExceededError'
    ? new DeviceCacheError('quota', '瀏覽器儲存容量不足，這次資料未寫入本機快取；目前工作區仍可匯出保存。')
    : new DeviceCacheError('unavailable', '此瀏覽器目前無法使用健康資料快取；資料仍可在本次頁面查看與匯出。');
}
function metadata(value: unknown): DeviceCacheMetadata | null {
  if (value === null) return null;
  if (!isObject(value) || Object.keys(value).some(key => !['size', 'sha256', 'saved_at', 'source_fetched_at', 'records', 'sources', 'version'].includes(key))) return invalid();
  const size = value['size'], records = value['records'], sources = value['sources'];
  if (typeof size !== 'number' || !Number.isSafeInteger(size) || size < 1 || size > MAX_DATASET_BYTES ||
    typeof records !== 'number' || !Number.isSafeInteger(records) || records < 0 || records > 50_000 ||
    typeof sources !== 'number' || !Number.isSafeInteger(sources) || sources < 0 || sources > 200 ||
    typeof value['sha256'] !== 'string' || !/^[a-f0-9]{64}$/.test(value['sha256']) ||
    typeof value['saved_at'] !== 'string' || !Number.isFinite(Date.parse(value['saved_at'])) || value['saved_at'].length > 100 ||
    typeof value['source_fetched_at'] !== 'string' || value['source_fetched_at'].length > 100 ||
    typeof value['version'] !== 'string' || !/^[A-Za-z0-9._+-]{1,100}$/.test(value['version'])) return invalid();
  return { size, records, sources, sha256: value['sha256'], saved_at: value['saved_at'], source_fetched_at: value['source_fetched_at'], version: value['version'] };
}
function profile(value: unknown, expected?: string): ProfileRecord | null {
  if (value === undefined) return null;
  if (!isObject(value) || Object.keys(value).some(key => !['profile_key', 'epoch', 'revision', 'metadata'].includes(key)) ||
    typeof value['profile_key'] !== 'string' || !/^[a-f0-9]{64}$/.test(value['profile_key']) || expected !== undefined && value['profile_key'] !== expected ||
    !validToken(value['epoch']) || !validToken(value['revision'])) return invalid();
  return { profile_key: value['profile_key'], epoch: value['epoch'], revision: value['revision'], metadata: metadata(value['metadata']) };
}
function globalEpoch(value: unknown): string {
  if (!isObject(value) || Object.keys(value).length !== 1 || !validToken(value['epoch'])) return invalid();
  return value['epoch'];
}
function leaseFor(epoch: string, value: ProfileRecord): DeviceCacheLease {
  return { profile_key: value.profile_key, global_epoch: epoch, profile_epoch: value.epoch, revision: value.revision };
}
export function sameDeviceCacheLease(first: DeviceCacheLease, second: DeviceCacheLease): boolean {
  return first.profile_key === second.profile_key && first.global_epoch === second.global_epoch && first.profile_epoch === second.profile_epoch && first.revision === second.revision;
}

function openDatabase(): Promise<IDBDatabase> {
  if (connection) return connection;
  if (typeof indexedDB === 'undefined') return Promise.reject(databaseError(null));
  const pending = new Promise<IDBDatabase>((resolve, reject) => {
    let settled = false;
    let request: IDBOpenDBRequest;
    try { request = indexedDB.open(DEVICE_CACHE_DATABASE, 1); }
    catch { reject(databaseError(null)); return; }
    const timeout = setTimeout(() => { if (!settled) { settled = true; reject(databaseError(null)); } }, 10_000);
    request.onupgradeneeded = () => {
      const db = request.result;
      db.createObjectStore('control').put({ epoch: token() }, 'global');
      db.createObjectStore('profiles', { keyPath: 'profile_key' });
      db.createObjectStore('payloads', { keyPath: 'profile_key' });
    };
    request.onerror = () => { clearTimeout(timeout); if (!settled) { settled = true; reject(databaseError(request.error)); } };
    request.onblocked = () => { clearTimeout(timeout); if (!settled) { settled = true; reject(new DeviceCacheError('unavailable', '另一個舊版分頁仍佔用本機快取；請關閉該分頁後再試。')); } };
    request.onsuccess = () => {
      clearTimeout(timeout); const db = request.result;
      if (settled) { db.close(); return; }
      settled = true; db.onversionchange = () => { db.close(); connection = null; }; resolve(db);
    };
  });
  connection = pending;
  void pending.catch(() => { if (connection === pending) connection = null; });
  return pending;
}

// All dependent IDB requests are queued in request callbacks, while their
// transaction is active. Validation/hashing happens before a write transaction.
function transact<T>(db: IDBDatabase, mode: IDBTransactionMode, work: (tx: IDBTransaction, done: (value: T) => void, fail: (error: unknown) => void) => void): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    const tx = db.transaction(STORES, mode); let result: T, provided = false, failure: Error | undefined;
    const fail = (error: unknown) => { failure = error instanceof Error ? error : databaseError(null); try { tx.abort(); } catch { reject(failure); } };
    tx.oncomplete = () => { if (provided) resolve(result); else reject(databaseError(null)); };
    tx.onabort = () => reject(failure ?? databaseError(tx.error));
    try { work(tx, value => { result = value; provided = true; }, fail); } catch (error) { fail(error); }
  });
}
function controls(tx: IDBTransaction, key: string, run: (epoch: string, value: ProfileRecord | null) => void, fail: (error: unknown) => void): void {
  const global = tx.objectStore('control').get('global');
  global.onsuccess = () => {
    try {
      const epoch = globalEpoch(global.result), request = tx.objectStore('profiles').get(key);
      request.onsuccess = () => { try { run(epoch, profile(request.result, key)); } catch (error) { fail(error); } };
    } catch (error) { fail(error); }
  };
}
function notify(change: DeviceCacheEvent): void {
  if (typeof BroadcastChannel === 'undefined') return;
  try { const channel = new BroadcastChannel(CHANNEL); channel.postMessage(change); channel.close(); }
  catch { /* Atomic revisions remain authoritative if cross-tab delivery is unavailable. */ }
}
export function watchDeviceCache(listener: (change: DeviceCacheEvent) => void): () => void {
  if (typeof BroadcastChannel === 'undefined') return () => {};
  let channel: BroadcastChannel;
  try { channel = new BroadcastChannel(CHANNEL); } catch { return () => {}; }
  channel.onmessage = (event: MessageEvent<unknown>) => {
    const value = event.data;
    if (isObject(value) && value['kind'] === 'all') listener({ kind: 'all' });
    else if (isObject(value) && value['kind'] === 'profile' && typeof value['profile_key'] === 'string' && /^[a-f0-9]{64}$/.test(value['profile_key']))
      listener({ kind: 'profile', profile_key: value['profile_key'] });
  };
  return () => { channel.close(); };
}

export async function inspectDeviceCache(key: string): Promise<DeviceCacheState> {
  profileKey(key); const db = await openDatabase();
  return transact(db, 'readonly', (tx, done, fail) => controls(tx, key, (epoch, value) => {
    done(value ? { enabled: true, lease: leaseFor(epoch, value), metadata: value.metadata } : { enabled: false, lease: null, metadata: null });
  }, fail));
}
export async function enableDeviceCache(key: string): Promise<DeviceCacheState> {
  profileKey(key); const db = await openDatabase();
  const state = await transact<DeviceCacheState>(db, 'readwrite', (tx, done, fail) => controls(tx, key, (epoch, current) => {
    if (current) { done({ enabled: true, lease: leaseFor(epoch, current), metadata: current.metadata }); return; }
    const request = tx.objectStore('profiles').count();
    request.onsuccess = () => {
      try {
        if (request.result >= MAX_DEVICE_CACHE_PROFILES) throw new DeviceCacheError('capacity', '這部裝置最多保留 8 位使用者的健康快取；請先清除不需要的快取。');
        const value: ProfileRecord = { profile_key: key, epoch: token(), revision: token(), metadata: null };
        tx.objectStore('profiles').put(value); done({ enabled: true, lease: leaseFor(epoch, value), metadata: null });
      } catch (error) { fail(error); }
    };
  }, fail));
  notify({ kind: 'profile', profile_key: key }); return state;
}
export async function clearDeviceCache(key: string, keepConsent = false): Promise<DeviceCacheState> {
  profileKey(key); const db = await openDatabase();
  const state = await transact<DeviceCacheState>(db, 'readwrite', (tx, done, fail) => controls(tx, key, (epoch, current) => {
    tx.objectStore('payloads').delete(key);
    if (keepConsent && current) {
      const next: ProfileRecord = { ...current, epoch: token(), revision: token(), metadata: null };
      tx.objectStore('profiles').put(next); done({ enabled: true, lease: leaseFor(epoch, next), metadata: null });
    } else { tx.objectStore('profiles').delete(key); done({ enabled: false, lease: null, metadata: null }); }
  }, fail));
  notify({ kind: 'profile', profile_key: key }); return state;
}
export async function clearAllDeviceCaches(): Promise<void> {
  const db = await openDatabase();
  await transact<undefined>(db, 'readwrite', (tx, done) => {
    // A new global epoch also rejects old leases after deletion and re-enabling.
    tx.objectStore('control').put({ epoch: token() }, 'global');
    tx.objectStore('profiles').clear(); tx.objectStore('payloads').clear(); done(undefined);
  });
  notify({ kind: 'all' });
}

export async function saveDeviceCache(lease: DeviceCacheLease, content: string, signal?: AbortSignal): Promise<DeviceCacheState> {
  profileKey(lease.profile_key); signal?.throwIfAborted();
  if (content.length > MAX_DATASET_BYTES) throw new DeviceCacheError('capacity', '完整快取超過 128 MiB；請改以分批資料包保存。');
  const bytes = new TextEncoder().encode(content);
  if (bytes.length > MAX_DATASET_BYTES) throw new DeviceCacheError('capacity', '完整快取超過 128 MiB；請改以分批資料包保存。');
  const imported = await importDataset(bytes); signal?.throwIfAborted(); if (!imported.producer) return invalid();
  const hash = new ArchiveSHA256(); hash.update(bytes);
  const records = JSON.parse(imported.laboratoryJSON) as unknown;
  if (!isObject(records) || !Array.isArray(records['records'])) return invalid();
  const details: DeviceCacheMetadata = { size: bytes.length, sha256: hash.digest(), saved_at: new Date().toISOString(),
    source_fetched_at: imported.laboratoryMetadata.fetched_at, records: records['records'].length,
    sources: Object.keys(imported.snapshot.reports).length, version: imported.producer.version };
  const blob = new Blob([bytes], { type: 'application/json;charset=utf-8' }), db = await openDatabase(); signal?.throwIfAborted();
  const state = await transact<DeviceCacheState>(db, 'readwrite', (tx, done, fail) => {
    const cancel = () => fail(new DOMException('快取保存已取消。', 'AbortError'));
    signal?.addEventListener('abort', cancel, { once: true });
    tx.addEventListener('complete', () => signal?.removeEventListener('abort', cancel), { once: true });
    tx.addEventListener('abort', () => signal?.removeEventListener('abort', cancel), { once: true });
    controls(tx, lease.profile_key, (epoch, current) => {
      signal?.throwIfAborted();
      if (!current || !sameDeviceCacheLease(lease, leaseFor(epoch, current))) throw new DeviceCacheError('stale', '本機快取已在其他操作或分頁更新／清除；舊資料沒有覆蓋新的快取。');
      const request = tx.objectStore('profiles').getAll(undefined, MAX_DEVICE_CACHE_PROFILES + 1);
      request.onsuccess = () => {
        try {
          signal?.throwIfAborted(); const values: unknown[] = request.result;
          if (values.length > MAX_DEVICE_CACHE_PROFILES) return invalid();
          const used = values.reduce<number>((sum, value) => { const other = profile(value); return sum + (other?.profile_key !== lease.profile_key ? other?.metadata?.size ?? 0 : 0); }, 0);
          if (used + details.size > MAX_DEVICE_CACHE_BYTES) throw new DeviceCacheError('capacity', '本機健康快取合計超過 256 MiB；沒有自動刪除其他使用者資料，請先清除不需要的快取。');
          const next = { ...current, revision: token(), metadata: details };
          const payload: PayloadRecord = { profile_key: lease.profile_key, epoch: next.epoch, revision: next.revision, global_epoch: epoch, content: blob };
          tx.objectStore('payloads').put(payload); tx.objectStore('profiles').put(next);
          done({ enabled: true, lease: leaseFor(epoch, next), metadata: details });
        } catch (error) { fail(error); }
      };
    }, fail);
  });
  notify({ kind: 'profile', profile_key: lease.profile_key }); return state;
}

export async function loadDeviceCache(key: string, signal?: AbortSignal): Promise<DeviceCacheLoaded> {
  profileKey(key); signal?.throwIfAborted(); const db = await openDatabase();
  const selected = await transact<{ state: DeviceCacheState; content: Blob | null }>(db, 'readonly', (tx, done, fail) => controls(tx, key, (epoch, current) => {
    const state = current ? { enabled: true, lease: leaseFor(epoch, current), metadata: current.metadata } : { enabled: false, lease: null, metadata: null };
    if (!current?.metadata) { done({ state, content: null }); return; }
    const request = tx.objectStore('payloads').get(key);
    request.onsuccess = () => {
      try {
        const value: unknown = request.result;
        if (!isObject(value) || Object.keys(value).some(key => !['profile_key', 'epoch', 'revision', 'global_epoch', 'content'].includes(key)) ||
          value['profile_key'] !== key || value['epoch'] !== current.epoch || value['revision'] !== current.revision || value['global_epoch'] !== epoch ||
          !(value['content'] instanceof Blob) || value['content'].size !== current.metadata?.size) return invalid();
        done({ state, content: value['content'] });
      } catch (error) { fail(error); }
    };
  }, fail));
  signal?.throwIfAborted(); if (!selected.content) return { state: selected.state, dataset: null };
  const bytes = new Uint8Array(await selected.content.arrayBuffer()); signal?.throwIfAborted();
  const hash = new ArchiveSHA256(); hash.update(bytes); if (hash.digest() !== selected.state.metadata?.sha256) return invalid();
  const dataset = await importDataset(bytes); signal?.throwIfAborted();
  const latest = await inspectDeviceCache(key); signal?.throwIfAborted();
  if (!selected.state.lease || !latest.lease || !sameDeviceCacheLease(selected.state.lease, latest.lease))
    throw new DeviceCacheError('stale', '快取在讀取期間已更新或清除，沒有將舊內容放入工作區。');
  return { state: selected.state, dataset };
}
