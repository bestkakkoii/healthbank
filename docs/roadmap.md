# Product roadmap and data research

Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT.

Implementation status reviewed: **2026-10-28**. This records the RC9 audited implementation;
publication and full verification remain tied to the actual [Release](https://github.com/bestkakkoii/healthbank/releases)
and its exact commit. See [real-environment acceptance](acceptance.zh-TW.md).

## Product direction

The official service is an authenticated data provider. Healthbank supplies its own
acquisition controls, source-aware medical timeline, search, laboratory comparison,
medication context, original-document access and portable datasets. The objective
is useful control over authorized records, not reproducing official pages or their
administrative workflows.

The existing Go/React architecture, embedded frontend and settings-only SQLite/MySQL
storage remain. Official health records, source document handles and credentials
stay transient. No daily-entry requirement, teaching portal or automatic treatment
recommendation is introduced.

## Six implemented work tracks

| Track | Current implementation | Remaining acceptance boundary |
| --- | --- | --- |
| Reliable acquisition | Independent acquisition versus display ranges; maximum supported source ranges; all selected lists then recursive details; progress, bounded retry, cancel/resume and partial-failure preservation. | Real account retention, eligibility and actual source totals cannot be proved by synthetic fixtures. |
| Useful source coverage | 38 additional provider categories join 14 existing categories and five original-file service entries; laboratory retrieval remains its own API and is included by the workspace's all-source action. | 57 entries are capability/service entries, not 57 independent medical datasets or a claim every account supports each one. |
| Source-aware model | Raw text/precise values, distinct dates, institutional/self-entered provenance, exact duplicates and explicit parent-detail links. | Unknown units, methods, specimen relationships and source code systems remain unknown. |
| Own reading and comparison UI | Unified medical/laboratory timeline, full-text/source/institution/date filters, paginated complete returned histories, coverage states and source-scoped prescription-days graphs. Existing age/sex, CKD-EPI and guarded lipid analysis remain. | Extra source values are not automatically merged into laboratory formula inputs without verified comparability. |
| Complete portable data | healthbank.dataset.v1 carries all fetched lists/details, source ranges/states and explicitly retained attachments; UTF-8/BOM, SHA-256 and strict atomic import into an unverified session. Large originals remain explicit separate downloads. | Original official JSON/XML/PDF are downloadable artifacts, not assumed compatible with the Healthbank schema. |
| Isolated connector maintenance | Fixed observed routes, opaque session/category handles, typed errors, bounds, no health persistence and public-source contract appendices. | Live official contract changes require source reinspection and authorized-account acceptance. |

Scientific research continues to supply named constants, coefficients, eligibility
and limitations. This data expansion adds no new diagnostic thresholds, treatment
recommendations or inferred medication adherence.

## Deep-audit maintenance

The RC9 pass rechecked the existing architecture rather than restarting it. It
repairs ambiguous/lossy JSON, raw-date export fidelity, cross-tab session binding,
late and ambiguous mutation responses, source-detail omissions, upstream expiry,
settings-transfer limits and release provenance. All findings and independent
regressions are recorded in [the deep audit](deep-audit.zh-TW.md). No new clinical
formula or treatment rule is enabled by this maintenance.

## Implemented source coverage

Detailed routes, field meanings, date behavior and immutable public-bundle evidence
are in [provider data](provider-data.md), [original documents](provider-documents.md)
and [the existing login/laboratory protocol](nhi_protocol.md).

| Data group | Implemented acquisition and interpretation boundary |
| --- | --- |
| Laboratory and examinations | Main full-period laboratory search, adult/self-paid examinations, source HbA1c/lipid views, bone-density text/images and source-specific checks. Preserve source identities; no automatic cross-method merge. |
| Prescribing and materials | Medicines, chronic prescriptions, ordinary/electronic prescriptions, actual source-confirmed dispensing versus suggested refill windows, self-paid drugs and materials. No proof of ingestion or adherence. |
| Encounters and reports | Western/Chinese/dental encounters, surgery and hospitalization, report text, available inpatient documents and original image/file access. Encounter, admission, discharge, claim and upload dates remain separate. |
| Personal clinical sources | Allergies, official/self-entered vaccinations, COVID source results, card wish annotations, catastrophic certificates, cochlear records and care participation. Read-only status does not modify legal wishes or registration. |
| Prevention and care | Colorectal/oral/breast/cervical/lung, HPV/HPSA screening; diabetes, early CKD, combined diabetes/CKD, metabolic syndrome, hepatitis and COPD tracking; family-doctor/health follow-up and home-care status. Eligibility and source windows remain explicit. |
| Existing measurements | Physiology/body measurements and source-device series/day details, existing diet/exercise/menstrual records. Reading existing entries does not add daily input workflows or native mobile health integration. |
| Original archives | Current job list/details, prepared HTML/XML/JSON/PDF/DICOM downloads, explicit source-offered generation requests and attachment download. Job history is not medical retention history. |

Default maximum-range acquisition uses empty application dates. Supported searches
resolve these to 1912-01-01 through Taiwan today; source-only routes return their
own available arrays. Recent summaries and two/three-examination windows are not
relabelled as lifetime data. An absent result means “not returned”, not “never
performed”. Without an independent same-period count, completeness remains unknown.

## Deliberately separate future decisions

Official SDK admission/contract review, additional identity mechanisms, family
delegation management, general parsing of unverified official file schemas and
new clinical calculations are separate extensions. They are not prerequisites for
this provider-backed workspace release. Administrative insurance operations,
notifications/calendars, surveys, reward programs and official daily-entry screens
are not copied to increase a feature-parity count.

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

Server persistence remains configuration-only. Official medical observations remain
transient; optional personal medication/product notes may be explicitly saved in
browser storage or exported by the user. These device-local records never enter
the server settings contract.

For each additional connector, verify authentication scope, date defaults, source
filters, pagination/count semantics, detail records, amendments and session expiry.
Use wholly synthetic fixtures and distinguish empty, unavailable and incomplete results.
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
