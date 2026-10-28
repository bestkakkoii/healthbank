// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import { useMemo } from 'react';
import type { LabRecord } from './api';
import { gradingContextLabel, gradingContextText, summarizeGradingContexts } from './grading-context';

export function GradingContextLabel({ record }: { record: LabRecord }) {
  const label = gradingContextLabel(record);
  if (!label) return null;
  return <span className="grading-context-label" title={gradingContextText(record)}>
    <span aria-hidden="true">{label}</span><span className="sr-only">{gradingContextText(record)}</span>
  </span>;
}

export function GradingContextNotes({ records }: { records: LabRecord[] }) {
  const contexts = useMemo(() => summarizeGradingContexts(records), [records]);
  if (!contexts.length) return null;
  return <details className="grading-context-notes">
    <summary>分級依據與適用限制</summary>
    <p className="helper-text">顏色是規則對照，不代表已確認所有個人適用條件。每筆結果保留其分級依據，原院旗標另外顯示。</p>
    <ul>{contexts.map(context => <li key={context.key}><strong>{context.label}</strong><p>{context.note}</p></li>)}</ul>
  </details>;
}
