// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import { useMemo, useState } from 'react';
import type { SupplementReport } from './api';
import { medicineSeries, prescribedDaySegments, type MedicineEvent } from './medicine-timeline';

// Prescription records are not medication administration or pharmacokinetic levels.
export function MedicineTimeline({ reports, onDate }: {
  reports: SupplementReport[];
  onDate: (date: string) => void;
}) {
  const groups = useMemo(() => medicineSeries(reports), [reports]);
  const [selected, setSelected] = useState('');
  const [scale, setScale] = useState<'elapsed' | 'ordinal'>('elapsed');
  const active = groups.find(group => group.key === selected) ?? groups[0];
  if (!active) return null;
  const events = active.events.filter(event => event.date);
  const known = events.filter((event): event is MedicineEvent & { validDays: number } => event.validDays !== null);
  const links = prescribedDaySegments(active);
  const dates = [...new Set(events.map(event => event.date))];
  const first = Date.parse(`${dates[0] ?? '2000-01-01'}T00:00:00Z`);
  const last = Date.parse(`${dates.at(-1) ?? '2000-01-01'}T00:00:00Z`);
  const width = 740, height = 256, left = 57, top = 31, plotW = 650, plotH = 158;
  const maxDays = Math.max(1, ...known.map(event => event.validDays));
  const x = (date: string) => {
    if (scale === 'ordinal') {
      const i = dates.indexOf(date);
      return left + (dates.length < 2 ? .5 : i / (dates.length - 1)) * plotW;
    }
    const time = Date.parse(`${date}T00:00:00Z`);
    return left + (first === last ? .5 : (time - first) / (last - first)) * plotW;
  };
  const y = (days: number) => top + plotH * (1 - days / maxDays);
  const labels = [dates[0], dates[Math.floor(dates.length / 2)], dates.at(-1)]
    .filter((value): value is string => Boolean(value)).filter((date, i, a) => a.indexOf(date) === i);
  return <section className="medicine-timeline panel" aria-label="同藥品歷次處方比較">
    <h3>同藥品處方時間圖</h3>
    <p className="helper-text">依官方藥品代碼聚合；無代碼時僅依相同原文名稱及來源比較。不推定實際服藥、用藥濃度、成分等價或處方中斷。只有給藥日數可確認時才連線。</p>
    <div className="scenario-controls">
      <label className="field">選擇藥品<select value={active.key} onChange={event => setSelected(event.target.value)}>
        {groups.map(group => <option key={group.key} value={group.key}>{group.name} · {group.code || '無官方代碼'} · {group.events.length}筆</option>)}
      </select></label>
      <label className="field">日期排列<select value={scale} onChange={event => setScale(event.target.value === 'ordinal' ? 'ordinal' : 'elapsed')}>
        <option value="elapsed">真實時間距離</option><option value="ordinal">處方日期等距（展開密集點）</option>
      </select></label>
    </div>
    <p role="status">{active.name}：{active.events.length}筆醫囑，{known.length}筆具有可解析的給藥日數。{scale === 'ordinal' ? '橫軸等距不代表時間間隔。' : ''}</p>
    {events.length > 0 && <div className="medicine-chart-scroll">
      <svg viewBox={`0 0 ${width} ${height}`} role="img" aria-label={`${active.name}之給藥日數趨勢，並非實際服用紀錄`}>
        <title>{active.name} · 歷次處方給藥日數</title>
        <text x={left} y="20" className="axis-title">每次處方給藥日數（日）</text>
        {[0, maxDays / 2, maxDays].map(value => <g key={value}>
          <line x1={left} x2={left + plotW} y1={y(value)} y2={y(value)} className="chart-grid" />
          <text x={left - 8} y={y(value) + 5} textAnchor="end" className="axis-label">{value.toLocaleString('zh-TW', { maximumFractionDigits: 1 })}</text>
        </g>)}
        {links.map((segment, index) => <line key={index} x1={x(segment.from.date)} y1={y(segment.from.validDays ?? 0)} x2={x(segment.to.date)} y2={y(segment.to.validDays ?? 0)} className="chart-line" strokeDasharray="4 4"><title>連線僅比較開立日數，不代表每天服用</title></line>)}
        {known.map(event => <g key={event.id}>
          <circle cx={x(event.date)} cy={y(event.validDays)} r="5" className="medicine-event-dot" />
          <title>{event.date} · 給藥日數{event.validDays}日 · {event.institution}</title>
        </g>)}
        {labels.map(date => <text key={date} x={Math.max(62, Math.min(width - 36, x(date)))} y={top + plotH + 25} textAnchor="middle" className="axis-label">{date.slice(5).replace('-', '/')}</text>)}
        <text x={left} y={height - 12} className="chart-copyright">© 2026 bestkakkoii llc. · 不代表實際用藥</text>
      </svg>
    </div>}
    {known.length === 0 && <p className="helper-text">缺乏可比較的給藥日數，因此只顯示下方逐次處方事件，不畫虛構的劑量曲線。</p>}
    <div className="medicine-events"><strong>逐次來源紀錄（不聚合、不推定停藥）</strong>
      {active.events.slice(-100).toReversed().map(event => <div key={event.id} className="medicine-event-row">
        <span>{event.date || event.sourceDate || '日期未知'}</span>
        <span>{event.institution || '院所未知'}</span>
        <span>給藥日數：{event.days || '未提供'}；原始數量：{event.quantity || '未提供'}（單位未確認）</span>
        <button className="button quiet" type="button" disabled={!event.date} onClick={() => onDate(event.date)}>標示檢驗日期</button>
      </div>)}
      {active.events.length > 100 && <p className="helper-text">畫面僅列出最新100筆，原始明細仍保留於本次取得資料。</p>}
    </div>
  </section>;
}
