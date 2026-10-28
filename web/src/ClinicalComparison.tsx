// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import { useMemo, useState } from 'react';
import type { LabRecord } from './api';
import { useAnalysis } from './AnalysisContext';
import { dateLabel, referenceLabel } from './model';
import {
  ANALYTE_KEYS, COMPARISON_ANALYTES, CLINICAL_SOURCE_DATE, CLINICAL_SOURCES, compareInterval, comparisonInput, intervalLabel, defaultAnalysis,
  egfrCreatinine2021, estimatedGFRLabel, exactAgeOn, recordAnalyte, scenarioInterval,
  type ComparisonAnalyte, type ComparisonScenario, type ReferenceSex,
} from './clinical-comparison';

function ScenarioFields({ label, value, onChange, ageDisabled = false }: {
  label: string; value: ComparisonScenario; onChange: (next: ComparisonScenario) => void; ageDisabled?: boolean;
}) {
  return <fieldset className="scenario-fields">
    <legend>{label}</legend>
    <label className="field">檢查當時年齡（整歲）
      <input type="number" min="1" max="130" step="1" inputMode="numeric" value={value.age ?? ''}
        disabled={ageDisabled} onChange={event => onChange({ ...value, age: event.target.value === '' ? null : Number(event.target.value) })} />
    </label>
    <label className="field">公式／範本的性別條件
      <select aria-label="公式／範本的性別條件" value={value.sex} onChange={event => {
        const sex: ReferenceSex = event.target.value === 'male' ? 'male' : event.target.value === 'female' ? 'female' : 'unknown';
        onChange({ ...value, sex });
      }}><option value="unknown">未提供</option><option value="male">男性</option><option value="female">女性</option></select>
    </label>
  </fieldset>;
}
export function ClinicalComparison({ records }: { records: LabRecord[] }) {
  const [analyte, setAnalyte] = useState<ComparisonAnalyte>('creatinine');
  const [selectedId, setSelectedId] = useState('');
  const analysis = useAnalysis();
  const [saveMessage, setSaveMessage] = useState('');
  const [saving, setSaving] = useState(false);
  const [confirmed, setConfirmed] = useState(false);
  const candidates = useMemo(() => records.filter(record => recordAnalyte(record) === analyte)
    .sort((left, right) => right.exam_date.localeCompare(left.exam_date) || left.id.localeCompare(right.id)), [records, analyte]);
  if (!analysis) return null;
  const { preferences, setPreferences, birthDate, setBirthDate } = analysis;
  const person = preferences.primary, comparison = preferences.comparison;
  const setPerson = (primary: ComparisonScenario) => setPreferences({ ...preferences, primary });
  const setComparison = (value: ComparisonScenario) => setPreferences({ ...preferences, comparison: value });
  async function savePreferences() {
    if (!analysis) return;
    setSaving(true); setSaveMessage('');
    try { await analysis.save(); setSaveMessage(analysis.persisted ? '判讀情境設定已同步；生日不傳送或保存。' : '已套用至本次使用；官方登入後才能跨裝置同步。'); }
    catch (error) { setSaveMessage(error instanceof Error ? error.message : '儲存情境設定失敗，請重新嘗試。'); }
    finally { setSaving(false); }
  }
  const selected = candidates.find(record => record.id === selectedId) ?? candidates[0];
  const input = selected ? comparisonInput(selected, analyte) : null;
  const value = input?.value ?? null;
  const activePerson = birthDate
    ? { ...person, age: selected ? exactAgeOn(birthDate, selected.exam_date) : null } : person;
  const scenarios = [{ label: '本人／基準情境', context: activePerson }, { label: '比較情境', context: comparison }];
  const source = CLINICAL_SOURCES[analyte];
  return <section className="clinical-comparison" aria-labelledby="clinical-comparison-title">
    <div className="panel-heading"><div><h2 id="clinical-comparison-title">年齡與性別比較</h2>
      <p>即時比較同一結果；不修改原院數值、旗標、個人基準或既有警示。切換即時套用表格、圖表與匯出；按儲存才同步判讀情境，生日只留當頁。</p></div></div>
    <div className="scenario-controls">
      <label className="field">參考項目
        <select value={analyte} onChange={event => {
          const next = ANALYTE_KEYS.find(key => key === event.target.value);
          if (next) setAnalyte(next); setSelectedId(''); setConfirmed(false);
        }}>{ANALYTE_KEYS.map(key => <option key={key} value={key}>{COMPARISON_ANALYTES[key].name}</option>)}</select>
      </label>
      <label className="field">原始檢驗紀錄
        <select value={selected?.id ?? ''} disabled={candidates.length === 0} onChange={event => {
          setSelectedId(event.target.value); setConfirmed(false);
        }}>{candidates.length === 0 ? <option value="">尚無結果，仍可比較內建區間</option> : candidates.map(record =>
          <option key={record.id} value={record.id}>{dateLabel(record.exam_date)} · {record.result} {record.unit} · {record.institution} · {record.id.slice(-6)}</option>)}</select>
      </label>
    </div>
    <div className="scenario-controls">
      <label className="field">表格與圖表套用條件
        <select aria-label="表格與圖表套用條件" value={preferences.mode} onChange={event => {
          const mode = event.target.value;
          if (mode === 'off' || mode === 'primary' || mode === 'comparison') setPreferences({ ...preferences, mode });
        }}><option value="off">僅原院／個人基準</option><option value="primary">本人／基準情境</option><option value="comparison">比較情境（模擬）</option></select>
      </label>
      <button className="button secondary" disabled={saving} onClick={() => void savePreferences()}>{saving ? '儲存中…' : '儲存判讀情境'}</button>
      <button className="button secondary" onClick={() => { setPreferences(defaultAnalysis()); setBirthDate(''); setSaveMessage('已回到原院／個人基準；需儲存才覆寫同步設定。'); }}>還原情境預設</button>
    </div>
    {saveMessage && <p role="status">{saveMessage}</p>}
    {selected && <p className="helper-text">原始結果：{selected.result} {selected.unit}；原院參考：{referenceLabel(selected)}；原院旗標：{selected.official_flag || '未提供'}。</p>}
    {selected && input && <details className="scenario-provenance"><summary>查看這筆比較的依據與限制</summary>
      <p>原始項目：{selected.item_name}；醫令代碼：{selected.order_code}；來源單位：{input.sourceUnit || '未提供'}；情境單位：{input.comparisonUnit}。</p>
      <p>{input.reason}</p>
      <p>具名範本方法：{COMPARISON_ANALYTES[analyte].method}。這筆原院檢驗的方法、檢體與前處理條件未由程式推定相同；不因此把跨院數值視為可直接互換。</p>
      {analyte === 'creatinine' && <p>CKD-EPI 2021 另需成人年齡、公式性別條件與下方明確確認；只在最後顯示時四捨五入至整數，計算中不先捨去精度。原院 eGFR 仍保留原值。</p>}
    </details>}
    <details className="scenario-birthdate"><summary>選填生日，依所選檢查日期計算本人當時年齡</summary>
      <label className="field">出生日期（不保存、不傳送）<input type="date" value={birthDate} onChange={event => setBirthDate(event.target.value)} /></label>
      {birthDate && activePerson.age === null && <p role="status">缺少有效檢查日期，或年齡不在本功能範圍；不推定生日或年齡。</p>}
      {birthDate && <button className="button secondary" onClick={() => setBirthDate('')}>清除生日，改用手動情境年齡</button>}
    </details>
    {!birthDate && <p className="helper-text">手動年齡視為固定比較情境，套用所有選定日期；要依每次採檢當時年齡計算，請在當頁選填生日。同步設定不包含生日。</p>}
    <div className="scenario-grid">
      <ScenarioFields label="本人／基準情境" value={activePerson} onChange={setPerson} ageDisabled={birthDate !== ''} />
      <ScenarioFields label="比較情境" value={comparison} onChange={setComparison} />
    </div>
    {analyte === 'creatinine' && <label className="checkbox-label scenario-confirmation"><input type="checkbox" checked={confirmed}
      onChange={event => setConfirmed(event.target.checked)} />
      <span>另作 CKD-EPI 2021 情境估算：我已確認所選檢驗為 IDMS 標準化血液肌酸酐、腎功能穩定，且此成人公式適用。不用來自行調整藥量。</span>
    </label>}
    <div className="scenario-grid" aria-live="polite" aria-atomic="true">
      {scenarios.map(({ label, context }) => {
        const interval = scenarioInterval(analyte, context);
        const estimated = egfrCreatinine2021(value, context, confirmed);
        return <article className="scenario-result" key={label}><h3>{label}</h3>
          <p>{context.age ?? '未提供'} 歲 · {context.sex === 'male' ? '男性' : context.sex === 'female' ? '女性' : '未提供性別'}</p>
          <p className="scenario-range">{intervalLabel(interval)}</p>
          <p>{compareInterval(selected, analyte, interval)}</p>
          {interval && <small>{interval.scope}；已列界限含等號。方法：{interval.method}。</small>}
          {analyte === 'creatinine' && <div className="scenario-estimate"><b>{estimated.value === null ? 'eGFR 未計算' : `情境 eGFR 約 ${estimatedGFRLabel(estimated.value)}`}</b><p>{estimated.reason}</p></div>}
        </article>;
      })}
    </div>
    <p className="helper-text">範本屬具名院所與方法，不是臺灣通用正常值。院所方法未核對時僅作情境對照；不是本人病情預測。未收錄項目不自造分齡界限。</p>
    <p className="helper-text"><a href={source} target="_blank" rel="noreferrer">成大檢驗範本</a> · <a href={CLINICAL_SOURCES.egfr} target="_blank" rel="noreferrer">NIDDK 公式與限制</a> · <a href={CLINICAL_SOURCES.egfr_reporting} target="_blank" rel="noreferrer">eGFR 顯示精度依據</a> · 範本來源查核 {CLINICAL_SOURCE_DATE}</p>
  </section>;
}
