// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import { portableReportJSON, portableReportXML } from './portable-report';
import { useMemo, useState } from 'react';
import type { LabRecord, Settings } from './api';
import { useAnalysis } from './AnalysisContext';
import { dateLabel, downloadText, finiteExtent, type MeasurementGroup } from './model';
import { assessLipids } from './lipid-analysis';
import { lipidSourceMatches } from './analysis-summary';
import { activeScenario, intervalLabel, referenceForRecord, recordScenarioStatus } from './clinical-comparison';
import { TrendChart } from './TrendChart';
import { Copyright } from './ui';
import { laboratoryDateSummary } from './coverage-status';
import { gradingContextText } from './grading-context';

export function MultiIndicator({ groups, settings, onDetail }: {
  groups: MeasurementGroup[]; settings: Settings; onDetail: (record: LabRecord) => void;
}) {
  const [selected, setSelected] = useState<string[]>([]);
  const chosen = groups.filter(group => selected.includes(group.id));
  const extent = finiteExtent(chosen.flatMap(group => group.records.map(record => Date.parse(`${record.exam_date}T00:00:00Z`))));
  return <details className="panel analysis-tools"><summary>多指標同步比較（選擇最多3項）</summary>
    <label className="field">新增比較項目<select value="" onChange={event => {
      const id = event.target.value;
      if (id && selected.length < 3 && !selected.includes(id)) setSelected([...selected, id]);
    }} disabled={selected.length >= 3}>
      <option value="">選擇項目</option>{groups.filter(group => !selected.includes(group.id)).map(group => <option key={group.id} value={group.id}>{group.name} · {group.unit}</option>)}
    </select></label>
    <p className="helper-text">共用日期範圍，各自保留單位與縱軸；選擇資料點時以同一天對照，不補造缺少的量測。</p>
    {chosen.map(group => <section className="multi-indicator-item" key={group.id}>
      <button className="button secondary" onClick={() => setSelected(selected.filter(id => id !== group.id))}>移除 {group.name}</button>
      <TrendChart group={group} override={settings.overrides[group.configurationKey]} onDetail={onDetail} timeDomain={extent ?? undefined} />
    </section>)}
  </details>;
}
export function LipidCalculation({ records }: { records: LabRecord[] }) {
  const [totalId, setTotalId] = useState(''), [hdlId, setHdlId] = useState(''), [confirmed, setConfirmed] = useState(false);
  const [tgId, setTgId] = useState(''), [estimate, setEstimate] = useState(false);
  const [typeIII, setTypeIII] = useState(false);
  const triglycerides = records.filter(record => lipidSourceMatches(record, 'triglyceride'));
  const totals = records.filter(record => lipidSourceMatches(record, 'total'));
  const hdls = records.filter(record => lipidSourceMatches(record, 'hdl'));
  const total = totals.find(record => record.id === totalId), hdl = hdls.find(record => record.id === hdlId);
  const tg = triglycerides.find(record => record.id === tgId);
  const result = assessLipids(total, hdl, tg, confirmed, estimate, typeIII);
  const options = (rows: LabRecord[]) => rows.map(record => <option key={record.id} value={record.id}>{dateLabel(record.exam_date)} · {record.result} {record.unit} · {record.institution} · {record.id.slice(-6)}</option>);
  return <details className="panel analysis-tools"><summary>血脂輔助分析（含高TG適用性）</summary><div className="scenario-grid">
    <label className="field">總膽固醇<select value={totalId} onChange={event => { setTotalId(event.target.value); setConfirmed(false); }}><option value="">選擇原始結果</option>{options(totals)}</select></label>
    <label className="field">HDL<select value={hdlId} onChange={event => { setHdlId(event.target.value); setConfirmed(false); }}><option value="">選擇原始結果</option>{options(hdls)}</select></label>
    <label className="field">TG（選填，用於核對LDL公式）<select value={tgId} onChange={event => { setTgId(event.target.value); setConfirmed(false); }}><option value="">不選TG，僅計算non-HDL</option>{options(triglycerides)}</select></label>
  </div><label className="checkbox-label"><input type="checkbox" checked={confirmed} onChange={event => setConfirmed(event.target.checked)} />已核對所選項目為同次採檢，且檢體、單位與檢驗方法適合比較</label>
    <label className="checkbox-label"><input type="checkbox" checked={typeIII} onChange={event => { setTypeIII(event.target.checked); setEstimate(false); }} />已知或疑似第三型高脂蛋白血症（Type III，停用LDL估算）</label>
    <label className="checkbox-label"><input type="checkbox" checked={estimate} disabled={typeIII} onChange={event => setEstimate(event.target.checked)} />另顯示Sampson/NIH研究輔助估算（不超出TG≤800 mg/dL）</label>
    <p role="status">{result.nonHdl === null ? '尚未計算' : `non-HDL-C：${result.nonHdl} ${total?.unit ?? ''}`}。{result.message}</p>
    {result.sampson !== null && <p>系統估算LDL-C：{result.sampson.toFixed(1)} mg/dL · Sampson/NIH 2020（非直接量測）</p>}
    <p className="helper-text">原院LDL、TG及其他結果不會被此計算覆寫。原始研究排除第三型高脂蛋白血症；未確認病型不代表公式已適用於個人，空腹狀態與檢驗方法仍須核對。非HDL亦不取代TG相關風險；不由數值診斷遺傳疾病。</p>
    <a href="https://pubmed.ncbi.nlm.nih.gov/32101259/" target="_blank" rel="noreferrer">Sampson原始研究與適用範圍</a> · <a href="https://eas-society.org/content/quantifying-atherogenic-lipoproteins-for-lipid-lowering-strategies-consensus-based-recommendations-from-eas-and-eflm/" target="_blank" rel="noreferrer">EAS／EFLM 依據</a>
  </details>;
}
export function AnalysisExport({ records }: { records: LabRecord[] }) {
  const analysis = useAnalysis();
  const dateSummary = useMemo(() => laboratoryDateSummary(records), [records]);
  if (!analysis) return null;
  const { preferences, birthDate } = analysis;
  const csvCell = (value: string) => `"${value.replaceAll('"', '""')}"`;
  function exportAnalysis() {
    const rows = [['檢查日期', '項目', '原始結果', '單位', '原院參考', '原院旗標', '院所', '情境模式', '當次情境年齡', '性別條件', '具名範本', '情境判讀', '來源', '原有顯示分級依據與適用限制', '上傳日期（來源原文）', '資料來源名稱（原文）']];
    for (const record of records) {
      const scenario = activeScenario(preferences, record.exam_date, birthDate);
      const interval = referenceForRecord(record, preferences, birthDate);
      rows.push([record.exam_date, record.display_name, record.result, record.unit, record.reference_text, record.official_flag ?? '', record.institution,
        preferences.mode, String(scenario?.age ?? ''), scenario?.sex ?? '', interval ? intervalLabel(interval) : '',
        recordScenarioStatus(record, preferences, birthDate), interval?.source ?? '', gradingContextText(record),
        record.raw_upload_date ?? record.upload_date ?? '', record.source_name ?? '']);
    }
    // Formula-like text is escaped even inside quoted CSV fields.
    const safe = rows.map(row => row.map(value => csvCell(/^[\s]*[=+\-@\t\r]/.test(value) ? `'${value}` : value)).join(',')).join('\r\n');
    downloadText(`\uFEFF${safe}\r\n"© 2026 bestkakkoii llc. · 情境比較非病情預測"\r\n`, 'healthbank-情境比較.csv', 'text/csv;charset=utf-8');
  }
  return <div className="analysis-export"><button className="button secondary" disabled={records.length === 0} onClick={exportAnalysis}>匯出目前 {records.length} 筆的情境比較 CSV</button>
    <span className="helper-text">範圍：{dateSummary}。依目前選擇保留原院結果與情境條件；檔案由您保管。<Copyright /></span></div>;
}

