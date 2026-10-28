// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import { useEffect, useRef, useState } from 'react';
import { useAnalysis } from './AnalysisContext';
import { downloadText, dateLabel } from './model';
import { INTAKE_CATEGORIES, parseIntakeRecords, decodeIntakeFile, intakeStorageKey, intakeOnDate, type IntakeRecord, type IntakeCategory } from './personal-records';
const blank = (): IntakeRecord => ({ id: '', name: '', category: 'other', amount: '', start: '', end: '', note: '' });
function load(key: string | null): { records: IntakeRecord[]; remember: boolean; error: string } {
  if (!key) return { records: [], remember: false, error: '' };
  try { const raw = localStorage.getItem(key); return { records: raw ? parseIntakeRecords(raw) : [], remember: raw !== null, error: '' }; }
  catch (error) { return { records: [], remember: false, error: error instanceof Error ? `本機紀錄未載入：${error.message}` : '瀏覽器無法讀取本機紀錄。' }; }
}
export function PersonalRecords({ profileKey }: { profileKey: string }) {
  const importInput = useRef<HTMLInputElement>(null);
  const importGeneration = useRef(0);
  useEffect(() => () => { importGeneration.current += 1; }, []);
  const key = intakeStorageKey(profileKey);
  const [initial] = useState(() => load(key));
  const [records, setRecords] = useState(initial.records), [remember, setRemember] = useState(initial.remember);
  const [error, setError] = useState(initial.error), [editing, setEditing] = useState<IntakeRecord | null>(null);
  const analysis = useAnalysis();
  function commit(next: IntakeRecord[], keep = remember) {
    // A later edit/clear/consent change supersedes any outstanding file read.
    importGeneration.current += 1;
    try {
      const checked = parseIntakeRecords(JSON.stringify(next));
      if (key) { if (keep) localStorage.setItem(key, JSON.stringify(checked)); else localStorage.removeItem(key); }
      setRecords(checked); setRemember(keep); setError(''); return true;
    } catch (failure) { setError(failure instanceof Error ? failure.message : '本機紀錄儲存失敗；尚未更新。'); return false; }
  }
  async function importRecords(file: File | undefined) {
    if (!file) return;
    const generation = ++importGeneration.current;
    if (importInput.current) importInput.current.value = '';
    try {
      if (file.size > 768_000) throw new Error('個人紀錄檔案過大。');
      const source = new Uint8Array(await file.arrayBuffer());
      if (generation !== importGeneration.current) return;
      const next = decodeIntakeFile(source);
      if (records.length && !window.confirm('以匯入檔取代目前個人品項？可先匯出備份。')) return;
      if (commit(next)) setEditing(null);
    } catch (failure) {
      if (generation === importGeneration.current) setError(failure instanceof Error ? failure.message : '匯入失敗；原紀錄仍保留。');
    }
  }
  function saveEdit() {
    if (!editing) return;
    const record = { ...editing, name: editing.name.trim(), id: editing.id || crypto.randomUUID() };
    const next = editing.id ? records.map(row => row.id === editing.id ? record : row) : [...records, record];
    if (commit(next)) setEditing(null);
  }
  const selectedDate = analysis?.selectedDate ?? '';
  const matching = intakeOnDate(records, selectedDate);
  return <details className="panel analysis-tools personal-records"><summary>選配：個人用藥／中藥／補充品與其他品項</summary>
    <p className="helper-text">僅自己記錄，不是官方處方或服藥證明。內容不傳送伺服器、不參與跨裝置設定同步；未啟用本機保存時，離開本頁即不保留。</p>
    <label className="checkbox-label"><input type="checkbox" checked={remember} disabled={!key} onChange={event => { commit(records, event.target.checked); }} />在此私人裝置保存這位使用者的紀錄</label>
    {!key && <p>官方登入後才可將紀錄存入該使用者的本機空間；示範與匯入模式僅暫存。</p>}
    {error && <p role="alert" className="error-text">{error}</p>}
    <input type="file" accept=".json,application/json" hidden ref={importInput} onChange={event => { void importRecords(event.target.files?.[0]); }} />
    <div className="scenario-controls"><button className="button secondary" onClick={() => importInput.current?.click()}>匯入個人品項JSON</button><button className="button secondary" onClick={() => setEditing(blank())} disabled={records.length >= 200}>新增品項</button>
      <button className="button secondary" disabled={!records.length} onClick={() => downloadText(JSON.stringify(records, null, 2), 'healthbank-個人品項.json', 'application/json')}>匯出本機紀錄</button>
      <button className="button secondary" disabled={!records.length} onClick={() => { if (window.confirm('清除此瀏覽器中這位使用者的全部個人品項？伺服器沒有備份；可先匯出。')) { commit([], false); setEditing(null); } }}>清除全部</button>
    </div>
    {editing && <div className="form-stack intake-editor"><label className="field">名稱（必填）<input value={editing.name} maxLength={128} onChange={event => setEditing({ ...editing, name: event.target.value })} /></label>
      <div className="scenario-grid"><label className="field">類別<select value={editing.category} onChange={event => { const category = event.target.value; if (Object.hasOwn(INTAKE_CATEGORIES, category)) setEditing({ ...editing, category: category as IntakeCategory }); }}>
        {Object.entries(INTAKE_CATEGORIES).map(([value, label]) => <option value={value} key={value}>{label}</option>)}</select></label>
        <label className="field">用量／頻率（選填）<input value={editing.amount} maxLength={128} onChange={event => setEditing({ ...editing, amount: event.target.value })} /></label>
        <label className="field">開始日期（選填）<input type="date" value={editing.start} onChange={event => setEditing({ ...editing, start: event.target.value })} /></label>
        <label className="field">停止日期（選填）<input type="date" value={editing.end} onChange={event => setEditing({ ...editing, end: event.target.value })} /></label></div>
      <label className="field">備註（選填；例如廠牌或約略期間）<textarea maxLength={512} value={editing.note} onChange={event => setEditing({ ...editing, note: event.target.value })} /></label>
      <div className="dialog-actions"><button className="button secondary" onClick={() => setEditing(null)}>取消</button><button className="button primary" onClick={saveEdit}>保存這筆紀錄</button></div>
    </div>}
    {selectedDate && <p role="status">趨勢選定 {dateLabel(selectedDate)}：{matching.length ? matching.map(row => row.name).join('、') : '沒有符合已填期間的個人品項'}。時間重疊不代表因果；未填開始日期者不推定。</p>}
    <div className="intake-list">{records.map(record => <article key={record.id} className="intake-item"><b>{record.name}</b><span>{INTAKE_CATEGORIES[record.category]} · {record.amount || '未填用量'}</span>
      <span>{record.start ? dateLabel(record.start) : '未填開始日'} — {record.end ? dateLabel(record.end) : '未填停止日'}</span><p>{record.note}</p>
      <div><button className="button secondary" onClick={() => setEditing(record)}>編輯</button><button className="button secondary" onClick={() => { if (window.confirm(`刪除「${record.name}」這筆本機紀錄？`)) commit(records.filter(row => row.id !== record.id)); }}>刪除</button></div>
    </article>)}</div>
  </details>;
}
