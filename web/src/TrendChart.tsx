// Copyright (c) 2026 bestkakkoii llc.
// SPDX-License-Identifier: MIT
import { useAnalysis, ScenarioBadge } from './AnalysisContext';
import { referenceForRecord } from './clinical-comparison';
import { DifferenceSummary } from './DifferenceSummary';
import { createDatePosition, dateTime, trendGeometry, type DateScale } from './trend-model';
import { useId, useLayoutEffect, useMemo, useRef, useState } from 'react';
import type { ItemOverride, LabRecord } from './api';
import {
  dateLabel,
  COPYRIGHT_YEAR,
  downloadText,
  isExact,
  finiteExtent,
  numeric,
  referenceLabel,
  sourceFlag,
  type MeasurementGroup,
  type ExactRecord,
} from './model';
import { Copyright, Icon, LevelBadge } from './ui';

const numberLabel = (n: number, compact: boolean) => {
  const label = new Intl.NumberFormat('zh-TW', {
    maximumSignificantDigits: compact ? 3 : 5,
    useGrouping: !compact,
  }).format(n);
  // Only axis text is shortened; coordinates and the complete record retain the value.
  return compact && label.length > 6 ? n.toExponential(1).replace('e+', 'e') : label;
};
const day = dateTime;
const graphemes = new Intl.Segmenter('zh-TW', { granularity: 'grapheme' });
function textWidth(text: string, size: number): number {
  let width = 0;
  for (const { segment } of graphemes.segment(text))
    width += size * ((segment.codePointAt(0) ?? 0) > 0x7f ? 1 : 0.63);
  return width;
}

function wrapChartText(text: string, width: number, size: number): string[] {
  const lines: string[] = [];
  let line = '';
  for (const { segment: character } of graphemes.segment(text)) {
    if (line && textWidth(line + character, size) > width) {
      const space = line.lastIndexOf(' ');
      if (space > line.length / 2) {
        lines.push(line.slice(0, space));
        line = line.slice(space + 1);
      } else {
        lines.push(line.trim());
        line = '';
      }
    }
    line += character;
  }
  if (line.trim()) lines.push(line.trim());
  return lines;
}