export function PortableReportExport({ records }: { records: LabRecord[] }) {
  const [error, setError] = useState('');
  const [textEditorMode, setTextEditorMode] = useState(false);
  const dateSummary = useMemo(() => laboratoryDateSummary(records), [records]);
  function exportReport(format: 'json' | 'xml') {
    try {
      const content = format === 'json' ? portableReportJSON(records, textEditorMode) : portableReportXML(records);
      downloadText(content, `healthbank-private-report.${format}`, `application/${format};charset=utf-8`);
      setError('');
    } catch (failure) { setError(failure instanceof Error ? failure.message : '匯出失敗；原始資料未變更。'); }
  }
  return <section className="portable-export no-print" aria-label="全部檢驗原始資料匯出">
    <h3>全部檢驗原始資料</h3>
    <p className="helper-text">包含全部 {records.length} 筆檢驗；{dateSummary}。不受目前項目、搜尋或年度篩選影響，請確認後再分享。</p>
    <div className="button-row"><button className="button secondary" type="button" disabled={records.length === 0} onClick={() => { exportReport('json'); }}>匯出全部 {records.length} 筆檢驗 JSON（UTF-8）</button><button className="button secondary" type="button" disabled={records.length === 0} onClick={() => { exportReport('xml'); }}>匯出全部 {records.length} 筆檢驗 XML</button></div>
    <details className="portable-export-options"><summary>文字編碼與檔案說明</summary><label className="checkbox-label small"><input type="checkbox" checked={textEditorMode} onChange={event => setTextEditorMode(event.target.checked)} />JSON 加 UTF-8 BOM（文字檢視器亂碼時使用）</label><p className="helper-text">檔案含健康資料，由您保管；可由本站重新匯入，不代表官方完整歷史。只匯出原始檢驗欄位，不把推算結果寫成原始量測。</p></details>
    {error && <p role="alert">{error}</p>}
  </section>;
}
