// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import { useEffect, useLayoutEffect, useRef, useState } from 'react';
import type { ReportResponse, Session } from './api';
import { readableError } from './api';
import type { DataCategory, DownloadedAttachment, HealthDataSnapshot } from './health-data';
import { exportDataset, type DatasetProducer, type ImportedDataset } from './health-dataset';
import { clearAllDeviceCaches, clearDeviceCache, DeviceCacheError, enableDeviceCache, inspectDeviceCache, loadDeviceCache,
  sameDeviceCacheLease, saveDeviceCache, verifiedCacheProfile, watchDeviceCache,
  type DeviceCacheLease, type DeviceCacheMetadata, type DeviceCacheState } from './device-health-cache';

const DISABLED: DeviceCacheState = { enabled: false, lease: null, metadata: null };
type Phase = 'idle' | 'loading' | 'changing' | 'refreshing';
interface Inputs {
  report: ReportResponse; snapshot: HealthDataSnapshot; attachments: DownloadedAttachment[];
  categories: DataCategory[]; producer: DatasetProducer;
}
interface Options extends Inputs {
  session: Session; ready: boolean; blocked: boolean; cacheView: boolean; loginRequest: number;
  restore: (dataset: ImportedDataset, metadata: DeviceCacheMetadata, lease: DeviceCacheLease, signal: AbortSignal) => Promise<void>;
  clearWorkspace: (key: string, scope: 'cache_only' | 'all_reports', signal: AbortSignal) => Promise<void>;
  initialFetch: (signal: AbortSignal) => Promise<void>;
  refresh: (signal: AbortSignal) => Promise<void>;
}
function sameInput(first: Inputs | null, second: Inputs): boolean {
  return first?.report === second.report && first.snapshot === second.snapshot && first.attachments === second.attachments &&
    first.categories === second.categories && JSON.stringify(first.producer) === JSON.stringify(second.producer);
}
function aborted(signal: AbortSignal): boolean { return signal.aborted; }

