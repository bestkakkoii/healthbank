// Copyright (c) 2026 bestkakkoii llc. All rights reserved.
// SPDX-License-Identifier: MIT
import { useEffect, useId, useMemo, useRef, useState, type ReactNode } from 'react';
import type { LabRecord } from './api';
import {
  categoryLabel,
  dateLabel,
  referenceLabel,
  sourceFlag,
  yearLabel,
  type MeasurementGroup,
} from './model';
import { Copyright, Icon, LevelBadge } from './ui';

const PRINT_ITEM_COLUMNS = 4;

type ComparisonMode = 'date' | 'year';

export interface ComparisonDateRow {
  date: string;
  recordCount: number;
  cells: Map<string, LabRecord[]>;
}

export interface ComparisonSection {
  key: string;
  year: string | null;
  rows: ComparisonDateRow[];
}

// Date is the only row key. Do not align unrelated same-day samples by occurrence,
// collapse shared configuration keys, or replace a missing check date with a visit date.
export function buildComparisonMatrix(groups: MeasurementGroup[]): ComparisonDateRow[] {
  const rows = new Map<string, ComparisonDateRow>();
  for (const group of groups) {
    for (const record of group.records) {
      let row = rows.get(record.exam_date);
      if (!row) {
        row = { date: record.exam_date, recordCount: 0, cells: new Map() };
        rows.set(record.exam_date, row);
      }
      const cell = row.cells.get(group.id);
      if (cell) cell.push(record);
      else row.cells.set(group.id, [record]);
      row.recordCount++;
    }
  }
  return [...rows.values()].sort((a, b) => b.date.localeCompare(a.date));
}

export function groupComparisonRows(
  rows: ComparisonDateRow[],
  mode: ComparisonMode,
): ComparisonSection[] {
  if (mode === 'date') return [{ key: 'all', year: null, rows }];
  const sections = new Map<string, ComparisonSection>();
  for (const row of rows) {
    const year = /^\d{4}-\d{2}-\d{2}$/.test(row.date) ? row.date.slice(0, 4) : '';
    let section = sections.get(year);
    if (!section) {
      section = { key: year || 'unknown-date', year, rows: [] };
      sections.set(year, section);
    }
    section.rows.push(row);
  }
  return [...sections.values()];
}

interface ComparisonTableProps {
  groups: MeasurementGroup[];
  favorites: string[];
  granularity: ComparisonMode;
  saving?: boolean;
  configurableKeys: string[];
  onFavorite: (key: string) => void;
  onDetail: (record: LabRecord) => void;
  onTrend: (id: string) => void;
  onConfigure: (group: MeasurementGroup) => void;
}

