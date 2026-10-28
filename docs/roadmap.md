# Product roadmap and data research

Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT.

Evidence checked: **2026-10-27**. This file distinguishes implemented rc.7 features, externally blocked live acceptance, and future source-dependent coverage. See [the real-environment acceptance checklist](acceptance.zh-TW.md).
The goal is to help people understand longitudinal observations and prepare useful
questions for their clinicians. The application does not establish diagnoses or
recommend unsupervised treatment experiments.

## Current boundary

Healthbank compares laboratory results, displays trends and configurable reference
rules, imports HTML/ZIP reports, and exports CSV, charts and browser print/PDF.
Reports and official sessions remain transient. SQL contains validated settings and
pseudonymous subject linkage, not medical observations. See the [README](../README.md),
[clinical interpretation rules](clinical-reference.zh-TW.md) and
[connector verification boundary](nhi_protocol.md) for implemented behaviour.

Additional category connectors and **Healthbank-format** JSON/XML imports are
available since **v1.0.3-rc.6** from publicly observed SPA contracts
and synthetic fixtures. They are **not authenticated-live verified**; third-party
NHI JSON/XML/PDF formats, official SDK integration and account completeness are
not implemented. Public documentation alone is not authentication evidence.

## Priorities

Order: complete retrieval → correct normalization → comfortable comparison →
evidence-bound analysis. Do not replace existing patient habits or grow a teaching
portal, daily task system or automatic treatment recommender.

| Priority | Outcome | Current boundary / acceptance gate |
| --- | --- | --- |
| P0 | Actual historical coverage | Verify requested periods, default filters, source selectors, pagination and independent counts against authorized live records. Synthetic protocol fixtures are not live completeness evidence. |
| P0 | Private-device login convenience | Multi-user opt-in remembered ID/card fields; password-manager-native form; manual CAPTCHA. Offline component/storage tests exist; real browser managers and devices still require verification. |
| P0 | Practical age/sex comparison | Two immediate scenarios with sourced intervals and guarded CKD-EPI; original values and flags immutable. Whole-table context bands and opt-in demographics setting sync are implemented in rc.6; source-bound interval coverage remains intentionally limited, not a universal age/sex catalogue. |
| P1 | Time-series continuity and portable Unicode export | Corrected same-day duplicate breaking, time/date-index x-axis modes, display of multiple source values, UTF-8 JSON (optional BOM), original source preservation and regression cases. No unobserved measurement, interpolation, arbitrary causal or danger score. |
| P1 | Additional official numeric results | First inspect preventive/self-paid examinations for actual extra results. Verify every authenticated contract and source overlap before integration. |
| P2 | Optional prescribing/encounter/report context | Listed medications, repeat prescriptions, Western/Chinese/dental encounters, surgeries and inpatient records have bounded source-bound read-only list/detail support, with synthetic contract tests. Per-drug prescription-days charts are present; neither records nor charts establish ingestion, dispensing, adherence, outcomes or live authenticated completeness. |
| Optional | Personal products or medication notes | An optional device-local list with explicit save/export/import/delete and interval matching is implemented; no daily entry or medical notes in server settings. |

Scientific research supplies internal constants, formula coefficients, eligibility
and invalid-calculation guards. It does not justify a knowledgebase or fabricated
five-level categories. Clinical rules, exact versions and provenance are separate
from UI presentation. No runtime or toolkit upgrade authorizes a new medical rule.

## Official data candidates

