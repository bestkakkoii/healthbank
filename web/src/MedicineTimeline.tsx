// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import { useLayoutEffect, useMemo, useRef, useState } from 'react';
import type { SupplementReport } from './api';
import { medicineSeries, prescribedDayBuckets, prescribedDaySegments, type MedicineEvent } from './medicine-timeline';
import { createDatePosition, dateTime } from './trend-model';

// Prescription records are not medication administration or pharmacokinetic levels.
export function MedicineTimeline({ reports, onDate, onOpenTrends }: {
  reports: SupplementReport[];
  onDate: (date: string) => void;
  onOpenTrends?: ((date: string) => void) | undefined;
}) {
  const groups = useMemo(() => medicineSeries(reports), [reports]);
  const [selected, setSelected] = useState('');
  const [scale, setScale] = useState<'elapsed' | 'ordinal'>('elapsed');
  const [page, setPage] = useState(0);
  const [markedId, setMarkedId] = useState('');
  const eventsHeading = useRef<HTMLHeadingElement>(null);
  const focusPending = useRef(false);
  const active = groups.find(group => group.key === selected) ?? groups[0];
  const pageCount = Math.max(1, Math.ceil((active?.events.length ?? 0) / 100));
  const currentPage = Math.min(page, pageCount - 1);
  useLayoutEffect(() => {
    if (!focusPending.current) return;
    focusPending.current = false;
    eventsHeading.current?.focus({ preventScroll: true });
    eventsHeading.current?.scrollIntoView({ block: 'start', behavior: 'auto' });
  }, [currentPage, active?.key]);
  if (!active) return null;
  const events = active.events.filter(event => event.date);
  const known = events.filter((event): event is MedicineEvent & { validDays: number } => event.validDays !== null);
  const links = prescribedDaySegments(active);
  const buckets = prescribedDayBuckets(active);
  const dates = [...new Set(events.map(event => event.date))];
  const first = dateTime(dates[0] ?? '2000-01-01');
  const last = dateTime(dates.at(-1) ?? '2000-01-01');
  const width = 740, height = 256, left = 57, top = 31, plotW = 650, plotH = 158;
  const maxDays = known.reduce((largest, event) => Math.max(largest, event.validDays), 1);
  const position = createDatePosition(dates, [first, last], scale);
  const x = (date: string) => left + position(date) * plotW;
  const y = (days: number) => top + plotH * (1 - days / maxDays);
  const firstDate = dates[0], lastDate = dates.at(-1), middleDate = dates[Math.floor(dates.length / 2)];
  const middleFits = middleDate && firstDate && lastDate && x(middleDate) - x(firstDate) >= 120 && x(lastDate) - x(middleDate) >= 120;
  const labels = [firstDate, ...(middleFits ? [middleDate] : []), lastDate]
    .filter((value): value is string => Boolean(value)).filter((date, i, a) => a.indexOf(date) === i);
  const pageEnd = active.events.length - currentPage * 100;
  const shownEvents = active.events.slice(Math.max(0, pageEnd - 100), pageEnd).toReversed();
  function showPage(next: number) { focusPending.current = true; setPage(next); }
  function pagination(position: 'top' | 'bottom') {
    return pageCount > 1 && <div className="medicine-event-pagination" role="group" aria-label={`處方紀錄分頁（${position === 'top' ? '上方' : '下方'}）`}>
      <button className="button secondary" type="button" disabled={currentPage === 0} onClick={() => showPage(currentPage - 1)}>較新紀錄</button>
      <span>第 {currentPage + 1}／{pageCount} 頁 · 每頁最多 100 筆</span>
      <button className="button secondary" type="button" disabled={currentPage + 1 >= pageCount} onClick={() => showPage(currentPage + 1)}>較早紀錄</button>
    </div>;
  }
  return <section className="medicine-timeline panel" aria-label="同藥品歷次處方比較">
    <h3>同藥品處方時間圖</h3>
    <p className="helper-text">比較處方給藥日數，不代表實際服用或停藥。</p>
    <details className="medicine-chart-method"><summary>來源分組與圖表判讀方式</summary><p className="helper-text">相容的健保申報藥品代碼放在同組；電子處方代碼另列，自費藥品依院所分開，未提供院所時保留個別來源。無代碼時僅依相同原文名稱及來源比較。同日多值保留範圍，不取平均；缺少日數的日期不連線。虛線只比較相鄰日期已知的給藥日數，不推定實際服藥、用藥濃度、成分等價或處方中斷。</p></details>
    <div className="scenario-controls">
      <label className="field">選擇藥品<select value={active.key} onChange={event => { setSelected(event.target.value); setPage(0); setMarkedId(''); }}>
        {groups.map(group => <option key={group.key} value={group.key}>{group.name} · {group.code || '無官方代碼'} · {group.events.length}筆</option>)}
      </select></label>
      <label className="field">日期排列<select value={scale} onChange={event => setScale(event.target.value === 'ordinal' ? 'ordinal' : 'elapsed')}>
        <option value="elapsed">真實時間距離</option><option value="ordinal">處方日期等距（展開密集點）</option>
      </select></label>
    </div>
    <p role="status">{active.name}：{active.events.length}筆來源紀錄，{known.length}筆具有有效日期及可解析的給藥日數。同一處方可能出現在不同來源，不以筆數推定開立或領藥次數。{scale === 'ordinal' ? '橫軸等距不代表時間間隔。' : ''}</p>
    {known.length > 0 && <div className="medicine-chart-scroll" tabIndex={0} role="region" aria-label="處方給藥日數圖表，可橫向捲動">
      <svg viewBox={`0 0 ${width} ${height}`} role="img" aria-label={`${active.name}之給藥日數趨勢，並非實際服用紀錄`}>
        <title>{active.name} · 歷次處方給藥日數</title>
        <text x={left} y="20" className="axis-title">每次處方給藥日數（日）</text>
        {[0, maxDays / 2, maxDays].map(value => <g key={value}>
          <line x1={left} x2={left + plotW} y1={y(value)} y2={y(value)} className="chart-grid" />
          <text x={left - 8} y={y(value) + 5} textAnchor="end" className="axis-label">{value.toLocaleString('zh-TW', { maximumFractionDigits: 1 })}</text>
        </g>)}
        {links.map((segment, index) => <line key={index} x1={x(segment.from.date)} y1={y(segment.from.validDays ?? 0)} x2={x(segment.to.date)} y2={y(segment.to.validDays ?? 0)} className="chart-line" strokeDasharray="4 4"><title>連線僅比較開立日數，不代表每天服用</title></line>)}
        {buckets.map(bucket => {
          const low = bucket.low?.validDays, high = bucket.high?.validDays;
          return typeof low === 'number' && typeof high === 'number' && low !== high ? <line key={bucket.date} x1={x(bucket.date)} x2={x(bucket.date)} y1={y(low)} y2={y(high)} className="chart-line" strokeDasharray="2 3"><title>同日給藥日數範圍，不推定先後</title></line> : null;
        })}
        {known.map(event => <g key={event.id}>
          <circle cx={x(event.date)} cy={y(event.validDays)} r="5" className="medicine-event-dot" />
          <title>{event.date} · 給藥日數{event.validDays}日 · {event.institution}</title>
        </g>)}
        {labels.map(date => <text key={date} x={Math.max(62, Math.min(width - 62, x(date)))} y={top + plotH + 25} textAnchor="middle" className="axis-label">{date.replaceAll('-', '/')}</text>)}
        <text x={left} y={height - 12} className="chart-copyright">© 2026 bestkakkoii llc. · 不代表實際用藥</text>
      </svg>
    </div>}
    {known.length === 0 && <p className="helper-text">缺乏可比較的給藥日數，因此只顯示下方逐次處方事件，不畫虛構的劑量曲線。</p>}
    <div className="medicine-events"><h4 ref={eventsHeading} tabIndex={-1}>逐次來源紀錄（不聚合、不推定停藥）</h4>
      {pagination('top')}
      <p className="sr-only" role="status">處方紀錄第 {currentPage + 1}／{pageCount} 頁</p>
      {shownEvents.map(event => <div key={event.id} className="medicine-event-row">
        <span>{event.date || event.sourceDate || '日期未知'}</span>
        <span>{event.institution || '院所未知'}</span>
        <span>{event.name} · {event.source}</span>
        <span>給藥日數：{event.days || '未提供'}；原始數量：{event.quantity || '未提供'}（單位未確認）</span>
        <button className="button quiet" type="button" disabled={!event.date} onClick={() => { onDate(event.date); setMarkedId(event.id); }}>標示檢驗日期</button>
        {markedId === event.id && <div className="medicine-date-feedback"><p role="status">已標示 {event.date}；請在趨勢確認當日是否有檢驗結果，同日不代表同次採檢。</p>{onOpenTrends && <button className="button secondary" type="button" onClick={() => { onDate(event.date); onOpenTrends(event.date); }}>前往檢驗趨勢</button>}</div>}
      </div>)}
      {pagination('bottom')}
    </div>
  </section>;
}
