// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import { portableReportJSON, portableReportXML } from './portable-report';
import { useState } from 'react';
import type { LabRecord, Settings } from './api';
import { useAnalysis } from './AnalysisContext';
import { dateLabel, downloadText, finiteExtent, type MeasurementGroup } from './model';
import { nonHdlCholesterol } from './analysis-summary';
import { activeScenario, intervalLabel, referenceForRecord, recordScenarioStatus } from './clinical-comparison';
import { TrendChart } from './TrendChart';
import { Copyright } from './ui';

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
  const totals = records.filter(record => record.order_code === '09001C');
  const hdls = records.filter(record => record.order_code === '09043C');
  const total = totals.find(record => record.id === totalId), hdl = hdls.find(record => record.id === hdlId);
  const result = nonHdlCholesterol(total, hdl, confirmed);
  const options = (rows: LabRecord[]) => rows.map(record => <option key={record.id} value={record.id}>{dateLabel(record.exam_date)} · {record.result} {record.unit} · {record.institution} · {record.id.slice(-6)}</option>);
  return <details className="panel analysis-tools"><summary>non-HDL 輔助計算</summary><div className="scenario-grid">
    <label className="field">總膽固醇<select value={totalId} onChange={event => { setTotalId(event.target.value); setConfirmed(false); }}><option value="">選擇原始結果</option>{options(totals)}</select></label>
    <label className="field">HDL<select value={hdlId} onChange={event => { setHdlId(event.target.value); setConfirmed(false); }}><option value="">選擇原始結果</option>{options(hdls)}</select></label>
  </div><label className="checkbox-label"><input type="checkbox" checked={confirmed} onChange={event => setConfirmed(event.target.checked)} />已核對為同次採檢</label>
    <p role="status">{result.value === null ? '尚未計算' : `non-HDL-C：${result.value} ${total?.unit ?? ''}`}。{result.reason}</p>
    <a href="https://eas-society.org/content/quantifying-atherogenic-lipoproteins-for-lipid-lowering-strategies-consensus-based-recommendations-from-eas-and-eflm/" target="_blank" rel="noreferrer">EAS／EFLM 依據</a>
  </details>;
}
export function AnalysisExport({ records }: { records: LabRecord[] }) {
  const analysis = useAnalysis();
  if (!analysis) return null;
  const { preferences, birthDate } = analysis;
  const csvCell = (value: string) => `"${value.replaceAll('"', '""')}"`;
  function exportAnalysis() {
    const rows = [['檢查日期', '項目', '原始結果', '單位', '原院參考', '原院旗標', '院所', '情境模式', '當次情境年齡', '性別條件', '具名範本', '情境判讀', '來源']];
    for (const record of records) {
      const scenario = activeScenario(preferences, record.exam_date, birthDate);
      const interval = referenceForRecord(record, preferences, birthDate);
      rows.push([record.exam_date, record.display_name, record.result, record.unit, record.reference_text, record.official_flag ?? '', record.institution,
        preferences.mode, String(scenario?.age ?? ''), scenario?.sex ?? '', interval ? intervalLabel(interval) : '',
        recordScenarioStatus(record, preferences, birthDate), interval?.source ?? '']);
    }
    // Formula-like text is escaped even inside quoted CSV fields.
    const safe = rows.map(row => row.map(value => csvCell(/^[\s]*[=+\-@\t\r]/.test(value) ? `'${value}` : value)).join(',')).join('\r\n');
    downloadText(`\uFEFF${safe}\r\n"© 2026 bestkakkoii llc. · 情境比較非病情預測"\r\n`, 'healthbank-情境比較.csv', 'text/csv;charset=utf-8');
  }
  return <div className="analysis-export"><button className="button secondary" disabled={records.length === 0} onClick={exportAnalysis}>匯出目前資料的情境比較 CSV</button>
    <span className="helper-text">保留原院結果與情境條件；檔案由使用者裝置保存。<Copyright /></span></div>;
}

export function PortableReportExport({ records }: { records: LabRecord[] }) {
  const [error, setError] = useState('');
  function exportReport(format: 'json' | 'xml') {
    try {
      const content = format === 'json' ? portableReportJSON(records) : portableReportXML(records);
      downloadText(content, `healthbank-private-report.${format}`, `application/${format}`);
      setError('');
    } catch (failure) { setError(failure instanceof Error ? failure.message : '匯出失敗；原始資料未變更。'); }
  }
  return <div className="button-row no-print"><button className="button secondary" type="button" disabled={records.length === 0} onClick={() => { exportReport('json'); }}>匯出可攜JSON</button><button className="button secondary" type="button" disabled={records.length === 0} onClick={() => { exportReport('xml'); }}>匯出可攜XML</button><small>僅保存至您下載的檔案，含健康資料；可由本站重新匯入，不代表官方完整歷史。</small>{error && <p role="alert">{error}</p>}</div>;
}
