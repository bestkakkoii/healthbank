// Copyright (c) 2026 bestkakkoi llc. All rights reserved.
// SPDX-License-Identifier: MIT
import type { LabRecord } from './api';
import {
  categoryLabel,
  dateLabel,
  referenceLabel,
  shortDate,
  type MeasurementGroup,
} from './model';
import { Icon, LevelBadge } from './ui';

export function ComparisonTable({
  groups,
  favorites,
  granularity,
  saving,
  configurableKeys,
  onFavorite,
  onDetail,
  onTrend,
  onConfigure,
}: {
  groups: MeasurementGroup[];
  favorites: string[];
  granularity: 'date' | 'year';
  saving?: boolean;
  configurableKeys: string[];
  onFavorite: (key: string) => void;
  onDetail: (record: LabRecord) => void;
  onTrend: (id: string) => void;
  onConfigure: (group: MeasurementGroup) => void;
}) {
  const columns = [
    ...new Set(
      groups.flatMap((group) =>
        group.records.map((record) =>
          granularity === 'year' ? record.exam_date.slice(0, 4) : record.exam_date,
        ),
      ),
    ),
  ]
    .sort()
    .reverse();
  let previousCategory = '';
  return (
    <div
      className="comparison-scroll"
      tabIndex={0}
      role="region"
      aria-label="檢驗數值比較表，可左右捲動"
    >
      <table className="comparison-table">
        <caption>
          依{granularity === 'year' ? '年度' : '檢查日期'}
          比較所有檢驗結果。同一日期的多筆結果分別列出，未檢驗不當作零。列中的參考值取自最近一筆原院資料，各次參考值請開啟結果明細查看。
        </caption>
        <thead>
          <tr>
            <th scope="col" className="item-column">
              <span>檢驗項目</span>
              <small>項目／單位／最近原院參考值</small>
            </th>
            {columns.map((column) => (
              <th scope="col" key={column}>
                {!column ? (
                  '日期未提供'
                ) : granularity === 'year' ? (
                  <>
                    {column} 年<small>民國 {Number(column) - 1911} 年</small>
                  </>
                ) : (
                  <>
                    {dateLabel(column)}
                    <small>民國 {Number(column.slice(0, 4)) - 1911} 年</small>
                  </>
                )}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {groups.map((group) => {
            const showCategory = previousCategory !== group.category;
            previousCategory = group.category;
            const favorite = favorites.includes(group.configurationKey);
            const configurable = configurableKeys.includes(group.configurationKey);
            return (
              <tr key={group.id} className={showCategory ? 'new-category' : ''}>
                <th scope="row" className="item-column">
                  <div className="table-item-heading">
                    <button
                      type="button"
                      className={`icon-button star-button ${favorite ? 'is-favorite' : ''}`}
                      onClick={() => onFavorite(group.configurationKey)}
                      aria-label={`${favorite ? '取消' : '加入'}關注：${group.name}`}
                      aria-pressed={favorite}
                      disabled={saving || !configurable}
                    >
                      <Icon name="star" size={18} />
                    </button>
                    <div>
                      {showCategory && (
                        <span className="category-caption">{categoryLabel(group.category)}</span>
                      )}
                      <button className="item-name-button" onClick={() => onTrend(group.id)}>
                        {group.name}
                      </button>
                      {group.abbreviation !== group.name && (
                        <span className="table-abbreviation">{group.abbreviation}</span>
                      )}
                      <div className="unit-line">
                        <span>{group.unit}</span>
                        <span className="unit-divider">·</span>
                        <span>{referenceLabel(group.records[0])}</span>
                      </div>
                    </div>
                  </div>
                  <div className="table-row-actions">
                    <button className="text-button" onClick={() => onTrend(group.id)}>
                      <Icon name="trend" size={15} />
                      趨勢
                    </button>
                    <button
                      className="text-button"
                      onClick={() => onConfigure(group)}
                      disabled={!configurable}
                    >
                      <Icon name="settings" size={15} />
                      基準
                    </button>
                  </div>
                </th>
                {columns.map((column) => {
                  const records = group.records.filter(
                    (record) =>
                      (granularity === 'year' ? record.exam_date.slice(0, 4) : record.exam_date) ===
                      column,
                  );
                  return (
                    <td key={column} className={records.length ? '' : 'empty-cell'}>
                      {records.length ? (
                        <div className="cell-records">
                          {records.map((record) => (
                            <button
                              className={`result-cell level-surface-${record.level ?? 0}`}
                              key={record.id}
                              onClick={() => onDetail(record)}
                              aria-label={`${group.name}，${dateLabel(record.exam_date)}，結果 ${record.result} ${record.unit}，查看明細`}
                            >
                              {granularity === 'year' && (
                                <span className="cell-date">{shortDate(record.exam_date)}</span>
                              )}
                              <span className="cell-value">
                                {record.result || '—'}
                                {record.reference_status === 'high' ||
                                record.reference_status === 'low' ? (
                                  <span
                                    className="original-flag"
                                    title={
                                      record.reference_status === 'high'
                                        ? '高於原院參考值'
                                        : '低於原院參考值'
                                    }
                                  >
                                    {record.reference_status === 'high' ? '↑' : '↓'}
                                  </span>
                                ) : null}
                              </span>
                              <LevelBadge level={record.level} compact />
                              {record.official_flag === '1' && (
                                <span className="official-flag-badge">原院異常</span>
                              )}
                              {records.length > 1 && record.institution && (
                                <span className="cell-institution">{record.institution}</span>
                              )}
                            </button>
                          ))}
                        </div>
                      ) : (
                        <span aria-label="當日無檢驗資料">—</span>
                      )}
                    </td>
                  );
                })}
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
