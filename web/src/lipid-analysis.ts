// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import type { LabRecord } from './api';
import { isExact } from './model.ts';
import { nonHdlCholesterol } from './analysis-summary.ts';

export interface LipidAssessment {
  nonHdl: string | null;
  sampson: number | null;
  tg: number | null;
  message: string;
}
// Sampson/NIH 2020, mg/dL coefficients; DOI:10.1001/jamacardio.2020.0013.
// Never extrapolate beyond the studied TG range or relabel an estimate as measured.
export function assessLipids(total: LabRecord | undefined, hdl: LabRecord | undefined,
  triglyceride: LabRecord | undefined, sameSample: boolean, enableEstimate: boolean): LipidAssessment {
  const nonHdl = nonHdlCholesterol(total, hdl, sameSample);
  const result: LipidAssessment = { nonHdl: nonHdl.value, sampson: null, tg: null, message: nonHdl.reason };
  if (nonHdl.value === null || !total || !hdl || !triglyceride || !sameSample) return result;
  if (!isExact(triglyceride) || triglyceride.order_code !== '09004C' || total.unit !== 'mg/dL' ||
      hdl.unit !== 'mg/dL' || triglyceride.unit !== 'mg/dL' || triglyceride.numeric_value < 0 ||
      Number(triglyceride.result.trim()) !== triglyceride.numeric_value ||
      triglyceride.exam_date !== total.exam_date || triglyceride.visit_date !== total.visit_date ||
      triglyceride.institution !== total.institution) {
    return { ...result, message: 'non-HDL 保留；TG須為同次採檢的精確mg/dL數值，否則不估算LDL。' };
  }
  const tg = triglyceride.numeric_value;
  result.tg = tg;
  if (tg > 800) return { ...result, message: 'TG超出Sampson/NIH原始研究範圍（≤800 mg/dL），不估算LDL。保留院所LDL原值；需核對直接量測／計算方法及脂血干擾，可對照non-HDL與已量測的apoB。基因相關病因不會使失效公式恢復適用。' };
  const limitation = tg >= 400 ? 'TG≥400 mg/dL，不適用Friedewald公式。' : '';
  if (!enableEstimate) return { ...result, message: `${limitation}non-HDL已計算。Sampson/NIH估算需另行啟用；不代表對個別遺傳性脂質疾病完成驗證。` };
  const tc = Number(total.result.trim()), hc = Number(hdl.result.trim()), nh = Number(nonHdl.value);
  const estimated = tc / 0.948 - hc / 0.971 - (tg / 8.56 + tg * nh / 2140 - tg * tg / 16100) - 9.44;
  if (!Number.isFinite(estimated) || estimated < 0 || estimated > nh) return { ...result, message: `${limitation}公式產生不合理估值，未輸出或裁切成0；請核對資料與檢驗方法。` };
  return { ...result, sampson: estimated, message: `${limitation}Sampson/NIH 2020研究輔助估算（mg/dL），非直接量測，不覆蓋院所結果，也不據此調整用藥。` };
}
