// Copyright (c) 2026 bestkakkoi llc. All rights reserved.
// SPDX-License-Identifier: MIT
import { useId, useMemo, useRef, useState } from 'react';
import type { ItemOverride, LabRecord } from './api';
import {
  dateLabel,
  downloadText,
  isExact,
  numeric,
  referenceLabel,
  sourceFlag,
  type MeasurementGroup,
} from './model';
import { Copyright, Icon, LevelBadge } from './ui';

const WIDTH = 920;
const HEIGHT = 390;
const MARGIN = { top: 34, right: 36, bottom: 80, left: 78 };
const PLOT_WIDTH = WIDTH - MARGIN.left - MARGIN.right;
const PLOT_HEIGHT = HEIGHT - MARGIN.top - MARGIN.bottom;
const numberLabel = (n: number) =>
  new Intl.NumberFormat('zh-TW', { maximumSignificantDigits: 5, maximumFractionDigits: 4 }).format(
    n,
  );
const day = (value: string) => Date.parse(`${value.slice(0, 10)}T00:00:00Z`);

export function TrendChart({
  group,
  override,
  onDetail,
}: {
  group: MeasurementGroup;
  override?: ItemOverride;
  onDetail: (record: LabRecord) => void;
}) {
  const [logarithmic, setLogarithmic] = useState(false);
  const [showReference, setShowReference] = useState(true);
  const [activeId, setActiveId] = useState<string | null>(null);
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
  const dated = sorted.filter((record) => Number.isFinite(day(record.exam_date)));
  const exact = dated.filter(isExact);
  const unsupported = group.records.length - exact.length;
  const canLog = exact.length > 1 && exact.every((record) => record.numeric_value! > 0);
  const useLog = logarithmic && canLog;
  const hasPersonalReference =
    override?.enabled &&
    (override.reference_low !== undefined || override.reference_high !== undefined);
  const usesPersonalReference = (record: LabRecord) =>
    hasPersonalReference && record.rule_source.startsWith('personal');
  const personalReferenceCount = exact.filter(usesPersonalReference).length;
  const referenceLegend =
    personalReferenceCount === 0
      ? '原院參考區間'
      : personalReferenceCount === exact.length
        ? '個人參考區間'
        : '個人／原院參考區間（依各筆適用規則）';
  const lowFor = (record: LabRecord) =>
    numeric(usesPersonalReference(record) ? override?.reference_low : record.reference_low);
  const highFor = (record: LabRecord) =>
    numeric(usesPersonalReference(record) ? override?.reference_high : record.reference_high);
  const transform = (n: number) => (useLog ? Math.log10(n) : n);
  const inverse = (n: number) => (useLog ? 10 ** n : n);
  const rangeValues = exact.flatMap((record) => [
    record.numeric_value!,
    ...(showReference
      ? [lowFor(record), highFor(record)].filter(
          (value): value is number => value !== null && (!useLog || value > 0),
        )
      : []),
  ]);
  let minY = rangeValues.length ? Math.min(...rangeValues.map(transform)) : 0;
  let maxY = rangeValues.length ? Math.max(...rangeValues.map(transform)) : 1;
  const padding = (maxY - minY || Math.max(Math.abs(maxY) * 0.1, 1)) * 0.14;
  minY -= padding;
  maxY += padding;
  if (!useLog && rangeValues.length && rangeValues.every((value) => value >= 0))
    minY = Math.max(0, minY);
  let minX = dated.length ? Math.min(...dated.map((record) => day(record.exam_date))) : 0;
  let maxX = dated.length ? Math.max(...dated.map((record) => day(record.exam_date))) : 1;
  const oneDate = minX === maxX;
  if (oneDate) {
    minX -= 86400000 * 15;
    maxX += 86400000 * 15;
  }
  const x = (date: string) => MARGIN.left + ((day(date) - minX) / (maxX - minX)) * PLOT_WIDTH;
  const y = (value: number) =>
    MARGIN.top + PLOT_HEIGHT - ((transform(value) - minY) / (maxY - minY)) * PLOT_HEIGHT;
  const yTicks = Array.from({ length: 5 }, (_, i) => inverse(minY + ((maxY - minY) * i) / 4));
  const dates = [...new Set(dated.map((record) => record.exam_date))];
  // Use actual elapsed time on the x-axis, not equally spaced visit positions.
  const tickDates =
    dates.length <= 5
      ? dates
      : [0, 1, 2, 3, 4].map((index) =>
          new Date(minX + ((maxX - minX) * index) / 4).toISOString().slice(0, 10),
        );
  const segments: LabRecord[][] = [];
  let segment: LabRecord[] = [];
  const sameDateCounts = new Map<string, number>();
  for (const record of dated)
    sameDateCounts.set(record.exam_date, (sameDateCounts.get(record.exam_date) ?? 0) + 1);
  for (const record of dated) {
    // Same-day records have no known time order; show every point without inventing a connecting sequence.
    if (!isExact(record) || (sameDateCounts.get(record.exam_date) ?? 0) > 1) {
      if (segment.length) segments.push(segment);
      segment = [];
    } else segment.push(record);
  }
  if (segment.length) segments.push(segment);
  const active = exact.find((record) => record.id === activeId);

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
    copy.setAttribute('width', String(WIDTH));
    copy.setAttribute('height', String(HEIGHT));
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
      <div className="chart-toolbar">
        <div className="chart-description">
          <span className="chart-line-key" />
          檢驗結果 <span className="chart-band-key" />
          {referenceLegend}
        </div>
        <div className="chart-options">
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
          <svg
            ref={svgRef}
            viewBox={`0 0 ${WIDTH} ${HEIGHT}`}
            className="trend-svg"
            role="img"
            aria-labelledby={`${titleId} ${descriptionId}`}
          >
            <title id={titleId}>{group.name}歷年趨勢</title>
            <desc id={descriptionId}>
              {exact.length} 筆確切數值，單位 {group.unit}，日期從 {dateLabel(dated[0]?.exam_date)}{' '}
              至 {dateLabel(dated.at(-1)?.exam_date ?? '')}。{useLog ? '縱軸使用對數刻度。' : ''}
              所有結果亦列於下方可操作的明細表。
            </desc>
            <rect width={WIDTH} height={HEIGHT} className="chart-background" />
            <text x={MARGIN.left} y={18} className="axis-title">
              {group.name} · {group.unit}
              {useLog ? '（對數）' : ''}
            </text>
            {showReference &&
              exact.map((record, index) => {
                const low = lowFor(record),
                  high = highFor(record);
                if (
                  (low === null && high === null) ||
                  (useLog && ((low !== null && low <= 0) || (high !== null && high <= 0)))
                )
                  return null;
                const left =
                  index === 0
                    ? MARGIN.left
                    : (x(exact[index - 1].exam_date) + x(record.exam_date)) / 2;
                const right =
                  index === exact.length - 1
                    ? MARGIN.left + PLOT_WIDTH
                    : (x(record.exam_date) + x(exact[index + 1].exam_date)) / 2;
                const top = high === null ? MARGIN.top : y(high),
                  bottom = low === null ? MARGIN.top + PLOT_HEIGHT : y(low);
                if (bottom < top) return null;
                return (
                  <g key={record.id} className="reference-area">
                    <rect
                      x={left}
                      y={Math.max(MARGIN.top, top)}
                      width={Math.max(0, right - left)}
                      height={Math.max(
                        0,
                        Math.min(MARGIN.top + PLOT_HEIGHT, bottom) - Math.max(MARGIN.top, top),
                      )}
                    />
                    {high !== null && <line x1={left} x2={right} y1={top} y2={top} />}{' '}
                    {low !== null && <line x1={left} x2={right} y1={bottom} y2={bottom} />}
                  </g>
                );
              })}
            {yTicks.map((tick, index) => (
              <g key={index}>
                <line
                  x1={MARGIN.left}
                  x2={WIDTH - MARGIN.right}
                  y1={y(tick)}
                  y2={y(tick)}
                  className="chart-grid"
                />
                <text x={MARGIN.left - 14} y={y(tick) + 5} textAnchor="end" className="axis-label">
                  {numberLabel(tick)}
                </text>
              </g>
            ))}
            {tickDates.map((date) => (
              <g key={date}>
                <line
                  x1={x(date)}
                  x2={x(date)}
                  y1={HEIGHT - MARGIN.bottom}
                  y2={HEIGHT - MARGIN.bottom + 5}
                  className="chart-axis"
                />
                <text
                  x={x(date)}
                  y={HEIGHT - MARGIN.bottom + 27}
                  textAnchor="middle"
                  className="axis-label"
                >
                  {dateLabel(date)}
                </text>
              </g>
            ))}
            {segments.map(
              (part, index) =>
                part.length >= 2 && (
                  <polyline
                    key={index}
                    points={part
                      .map((record) => `${x(record.exam_date)},${y(record.numeric_value!)}`)
                      .join(' ')}
                    className="chart-line"
                  />
                ),
            )}
            {exact.map((record) => (
              <g key={record.id} className={`chart-point level-${record.level ?? 0}`}>
                <circle
                  cx={x(record.exam_date)}
                  cy={y(record.numeric_value!)}
                  r={activeId === record.id ? 8 : 6}
                  className="point-dot"
                />
                <circle
                  cx={x(record.exam_date)}
                  cy={y(record.numeric_value!)}
                  r="28"
                  fill="transparent"
                  stroke="none"
                  tabIndex={0}
                  role="button"
                  aria-label={`${dateLabel(record.exam_date)}，${record.result} ${record.unit}，${record.institution || ''}，開啟明細`}
                  onMouseEnter={() => setActiveId(record.id)}
                  onMouseLeave={() => setActiveId(null)}
                  onFocus={() => setActiveId(record.id)}
                  onBlur={() => setActiveId(null)}
                  onClick={() => onDetail(record)}
                  onKeyDown={(event) => {
                    if (event.key === 'Enter' || event.key === ' ') {
                      event.preventDefault();
                      onDetail(record);
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
            <text
              x={WIDTH - MARGIN.right}
              y={HEIGHT - 14}
              textAnchor="end"
              className="chart-copyright"
            >
              © {new Date().getFullYear()} bestkakkoi llc. All rights reserved.
            </text>
          </svg>
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
              <span>點選資料點查看原始報告；橫軸依實際日期間隔呈現。</span>
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
      {unsupported > 0 && (
        <p className="chart-footnote">
          <Icon name="info" size={16} />
          {unsupported} 筆為文字、範圍、界限值或缺少有效日期，未繪製為確切數值；詳見以下完整紀錄。
        </p>
      )}
      {exact.length > 0 && (
        <p className="chart-footnote">
          <Icon name="info" size={16} />
          陰影依各次報告的適用參考值分段呈現。同日多筆結果全部保留為資料點，不推測當日先後順序。
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
                    <LevelBadge level={record.level} compact />
                  </td>
                  <td>{referenceLabel(record)}</td>
                  <td>{record.institution || '未提供'}</td>
                  <td>
                    <button
                      className="icon-button"
                      aria-label={`查看 ${dateLabel(record.exam_date)} ${record.result} 的詳細資料`}
                      onClick={() => onDetail(record)}
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
