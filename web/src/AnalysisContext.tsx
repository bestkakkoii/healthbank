// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import { createContext, useContext, useState, type ReactNode } from 'react';
import type { LabRecord } from './api';
import { defaultAnalysis, intervalLabel, recordScenarioStatus, referenceForRecord, type AnalysisPreferences } from './clinical-comparison';

interface AnalysisState {
  preferences: AnalysisPreferences;
  setPreferences: (value: AnalysisPreferences) => void;
  birthDate: string;
  setBirthDate: (value: string) => void;
  selectedDate: string;
  setSelectedDate: (value: string) => void;
  save: () => Promise<void>;
  persisted: boolean;
}
const AnalysisContext = createContext<AnalysisState | null>(null);
export function AnalysisProvider({ initial, persisted, onSave, children }: {
  initial: AnalysisPreferences | null | undefined;
  persisted: boolean;
  onSave: (value: AnalysisPreferences) => Promise<boolean>;
  children: ReactNode;
}) {
  const [preferences, setPreferences] = useState(initial ?? defaultAnalysis());
  const [birthDate, setBirthDate] = useState('');
  const [selectedDate, setSelectedDate] = useState('');
  async function save() {
    if (!(await onSave(preferences))) throw new Error('使用階段已改變，這次設定未套用；請重新確認登入者。');
  }
  return <AnalysisContext value={{ preferences, setPreferences, birthDate, setBirthDate, selectedDate, setSelectedDate, save, persisted }}>{children}</AnalysisContext>;
}
export function useAnalysis(): AnalysisState | null { return useContext(AnalysisContext); }
export function ScenarioBadge({ record }: { record: LabRecord }) {
  const analysis = useAnalysis();
  if (!analysis || analysis.preferences.mode === 'off') return null;
  const status = recordScenarioStatus(record, analysis.preferences, analysis.birthDate);
  if (status === 'unknown') return null;
  const interval = referenceForRecord(record, analysis.preferences, analysis.birthDate);
  const labels = { low: '低於', high: '高於', within: '位於' };
  return <span className={`scenario-badge scenario-${status}`}>
    {analysis.preferences.mode === 'primary' ? '基準' : '比較'}情境：{labels[status]}範本{status === 'within' ? '內' : ''}
    <span>{intervalLabel(interval)}</span>
  </span>;
}
export function AnalysisStamp() {
  const analysis = useAnalysis();
  if (!analysis || analysis.preferences.mode === 'off') return null;
  const scenario = analysis.preferences.mode === 'comparison' ? analysis.preferences.comparison : analysis.preferences.primary;
  const age = analysis.preferences.mode === 'primary' && analysis.birthDate ? '依各次採檢日期計齡' : `${scenario.age ?? '未提供'}歲（固定情境）`;
  return <p className="analysis-context-stamp" role="status">目前額外對照：{analysis.preferences.mode === 'comparison' ? '模擬比較' : '本人／基準'} · {age} · {scenario.sex === 'male' ? '男性' : scenario.sex === 'female' ? '女性' : '性別未提供'}。原院結果與警示保留；無適用範本的項目不推定。</p>;
}
