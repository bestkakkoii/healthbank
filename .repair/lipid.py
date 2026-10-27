# fix: enforce LDL formula boundaries and preserve source lipid measurements
from pathlib import Path
r=Path.cwd()
(r/'web/src/lipid-analysis.ts').write_text('''// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
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
''')
p=r/'internal/lab/value.go';s=p.read_text();s+='''
// DefinitelyAtLeast checks the lower bound without rounding or inventing a point.
func DefinitelyAtLeast(raw, threshold string) bool {
	p := parseValue(raw)
	limit, ok := decimal(threshold)
	return ok && numericKind(p) && p.span.low != nil && p.span.low.Cmp(limit) >= 0
}
''';p.write_text(s)
p=r/'internal/server/reports.go';s=p.read_text().replace('lab.DefinitelyAbove(r.Result, "400")', 'r.Unit == "mg/dL" && lab.DefinitelyAtLeast(r.Result, "400")');s=s.replace('同日三酸甘油脂超過400 mg/dL：請核對LDL是否為直接量測；常見Friedewald計算法不適用於此濃度。','同日同院有TG≥400 mg/dL：須核對是否同次採檢及LDL方法；Friedewald不適用，直接量測亦須核對脂血干擾。保留原院結果，不自動判成失真或正常；可對照同次non-HDL／已量測apoB。');p.write_text(s)
p=r/'web/src/AnalysisTools.tsx';s=p.read_text().replace("import { nonHdlCholesterol } from './analysis-summary';", "import { assessLipids } from './lipid-analysis';")
s=s.replace("  const totals = records.filter", "  const [tgId, setTgId] = useState(''), [estimate, setEstimate] = useState(false);\n  const triglycerides = records.filter(record => record.order_code === '09004C');\n  const totals = records.filter",1)
s=s.replace('  const result = nonHdlCholesterol(total, hdl, confirmed);', "  const tg = triglycerides.find(record => record.id === tgId);\n  const result = assessLipids(total, hdl, tg, confirmed, estimate);")
s=s.replace('<summary>non-HDL 輔助計算</summary>', '<summary>血脂輔助分析（含高TG適用性）</summary>')
s=s.replace('  </div><label className="checkbox-label"><input type="checkbox" checked={confirmed}', '    <label className="field">TG（選填，用於核對LDL公式）<select value={tgId} onChange={event => { setTgId(event.target.value); setConfirmed(false); }}><option value="">不選TG，僅計算non-HDL</option>{options(triglycerides)}</select></label>\n  </div><label className="checkbox-label"><input type="checkbox" checked={confirmed}')
s=s.replace('已核對為同次採檢</label>', '已核對所選項目為同次採檢</label>\n    <label className="checkbox-label"><input type="checkbox" checked={estimate} onChange={event => setEstimate(event.target.checked)} />另顯示Sampson/NIH研究輔助估算（不超出TG≤800 mg/dL）</label>')
s=s.replace("{result.value === null ? '尚未計算' : `non-HDL-C：${result.value} ${total?.unit ?? ''}`}。{result.reason}", "{result.nonHdl === null ? '尚未計算' : `non-HDL-C：${result.nonHdl} ${total?.unit ?? ''}`}。{result.message}")
s=s.replace('    <a href="https://eas-society.org', '    {result.sampson !== null && <p>系統估算LDL-C：{result.sampson.toFixed(1)} mg/dL · Sampson/NIH 2020（非直接量測）</p>}\n    <p className="helper-text">原院LDL、TG及其他結果不會被此計算覆寫。非HDL亦不取代TG相關風險；不由數值診斷遺傳疾病。</p>\n    <a href="https://pubmed.ncbi.nlm.nih.gov/32101259/" target="_blank" rel="noreferrer">Sampson原始研究與適用範圍</a> · <a href="https://eas-society.org');p.write_text(s)
