# Clinical comparison contract

Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT.

## Implemented boundary

`web/src/clinical-comparison.ts` is a pure browser-side engine. `ClinicalComparison.tsx`
exposes two independent scenarios in Settings and Trends, even without observations.
It never sends demographics or dates of birth, persists them, or replaces original
results, institutional flags, personal thresholds or independent alerts.

A manually entered age means **age at the selected examination**. An optional exact
birth date calculates age separately for each selected examination. No birthday is
inferred from a current age. Age 1–130 is a UI input guard, not a clinical validation
range; infant day/month tables are intentionally not approximated by completed years.

Only named NCKU blood-creatinine and ALP templates are currently implemented. Adult
ranges without finer age partitions remain identical for older ages. Sex is the
variable specified by the source, not inferred from names or identity numbers.
Cross-institution method equivalence is unknown: these are named scenario comparisons,
not automatic fallback ranges or a universal five-level risk scale.

Records require a recognized item name **and** NHI code. Urine creatinine and eGFR
are not blood creatinine. Numeric values must agree with the finite raw decimal;
qualitative values, inequalities, ranges, missing/unknown units and negatives are
not converted into point estimates. Creatinine µmol/L → mg/dL uses /88.4. Reference
comparison uses decimal rationals before any formatting, including unit conversion.

CKD-EPI 2021 is opt-in per selected record, for age ≥18, known source sex variable,
positive exact standardized blood creatinine and confirmation of stable renal
function/formula suitability. The output is explicitly a derived scenario estimate,
not a diagnosis, a dose recommendation, or a replacement for reported eGFR.
Every original row, including same-date duplicates, remains individually selectable.

## Verified sources (2026-10-27)

- [NCKU blood creatinine](https://web.hosp.ncku.edu.tw/pathology/Pathology/Page/72):
  Enzymatic method; completed-year partitions 1–2, 3–4, 5–6, 7–8, 9–10, 11–12,
  13–14; ≥15 male 0.67–1.17 and female 0.51–0.95 mg/dL.
- [NCKU ALP](https://web.hosp.ncku.edu.tw/pathology/Pathology/Page/83): Enzymatic
  colorimetric IFCC reference method; age 1–9, 10–12, sex-specific 13–14, 15–16,
  17–18 and ≥19; adult male 40–129 and female 35–104 U/L.
- [NIDDK adult equations](https://www.niddk.nih.gov/research-funding/research-programs/kidney-clinical-research-epidemiology/laboratory/glomerular-filtration-rate-equations/adults):
  2021 CKD-EPI creatinine coefficients, age/sex conditions and /88.4 conversion.

## Still pending

Whole-table scenario shading and reference bands; explicit opt-in cross-device
analysis settings; additional verified assay profiles; non-HDL pairing with actual
specimen/encounter evidence; multi-indicator chart coordination; official additional
datasets and JSON/XML schema validation. Do not claim these are implemented.
