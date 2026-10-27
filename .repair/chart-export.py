# fix: preserve duplicate-date trends and export explicit UTF-8 JSON
from pathlib import Path
r=Path.cwd()
(r/'web/src/trend-model.ts').write_text(r'''// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import type { LabRecord } from './api';
import { isExact, type ExactRecord } from './model.ts';

export type DateScale = 'elapsed' | 'ordinal';
export function dateTime(date: string): number {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(date)) return Number.NaN;
  const time = Date.parse(`${date}T00:00:00Z`);
  return Number.isFinite(time) && new Date(time).toISOString().slice(0, 10) === date ? time : Number.NaN;
}
export interface TrendBucket {
  date: string;
  records: LabRecord[];
  exact: ExactRecord[];
  low: number | null;
  high: number | null;
}
export interface TrendLink {
  from: string;
  to: string;
  fromValue: number;
  toValue: number;
  ambiguous: boolean;
}
// Bucket by date, NOT by record count. Identical values may share coordinates;
// all source rows remain selectable. No mean/median or intra-day order is invented.
export function trendGeometry(records: LabRecord[]): { buckets: TrendBucket[]; links: TrendLink[] } {
  const dates = new Map<string, LabRecord[]>();
  for (const record of records) {
    if (!Number.isFinite(dateTime(record.exam_date))) continue;
    const rows = dates.get(record.exam_date) ?? [];
    rows.push(record); dates.set(record.exam_date, rows);
  }
  const buckets = [...dates.entries()].toSorted(([a], [b]) => a.localeCompare(b)).map(([date, rows]) => {
    const exact = rows.filter(isExact);
    let low: number | null = null, high: number | null = null;
    for (const row of exact) {
      low = low === null ? row.numeric_value : Math.min(low, row.numeric_value);
      high = high === null ? row.numeric_value : Math.max(high, row.numeric_value);
    }
    return { date, records: rows, exact, low, high };
  });
  const links: TrendLink[] = [];
  for (let index = 1; index < buckets.length; index++) {
    const a = buckets[index - 1], b = buckets[index];
    if (!a || !b || a.low === null || a.high === null || b.low === null || b.high === null) continue;
    const ambiguous = a.low !== a.high || b.low !== b.high || a.exact.length !== a.records.length || b.exact.length !== b.records.length;
    links.push({ from: a.date, to: b.date, fromValue: a.low, toValue: b.low, ambiguous });
    if (a.low !== a.high || b.low !== b.high)
      links.push({ from: a.date, to: b.date, fromValue: a.high, toValue: b.high, ambiguous: true });
  }
  return { buckets, links };
}
export function createDatePosition(dates: string[], domain: readonly [number, number], mode: DateScale): (date: string) => number {
  const indices = new Map(dates.map((date, index) => [date, index]));
  return (date: string) => {
    if (mode === 'ordinal') {
      const index = indices.get(date);
      return index === undefined ? Number.NaN : dates.length < 2 ? 0.5 : index / (dates.length - 1);
    }
    const [start, end] = domain;
    return start === end ? 0.5 : (dateTime(date) - start) / (end - start);
  };
}
''',encoding='utf-8')
p=r/'web/src/TrendChart.tsx';s=p.read_text()
assert "import { DifferenceSummary } from './DifferenceSummary';" in s
s=s.replace("import { DifferenceSummary } from './DifferenceSummary';", "import { DifferenceSummary } from './DifferenceSummary';\nimport { createDatePosition, dateTime, trendGeometry, type DateScale } from './trend-model';")
s=s.replace("const day = (value: string) => Date.parse(`${value.slice(0, 10)}T00:00:00Z`);", 'const day = dateTime;')
s=s.replace('  const [showReference, setShowReference] = useState(true);', "  const [showReference, setShowReference] = useState(true);\n  const [dateScale, setDateScale] = useState<DateScale>('elapsed');\n  const [selection, setSelection] = useState<LabRecord[]>([]);")
s=s.replace('  const dated = sorted.filter', '  const geometry = useMemo(() => trendGeometry(group.records), [group.records]);\n  const dated = sorted.filter')
s=s.replace('  const x = (date: string) => margin.left + ((day(date) - minX) / (maxX - minX)) * plotWidth;', "  const dates = geometry.buckets.map(bucket => bucket.date);\n  // Shared-domain comparisons stay proportional across panels.\n  const scale = timeDomain ? 'elapsed' : dateScale;\n  const position = createDatePosition(dates, [minX, maxX], scale);\n  const x = (date: string) => margin.left + position(date) * plotWidth;")
s=s.replace("  const dates = [...new Set(dated.map((record) => record.exam_date))];\n",'')
s=s.replace('  const tickDates =\n    dates.length', "  const tickDates = scale === 'ordinal'\n    ? [...new Set(Array.from({ length: Math.min(maxDateTicks, dates.length) }, (_, index) => dates[Math.round(index * (dates.length - 1) / Math.max(1, Math.min(maxDateTicks, dates.length) - 1))] ?? ''))].filter(Boolean)\n    : dates.length")
a=s.index('  const segments:');b=s.index('  const active =',a);s=s[:a]+s[b:]
s=s.replace('function nearestPoint(clientX: number, clientY: number): LabRecord | null', 'function nearestPoint(clientX: number, clientY: number): ExactRecord | null')
s=s.replace('let nearest: LabRecord | null = null;', 'let nearest: ExactRecord | null = null;')
a=s.index('  function exportSvg()');s=s[:a]+'''  function activatePoint(record: ExactRecord, pointer = false) {
    const nearby = pointer ? exact.filter(candidate => Math.abs(x(candidate.exam_date) - x(record.exam_date)) <= 12 && Math.abs(y(candidate.numeric_value) - y(record.numeric_value)) <= 12) : [record];
    analysis?.setSelectedDate(record.exam_date);
    if (nearby.length > 1) setSelection(nearby);
    else { setSelection([]); onDetail(record); }
  }

'''+s[a:]
s=s.replace('        <div className="chart-options">', '''        <div className="chart-options">
          {!timeDomain && <label className="field chart-scale">橫軸排列<select value={dateScale} onChange={event => { setDateScale(event.target.value === 'ordinal' ? 'ordinal' : 'elapsed'); setSelection([]); }}><option value="elapsed">實際日期距離</option><option value="ordinal">檢查日期等距（展開密集點）</option></select></label>}''')
