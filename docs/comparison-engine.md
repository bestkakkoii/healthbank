# Clinical comparison contract

Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT.

## Implemented boundary

The source workspace and portable archive do not expand formula eligibility.
The RC9 audit rechecked the source constraints, catalog integrity and eight
independently calculated high-precision CKD-EPI vectors; it enables no new formula
or treatment rule. Personal interval inputs preserve their source decimals and
use authoritative server rational validation instead of approximate browser
ordering/overlap checks.
Additional source measurements and prescriptions retain their own provenance;
matching dates or institutions never confirms one specimen or actual ingestion.
Archive laboratory rows contain source fields only and are normalized again by
the current engine. Historical acquisition metadata is preserved separately from
the imported session's unverified identity and completeness. See
[data-workspace.zh-TW.md](data-workspace.zh-TW.md) for that contract.

`web/src/clinical-comparison.ts` is a pure browser-side engine. `ClinicalComparison.tsx`
exposes two independent scenarios in Settings and Trends, even without observations.
Calculations run in the browser and never replace original results, institutional
flags, personal thresholds or independent alerts. Explicitly saving sends only the
validated comparison mode and scenario age/sex settings to the settings API. A
verified official subject permits SQL persistence under its pseudonymous key;
other session modes retain these settings only in server session memory. Dates
of birth and calculated medical results are not included in the settings payload.

A manually entered age is a **fixed scenario age**: the same entered number is
used across every selected historical date, not back-calculated from a current age.
An optional exact birth date calculates age separately for each selected examination.
No birthday is inferred from a current age. Age 1–130 is a UI input guard, not a clinical validation
range; infant day/month tables are intentionally not approximated by completed years.

Nine named NCKU analytes are implemented: blood creatinine, ALP, BUN, calcium,
uric acid, ALT, CK, phosphorus and blood albumin. Adult
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
Confirmation is bound to the selected observation's content-derived record ID.
A same-session refresh that replaces the selected observation requires explicit
confirmation again, even when the component remains open and selects a new row
automatically. No confirmation is transferred merely because the analyte is the same.

## Applied grading context

Baseline colors now include computed `grading_contexts` identifying the actual
source-reference, named-profile, qualitative, personal or independent-alert basis.
These are qualifications of a numeric comparison, not proof that the individual
meets the source population, assay or collection requirements. The catalog exposes
the preserved `conditions` plus readable `scope_label` and `applicability_note`.
Hypothetical age/sex settings and transient birth-date comparisons do not certify
baseline-profile eligibility. Adult fasting TG and historical adult PCR profiles,
KDIGO classifications and independently triggered alerts retain their explicit
limits; no numerical bands or source abnormal flags are changed.

A personal override replaces the unused base context; independent triggered alert
contexts remain separately visible. Compact observed-value labels link the color to
its basis, and corresponding section/detail notes provide the full qualification.
CSV, SVG and print presentations retain the explanation. Raw portable laboratory
documents contain source fields only and regenerate grading context on import.
The exact sources and population limitations are documented in the
[clinical reference](clinical-reference.zh-TW.md#分級依據與尚未確認的適用條件).

## Display terminology and original source fields

The catalog's bilingual display metadata and exact-source-name glossary do not
participate in clinical identity matching, grouping, unit conversion, rule
selection or formula eligibility. Source item/order names remain available beside
the reviewed display name. A billing code alone does not identify an analyte, a
specimen or a method; translations cannot turn an unknown result into an eligible
measurement. The public API distinguishes source-attributed terms from explicit
literal source-label translations. See the [terminology provenance and complete
item table](clinical-reference.zh-TW.md#中英文名稱與來源原文).

Generic gas parameter names do not establish arterial or venous collection.
Blood creatinine/albumin names do not add a serum/plasma assumption; source AC
pre-meal wording does not confirm a fasting duration. A source upload date remains
separate from examination and visit dates, with its original spelling retained;
it never substitutes for a specimen date or enables a calculation. The raw source
label is also separate from its source code. No clinical cutoffs change because of
these terminology or source-metadata additions.

## Verified sources (rechecked 2026-10-28)

- [NCKU blood creatinine](https://web.hosp.ncku.edu.tw/pathology/Pathology/Page/72):
  Enzymatic method; completed-year partitions 1–2, 3–4, 5–6, 7–8, 9–10, 11–12,
  13–14; ≥15 male 0.67–1.17 and female 0.51–0.95 mg/dL.
- [NCKU ALP](https://web.hosp.ncku.edu.tw/pathology/Pathology/Page/83): Enzymatic
  colorimetric IFCC reference method; age 1–9, 10–12, sex-specific 13–14, 15–16,
  17–18 and ≥19; adult male 40–129 and female 35–104 U/L.
- [NIDDK adult equations](https://www.niddk.nih.gov/research-funding/research-programs/kidney-clinical-research-epidemiology/laboratory/glomerular-filtration-rate-equations/adults):
  2021 CKD-EPI creatinine coefficients, age/sex conditions and /88.4 conversion.

The other seven implemented NCKU tables were also re-read: [BUN](https://web.hosp.ncku.edu.tw/pathology/Pathology/Page/71),
[calcium](https://web.hosp.ncku.edu.tw/pathology/Pathology/Page/87),
[uric acid](https://web.hosp.ncku.edu.tw/pathology/Pathology/Page/74),
[ALT](https://web.hosp.ncku.edu.tw/pathology/Pathology/Page/80),
[CK](https://web.hosp.ncku.edu.tw/pathology/Pathology/Page/85),
[phosphorus](https://web.hosp.ncku.edu.tw/pathology/Pathology/Page/88), and
[blood albumin](https://web.hosp.ncku.edu.tw/pathology/Pathology/Page/104).
The independent expected CKD-EPI values use 70-digit decimal arithmetic and cover
both formula sex variables and both sides of the creatinine knot; numerical
agreement does not establish an individual patient's clinical applicability.

## Implemented extension and remaining acceptance

Nine strictly source-bound analytes are now implemented in `clinical-comparison.ts`:
creatinine, ALP, BUN, calcium, uric acid, ALT, CK, phosphorus and blood albumin.
The selected age and formula sex affect only applicable named scenario bounds.
Source intervals that lack age splits remain unsplit. Missing required age/sex
information, incompatible units or an unrecognized item-name/code pairing prevents
calculation. Template methods and specimens are displayed as source requirements;
the application does not receive enough assay metadata to verify that an individual
original result used the same method. The original hospital flags are never
overwritten. Scenario badges in the date-by-item table, reference bands
on trend charts and source-labelled CSV/print information use the same model.

The analysis settings document may be explicitly saved alongside pseudonymous
settings after official authentication. Date of birth is session-only and never
persisted. A one-click reset changes local scenario immediately but is not a
server update until explicitly saved. This is deliberately different from a
clinical statement of the patient's sex, age or disease risk.

A separate guarded non-HDL calculation requires both compatible exact results
and user confirmation of one collection. Optional Sampson/NIH LDL estimates use
exact TG eligibility boundaries and explicitly exclude known/suspected Type III;
see [the source and formula limits](clinical-reference.zh-TW.md). The NCKU creatinine
method label was rechecked against the primary page on 2026-10-28: Enzymatic method.
Multi-indicator charts use a common
date domain but maintain their own units and axes. No interpolation or
between-visit causal interpretation is generated.

Still requiring external acceptance: institution-specific assay provenance for
individual real results, authorized NHI account completeness, source revisions
and cross-browser real-device testing. These cannot be simulated away.
