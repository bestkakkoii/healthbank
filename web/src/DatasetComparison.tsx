// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import { useEffect, useRef, useState } from 'react';
import { importDataset, MAX_DATASET_BYTES, type ImportedDataset } from './health-dataset';
import { compareDatasets, datasetDifferenceFields, type DatasetComparison as Comparison } from './dataset-comparison';
import { downloadText, formatTime } from './model';

export function DatasetComparison({ disabled }: { disabled: boolean }) {
  const [files, setFiles] = useState<{ name: string; dataset: ImportedDataset }[]>([]);
  const [result, setResult] = useState<Comparison | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [page, setPage] = useState(0);
  const input = useRef<HTMLInputElement>(null);
  const generation = useRef(0);
  useEffect(() => () => { generation.current++; }, []);
  function clear() { generation.current++; setFiles([]); setResult(null); setBusy(false); setError(''); setPage(0); if (input.current) input.current.value = ''; }
  async function load(file: File) {
    const current = generation.current;
    setBusy(true); setError('');
    try {
      if (file.size > MAX_DATASET_BYTES) throw new Error('每份資料包不得超過 128 MiB。');
      const dataset = await importDataset(new Uint8Array(await file.arrayBuffer()));
      if (generation.current !== current) return;
      const next = [...files, { name: file.name, dataset }].slice(-2);
      setFiles(next); setResult(null); setPage(0);
      if (next.length === 2 && next[0] && next[1]) {
        const compared = await compareDatasets(next[0].dataset, next[1].dataset);
        if (generation.current !== current) return;
        setResult(compared);
      }
    } catch (failure) { if (generation.current === current) setError(failure instanceof Error ? failure.message : '資料包比較失敗。'); }
    finally { if (generation.current === current) { setBusy(false); if (input.current) input.current.value = ''; } }
  }
  const pages = Math.max(1, Math.ceil((result?.differences.length ?? 0) / 50));
  const categoryLabels = new Map(files.flatMap(file => file.dataset.catalog.map(category => [category.id, category.label] as const)));
  const kindLabels = { laboratory: '檢驗紀錄', list: '來源清單', detail: '明細紀錄', attachment: '附件' };
  return <details className="dataset-comparison"><summary>比較兩份自行保存的資料包</summary>
    <p>依序選擇前次與本次檔案。只在目前頁面比較，不會取代工作區、不會登入官方，也不會將檔案內容寫入伺服器。</p>
    <div className="button-row"><button type="button" className="button secondary" disabled={disabled || busy || files.length === 2} onClick={() => input.current?.click()}>{files.length ? '選擇本次資料包' : '選擇前次資料包'}</button>
      <button type="button" className="button secondary" onClick={clear} disabled={!files.length && !busy}>清除比較檔案</button>
      <input ref={input} className="sr-only" type="file" accept=".json,application/json" aria-label="選擇比較用資料包" disabled={disabled || busy || files.length === 2} onChange={event => { const file = event.target.files?.[0]; if (file) void load(file); }} /></div>
    {files.map((file, index) => <p key={index}>{index ? '本次' : '前次'}：{file.name} · 保存時間 {formatTime(file.dataset.exportedAt)} · {file.dataset.producer?.version ?? '版本未知'}</p>)}
    {busy && <p role="status">正在驗證檔案並比較原始內容…</p>}{error && <p role="alert">{error}</p>}
    {result && <><p role="status">相同 {result.unchangedOccurrences} 筆；本次多出 {result.addedOccurrences} 筆；僅前次保有 {result.previousOnlyOccurrences} 筆。相同內容的重複筆數分別保留。</p>
      <p>「僅前次保有」表示本次檔案沒有該內容或所屬來源關係，不能推定官方刪除；更正資料或關係變動可能表現為前後各一筆差異，不自行配對為同一事件。</p>
      {result.rangeWarnings.map(warning => <p className="notice" key={warning}>{warning}</p>)}
      {result.clinicalRulesChanged && <p className="notice">兩份檔案的判讀規則版本不同；本比較只比較原始內容，不把規則變更當作新的醫療結果。</p>}
      {result.producerChanged && <details><summary>產生版本不同</summary><pre>{JSON.stringify(files.map(file => file.dataset.producer), null, 2)}</pre></details>}
      <button type="button" className="button secondary" onClick={() => downloadText(`${JSON.stringify({ schema: 'healthbank.dataset-diff.v1', comparison: result }, null, 2)}\n`, '健康映覽-資料包差異.json', 'application/json;charset=utf-8')}>下載全部差異（{result.differences.length} 組）</button>
      <div className="health-data-pagination"><button type="button" className="button secondary" disabled={page === 0} onClick={() => setPage(page - 1)}>上一頁差異</button><span>差異第 {page + 1}／{pages} 頁</span><button type="button" className="button secondary" disabled={page + 1 >= pages} onClick={() => setPage(page + 1)}>下一頁差異</button></div>
      {result.differences.slice(page * 50, (page + 1) * 50).map((row, index) => <details key={`${page}:${index}`}><summary>{row.category === 'laboratory' ? '檢驗結果' : categoryLabels.get(row.category) ?? row.category} · {kindLabels[row.kind]} · {row.date || '日期未提供'} · {row.title} · 前次 {row.before}／本次 {row.after} 筆</summary>
        <dl className="dataset-diff-fields">{datasetDifferenceFields(row).map((field, fieldIndex) => <div key={fieldIndex}><dt>{field.name}</dt><dd>{field.value || '未提供'}</dd></div>)}</dl></details>)}
    </>}
  </details>;
}
