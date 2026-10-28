// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import type { LabRecord } from './api';

export interface GradingContextDisplay { key: string; label: string; note: string }

// The server supplies the applied basis after personal overrides and independent
// alerts. Do not infer it from a billing code, an item name or a scenario age.
export function recordGradingContexts(record: LabRecord): GradingContextDisplay[] {
  const contexts = (record.grading_contexts ?? [])
    .filter(context => context.label.trim() !== '' && context.note.trim() !== '')
    .map(context => ({ key: JSON.stringify([context.kind, context.rule_id, context.label, context.note]),
      label: context.label, note: context.note }));
  if (contexts.length || record.level == null) return contexts;
  return [{ key: 'missing-grading-context', label: '分級依據未附註',
    note: '這筆顯示分級未附完整來源與適用條件；請核對明細，不能由顏色推定已符合個人判讀條件。' }];
}

export function gradingContextLabel(record: LabRecord): string {
  return recordGradingContexts(record).map(context => context.label).join(' ＋ ');
}

// Complete text is shared by accessible descriptions and derived presentation
// exports. Raw portable observations deliberately contain no computed contexts.
export function gradingContextText(record: LabRecord): string {
  return recordGradingContexts(record).map(context => `${context.label}：${context.note}`).join('\n');
}

// Only repeated explanatory metadata is consolidated. No observations or source
// values are combined, filtered, averaged or selected by these helpers.
export function summarizeGradingContexts(records: readonly LabRecord[]): GradingContextDisplay[] {
  const contexts = new Map<string, GradingContextDisplay>();
  for (const record of records) {
    for (const context of recordGradingContexts(record)) {
      if (!contexts.has(context.key)) contexts.set(context.key, context);
    }
  }
  return [...contexts.values()];
}
