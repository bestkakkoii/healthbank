// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import { dateLabel, type MeasurementGroup } from './model';
import { latestDistinctPair, recordDifference } from './analysis-summary';
export function DifferenceSummary({ group }: { group: MeasurementGroup }) {
  const [latest, previous] = latestDistinctPair(group.records);
  const comparison = recordDifference(latest, previous);
  return <div className="difference-summary">
    {comparison && latest && previous ? <>
      <b>{dateLabel(latest.exam_date)}：{latest.result} {latest.unit}</b>
      <span>前次 {dateLabel(previous.exam_date)}：{previous.result} {previous.unit}</span>
      <span>間隔 {comparison.days} 天 · 差值 {comparison.difference} {latest.unit}
        {comparison.percent !== null && `（約 ${comparison.percent.toFixed(1)}%）`}</span>
      <small>{comparison.crossInstitution ? '不同院所：方法與可比較性需核對；' : ''}原始差異不等同臨床顯著變化或治療效果。</small>
    </> : <p>同日多筆、非精確值或資料不足時不自動挑一筆相減；請查看原始明細。</p>}
  </div>;
}
