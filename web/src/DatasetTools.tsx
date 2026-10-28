// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import { useEffect, useRef, useState } from 'react';
import type { ReportResponse } from './api';
import type { DataCategory, DownloadedAttachment, HealthDataSnapshot } from './health-data';
import { categoryCoverage } from './health-data';
import { exportDataset, importDataset, MAX_DATASET_BYTES, type ImportedDataset, type DatasetProducer } from './health-dataset';
import { DatasetComparison } from './DatasetComparison';
import { downloadText } from './model';

export function DatasetTools({ report, snapshot, attachments, catalog, producer, importedFrom, disabled, onImport }: {
  report: ReportResponse;
  snapshot: HealthDataSnapshot;
  attachments: DownloadedAttachment[];
  catalog: DataCategory[];
  producer: DatasetProducer;
  importedFrom: DatasetProducer | null;
  disabled: boolean;
  onImport: (dataset: ImportedDataset) => Promise<void>;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [message, setMessage] = useState('');
  const [bom, setBOM] = useState(false);
  const input = useRef<HTMLInputElement>(null);
  const lifecycle = useRef(0);
  useEffect(() => () => { lifecycle.current += 1; }, []);
  const loaded = Object.keys(snapshot.reports);
  const pending = loaded.reduce((sum, category) => sum + categoryCoverage(snapshot, category).pendingCount, 0);
  const attachmentReferences = new Set([...Object.entries(snapshot.reports), ...Object.entries(snapshot.details).flatMap(([category, records]) => Object.values(records).map(value => [category, value] as const))]
    .flatMap(([category, source]) => source.entries.flatMap(row => row.attachments?.map(file => `${category}:${file.id}`) ?? [])));
  const retained = new Set(attachments.map(file => `${file.category}:${file.id}`));
  const absent = [...attachmentReferences].filter(key => !retained.has(key)).length;
  async function save() {
    const generation = lifecycle.current;
    setBusy(true); setError(''); setMessage('');
    try {
      const content = await exportDataset(report, snapshot, attachments, catalog, bom, producer, importedFrom ?? undefined);
      if (generation !== lifecycle.current) return;
      downloadText(content, `healthbank-dataset-${new Date().toLocaleDateString('en-CA', { timeZone: 'Asia/Taipei' })}.json`, 'application/json;charset=utf-8');
      setMessage('已產生本次資料包；所有已取得來源與明細均已包含，未取得內容仍明確標示。');
    } catch (failure) { if (generation === lifecycle.current) setError(failure instanceof Error ? failure.message : '資料包無法產生，請稍後再試。'); }
    finally { if (generation === lifecycle.current) setBusy(false); }
  }
  async function restore(file: File) {
    const generation = lifecycle.current;
    setBusy(true); setError(''); setMessage('');
    try {
      if (file.size > MAX_DATASET_BYTES) throw new Error('資料包不得超過 128 MiB。');
      const dataset = await importDataset(new Uint8Array(await file.arrayBuffer()));
      if (generation !== lifecycle.current) return;
      await onImport(dataset);
      if (generation !== lifecycle.current) return;
      setMessage('資料包已還原至本次頁面，未登入官方帳號，也未寫入設定資料庫。');
    } catch (failure) { if (generation === lifecycle.current) setError(failure instanceof Error ? failure.message : '資料包無法匯入。'); }
    finally { if (generation === lifecycle.current) { setBusy(false); if (input.current) input.current.value = ''; } }
  }
  function downloadAttachment(file: DownloadedAttachment) {
    const url = URL.createObjectURL(new Blob([new Uint8Array(file.bytes)], { type: file.content_type }));
    const link = document.createElement('a');
    link.href = url; link.download = file.name; link.click();
    setTimeout(() => URL.revokeObjectURL(url), 30_000);
  }
  return <details id="dataset-tools" className="panel analysis-tools dataset-tools" aria-label="完整資料包" aria-busy={busy}>
    <summary>保存／還原資料包 <span className="dataset-tools-count">{report.records.length} 筆檢驗 · {loaded.length} 個來源 · {attachments.length} 個附件</span></summary>
    <p>匯出全部已取得資料，不受畫面搜尋或年度篩選影響。還原會結束目前官方登入，以獨立的匯入模式開啟。</p>
    <p className="helper-text">目前 {report.records.length} 筆檢驗、{loaded.length} 個已查詢來源、{pending} 筆明細待取得；{attachments.length} 個附件已加入資料包，另有 {absent} 個附件尚未加入。已讀取回應不代表官方終身資料完整。</p>
    <div className="health-data-actions">
      <button className="button secondary" type="button" disabled={disabled || busy || !report.fetched_at && loaded.length === 0} onClick={() => { void save(); }}>{busy ? '處理中…' : '匯出全部已取得資料'}</button>
      <button className="button secondary" type="button" disabled={disabled || busy} onClick={() => input.current?.click()}>還原資料包</button>
      <input className="sr-only" ref={input} type="file" accept=".json,application/json" aria-label="選擇 Healthbank 完整資料包" disabled={disabled || busy} onChange={event => { const file = event.target.files?.[0]; if (file) void restore(file); }} />
    </div>
    <details className="dataset-format-options"><summary>檔案格式與附件限制</summary>
      <label className="checkbox-label"><input type="checkbox" checked={bom} onChange={event => setBOM(event.target.checked)} disabled={busy} />文字編輯器需要時加 UTF-8 BOM</label>
      <p className="helper-text">資料包使用 UTF-8，總量上限 128 MiB；其中檢驗內容及已加入附件各上限 32 MiB，附件附 SHA-256。大型原始檔請另存，未加入的檔案只保留來源描述，沒有宣稱已備份。檔案由您保管；登入憑證與官方操作識別不會寫入資料包。</p>
      <p className="helper-text">新版資料包 v2 記錄產生版本、連接器與判讀規則識別；仍可讀取 v1。版本資訊由檔案提供，不是身分或官方真實性的證明。檢驗重新匯入時依目前規則判讀，保留原始結果。</p>
      <p className="helper-text">本次產生版本：{producer.version}；連接器：{producer.connector_revision}。{importedFrom ? `匯入來源版本：${importedFrom.version}。` : ''}</p>
    </details>
    {attachments.length > 0 && <details><summary>已加入的原始附件（{attachments.length}）</summary><ul>{attachments.map(file => <li key={`${file.category}:${file.id}`}><button className="text-button" type="button" onClick={() => downloadAttachment(file)}>{file.name}</button> · {Math.ceil(file.bytes.length / 1024)} KiB</li>)}</ul></details>}
    <DatasetComparison disabled={disabled || busy} />
    {message && <p role="status">{message}</p>}
    {error && <p className="error-text" role="alert">{error}</p>}
  </details>;
}