s=s.replace('if (record) onDetail(record);','if (record) activatePoint(record, true);')
s=s.replace("{useLog ? '縱軸使用對數刻度。' : ''}", "{useLog ? '縱軸使用對數刻度。' : ''}{scale === 'ordinal' ? '橫軸日期等距，不表示實際時間間隔。' : '橫軸依實際時間間隔。'}")
a=s.index('              {segments.map(');b=s.index('              {exact.map',a)
s=s[:a]+'''              {geometry.links.map((link, index) => <line key={`link-${index}`} x1={x(link.from)} x2={x(link.to)} y1={y(link.fromValue)} y2={y(link.toValue)} className="chart-line" strokeDasharray={link.ambiguous ? '5 5' : undefined}><title>{link.ambiguous ? '同日多值／非精確紀錄：範圍邊界連線，不推定先後或中間值' : '兩次觀測值連線，不代表中間曾測量'}</title></line>)}
              {geometry.buckets.map(bucket => bucket.low !== null && bucket.high !== null && bucket.low !== bucket.high ? <line key={`range-${bucket.date}`} x1={x(bucket.date)} x2={x(bucket.date)} y1={y(bucket.low)} y2={y(bucket.high)} className="chart-line" strokeDasharray="2 3"><title>同日 {bucket.records.length} 筆原始結果，不推測先後</title></line> : null)}
'''+s[b:]
start=s.index('              {exact.map');before=s[:start];tail=s[start:]
tail=tail.replace('                      onDetail(record);', '                      const nearest = event.detail > 0 ? nearestPoint(event.clientX, event.clientY) ?? record : record;\n                      activatePoint(nearest, event.detail > 0);',1)
tail=tail.replace('                        onDetail(record);','                        activatePoint(record);',1);s=before+tail
s=s.replace('<span>點選資料點查看原始報告；橫軸依實際日期間隔呈現。</span>', "<span>點選資料點查看原始報告；{scale === 'ordinal' ? '日期等距排列（非實際時間比例）' : '依實際日期間隔排列'}。密集點可展開逐筆選擇。</span>")
s=s.replace('      {unsupported > 0 && (', '''      {selection.length > 0 && <div className="chart-selection" role="region" aria-label="重疊資料點明細"><div className="button-row"><strong>此位置有 {selection.length} 筆結果</strong><button type="button" className="button quiet" onClick={() => setSelection([])}>關閉選擇</button></div>{selection.map(record => <button className="button secondary" key={record.id} type="button" onClick={() => { analysis?.setSelectedDate(record.exam_date); onDetail(record); }}>{dateLabel(record.exam_date)} · {record.result} {record.unit} · {record.institution} · {record.source}</button>)}</div>}
      {unsupported > 0 && (''')