// Keep scroll-only state outside the data matrix: dragging the control must not
// rebuild every observation. The table remains the native scroll/touch surface.
function ComparisonScroller({ id, children }: { id: string; children: ReactNode }) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const [scroll, setScroll] = useState({ position: 0, maximum: 0 });

  useEffect(() => {
    const element = scrollRef.current;
    if (!element) return;
    const update = () => {
      const maximum = Math.max(0, element.scrollWidth - element.clientWidth);
      // Safari's elastic overscroll can briefly report positions outside the range.
      const position = Math.min(maximum, Math.max(0, element.scrollLeft));
      setScroll((previous) =>
        previous.position === position && previous.maximum === maximum
          ? previous
          : { position, maximum },
      );
    };
    const observer = new ResizeObserver(update);
    observer.observe(element);
    if (element.firstElementChild) observer.observe(element.firstElementChild);
    element.addEventListener('scroll', update, { passive: true });
    update();
    return () => {
      observer.disconnect();
      element.removeEventListener('scroll', update);
    };
  }, [children]);

  function moveTo(position: number) {
    const element = scrollRef.current;
    if (!element) return;
    const maximum = Math.max(0, element.scrollWidth - element.clientWidth);
    element.scrollLeft = Math.min(maximum, Math.max(0, position));
    setScroll({ position: Math.min(maximum, Math.max(0, element.scrollLeft)), maximum });
  }

  function movePage(direction: -1 | 1) {
    const element = scrollRef.current;
    if (!element) return;
    const dateColumn = element.querySelector('.date-column');
    // Compact layouts scroll every column; only reserve space for an actual frozen column.
    const dateWidth = dateColumn && getComputedStyle(dateColumn).position === 'sticky'
      ? dateColumn.getBoundingClientRect().width
      : 0;
    // Advance within the visible item area, retaining a little overlap for context.
    const step = Math.max(48, (element.clientWidth - dateWidth) * 0.85);
    moveTo(element.scrollLeft + direction * step);
  }

  const overflowing = scroll.maximum > 1;
  const atStart = scroll.position <= 1;
  const atEnd = scroll.maximum - scroll.position <= 1;
  const percent = overflowing ? Math.round((scroll.position / scroll.maximum) * 100) : 0;
  const positionLabel = !overflowing
    ? '全部項目已顯示'
    : atStart
      ? '最左側'
      : atEnd
        ? '最右側'
        : `${percent}%`;

  return (
    <div className="comparison-screen">
      <div className="comparison-scroll-controls" role="group" aria-label="比較表左右捲動控制">
        <div className="comparison-scroll-caption">
          <label htmlFor={`${id}-range`}>左右捲動</label>
          <span aria-hidden="true">{positionLabel}</span>
        </div>
        <div className="comparison-scroll-actions">
          <button
            type="button"
            className="comparison-scroll-button comparison-scroll-left"
            aria-label="向左捲動比較表"
            aria-controls={id}
            title="向左查看檢驗項目"
            disabled={!overflowing || atStart}
            onClick={() => movePage(-1)}
          >
            <Icon name="arrow" size={24} />
          </button>
          <input
            id={`${id}-range`}
            className="comparison-scroll-range"
            type="range"
            min={0}
            max={100}
            step={1}
            value={percent}
            aria-controls={id}
            aria-valuetext={positionLabel}
            title="拖曳滑桿，或使用鍵盤左右方向鍵、Home 與 End"
            disabled={!overflowing}
            onChange={(event) => moveTo((event.currentTarget.valueAsNumber / 100) * scroll.maximum)}
          />
          <button
            type="button"
            className="comparison-scroll-button"
            aria-label="向右捲動比較表"
            aria-controls={id}
            title="向右查看檢驗項目"
            disabled={!overflowing || atEnd}
            onClick={() => movePage(1)}
          >
            <Icon name="arrow" size={24} />
          </button>
        </div>
      </div>
      <div
        id={id}
        ref={scrollRef}
        className="comparison-scroll"
        tabIndex={0}
        role="region"
        aria-label="日期為列、檢驗項目為欄的比較表，可左右捲動"
      >
        {children}
      </div>
    </div>
  );
}

function ResultCell({
  group,
  records,
  interactive,
  onDetail,
}: {
  group: MeasurementGroup;
  records: LabRecord[];
  interactive: boolean;
  onDetail: (record: LabRecord) => void;
}) {
  if (records.length === 0) return <><span aria-hidden="true">—</span><span className="sr-only">這個日期未取得此項檢驗資料</span></>;
  const recent = group.records[0];
  const recentReference = recent ? referenceLabel(recent) : '未提供';
  return (
    <div className="cell-records">
      {records.map((record, index) => {
        const reference = referenceLabel(record);
        const content = (
          <>
            {records.length > 1 && (
              <span className="cell-occurrence">
                同日紀錄 {index + 1}／{records.length}
              </span>
            )}
            <span className="cell-summary">
              <span className="cell-value">
                {record.result || '—'}
                {record.reference_status === 'high' || record.reference_status === 'low' ? (
                  <span
                    className="original-flag"
                    title={record.reference_status === 'high' ? '高於原院參考值' : '低於原院參考值'}
                  >
                    {record.reference_status === 'high' ? '↑' : '↓'}
                  </span>
                ) : null}
              </span>
              <LevelBadge level={record.level} compact />
              {record.official_flag === '1' && <span className="official-flag-badge">原院異常</span>}
            </span>
            {record.institution && <span className="cell-institution">{record.institution}</span>}
            {reference !== recentReference && (
              <span className="cell-reference">當次原院參考：{reference}</span>
            )}
            {records.length > 1 && record.visit_date && record.visit_date !== record.exam_date && (
              <span className="cell-date">就醫 {dateLabel(record.visit_date)}</span>
            )}
          </>
        );
        const className = `result-cell level-surface-${record.level ?? 0}`;
        return interactive ? (
          <button
            type="button"
            className={className}
            key={record.id}
            onClick={() => onDetail(record)}
          >
            <span className="sr-only">{group.name}，{dateLabel(record.exam_date)}，單位 {record.unit}；</span>
            {content}
            <span className="sr-only">；原院參考 {reference}，{sourceFlag(record.reference_status)}，查看明細</span>
          </button>
        ) : (
          <div className={className} key={record.id}>
            {content}
          </div>
        );
      })}
    </div>
  );
}

