// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import { useEffect, useRef, useState } from 'react';
import { readableError } from './api';
import { appendAcquisitionParts, inspectAcquisitionZIP, MAX_RESTORE_PARTS, recoveredAcquisitionDataset, recoveredOriginalBlob,
  restoreAcquisitionOriginals, type AcquisitionRestorePart, type OriginalRestoreProgress, type OriginalRestoreResult } from './acquisition-restore';
import { MAX_BATCH_COLLECTION_BYTES, matchBatchFiles, parseBatchCollection, restoreBatchMember } from './batch-collection';
import type { CollectionRestoration } from './collection-restoration';
import type { ImportedDataset } from './health-dataset';
import { MAX_DATASET_BYTES } from './health-dataset';

const PAGE_SIZE = 50;
const restoreLabels: Record<OriginalRestoreResult['state'], string> = {
  not_checked: '尚未驗證', loaded: '已驗證，保留供工作區使用', verified_external: '已驗證；保留在 ZIP，未加入記憶體',
  source_unusable: '來源取得失敗／未完成，不能使用', missing_part: '尚未選到含此檔案的 ZIP', conflict: '同一原始檔出現不同內容，未採用', failed: '校驗或格式失敗，未採用',
};

export function CollectionTools({ initial, disabled, onImport, onChange }: {
  initial: CollectionRestoration | null;
  disabled: boolean;
  onImport: (dataset: ImportedDataset, collection?: CollectionRestoration) => Promise<void>;
  onChange: (collection: CollectionRestoration | null) => void;
}) {
  const [selection, setSelection] = useState(initial);
  const selectionRef = useRef(selection);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [message, setMessage] = useState('');
  const [progress, setProgress] = useState<OriginalRestoreProgress | null>(null);
  const [page, setPage] = useState(0);
  const [filter, setFilter] = useState<'all' | 'unresolved' | 'verified'>('all');
  const indexInput = useRef<HTMLInputElement>(null), membersInput = useRef<HTMLInputElement>(null), zipInput = useRef<HTMLInputElement>(null), externalInput = useRef<HTMLInputElement>(null);
  const controller = useRef<AbortController | null>(null), lifecycle = useRef(0);
  const callbacks = useRef({ onImport, onChange });
  useEffect(() => { callbacks.current = { onImport, onChange }; }, [onImport, onChange]);
  useEffect(() => () => { lifecycle.current++; controller.current?.abort(); controller.current = null; }, []);

  function publish(next: CollectionRestoration | null) {
    selectionRef.current = next; setSelection(next); callbacks.current.onChange(next);
  }
  async function operate(action: (signal: AbortSignal, valid: () => boolean) => Promise<void>) {
    if (disabled || controller.current) return;
    const current = new AbortController(), generation = lifecycle.current; controller.current = current;
    const valid = () => generation === lifecycle.current && controller.current === current && !current.signal.aborted;
    setBusy(true); setError(''); setMessage(''); setProgress(null);
    try { await action(current.signal, valid); }
    catch (failure) {
      if (generation === lifecycle.current && controller.current === current) {
        if (current.signal.aborted) setMessage('已停止本次讀取；已完成的校驗結果保留，可接續。尚未完成的檔案不算成功。');
        else setError(readableError(failure));
      }
    } finally { if (generation === lifecycle.current && controller.current === current) { controller.current = null; setBusy(false); } }
  }

  function chooseIndex(file: File) {
    void operate(async (signal, valid) => {
      if (file.size > MAX_BATCH_COLLECTION_BYTES) throw new Error('分批保存索引不得超過 2 MiB。');
      const bytes = new Uint8Array(await file.arrayBuffer()); signal.throwIfAborted();
      const manifest = parseBatchCollection(bytes); if (!valid()) return;
      publish({ kind: 'batches', manifest, files: {}, current_index: null }); setPage(0);
      setMessage('分批索引已開啟；請選取各批 JSON。只有點選某批並通過 SHA-256、查詢期間與完整性檢查後，才會取代工作區。');
    });
  }
  function chooseBatchFiles(files: File[]) {
    const current = selectionRef.current; if (current?.kind !== 'batches' || disabled || busy) return;
    try {
      const matched = matchBatchFiles(current.manifest, files, current.files);
      publish({ ...current, files: matched.files }); setError('');
      setMessage(`已對應 ${Object.keys(matched.files).length} 批檔案；${matched.unmatched} 個檔案未自動對應、${matched.ambiguous} 批檔名重複。檔名被改過時可逐批指定，開啟仍須通過 SHA-256。`);
    } catch (failure) { setError(readableError(failure)); }
  }
  function openBatch(index: number) {
    const current = selectionRef.current; if (current?.kind !== 'batches') return;
    const file = current.files[index]; if (!file) return;
    void operate(async (signal, valid) => {
      const dataset = await restoreBatchMember(current.manifest, index, file, signal); if (!valid()) return;
      await callbacks.current.onImport(dataset, { ...current, current_index: index });
    });
  }
  function chooseZIPs(files: File[]) {
    void operate(async (signal, valid) => {
      if (!files.length || files.length > MAX_RESTORE_PARTS) throw new Error('每次最多選擇 1,000 個原始檔 ZIP 部分。');
      const current = selectionRef.current, previous = current?.kind === 'originals' ? current : null;
      const additions: AcquisitionRestorePart[] = [];
      for (const [index, file] of files.entries()) {
        if (!valid()) return;
        setMessage(`正在檢查 ZIP 索引 ${index + 1}／${files.length}：${file.name}`);
        additions.push(await inspectAcquisitionZIP(file, signal));
        // Enforce aggregate bounds while inspecting, before retaining another
        // potentially large manifest. No partial selection is installed on error.
        appendAcquisitionParts(previous, additions);
      }
      if (!valid()) return;
      publish(appendAcquisitionParts(previous, additions)); setPage(0);
      setMessage('ZIP 索引已讀取。原始檔仍需逐檔驗證，選入檔案不等於已還原。不同部分及補取包只依同一份完整資料 JSON 的 SHA-256 與原始附件識別合併。');
    });
  }
  function recoverZIPs() {
    const current = selectionRef.current; if (current?.kind !== 'originals') return;
    void operate(async (signal, valid) => {
      const restored = await restoreAcquisitionOriginals(current, signal, next => { if (valid()) publish(next); }, next => { if (valid()) setProgress(next); });
      if (!valid()) return;
      await callbacks.current.onImport(recoveredAcquisitionDataset(restored), restored);
    });
  }
  function saveOriginal(result: OriginalRestoreResult) {
    const current = selectionRef.current; if (current?.kind !== 'originals') return;
    try {
      const url = URL.createObjectURL(recoveredOriginalBlob(current, result)), link = document.createElement('a');
      link.href = url; link.download = result.name; link.click();
      setTimeout(() => URL.revokeObjectURL(url), 30_000);
      setMessage('已啟動原始檔另存；瀏覽器管理下載，請自行確認檔案保存成功。大型檔使用原始 ZIP 的範圍，不整包讀入記憶體。');
    } catch (failure) { setError(readableError(failure)); }
  }

  const results = selection?.kind === 'originals' ? Object.values(selection.results) : [];
  const filtered = results.filter(item => filter === 'all' || (['loaded', 'verified_external'].includes(item.state) ? filter === 'verified' : filter === 'unresolved'));
  const batches = selection?.kind === 'batches' ? selection.manifest.batches : [];
  const count = selection?.kind === 'batches' ? batches.length : filtered.length;
  const lastPage = Math.max(0, Math.ceil(count / PAGE_SIZE) - 1), shownPage = Math.min(page, lastPage);
  const loaded = results.filter(item => item.state === 'loaded').length, external = results.filter(item => item.state === 'verified_external').length;

  return <details className="collection-tools" open={selection !== null || undefined} aria-busy={busy}>
    <summary>還原分批資料與原始檔 ZIP</summary>
    <p>分批索引可讓您逐批開啟各自的原始資料；原始檔 ZIP 可直接驗證及還原。這些操作讀取您選擇的檔案，檔案不會上傳到伺服器保存。</p>
    <div className="health-data-actions">
      <button type="button" className="button secondary" disabled={disabled || busy} onClick={() => indexInput.current?.click()}>選擇分批保存索引</button>
      <button type="button" className="button secondary" disabled={disabled || busy} onClick={() => zipInput.current?.click()}>{selection?.kind === 'originals' ? '加入其他 ZIP 部分／補取包' : '選擇原始檔 ZIP'}</button>
      {busy && <button type="button" className="button secondary" onClick={() => controller.current?.abort()}>停止檔案讀取</button>}
      {selection && <button type="button" className="button quiet" disabled={busy} onClick={() => { publish(null); setProgress(null); setMessage('已釋放這次選取的索引與檔案；目前已還原的工作區資料仍保留。'); setError(''); }}>清除選取的本機檔案</button>}
    </div>
    <input className="sr-only" ref={indexInput} type="file" accept=".json,application/json" aria-label="選擇分批保存索引 JSON" disabled={disabled || busy}
      onChange={event => { const file = event.target.files?.[0]; event.target.value = ''; if (file) chooseIndex(file); }} />
    <input className="sr-only" ref={zipInput} type="file" multiple accept=".zip,application/zip" aria-label="選擇原始檔 ZIP 部分" disabled={disabled || busy}
      onChange={event => { const files = Array.from(event.target.files ?? []); event.target.value = ''; if (files.length) chooseZIPs(files); }} />
    {selection?.kind === 'batches' && <section className="collection-summary" aria-label="分批保存還原">
      <h4>分批查詢：{selection.manifest.query.from} 至 {selection.manifest.query.to}</h4>
      <p>{selection.manifest.batches.length} 批；{selection.manifest.batches.filter(item => item.dataset).length} 批有匯出描述，{Object.keys(selection.files).length} 批已選取檔案。{selection.current_index ? `目前開啟第 ${selection.current_index} 批。` : ''}</p>
      <button type="button" className="button secondary" disabled={disabled || busy} onClick={() => membersInput.current?.click()}>選取各批資料 JSON</button>
      <input className="sr-only" ref={membersInput} type="file" multiple accept=".json,application/json" aria-label="選擇分批資料 JSON 檔案" disabled={disabled || busy}
        onChange={event => { const files = Array.from(event.target.files ?? []); event.target.value = ''; chooseBatchFiles(files); }} />
      <p className="helper-text">每次只載入一批，保留該批查詢、未完成狀態與重複紀錄；不把不同批次的本機識別視為同一筆醫療紀錄。保存確認由當時的使用者提供，仍需核對檔案內容。</p>
      <ol className="collection-file-list" start={shownPage * PAGE_SIZE + 1}>{batches.slice(shownPage * PAGE_SIZE, (shownPage + 1) * PAGE_SIZE).map(batch => <li key={batch.index}>
        <div><strong>{batch.query.from} 至 {batch.query.to}</strong><p>{batch.dataset ? `${batch.dataset.acquisition.source_entries} 筆來源紀錄、${batch.dataset.acquisition.laboratory_records} 筆檢驗；${batch.dataset.acquisition.acquired ? '可讀取項目已處理' : '仍有未完成項目'}；${batch.dataset.saved_acknowledged ? '當時已確認保存' : '當時未確認保存'}。` : '這批沒有匯出描述，無法視為已保存。'}</p>
          {batch.dataset && <p className="helper-text">{selection.files[batch.index]?.name ?? batch.dataset.name} · {selection.files[batch.index] ? '已選取，開啟時校驗' : '尚未選取檔案'}</p>}</div>
        {batch.dataset && <div className="health-data-actions">
          <button type="button" className="button secondary" disabled={disabled || busy || !selection.files[batch.index]} onClick={() => openBatch(batch.index)}>{selection.current_index === batch.index ? '重新開啟此批' : '開啟此批資料'}</button>
          <label className="collection-file-picker">指定此批 JSON<input type="file" accept=".json,application/json" aria-label={`指定第 ${batch.index} 批 JSON`} disabled={disabled || busy} onChange={event => {
            const file = event.target.files?.[0]; event.target.value = '';
            if (file) { publish({ ...selection, files: { ...selection.files, [batch.index]: file } }); setError(''); }
          }} /></label>
        </div>}
      </li>)}</ol>
    </section>}
    {selection?.kind === 'originals' && <section className="collection-summary" aria-label="原始檔 ZIP 還原">
      <h4>{selection.parts.length} 個 ZIP 部分／補取包</h4>
      <p>已加入記憶體 {loaded} 個附件；{external} 個大型或超過本次保留上限的檔案已驗證，可直接從 ZIP 另存；{results.length - loaded - external} 個仍未完成。{!results.length ? '請開始驗證以核對完整 JSON 及所有原始檔關聯。' : ''}</p>
      {Object.values(selection.checked).includes('failed') && <p className="error-text">所選 ZIP 有 {Object.values(selection.checked).filter(state => state === 'failed').length} 份原始檔副本校驗失敗。若同一附件另有校驗成功的補取副本，只採用成功副本；損壞副本仍明確列為失敗。</p>}
      {!selection.parts.some(part => part.manifest.dataset.included) && <>
        <p>這些 ZIP 不包含完整資料 JSON。請同時提供另存的 healthbank.dataset.v2.json（可改名，仍以長度與 SHA-256 核對）。</p>
        <button type="button" className="button secondary" disabled={disabled || busy} onClick={() => externalInput.current?.click()}>選擇對應的完整資料 JSON</button>
        <input className="sr-only" ref={externalInput} type="file" accept=".json,application/json" aria-label="選擇 ZIP 對應的完整資料 JSON" disabled={disabled || busy} onChange={event => {
          const file = event.target.files?.[0]; event.target.value = '';
          if (file && file.size <= MAX_DATASET_BYTES) { publish({ ...selection, external: file, dataset: null, retained: [], results: {}, checked: {} }); setError(''); }
          else if (file) setError('完整資料 JSON 不得超過 128 MiB。');
        }} />
        {selection.external && <p>已選取：{selection.external.name}</p>}
      </>}
      <button type="button" className="button primary" disabled={disabled || busy || !selection.external && !selection.parts.some(part => part.manifest.dataset.included)} onClick={recoverZIPs}>{results.length ? '接續校驗並還原原始資料' : '驗證原始檔並還原資料'}</button>
      <p className="helper-text">逐檔檢查 SHA-256、ZIP CRC32 及格式。附件最多保留 32 MiB／10,000 個；其餘驗證成功檔案仍在您選取的 ZIP，可另存。來源失敗、缺少部分、內容衝突或校驗失敗均保留狀態，不冒充成功。原始檔傳輸完成不能證明官方終身資料完整。</p>
      <details><summary>已選 ZIP 與原始取得狀態</summary><ul>{selection.parts.map((part, index) => <li key={`${part.manifest.plan.id}:${part.manifest.plan.index}`}>
        {part.file.name}：第 {part.manifest.plan.index}／{part.manifest.plan.total} 部分，{part.manifest.files.filter(file => file.usable).length}／{part.manifest.files.length} 個來源檔案取得成功；{part.manifest.acquisition.statuses.filter(status => status.code || !['ready', 'source_refreshed'].includes(status.state)).length} 項取得限制／未完成狀態保留在 manifest.json。
        <button type="button" className="text-button" disabled={busy || disabled} onClick={() => {
          const remaining = selection.parts.filter((_, ordinal) => ordinal !== index);
          publish(remaining.length ? { ...appendAcquisitionParts(null, remaining), external: selection.external } : null);
          setProgress(null); setMessage('已從本次選取中移除這個 ZIP；請重新驗證其餘部分，工作區已開啟資料仍保留。');
        }}>移除此 ZIP 部分</button>
      </li>)}</ul></details>
      {results.length > 0 && <>
        <label className="field">原始檔顯示<select value={filter} onChange={event => { setFilter(event.target.value === 'unresolved' ? 'unresolved' : event.target.value === 'verified' ? 'verified' : 'all'); setPage(0); }}>
          <option value="all">全部原始檔</option><option value="unresolved">未完成／缺少／衝突</option><option value="verified">校驗成功</option>
        </select></label>
        <ul className="collection-file-list">{filtered.slice(shownPage * PAGE_SIZE, (shownPage + 1) * PAGE_SIZE).map(result => <li key={`${result.category}:${result.id}`}>
          <div><strong>{result.name}</strong><p>{restoreLabels[result.state]} · {Math.ceil(result.size / 1024)} KiB</p></div>
          {['loaded', 'verified_external'].includes(result.state) && result.path && <button type="button" className="button secondary" disabled={busy} onClick={() => saveOriginal(result)}>另存此原始檔</button>}
        </li>)}</ul>
      </>}
    </section>}
    {count > PAGE_SIZE && <nav className="health-data-actions" aria-label="還原檔案分頁"><button type="button" className="button secondary" disabled={shownPage === 0} onClick={() => setPage(shownPage - 1)}>上一頁檔案</button>
      <span>第 {shownPage + 1}／{lastPage + 1} 頁</span><button type="button" className="button secondary" disabled={shownPage === lastPage} onClick={() => setPage(shownPage + 1)}>下一頁檔案</button></nav>}
    {progress && <p role="status">{progress.phase === 'dataset' ? '核對完整資料 JSON' : `原始檔 ${progress.done}／${progress.total}`}：{progress.name} · {Math.ceil(progress.bytes / 1024 / 1024)}／{Math.ceil(progress.size / 1024 / 1024)} MiB</p>}
    {message && <p role="status">{message}</p>}
    {error && <p role="alert" className="error-text">{error}</p>}
  </details>;
}