s=s.replace('圖表完整呈現所選期間，點選資料點查看原始報告；密集或重疊的資料點可由下方完整紀錄逐筆查閱。陰影依各次報告的適用參考值分段呈現；同日多筆結果全部保留，不推測當日先後順序。','同日同值共用座標但保留全部明細；同日多值用垂直範圍與虛線邊界表示，不取平均或中位數。非精確結果仍保留於明細；實線只連接相鄰可比較日期，不補造中間量測。')
p.write_text(s)
p=r/'web/src/model.ts';s=p.read_text();s=s.replace('  const url = URL.createObjectURL(new Blob([text], { type: mime }));', "  const type = /charset=/i.test(mime) ? mime : `${mime};charset=utf-8`;\n  const bytes = new TextEncoder().encode(text);\n  const url = URL.createObjectURL(new Blob([bytes], { type }));")
s=s.replace('URL.revokeObjectURL(url), 1000)', 'URL.revokeObjectURL(url), 60000)');p.write_text(s)
p=r/'web/src/portable-report.ts';s=p.read_text();s=s.replace("export function portableReportJSON(records: LabRecord[]): string {\n  return JSON.stringify({ schema: PORTABLE_REPORT_SCHEMA, records: portableRows(records) }, null, 2);\n}", "export function portableReportJSON(records: LabRecord[], textEditorMode = false): string {\n  // BOM is opt-in for downloaded files only, never an API JSON response.\n  return (textEditorMode ? '\\uFEFF' : '') + JSON.stringify({ schema: PORTABLE_REPORT_SCHEMA, records: portableRows(records) }, null, 2) + '\\n';\n}");p.write_text(s)
p=r/'web/src/AnalysisTools.tsx';s=p.read_text();s=s.replace("  const [error, setError] = useState('');\n  function exportReport", "  const [error, setError] = useState('');\n  const [textEditorMode, setTextEditorMode] = useState(false);\n  function exportReport")
s=s.replace('portableReportJSON(records) :', 'portableReportJSON(records, textEditorMode) :');s=s.replace('`application/${format}`', '`application/${format};charset=utf-8`');s=s.replace('匯出可攜JSON</button>', '匯出JSON（UTF-8）</button>');s=s.replace('<small>僅保存至您下載的檔案，含健康資料；', '<label className="checkbox-label small"><input type="checkbox" checked={textEditorMode} onChange={event => setTextEditorMode(event.target.checked)} />JSON加UTF-8識別碼（文字檢視器亂碼時使用）</label><small>僅保存至您下載的檔案，含健康資料；');p.write_text(s)
p=r/'web/src/styles.css';s=p.read_text();s+='''
/* Dense-date inspection keeps original points and text within the local panel. */
.chart-scale { min-width: 0; max-width: 100%; margin: 0; }
.chart-scale select { max-width: 100%; }
.chart-selection { display: flex; flex-direction: column; gap: .5rem; padding: .75rem; border: 1px solid var(--border); border-radius: .5rem; max-height: 22rem; overflow: auto; }
.chart-selection .button { white-space: normal; overflow-wrap: anywhere; text-align: start; }
''';p.write_text(s)