export function ComparisonTable(props: ComparisonTableProps) {
  const {
    groups,
    favorites,
    granularity,
    saving,
    configurableKeys,
    onFavorite,
    onDetail,
    onTrend,
    onConfigure,
  } = props;
  const id = useId();
  const rows = useMemo(() => buildComparisonMatrix(groups), [groups]);
  const sections = useMemo(() => groupComparisonRows(rows, granularity), [rows, granularity]);
  const printBatches = Array.from(
    { length: Math.ceil(groups.length / PRINT_ITEM_COLUMNS) },
    (_, index) => groups.slice(index * PRINT_ITEM_COLUMNS, (index + 1) * PRINT_ITEM_COLUMNS),
  );

  function renderTable(columns: MeasurementGroup[], tableId: string, interactive: boolean) {
    return (
      <table className="comparison-table">
        <caption>
          檢查日期為列、檢驗項目為欄，日期由新到舊排列。
          {granularity === 'year' ? '依年度分組，保留各次檢查日期，不計算年度平均。' : ''}
          同日多筆結果分別列出，不推定採檢先後或不同項目之間的配對。未取得資料不當作零。
          欄首參考值取自篩選範圍內最近一筆原院資料；不同的當次參考值另列於結果下方。
        </caption>
        <thead>
          <tr>
            <th scope="col" className="date-column">
              <span>檢查日期</span>
              <small>同日多筆分別保留</small>
            </th>
            {columns.map((group, columnIndex) => {
              const favorite = favorites.includes(group.configurationKey);
              const configurable = configurableKeys.includes(group.configurationKey);
              return (
                <th
                  scope="col"
                  className="measurement-column"
                  id={`${tableId}-item-${columnIndex}`}
                  key={group.id}
                >
                  <div className="table-item-heading">
                    <div>
                      <span className="category-caption">{categoryLabel(group.category)}</span>
                      {interactive ? (
                        <button
                          type="button"
                          className="item-name-button"
                          onClick={() => onTrend(group.id)}
                        >
                          {group.name}
                        </button>
                      ) : (
                        <span className="item-name-button">{group.name}</span>
                      )}
                      <div className="table-item-meta">
                        {group.abbreviation !== group.name && (
                          <span className="table-abbreviation">{group.abbreviation}</span>
                        )}
                        <span className="unit-line">單位：{group.unit}</span>
                      </div>
                      <div className="header-reference">
                        最近原院參考：{group.records[0] ? referenceLabel(group.records[0]) : '未提供'}
                      </div>
                    </div>
                  </div>
                  {interactive && (
                    <div className="table-item-actions">
                      <button
                        type="button"
                        className={`icon-button star-button ${favorite ? 'is-favorite' : ''}`}
                        onClick={() => onFavorite(group.configurationKey)}
                        aria-label={`${favorite ? '取消' : '加入'}關注：${group.name}，${group.unit}`}
                        aria-pressed={favorite}
                        disabled={saving || !configurable}
                      >
                        <Icon name="star" size={18} />
                      </button>
                      <button
                        type="button"
                        className="text-button"
                        onClick={() => onTrend(group.id)}
                        aria-label={`查看${group.name}趨勢`}
                      >
                        <Icon name="trend" size={15} />
                        趨勢
                      </button>
                      <button
                        type="button"
                        className="text-button"
                        onClick={() => onConfigure(group)}
                        disabled={saving || !configurable}
                        aria-label={`設定${group.name}基準`}
                      >
                        <Icon name="settings" size={15} />
                        基準
                      </button>
                    </div>
                  )}
                </th>
              );
            })}
          </tr>
        </thead>
        {sections.map((section) => (
          <tbody key={section.key}>
            {section.year !== null && (
              <tr className="comparison-year-row">
                <th scope="rowgroup" colSpan={columns.length + 1}>
                  <span className="comparison-year-title">
                    <b>{section.year ? yearLabel(section.year) : '檢查日期未提供'}</b>
                    <small>
                      {section.year
                        ? `${section.rows.length} 個檢查日期`
                        : '保留原始紀錄，不以就醫日期代替'}
                    </small>
                  </span>
                </th>
              </tr>
            )}
            {section.rows.map((row) => {
              const rowId = `${tableId}-date-${row.date || 'unknown'}`;
              const calendarDate = /^\d{4}-\d{2}-\d{2}$/.test(row.date);
              return (
                <tr key={row.date}>
                  <th scope="row" className="date-column" id={rowId}>
                    {row.date ? (
                      <time dateTime={row.date} className="comparison-date-label">
                        {calendarDate ? (
                          <>
                            <span className="sr-only">{dateLabel(row.date)}</span>
                            <span className="comparison-date-year" aria-hidden="true">
                              {row.date.slice(0, 4)}
                            </span>
                            <span className="comparison-date-slash" aria-hidden="true">/</span>
                            <span className="comparison-date-monthday" aria-hidden="true">
                              {row.date.slice(5).replace('-', '/')}
                            </span>
                          </>
                        ) : dateLabel(row.date)}
                      </time>
                    ) : (
                      <span className="comparison-date-label">日期未提供</span>
                    )}
                    {calendarDate && (
                      <small>
                        <span className="comparison-date-meta-full">
                          民國 {Number(row.date.slice(0, 4)) - 1911} 年
                        </span>
                        <span className="comparison-date-meta-short" aria-hidden="true">
                          民{Number(row.date.slice(0, 4)) - 1911}
                        </span>
                      </small>
                    )}
                    {interactive && (
                      <small className="comparison-date-count">
                        <span className="comparison-date-meta-full">{row.recordCount} 筆原始結果</span>
                        <span className="comparison-date-meta-short" aria-hidden="true">
                          {row.recordCount} 筆
                        </span>
                      </small>
                    )}
                  </th>
                  {columns.map((group, columnIndex) => {
                    const records = row.cells.get(group.id) ?? [];
                    return (
                      <td
                        key={group.id}
                        className={records.length ? '' : 'empty-cell'}
                        headers={`${rowId} ${tableId}-item-${columnIndex}`}
                      >
                        <ResultCell
                          group={group}
                          records={records}
                          interactive={interactive}
                          onDetail={onDetail}
                        />
                      </td>
                    );
                  })}
                </tr>
              );
            })}
          </tbody>
        ))}
      </table>
    );
  }

  return (
    <>
      <ComparisonScroller id={`${id}-scroll`}>
        {renderTable(groups, `${id}-screen`, true)}
      </ComparisonScroller>
      {/* display:none removes the print-only tables from the screen accessibility tree. */}
      <div className="comparison-print">
        {printBatches.map((columns, index) => (
          <section className="comparison-print-section" key={index}>
            <div className="comparison-print-heading">
              <h3>
                檢驗數值對照 · 項目 {index * PRINT_ITEM_COLUMNS + 1}–
                {Math.min((index + 1) * PRINT_ITEM_COLUMNS, groups.length)}／{groups.length}
              </h3>
              <p>日期為列、項目為欄；本段之外的項目接續於其他頁面。</p>
              <Copyright />
            </div>
            {renderTable(columns, `${id}-print-${index}`, false)}
          </section>
        ))}
      </div>
    </>
  );
}
