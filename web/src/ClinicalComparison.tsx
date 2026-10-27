// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import { useMemo, useState } from 'react';
import type { LabRecord } from './api';
import { dateLabel, referenceLabel } from './model';
import {
  CLINICAL_SOURCE_DATE, CLINICAL_SOURCES, compareInterval, comparisonValue,
  egfrCreatinine2021, exactAgeOn, recordAnalyte, scenarioInterval,
  type ComparisonAnalyte, type ComparisonScenario, type ReferenceSex,
} from './clinical-comparison';

const emptyScenario = (): ComparisonScenario => ({ age: null, sex: 'unknown' });
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
      <select value={value.sex} onChange={event => {
        const sex: ReferenceSex = event.target.value === 'male' ? 'male' : event.target.value === 'female' ? 'female' : 'unknown';
        onChange({ ...value, sex });
      }}><option value="unknown">未提供</option><option value="male">男性</option><option value="female">女性</option></select>
    </label>
  </fieldset>;
}
export function ClinicalComparison({ records }: { records: LabRecord[] }) {
  const [analyte, setAnalyte] = useState<ComparisonAnalyte>('creatinine');
  const [selectedId, setSelectedId] = useState('');
  const [person, setPerson] = useState(emptyScenario);
  const [comparison, setComparison] = useState(emptyScenario);
  const [birthDate, setBirthDate] = useState('');
  const [confirmed, setConfirmed] = useState(false);
  const candidates = useMemo(() => records.filter(record => recordAnalyte(record) === analyte)
    .sort((left, right) => right.exam_date.localeCompare(left.exam_date) || left.id.localeCompare(right.id)), [records, analyte]);
  const selected = candidates.find(record => record.id === selectedId) ?? candidates[0];
  const value = selected ? comparisonValue(selected, analyte) : null;
  const activePerson = birthDate
    ? { ...person, age: selected ? exactAgeOn(birthDate, selected.exam_date) : null } : person;
  const scenarios = [{ label: '本人／基準情境', context: activePerson }, { label: '比較情境', context: comparison }];
  const source = CLINICAL_SOURCES[analyte];
  return <section className="panel clinical-comparison" aria-labelledby="clinical-comparison-title">
    <div className="panel-heading"><div><h2 id="clinical-comparison-title">年齡與性別比較</h2>
      <p>即時比較同一結果；不修改原院數值、旗標、個人基準或既有警示。此處條件僅留在當前頁面記憶體。</p></div></div>
    <div className="scenario-controls">
      <label className="field">參考項目
        <select value={analyte} onChange={event => {
          setAnalyte(event.target.value === 'alp' ? 'alp' : 'creatinine'); setSelectedId(''); setConfirmed(false);
        }}><option value="creatinine">血液肌酸酐</option><option value="alp">鹼性磷酸酶 ALP</option></select>
      </label>
      <label className="field">原始檢驗紀錄
        <select value={selected?.id ?? ''} disabled={candidates.length === 0} onChange={event => {
          setSelectedId(event.target.value); setConfirmed(false);
        }}>{candidates.length === 0 ? <option value="">尚無結果，仍可比較內建區間</option> : candidates.map(record =>
          <option key={record.id} value={record.id}>{dateLabel(record.exam_date)} · {record.result} {record.unit} · {record.institution} · {record.id.slice(-6)}</option>)}</select>
      </label>
    </div>
    {selected && <p className="helper-text">原始結果：{selected.result} {selected.unit}；原院參考：{referenceLabel(selected)}；原院旗標：{selected.official_flag || '未提供'}。</p>}
    <details className="scenario-birthdate"><summary>選填生日，依所選檢查日期計算本人當時年齡</summary>
      <label className="field">出生日期（不保存、不傳送）<input type="date" value={birthDate} onChange={event => setBirthDate(event.target.value)} /></label>
      {birthDate && activePerson.age === null && <p role="status">缺少有效檢查日期，或年齡不在本功能範圍；不推定生日或年齡。</p>}
      {birthDate && <button className="button secondary" onClick={() => setBirthDate('')}>清除生日，改用手動情境年齡</button>}
    </details>
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
          <p className="scenario-range">{interval ? `${interval.low}–${interval.high} ${interval.unit}` : '缺少條件或尚無適用範本'}</p>
          <p>{compareInterval(selected, analyte, interval)}</p>
          {interval && <small>{interval.scope}；含上下界。方法：{interval.method}。</small>}
          {analyte === 'creatinine' && <div className="scenario-estimate"><b>{estimated.value === null ? 'eGFR 未計算' : `情境 eGFR ${estimated.value.toFixed(1)}`}</b><p>{estimated.reason}</p></div>}
        </article>;
      })}
    </div>
    <p className="helper-text">範本屬具名院所與方法，不是臺灣通用正常值。院所方法未核對時僅作情境對照；不是本人病情預測。未收錄項目不自造分齡界限。</p>
    <p className="helper-text"><a href={source} target="_blank" rel="noreferrer">成大檢驗範本</a> · <a href={CLINICAL_SOURCES.egfr} target="_blank" rel="noreferrer">NIDDK 公式與限制</a> · 來源查核 {CLINICAL_SOURCE_DATE}</p>
  </section>;
}