Unless stated otherwise, the following facts come from the freshly retrieved
[official usage guide][guide]. Its immutable bundle evidence is recorded in
[the protocol document](nhi_protocol.md#evidence). Windows describe the guide, not
guaranteed retention or an application-enforced cutoff.

| Dataset | Documented origin/window | Connector status |
| --- | --- | --- |
| Other laboratory results | Institutional uploads; generally recent three years, with the detailed guide ending coverage two days before login. | Existing connector; authenticated completeness unverified. |
| Outpatient visits, medication and chronic prescriptions | Institutional claims/card uploads; generally recent three years. | Candidate; contract unverified. |
| Hospitalization and procedures | Institutional claims/card uploads from ROC 105 (2016). | Candidate; contract unverified. |
| Imaging/pathology reports | Institutional uploads; generally recent three years. | Candidate; report access does not establish DICOM/image access. |
| Discharge summaries | Downloads announced by MOHW in [2015][discharge]. | Historical existence confirmed; current route/window unverified. |
| Allergies | Institutional entries from ROC 104 (2015), plus separately entered personal records. | Candidate; provenance must remain distinct. |
| Adult preventive examinations | Health Promotion Administration; reporting lag and manual-entry omissions are documented. | Candidate; contract unverified. |
| Cancer screening | HPA results from ROC 105 (2016). | Candidate; contract unverified. |
| Vaccination | CDC-supplied records. | Candidate; contract unverified. |
| Physiological measurements, food/exercise and self-paid examinations | Personal entries; the mobile app also supports health-data upload. | Candidate; no browser-to-Apple-Health capability established. |
| HTML/PDF/XML/JSON downloads | Official download service. | HTML/ZIP imports exist; other schemas require validation. |

An absent result means “not returned”, not “never performed”. The guide describes
aggregated information rather than a formal medical record. Claims diagnoses must
not silently become confirmed active conditions. The conservative laboratory query
floor is not evidence that the official service holds lifetime records.

## Interpretation and provenance

- Keep specimen, unit, assay/equation and institution differences visible. Unknown
  comparability must remain unknown; never silently merge unlike measurements.
- Retain examination, encounter and upload dates separately when supplied. Show
  observation counts and gaps; do not fabricate intervening measurements.
- Distinguish original laboratory references, published guideline rules and personal
  targets. Existing independent warnings must survive personal overrides.
- A medication timeline records prescribing/dispensing evidence, not adherence.
  Patient-confirmed use, OTC products, supplements and allergy notes need attribution.
- Before/after changes are descriptive observations, not proof of cause. Optional
  notes can capture fasting, timing, illness, exercise or device context without
  suggesting a treatment change. [KDIGO][kdigo] discusses biological and analytical
  variability, contextual interpretation and externally validated risk equations.
- Clinical rules need identified sources, eligible populations, units, versions and
  uncertainty. Software prereleases do not justify promoting clinical review drafts
  into defaults; the [current guideline index][kdigo-index] distinguishes them.

## Storage and integration decisions

Durable medical notes, medication history and archives cannot be stored as settings.
Keep new observations session-only or user-exported unless an explicit, separate
opt-in medical-data design establishes ownership, deletion, backup and access rules.
Preserve the current configuration-only SQL contract until that decision changes.

For each additional connector, verify authentication scope, date defaults, source
filters, pagination/count semantics, detail records, amendments and session expiry.
Use sanitized fixtures and distinguish empty, unavailable and incomplete results.
Do not attach an imported report to a verified identity merely because its text matches.

The [official SDK application route][sdk] requires staged review and integration
testing; the linked [management rules][sdk-rules] were revised on 2026-10-12.
Eligibility and suitability for this web application remain unverified. SDK access
is a separate project decision, not an assumed public API or a completed integration.

## Primary sources

[guide]: https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3115S01.be579f6a.js
[discharge]: https://www.mohw.gov.tw/cp-16-20431-1.html
[kdigo]: https://kdigo.org/wp-content/uploads/2026/04/KDIGO-2024-CKD-Guideline.pdf
[kdigo-index]: https://kdigo.org/guidelines/ckd-evaluation-and-management/
[sdk]: https://www.nhi.gov.tw/ch/cp-5901-e351d-3346-1.html
[sdk-rules]: https://www.nhi.gov.tw/ch/dl-26950-70acff31efa540ee939ca186e139b2a4-1.pdf

- NHI usage guide: public-source scope and download formats; immutable evidence above.
- MOHW announcement, 2015-07-21: discharge-summary availability, not current retention.
- [Taiwan FDA, 2024-03-29](https://www.fda.gov.tw/tc/PublishOtherEpaperContent.aspx?id=1502&r=1734024045&tid=4689): cross-institution medication/history review and continuity of care.
- Published KDIGO guideline, practice points 2.1.3–2.1.5, Table 16 and chapter 2.2:
  variability, context and validated risk assessment; not personalized treatment advice.
- NHI SDK application page and rules: current application route and integration scope.