/** Medical storage remains opt-in and separate from session/login-field memory. */
export function useDeviceHealthCache(options: Options) {
  const key = verifiedCacheProfile(options.session);
  const identity = `${options.session.mode}:${options.session.csrf_token ?? ''}:${key ?? ''}`;
  const [owner, setOwner] = useState('');
  const [state, setState] = useState<DeviceCacheState>(DISABLED);
  const [phase, setPhase] = useState<Phase>('idle');
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const latest = useRef(options), currentIdentity = useRef(identity);
  const lease = useRef<DeviceCacheLease | null>(null), lastSaved = useRef<Inputs | null>(null);
  const load = useRef<AbortController | null>(null), write = useRef<AbortController | null>(null), mutation = useRef<AbortController | null>(null);
  const paused = useRef(false), reconciling = useRef(false);
  useLayoutEffect(() => { latest.current = options; currentIdentity.current = identity; }, [options, identity]);

  function publish(next: DeviceCacheState) { lease.current = next.lease; setState(next); }
  function cancelPending() { load.current?.abort(); write.current?.abort(); load.current = null; write.current = null; }

  useEffect(() => {
    const controller = new AbortController();
    load.current?.abort(); write.current?.abort(); mutation.current?.abort();
    load.current = controller; write.current = null; mutation.current = null;
    lease.current = null; paused.current = false; lastSaved.current = null;
    if (!options.ready) return () => controller.abort();
    async function initialize() {
      try {
        if (!key) {
          // No database read, user enumeration or retained lease for anonymous,
          // imported or synthetic sessions.
          await Promise.resolve();
          controller.signal.throwIfAborted();
          publish(DISABLED); setOwner(identity); setPhase('idle'); setError(''); setNotice(''); return;
        }
        const loaded = await loadDeviceCache(key, controller.signal);
        controller.signal.throwIfAborted();
        if (currentIdentity.current !== identity) return;
        publish(loaded.state); setPhase('loading'); setError(''); setNotice('');
        if (loaded.dataset && loaded.state.metadata && loaded.state.lease) {
          await latest.current.restore(loaded.dataset, loaded.state.metadata, loaded.state.lease, controller.signal);
          controller.signal.throwIfAborted();
          setNotice('已開啟這位使用者在此裝置保存的資料；本次登入未自動重新拉取全部歷史。');
        } else {
          await latest.current.clearWorkspace(key, 'cache_only', controller.signal);
          controller.signal.throwIfAborted();
          if (options.loginRequest > 0) await latest.current.initialFetch(controller.signal);
        }
      } catch (failure) {
        if (!controller.signal.aborted && currentIdentity.current === identity) {
          paused.current = true;
          setError(`本機快取未能載入。${readableError(failure)} 請使用重新取得，或先清除此使用者快取。`);
          if (key) {
            try {
              const current = await inspectDeviceCache(key); controller.signal.throwIfAborted();
              if (currentIdentity.current === identity) publish(current);
            } catch { /* Failed storage must not become an implicit opt-in. */ }
          }
          if (failure instanceof DeviceCacheError && failure.code === 'unavailable' && options.loginRequest > 0) {
            // Device storage is optional; a browser refusing IndexedDB must
            // still be able to use the verified, transient official workflow.
            try { await latest.current.initialFetch(controller.signal); }
            catch (fetchFailure) { if (!aborted(controller.signal)) setError(readableError(fetchFailure)); }
          }
        }
      } finally {
        if (!controller.signal.aborted && currentIdentity.current === identity) { setOwner(identity); setPhase('idle'); load.current = null; }
      }
    }
    void initialize();
    return () => { controller.abort(); write.current?.abort(); mutation.current?.abort(); };
  }, [identity, key, options.ready, options.loginRequest]);

  useEffect(() => {
    if (!key || !options.ready) return;
    const profileKey = key;
    const controller = new AbortController();
    const operationActive = () => mutation.current !== null || load.current !== null;
    async function reconcile() {
      if (aborted(controller.signal) || reconciling.current || operationActive()) return;
      reconciling.current = true;
      try {
        const previous = lease.current, next = await inspectDeviceCache(profileKey);
        if (aborted(controller.signal) || currentIdentity.current !== identity || operationActive()) return;
        if (!previous || next.lease && sameDeviceCacheLease(previous, next.lease)) return;
        write.current?.abort(); write.current = null; paused.current = true;
        publish(next);
        const cleared = !next.lease || previous.global_epoch !== next.lease.global_epoch || previous.profile_epoch !== next.lease.profile_epoch;
        if (cleared) {
          await latest.current.clearWorkspace(profileKey, 'cache_only', controller.signal);
          controller.signal.throwIfAborted();
          setNotice('本機快取已在另一個分頁清除或重新取得；本頁的舊快取已移除，沒有自動重新建立。');
        } else setNotice('另一個分頁已保存較新的快取。本頁仍保留目前畫面；重新取得前，不會覆蓋另一個分頁的版本。');
      } catch (failure) {
        if (!aborted(controller.signal)) { paused.current = true; setError(readableError(failure)); }
      } finally { reconciling.current = false; }
    }
    const unwatch = watchDeviceCache(event => { if (event.kind === 'all' || event.profile_key === key) void reconcile(); });
    const onFocus = () => { void reconcile(); };
    window.addEventListener('focus', onFocus);
    return () => { controller.abort(); unwatch(); window.removeEventListener('focus', onFocus); };
  }, [identity, key, options.ready]);

  const producerKey = JSON.stringify(options.producer);
  useEffect(() => {
    if (!key || owner !== identity || !state.enabled || !lease.current || phase !== 'idle' || options.blocked || options.cacheView || paused.current ||
      !options.report.fetched_at && !Object.keys(options.snapshot.reports).length || sameInput(lastSaved.current, options)) return;
    const controller = new AbortController(), expected = lease.current, selected: Inputs = { report: options.report, snapshot: options.snapshot,
      attachments: options.attachments, categories: options.categories, producer: options.producer };
    write.current?.abort(); write.current = controller;
    const timer = window.setTimeout(() => {
      async function save() {
        try {
          const content = await exportDataset(selected.report, selected.snapshot, selected.attachments, selected.categories, false, selected.producer);
          controller.signal.throwIfAborted();
          // Do not change phase here: saving must remain cancellable by clear or
          // logout, and an effect cleanup must not abort its own write.
          const next = await saveDeviceCache(expected, content, controller.signal);
          controller.signal.throwIfAborted();
          if (currentIdentity.current !== identity) return;
          lastSaved.current = selected; publish(next); setError(''); setNotice('目前已取得的資料已保存在此裝置。');
        } catch (failure) {
          if (!controller.signal.aborted && currentIdentity.current === identity) {
            paused.current = true;
            setError(`這次資料尚未保存至本機快取。${readableError(failure)}`);
          }
        } finally { if (write.current === controller) write.current = null; }
      }
      void save();
    }, 400);
    return () => { window.clearTimeout(timer); controller.abort(); if (write.current === controller) write.current = null; };
  }, [identity, key, owner, state.enabled, phase, options.blocked, options.cacheView, options.report, options.snapshot,
    options.attachments, options.categories, producerKey, options.producer, options]);

  async function enable() {
    if (!key || mutation.current) return;
    const controller = new AbortController(); mutation.current = controller; cancelPending(); setPhase('changing'); setError('');
    try {
      const next = await enableDeviceCache(key); controller.signal.throwIfAborted();
      if (currentIdentity.current !== identity) return;
      paused.current = false; lastSaved.current = null; publish(next); setOwner(identity);
      setNotice('已同意此裝置保存目前官方驗證使用者的健康資料。');
    } catch (failure) { if (!controller.signal.aborted) setError(readableError(failure)); }
    finally { if (mutation.current === controller) { mutation.current = null; setPhase('idle'); } }
  }

  async function clear(all = false) {
    if (mutation.current) return;
    const controller = new AbortController(); mutation.current = controller; cancelPending(); paused.current = true;
    setPhase('changing'); setError('');
    try {
      if (all) await clearAllDeviceCaches(); else if (key) await clearDeviceCache(key);
      controller.signal.throwIfAborted();
      if (currentIdentity.current !== identity) return;
      publish(DISABLED); lastSaved.current = null;
      if (key) await latest.current.clearWorkspace(key, 'all_reports', controller.signal);
      controller.signal.throwIfAborted();
      setNotice(all ? '已清除此網站在這部裝置的所有健康快取與保存同意。自行下載的檔案須另外管理。' : '已清除此使用者的健康快取、保存同意與目前工作區。');
    } catch (failure) { if (!controller.signal.aborted) setError(readableError(failure)); }
    finally { if (mutation.current === controller) { mutation.current = null; setPhase('idle'); } }
  }

  async function refresh() {
    if (!key || mutation.current) return;
    const controller = new AbortController(); mutation.current = controller; cancelPending(); paused.current = true;
    setPhase('refreshing'); setError(''); setNotice('正在清除這位使用者的舊快取與工作區，再重新取得全部可讀取來源…');
    let cleared = false;
    try {
      const next = await clearDeviceCache(key, true); controller.signal.throwIfAborted();
      cleared = true;
      if (currentIdentity.current !== identity) return;
      publish(next); lastSaved.current = null;
      await latest.current.clearWorkspace(key, 'all_reports', controller.signal);
      controller.signal.throwIfAborted();
      await latest.current.refresh(controller.signal);
      controller.signal.throwIfAborted();
      paused.current = false; setError(''); setNotice('重新取得已結束。已成功取得的資料可查看／保存；各來源仍保留其失敗、缺漏及完整性限制。');
    } catch (failure) {
      if (!controller.signal.aborted) { paused.current = !cleared; setError(readableError(failure)); }
      else if (currentIdentity.current === identity) { paused.current = !cleared; setNotice(cleared ? '重新取得已停止；目前只保留本輪成功取得的內容，舊快取沒有復原。' : '重新取得已停止，尚未確認舊快取已清除。'); }
    } finally { if (mutation.current === controller) { mutation.current = null; setPhase('idle'); } }
  }
  const active = owner === identity ? state : DISABLED;
  return { state: active, error, notice, phase, official: key !== null,
    busy: phase === 'loading' || phase === 'changing' || phase === 'refreshing' || options.ready && key !== null && owner !== identity,
    enable, clear, refresh, cancel: () => mutation.current?.abort() };
}