export function TrendChart({
  group,
  override,
  onDetail,
  timeDomain,
}: {
  timeDomain?: readonly [number, number] | undefined;
  group: MeasurementGroup;
  override: ItemOverride | undefined;
  onDetail: (record: LabRecord) => void;
}) {
  const analysis = useAnalysis();
  const scenarioEnabled = analysis !== null && analysis.preferences.mode !== 'off';
  const scenario = analysis ? (analysis.preferences.mode === 'comparison' ? analysis.preferences.comparison : analysis.preferences.primary) : null;
  const scenarioCaption = scenarioEnabled && scenario ? `${analysis.preferences.mode === 'comparison' ? '模擬比較' : '基準'}：${analysis.preferences.mode === 'primary' && analysis.birthDate ? '依採檢日計齡' : `${scenario.age ?? '未知'}歲`}／${scenario.sex === 'male' ? '男性' : scenario.sex === 'female' ? '女性' : '性別未知'}` : '原院／個人基準';
  const scenarioReference = (record: LabRecord) => analysis ? referenceForRecord(record, analysis.preferences, analysis.birthDate) : null;
  const [logarithmic, setLogarithmic] = useState(false);
  const [showReference, setShowReference] = useState(true);
  const [dateScale, setDateScale] = useState<DateScale>('elapsed');
  const [selection, setSelection] = useState<LabRecord[]>([]);
  const [activeId, setActiveId] = useState<string | null>(null);
  const [viewport, setViewport] = useState({ width: 920, fontSize: 16 });
  const viewportRef = useRef<HTMLDivElement>(null);
  const fontProbeRef = useRef<HTMLSpanElement>(null);
  const svgRef = useRef<SVGSVGElement>(null);
  const titleId = useId();
  const descriptionId = useId();
  const sorted = useMemo(
    () =>
      [...group.records].sort(
        (a, b) => a.exam_date.localeCompare(b.exam_date) || a.id.localeCompare(b.id),
      ),
    [group],
  );
  const geometry = useMemo(() => trendGeometry(group.records), [group.records]);
  const dated = sorted.filter((record) => Number.isFinite(day(record.exam_date)));
  const exact = dated.filter(isExact);
  const hasPoints = exact.length > 0;
  useLayoutEffect(() => {
    const element = viewportRef.current;
    const probe = fontProbeRef.current;
    if (!element || !probe) return;
    const measure = () => {
      const width = element.clientWidth;
      if (!width) return;
      const fontSize = Number.parseFloat(getComputedStyle(probe).fontSize) || 16;
      setViewport((previous) =>
        previous.width === width && previous.fontSize === fontSize
          ? previous
          : { width, fontSize },
      );
    };
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    // This em-sized probe also responds to app/browser text-size changes when
    // the container's width itself has not changed.
    observer.observe(probe);
    measure();
    return () => observer.disconnect();
  }, [hasPoints]);
  const unsupported = group.records.length - exact.length;
  const canLog = exact.length > 1 && exact.every((record) => record.numeric_value > 0);
  const useLog = logarithmic && canLog;
  const hasPersonalReference =
    override?.enabled &&
    (override.reference_low !== undefined || override.reference_high !== undefined);
  const usesPersonalReference = (record: LabRecord) =>
    hasPersonalReference && record.rule_source.startsWith('personal');
  const personalReferenceCount = exact.filter(usesPersonalReference).length;
  const referenceLegend = scenarioEnabled && !exact.some(record => scenarioReference(record) !== null) ? '此項目前沒有適用的情境範本；原院旗標仍保留' : scenarioEnabled ? `${analysis.preferences.mode === 'comparison' ? '比較' : '基準'}情境具名範本（非原院旗標）` :
    personalReferenceCount === 0
      ? '原院參考區間'
      : personalReferenceCount === exact.length
        ? '個人參考區間'
        : '個人／原院參考區間（依各筆適用規則）';
  const lowFor = (record: LabRecord) =>
    scenarioEnabled ? scenarioReference(record)?.low ?? null : numeric(usesPersonalReference(record) ? override?.reference_low : record.reference_low);
  const highFor = (record: LabRecord) =>
    scenarioEnabled ? scenarioReference(record)?.high ?? null : numeric(usesPersonalReference(record) ? override?.reference_high : record.reference_high);
  const transform = (n: number) => (useLog ? Math.log10(n) : n);
  const inverse = (n: number) => (useLog ? 10 ** n : n);
  const rangeValues = exact.flatMap((record) => [
    record.numeric_value,
    ...(showReference
      ? [lowFor(record), highFor(record)].filter(
          (value): value is number => value !== null && (!useLog || value > 0),
        )
      : []),
  ]);
  // Full-history reports can contribute 150,000 values including references.
  // Iteration avoids the engine-dependent argument limit of Math.min(...values).
  let [minY, maxY] = finiteExtent(rangeValues.map(transform)) ?? [0, 1];
  const padding = (maxY - minY || Math.max(Math.abs(maxY) * 0.1, 1)) * 0.14;
  minY -= padding;
  maxY += padding;
  if (!useLog && rangeValues.length && rangeValues.every((value) => value >= 0))
    minY = Math.max(0, minY);
  let [minX, maxX] = timeDomain ?? finiteExtent(dated.map((record) => day(record.exam_date))) ?? [0, 1];
  const oneDate = minX === maxX;
  if (oneDate) {
    minX -= 86400000 * 15;
    maxX += 86400000 * 15;
  }
  const width = viewport.width;
  const fontSize = viewport.fontSize;
  const compact = width < 560;
  const plotHeight = Math.round(Math.max(200, Math.min(280, width * 0.4 + 90), fontSize * 5));
  const yTickCount = plotHeight >= fontSize * 7 ? 5 : 3;
  const yTicks = Array.from({ length: yTickCount }, (_, i) =>
    inverse(minY + ((maxY - minY) * i) / (yTickCount - 1)),
  );
  const yTickLabels = yTicks.map((tick) => numberLabel(tick, compact));
  const titleLines = wrapChartText(
    `${group.name} · ${group.unit}${useLog ? '（對數）' : ''} · ${scenarioCaption}`,
    Math.max(1, width - 24),
    fontSize,
  );
  const margin = {
    top: 14 + titleLines.length * fontSize * 1.35,
    right: Math.max(24, fontSize * 0.8),
    left: Math.max(48, ...yTickLabels.map((label) => textWidth(label, fontSize) + 12)),
  };
  const plotWidth = Math.max(1, width - margin.left - margin.right);
  const dateRows = compact ? (plotWidth < fontSize * 6.5 ? 3 : 2) : 1;
  const dateLines = (date: string) =>
    dateRows === 1
      ? [dateLabel(date)]
      : dateRows === 2
        ? [date.slice(0, 4), date.slice(5, 10).replace('-', '/')]
        : [date.slice(0, 4), `${Number(date.slice(5, 7))}月`, `${Number(date.slice(8, 10))}日`];
  const dateWidth = fontSize * (dateRows === 1 ? 6.3 : dateRows === 2 ? 3.15 : 2.52);
  const dateBaseY = margin.top + plotHeight + fontSize + 12;
  const copyrightSize = Math.max(14, fontSize * 0.85);
  const copyrightLines = wrapChartText(
    `© ${COPYRIGHT_YEAR} bestkakkoii llc.`,
    Math.max(1, width - 24),
    copyrightSize,
  );
  const copyrightY =
    dateBaseY + (dateRows - 1) * fontSize * 1.2 + fontSize * 0.2 + copyrightSize + 18;
  const height = Math.ceil(
    copyrightY +
      (copyrightLines.length - 1) * copyrightSize * 1.25 +
      copyrightSize * 0.35 +
      12,
  );
  const dates = geometry.buckets.map(bucket => bucket.date);
  // Shared-domain comparisons stay proportional across panels.
  const scale = timeDomain ? 'elapsed' : dateScale;
  const position = createDatePosition(dates, [minX, maxX], scale);
  const x = (date: string) => margin.left + position(date) * plotWidth;
  const y = (value: number) =>
    margin.top + plotHeight - ((transform(value) - minY) / (maxY - minY)) * plotHeight;
  const maxDateTicks = Math.max(2, Math.min(6, Math.floor(plotWidth / (dateWidth + fontSize)) + 1));
  // Thin labels only. Every observation keeps its real elapsed-time coordinate.
  const tickDates = scale === 'ordinal'
    ? [...new Set(Array.from({ length: Math.min(maxDateTicks, dates.length) }, (_, index) => dates[Math.round(index * (dates.length - 1) / Math.max(1, Math.min(maxDateTicks, dates.length) - 1))] ?? ''))].filter(Boolean)
    : dates.length <= maxDateTicks &&
    dates.every((date, index) => {
      const previous = dates[index - 1];
      return previous === undefined || x(date) - x(previous) >= dateWidth + fontSize;
    })
      ? dates
      : [
          ...new Set(
            Array.from({ length: maxDateTicks }, (_, index) =>
              new Date(minX + ((maxX - minX) * index) / (maxDateTicks - 1))
                .toISOString()
                .slice(0, 10),
            ),
          ),
        ];
  const active = exact.find((record) => record.id === activeId);
  const sharedDate = analysis?.selectedDate ?? '';
  const synchronized = exact.filter(record => record.exam_date === sharedDate);

  function nearestPoint(clientX: number, clientY: number): ExactRecord | null {
    const bounds = svgRef.current?.getBoundingClientRect();
    if (!bounds || !bounds.width || !bounds.height) return null;
    const pointerX = ((clientX - bounds.left) / bounds.width) * width;
    const pointerY = ((clientY - bounds.top) / bounds.height) * height;
    if (pointerY < margin.top - 14 || pointerY > margin.top + plotHeight + 14) return null;
    let nearest: ExactRecord | null = null;
    let distance = Math.max(28, fontSize) ** 2;
    for (const record of exact) {
      const candidate =
        (x(record.exam_date) - pointerX) ** 2 + (y(record.numeric_value) - pointerY) ** 2;
      if (candidate < distance) {
        nearest = record;
        distance = candidate;
      }
    }
    return nearest;
  }

  function activatePoint(record: ExactRecord, pointer = false) {
    const nearby = pointer ? exact.filter(candidate => Math.abs(x(candidate.exam_date) - x(record.exam_date)) <= 12 && Math.abs(y(candidate.numeric_value) - y(record.numeric_value)) <= 12) : [record];
    analysis?.setSelectedDate(record.exam_date);
    if (nearby.length > 1) setSelection(nearby);
    else { setSelection([]); onDetail(record); }
  }

  function exportSvg() {
    const element = svgRef.current;
    if (!element) return;
    const copy = element.cloneNode(true) as SVGSVGElement;
    const originals = [element, ...element.querySelectorAll('*')];
    const copies = [copy, ...copy.querySelectorAll('*')];
    originals.forEach((original, index) => {
      const style = getComputedStyle(original);
      const target = copies[index] as SVGElement;
      for (const property of [
        'fill',
        'stroke',
        'stroke-width',
        'font-family',
        'font-size',
        'font-weight',
        'opacity',
        'text-anchor',
      ])
        target.style.setProperty(property, style.getPropertyValue(property));
    });
    copy.setAttribute('xmlns', 'http://www.w3.org/2000/svg');
    copy.setAttribute('width', String(width));
    copy.setAttribute('height', String(height));
    copy.style.background = getComputedStyle(document.documentElement)
      .getPropertyValue('--surface')
      .trim();
    downloadText(
      `<?xml version="1.0" encoding="UTF-8"?>\n${new XMLSerializer().serializeToString(copy)}`,
      `${group.name.replace(/[\\/:*?"<>|]/g, '_')}-趨勢.svg`,
      'image/svg+xml;charset=utf-8',
    );
  }

  return (
    <section className="trend-section">
      <DifferenceSummary group={group} />
      <p className="helper-text">{referenceLegend}；色點仍保留原院／獨立警示。</p>
      {sharedDate && <p role="status">同步日期 {dateLabel(sharedDate)}：{synchronized.length ? synchronized.map(record => `${record.result} ${record.unit}`).join('；') : '此項當日未取得精確值'}</p>}
      <div className="chart-toolbar">
        <div className="chart-description">
          <span className="chart-line-key" />
          檢驗結果 <span className="chart-band-key" />
          {referenceLegend}
        </div>
        <div className="chart-options">
          {!timeDomain && <label className="field chart-scale">橫軸排列<select value={dateScale} onChange={event => { setDateScale(event.target.value === 'ordinal' ? 'ordinal' : 'elapsed'); setSelection([]); }}><option value="elapsed">實際日期距離</option><option value="ordinal">檢查日期等距（展開密集點）</option></select></label>}
          <label className="checkbox-label small">
            <input
              type="checkbox"
              checked={showReference}
              onChange={(event) => setShowReference(event.target.checked)}
            />
            顯示參考區間
          </label>
          {canLog && (
            <label className="checkbox-label small">
              <input
                type="checkbox"
                checked={useLog}
                onChange={(event) => setLogarithmic(event.target.checked)}
              />
              對數刻度
            </label>
          )}
          <button
            className="button quiet small-button"
            onClick={exportSvg}
            disabled={!exact.length}
          >
            <Icon name="download" size={17} />
            下載圖表
          </button>
        </div>
      </div>
      {exact.length ? (
        <div className="chart-container">
          <div className="chart-viewport" ref={viewportRef}>
            <span className="chart-font-probe" ref={fontProbeRef} aria-hidden="true" />
            <svg
              ref={svgRef}
              viewBox={`0 0 ${width} ${height}`}
              width={width}
              height={height}
              className="trend-svg"
              role="group"
              aria-labelledby={titleId}
              aria-describedby={descriptionId}
              onPointerMove={(event) => setActiveId(nearestPoint(event.clientX, event.clientY)?.id ?? null)}
              onPointerLeave={() => setActiveId(null)}
              onClick={(event) => {
                const record = nearestPoint(event.clientX, event.clientY);
                if (record) activatePoint(record, true);
              }}
            >
              <title id={titleId}>{group.name}歷年趨勢 · {scenarioCaption}</title>
              <desc id={descriptionId}>
                {exact.length} 筆確切數值，單位 {group.unit}，日期從 {dateLabel(dated[0]?.exam_date ?? '')}{' '}
                至 {dateLabel(dated.at(-1)?.exam_date ?? '')}。{useLog ? '縱軸使用對數刻度。' : ''}{scale === 'ordinal' ? '橫軸日期等距，不表示實際時間間隔。' : '橫軸依實際時間間隔。'}
                所有結果亦列於下方可操作的明細表。
              </desc>
              <rect width={width} height={height} className="chart-background" />
              {titleLines.map((line, index) => (
                <text x={12} y={8 + fontSize + index * fontSize * 1.35} className="axis-title" key={index}>
                  {line}
                </text>
              ))}
              {showReference &&
                exact.map((record, index) => {
                  const low = lowFor(record),
                    high = highFor(record);
                  if (
                    (low === null && high === null) ||
                    (useLog && ((low !== null && low <= 0) || (high !== null && high <= 0)))
                  )
                    return null;
                  const previous = exact[index - 1], next = exact[index + 1];
                  const left = previous
                    ? (x(previous.exam_date) + x(record.exam_date)) / 2
                    : margin.left;
                  const right = next
                    ? (x(record.exam_date) + x(next.exam_date)) / 2
                    : margin.left + plotWidth;
                  const top = high === null ? margin.top : y(high),
                    bottom = low === null ? margin.top + plotHeight : y(low);
                  if (bottom < top) return null;
                  return (
                    <g key={record.id} className="reference-area">
                      <rect
                        x={left}
                        y={Math.max(margin.top, top)}
                        width={Math.max(0, right - left)}
                        height={Math.max(
                          0,
                          Math.min(margin.top + plotHeight, bottom) - Math.max(margin.top, top),
                        )}
                      />
                      {high !== null && <line x1={left} x2={right} y1={top} y2={top} />}{' '}
                      {low !== null && <line x1={left} x2={right} y1={bottom} y2={bottom} />}
                    </g>
                  );
                })}
              {sharedDate && day(sharedDate) >= minX && day(sharedDate) <= maxX && Number.isFinite(x(sharedDate)) && <line className="chart-cursor" x1={x(sharedDate)} x2={x(sharedDate)} y1={margin.top} y2={margin.top + plotHeight} />}
              {yTicks.map((tick, index) => (
                <g key={index}>
                  <line
                    x1={margin.left}
                    x2={width - margin.right}
                    y1={y(tick)}
                    y2={y(tick)}
                    className="chart-grid"
                  />
                  <text x={margin.left - 12} y={y(tick) + fontSize * 0.35} textAnchor="end" className="axis-label">
                    {yTickLabels[index]}
                  </text>
                </g>
              ))}
              {tickDates.map((date) => (
                <g key={date}>
                  <line
                    x1={x(date)}
                    x2={x(date)}
                    y1={margin.top + plotHeight}
                    y2={margin.top + plotHeight + 5}
                    className="chart-axis"
                  />
                  <text
                    x={Math.max(12 + dateWidth / 2, Math.min(width - 12 - dateWidth / 2, x(date)))}
                    y={dateBaseY}
                    textAnchor="middle"
                    className="axis-label"
                  >
                    <title>{dateLabel(date)}</title>
                    {dateLines(date).map((line, index) => (
                      <tspan
                        x={Math.max(12 + dateWidth / 2, Math.min(width - 12 - dateWidth / 2, x(date)))}
                        dy={index === 0 ? 0 : fontSize * 1.2}
                        key={index}
                      >
                        {line}
                      </tspan>
                    ))}
                  </text>
                </g>
              ))}
              {geometry.links.map((link, index) => <line key={`link-${index}`} x1={x(link.from)} x2={x(link.to)} y1={y(link.fromValue)} y2={y(link.toValue)} className="chart-line" strokeDasharray={link.ambiguous ? '5 5' : undefined}><title>{link.ambiguous ? '同日多值／非精確紀錄：範圍邊界連線，不推定先後或中間值' : '兩次觀測值連線，不代表中間曾測量'}</title></line>)}
              {geometry.buckets.map(bucket => bucket.low !== null && bucket.high !== null && bucket.low !== bucket.high ? <line key={`range-${bucket.date}`} x1={x(bucket.date)} x2={x(bucket.date)} y1={y(bucket.low)} y2={y(bucket.high)} className="chart-line" strokeDasharray="2 3"><title>同日 {bucket.records.length} 筆原始結果，不推測先後</title></line> : null)}
              {exact.map((record) => (
                <g key={record.id} className={`chart-point level-${record.level ?? 0}`}>
                  <circle
                    cx={x(record.exam_date)}
                    cy={y(record.numeric_value)}
                    r={activeId === record.id ? 8 : 6}
                    className="point-dot"
                  />
                  <circle
                    cx={x(record.exam_date)}
                    cy={y(record.numeric_value)}
                    r="22"
                    fill="transparent"
                    stroke="none"
                    className="chart-point-target"
                    tabIndex={0}
                    role="button"
                    aria-label={`${dateLabel(record.exam_date)}，${record.result} ${record.unit}，${record.institution || ''}，開啟明細`}
                    onFocus={() => { setActiveId(record.id); analysis?.setSelectedDate(record.exam_date); }}
                    onBlur={() => setActiveId(null)}
                    onClick={(event) => {
                      // Assistive technology may activate the focused SVG button
                      // without pointer coordinates; retain its exact identity.
                      event.stopPropagation();
                      const nearest = event.detail > 0 ? nearestPoint(event.clientX, event.clientY) ?? record : record;
                      activatePoint(nearest, event.detail > 0);
                    }}
                    onKeyDown={(event) => {
                      if (event.key === 'Enter' || event.key === ' ') {
                        event.preventDefault();
                        activatePoint(record);
                      }
                    }}
                  >
                    <title>
                      {dateLabel(record.exam_date)} · {record.result} {record.unit} ·{' '}
                      {record.institution}
                    </title>
                  </circle>
                </g>
              ))}
              {copyrightLines.map((line, index) => (
                <text
                  x={width - 12}
                  y={copyrightY + index * copyrightSize * 1.25}
                  textAnchor="end"
                  className="chart-copyright"
                  key={index}
                >
                  {line}
                </text>
              ))}
            </svg>
          </div>
          <div className="chart-live-value" aria-live="polite">
            {active ? (
              <>
                <b>{dateLabel(active.exam_date)}</b>
                <strong>
                  {active.result} <small>{group.unit}</small>
                </strong>
                <LevelBadge level={active.level} />
                <span>{active.institution}</span>
              </>
            ) : (
              <span>點選資料點查看原始報告；{scale === 'ordinal' ? '日期等距排列（非實際時間比例）' : '依實際日期間隔排列'}。密集點可展開逐筆選擇。</span>
            )}
          </div>
        </div>
      ) : (
        <div className="chart-empty">
          <Icon name="trend" size={36} />
          <h3>此項目沒有可繪製的確切數值</h3>
          <p>文字、範圍或「大於／小於」結果保留在下方明細，不換算成推測數字。</p>
          <Copyright />
        </div>
      )}
      {selection.length > 0 && <div className="chart-selection" role="region" aria-label="重疊資料點明細"><div className="button-row"><strong>此位置有 {selection.length} 筆結果</strong><button type="button" className="button quiet" onClick={() => setSelection([])}>關閉選擇</button></div>{selection.map(record => <button className="button secondary" key={record.id} type="button" onClick={() => { analysis?.setSelectedDate(record.exam_date); onDetail(record); }}>{dateLabel(record.exam_date)} · {record.result} {record.unit} · {record.institution} · {record.source}</button>)}</div>}
      {unsupported > 0 && (
        <p className="chart-footnote">
          <Icon name="info" size={16} />
          {unsupported} 筆為文字、範圍、界限值或缺少有效日期，未繪製為確切數值；詳見以下完整紀錄。
        </p>
      )}
      {exact.length > 0 && (
        <p className="chart-footnote">
          <Icon name="info" size={16} />
          同日同值共用座標但保留全部明細；同日多值用垂直範圍與虛線邊界表示，不取平均或中位數。非精確結果仍保留於明細；實線只連接相鄰可比較日期，不補造中間量測。
        </p>
      )}
      <div className="chart-records">
        <h3>
          完整檢查紀錄 <span>{group.records.length} 筆</span>
        </h3>
        <div className="simple-table-scroll">
          <table className="detail-table">
            <thead>
              <tr>
                <th scope="col">檢查日期</th>
                <th scope="col">結果 · {group.unit}</th>
                <th scope="col">分級</th>
                <th scope="col">當次原院參考值</th>
                <th scope="col">醫事機構</th>
                <th scope="col">
                  <span className="sr-only">操作</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {group.records.map((record) => (
                <tr key={record.id}>
                  <td>{dateLabel(record.exam_date)}</td>
                  <td>
                    <strong>{record.result || '—'}</strong>
                    <small>{sourceFlag(record.reference_status)}</small>
                    {record.official_flag === '1' && (
                      <span className="official-flag-badge">原院異常標記</span>
                    )}
                  </td>
                  <td>
                    <ScenarioBadge record={record} /><LevelBadge level={record.level} compact />
                  </td>
                  <td>{referenceLabel(record)}</td>
                  <td>{record.institution || '未提供'}</td>
                  <td>
                    <button
                      className="icon-button"
                      aria-label={`查看 ${dateLabel(record.exam_date)} ${record.result} 的詳細資料`}
                      onClick={() => { analysis?.setSelectedDate(record.exam_date); onDetail(record); }}
                    >
                      <Icon name="chevron" />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </section>
  );
}
