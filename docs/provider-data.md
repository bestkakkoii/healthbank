# Healthbank provider data contracts

Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT.

Reviewed **2026-10-28** (Asia/Taipei). This appendix records the additional
provider connectors in `internal/nhi/provider.go`, `provider_specs.go` and
`provider_extensions.go`.
It extends the [session and laboratory protocol](nhi_protocol.md); it is not
an official, supported public API specification or evidence of live account access.

## Product boundary

The official service supplies authorized data. Healthbank organizes the returned
records using its own search, timeline, comparison, retrieval progress and portable
dataset. The connector does not reproduce the website's calendars, daily-entry
workflow, administrative forms, notifications or risk calculators. Reading an
existing self-entered record does not add a new official record.

Every request uses the existing fixed official HTTPS origin, per-person in-memory
Cookie/JWT session, timeouts, typed errors and expiry handling. There is no generic
upstream URL proxy and no medical data write to the settings database. Some
observed read operations use POST; their request payloads are fixed read selectors,
not the nearby add/update/delete operations present in the same bundles.

## Acquisition contract

`GET /api/data/catalog` describes acquisition capability. The provider additions
expose `id`, `label`, `group`, `range_mode` and `has_details`. The application uses
the existing authenticated `GET /api/supplementary?category=...` response shape.
An opaque, session/category-bound `detail_id` is passed back as `detail=...` to
retrieve its associated record. No official row identifier is accepted from the
browser or exported as a reusable authentication credential.

- `source`: the official route has no observed date search. Application display
  filters do not add invented date parameters. The response states the source's
  returned window and never declares an all-lifetime guarantee.
- `date_search`: empty application dates resolve to **1912-01-01 through Taiwan
  today**. HbA1c reads its default page and then explicitly searches that period,
  even when the default page has no recent observations. Lipid, physiology,
  body-measurement, family-doctor and exercise-device lists discover source items;
  their details issue the explicit period search. A list alone is not completion
  of those detail searches.
- Physiology and exercise-device reads include source raw records when provided,
  explicitly requested date-series rows,
  and any returned device-day indices. Device-day records have nested opaque
  references; the acquisition queue must continue through those references.
  Source indices carry `kind: source_index` and are not fabricated measurements.
- The complete response body is bounded to 16 MiB per request, 50,000 records,
  and 65,536 bytes per projected scalar. Nested prescription refill arrays are
  capped at 1,000 entries. Exceeding a bound fails the affected operation instead
  of presenting a truncated array as a successful complete response.
- Every category has `complete: false` because these contracts lack an
  independently verified total for the same requested period. Successfully
  reading all returned records is different from proving that the official
  system retains every historical record.

Successful list refreshes replace only that category's detail/document references.
Failed refreshes keep the earlier successful references. Authentication expiry
clears all provider references. Cancellation propagates to the official HTTP
request. No automatic login retry or CAPTCHA interpretation is added.
Refreshing a parent device index stages replacement child references and retires
the previous descendants only after every response in that operation validates.
A failed parent refresh keeps earlier successful child references usable. Ordinary
clinical detail reads do not copy the entire reference map for each historical row.

## Additional categories

Paths below are relative to `https://myhealthbank.nhi.gov.tw/api/ihke3000/`.
The code preserves the observed path casing. `GET` is the default unless marked
`POST`. Roots are exact observed source arrays/objects; parsing is case-insensitive
only after rejecting conflicting case variants.

These **38 provider categories** comprise seven `date_search` categories
(`glucose`, `lipids`, `physiology`, `body_measurements`, `family_followup`,
`family_health_followup`, `device_activity`) and 31 `source` categories. They are
additional to the existing encounter, laboratory and document connectors. A
category count describes acquisition routes, not a count of available records or
a statement that every authenticated person is eligible for each service.

| Category ID | List/read contract | Data scope |
| --- | --- | --- |
| `allergies` | `ihke3202s01/SP_IHKE3202S01`, root `sP_IHKE3202S01`; `SP_IHKE3202S04`, root `sP_IHKE3202S04` | Institutional registration and self-entered records remain separate; upload date is not onset date. |
| `vaccinations` | `ihke3203s01/SP_IHKE3203S01`, root `sP_IHKE3203S01`; `SP_IHKE3203S03`, root `sP_IHKE3203S03` | Official vaccination source versus self-entered vaccination records. |
| `organ_wishes` | `ihke3204s01/pageload`, status object | Existing card annotation only; card annotation date is not signature date. |
| `catastrophic` | `ihke3209s01/SP_IHKE3209S01`, root `sP_IHKE3209S01` | Certificate item and validity dates; source excludes category six. |
| `cochlear` | `ihke3211s01/page_load`, roots `maindata01`, `maindata02`, `maindata03` | Claims, provider-entered and self-entered implant records are labeled separately. |
| `e_prescriptions` | `IHKE3212S01/page_load`, roots `main_data`, `hist_list` | Both recent and historical electronic prescriptions; observed refill evidence preserved. |
| `self_paid_medicine` | `IHKE3220s01/getItem`, root `sp_IHKE3220S01_data` | Provider-uploaded self-paid drugs, separate from local supplements and self-paid health checks. |
| `self_paid_material` | `IHKE3221s01/getItem`, root `sp_IHKE3221S01_data` | Provider-uploaded medical materials. |
| `care_plans` | `IHKE3213S01/page_load`, root `myplan` | Existing plan enrollment and closure dates. |
| `screen_colorectal` | `ihke3404s02/SP_IHKE3404S02`, root `sP_IHKE3404S02_Data` | Colorectal screening; source normally limits display to three results. |
| `screen_oral` | `ihke3404s03/SP_IHKE3404S03`, root `sP_IHKE3404S03_Data` | Oral screening. |
| `screen_breast` | `ihke3404s04/page_load`, root `sP_IHKE3404S04_Data` | Breast screening. |
| `screen_cervical` | `ihke3404S05/SP_IHKE3404S05`, root `sP_IHKE3404S05_Data` | Cervical screening and source cytopathology text/code. |
| `screen_lung` | `ihke3404s06/page_load`, root `sp_IHKE3404S06_Data` | Lung screening. |
| `screen_hpv` | `ihke3404s07/page_load`, root `sp_IHKE3404S07_Data` | HPV testing. |
| `screen_hpsa` | `ihke3404s08/page_load`, root `sp_IHKE3404S08_Data` | Stool H. pylori antigen testing. |
| `kinmen` | `ihke3405s01/SP_IHKE3405S01`, root `sP_IHKE3405S01_Data` | Source's most recent eligible Kinmen examination, not a lifetime archive. |
| `glucose` | `ihke3406s01/SP_IHKE3406S01`, then POST `ihke3406s02/SP_IHKE3406S02` | HbA1c (`dCode: 09006C`) full requested period; the page name does not establish coverage of every glucose analyte. |
| `lipids` | `ihke3407s01/SP_IHKE3407S01`, root `RecordList` | Source item discovery/current summaries, followed by each item's full-period detail. |
| `physiology` | `ihke3205s01/page_load`, roots `maindata`, `maindata_10568` | Source item discovery; raw entries and explicitly searched series in details. |
| `covid` | `IHKE3410S01/page_load`, roots `sp_IHKE3410S01`, `S02`, `S04`, `S05`, `S09` | Tests, source observations, vaccination and sample/confirmation dates; no personal QR identifier forwarded. |
| `diabetes_care` | `ihke3504s01/SP_IHKE3504S01`, root `sP_IHKE3504S01_Data` | Diabetes care enrollment and follow-up details. |
| `ckd_care` | `ihke3505s01/SP_IHKE3505S01`, root `sP_IHKE3505S01_Data` | Early CKD enrollment and follow-up details. |
| `hepatitis_care` | `ihke3507s01/SP_IHKE3507S01`, root `sP_IHKE3507S01_Data` | B/C hepatitis care enrollment and follow-up details. |
| `copd_care` | `IHKE3513S01/page_load`, root `main_Data` | COPD enrollment and follow-up details. |
| `diabetes_ckd_care` | `ihke3516s01/SP_IHKE3516S01`, root `sP_IHKE3516S01_Data` | Combined diabetes/early CKD enrollment and follow-up details; source page describes a recent three-year window. |
| `metabolic_care` | `ihke3517s01/SP_IHKE3517S01`, root `sP_IHKE3517S01_Data` | Metabolic-syndrome care enrollment and follow-up details; no independent diagnosis or risk calculation. |
| `hepatitis_test` | `ihke3514s01/page_load`, root `maindata` | Source C hepatitis antibody observations. |
| `home_care` | `IHKE3712S01/page_load`, `hasdata` plus source status fields | Source's recent home-care enrollment, closure, phase and care institutions. |
| `body_measurements` | `IHKE3512S01/page_load`, root `maindata`, then POST `IHKE3512S02/search` per returned item | Current summaries plus full-period history; institutional, self-entered and preventive-care source labels remain distinct. |
| `family_doctor` | `ihke3720s01/SP_IHKE3720S01`, objects `sP_IHKE3720S01`, `sP_IHKE3720S02` | Existing plan status and care provider, no consent/enrollment mutation. |
| `family_followup` | `ihke3721s01/SP_IHKE3721S01`, root `sP_IHKE3721S02` | Existing follow-up items with explicit period detail searches. |
| `family_health_followup` | `ihke3726s01/SP_IHKE3726S01`, array `sP_IHKE3726S02` and object `bp_IHKE3726S02` | Health and blood-pressure follow-up items, with full-period detail search; suggested next-check dates remain metadata, not observed dates. |
| `diet` | POST `ihke3206s01/IHKE3206S01`, root `dietRecordList`; POST `SP_IHKE3206S08` per meal | Existing self-entered date, food text, source calorie estimate and selected-food details; not re-estimated. |
| `exercise` | POST `ihke3207s02/page_load`, root `sportsRecordList` | Existing self-entered activity; duration/source intensity available in details. |
| `device_activity` | POST `IHKEMASTER/SP_IHKE3099S05`, body `{ "c_parent_fun_code": "IHKE3207" }`, root `sp_IHKE3207S01` | Discovers only source-provided `IHKE3207S05?s=...` activity selectors, then reads period series and nested device-day records. |
| `menstrual` | `ihke3208s01/SP_IHKE3208S06`, root `sP_IHKE3208S06` | Existing recorded periods; no cycle/fertility prediction or calendar mutation. |
| `osteoporosis` | `ihke3411s01/page_load?s_type=&s_sort=A1`, root `sp_IHKE3411S01_data` | All source types, report details and source-available image attachments. |

## Details and record semantics

Electronic prescription details use `IHKE3212S02/page_load?tid=...&recipeType=...`,
with a `treatData` object and a `list` drug array. Drug names, code, generic name,
source dosage/frequency/route, unit, quantity and prescribed days are retained.
`refillList[].rel_DATE_STR` produces an explicit source-confirmed dispensing event;
`recipe_MEMO` without a dispensing date remains a **suggested** window field.
Neither is evidence of ingestion or adherence. An available original PDF is bound
by the document connector to the same opaque source reference.

Self-paid drug/material details use the same `getItem` route with source-bound
`rid`. If a response supplies a different row identifier, the detail is rejected.
Original internal row IDs, personal IDs and source URLs are not returned as fields.
The public components do not establish that e-prescription `drug_CODE`, self-paid
`order_CODE` and claimed-drug codes share one identifier namespace. Electronic
prescription series remain category-scoped; self-paid series also require the same
institution. A missing institution cannot establish equivalence between custom
self-paid codes. The original code string is preserved.

Care details use observed read-only POST bodies `{ "rid": <bound row> }`:

| Category | Route | Root |
| --- | --- | --- |
| Diabetes | `ihke3504s02/SP_IHKE3504S02` | `sP_IHKE3504S02_Data` |
| Early CKD | `ihke3505s02/SP_IHKE3505S02` | `sP_IHKE3505S02_Data` |
| Hepatitis | `ihke3507s02/SP_IHKE3507S02` | `sP_IHKE3507S02_Data` |
| COPD | `ihke3513s02/page_load` | `sp_IHKE3513S02_Data` |
| Combined diabetes/early CKD | `ihke3516s02/SP_IHKE3516S02` | `sP_IHKE3516S02_Data` |
| Metabolic-syndrome care | `ihke3517s02/SP_IHKE3517S02` | `sP_IHKE3517S02_Data` |

Care enrollment, visit, blood collection, urine collection, lung-function, CAT and
mMRC dates remain separately labeled. The diabetes source oddly names a displayed
eGFR field `blood_ALB`; Healthbank follows the verified **source display meaning**,
records that original field name, and does not mislabel it as albumin. Source
sentinels such as `9999` are retained as source text; these additional sections are
not automatically fed into laboratory grading, eGFR formulas or numerical trends.
The combined-care source labels `blood_GA` with `mg/dl`; the projection explicitly
identifies this as the source's unit rather than silently correcting or interpreting
it. Metabolic-care BMI, waist, blood-pressure and lipid values retain their supplied
precision without independently classifying the person.

Lipid detail POST `ihke3407s02/get_point` uses `dCode`, `dStr`, `dEnd` and root
`sP_IHKE3406S02_Data`. Family follow-up POST
`ihke3721s01/SP_IHKE3721S07` uses `dType`, `dStr`, `dEnd` and root
`sP_IHKE3721S07_Data`. Codes/names come from the retrieved source index; clients
cannot provide arbitrary upstream selectors. A current summary's unit is not
silently assigned to older detail rows lacking a unit.

Health/BP follow-up POST `ihke3726s01/SP_IHKE3726S07` uses the same three parameter
names and root `sP_IHKE3726S07_Data`. It retains `assay_VALUE`, `assay_VALUE_1` and
the source's combined `c_ASSAY_VALUE` separately. The `IHKE3721S01` and
`IHKE3721S03` public bundles also contain these 3726 components; file naming does
not imply that the 3726 contract was invented.

Body-measurement POST `IHKE3512S02/search` sends `dCode` from the returned `code`
field, plus `dStr` and `dEnd`; it does not substitute the similarly named summary
field `c_ORDER_CODE`. Root `maindata` retains actual `real_INSPECT_DATE` and source
provenance. For the observed `BP` selector, the source identifies `assay_VALUE` as
systolic and `assay_VALUE_1` as diastolic. No historical unit is inferred from a
current summary, and other selectors keep their raw channel identity.

Physiology detail first reads POST `ihke3205s02/page_load` with `cCode`, keeping
`sP_IHKE3205S04` raw records and any `devicelist` indices. It then reads POST
`ihke3205s02/SP_IHKE3205S08` with `cSDate`, `cEDate` (YYYYMMDD), and three source
item selectors. The combined blood-pressure/heart-rate selector `10568` maps to
the observed `105`, `106`, `108` selectors; BMI source `103` requests the source's
height/weight selectors `101`, `102`, without inventing a measured BMI.
The returned `sP_IHKE3205S09` value channels retain their source identities rather
than assuming whether they represent point observations, bounds or aggregates.
Source-available device days use
`ihke3205s04/pageload?code=...&cal_date=...`, bound exclusively from returned
`devicelist[].edate`. The official source can limit mobile-ingested history to
recent months even when the application requests a broader period.

Exercise-device discovery extracts a bounded `s` selector only from the known
relative `IHKE3207S05` page path. Other menu paths and absolute URLs are ignored;
they are never fetched as arbitrary endpoints or forwarded to the browser. POST
`ihke3207s05/page_load` sends `cCode`, `cSDate`, `cEDate` for the full selected
period, followed by the same verified series and device-day contracts described
above. Metadata-only or empty source responses create no fabricated measurements.
The application does not access the phone's health database directly and cannot
prove that an absent source menu means the person has no device records.

Exercise detail POST `ihke3207s02/SP_IHKE3207S04` sends only `c_rowid` and reads
`oneRecordList`; boolean `ResultStatus: true` is required. Empty detail arrays
and a provided nonempty `ROW_ID` that differs from the selected record are
rejected; missing/empty row IDs do not independently prove identity. `SPORTDATE_DATE`, `SPORTDATE_HOUR`,
`SPORTDATE_MINUTE`, duration, source code and intensity retain their separate
source meanings. No save/delete or calorie-calculation endpoints are invoked.

The diet source's Vue loop passes its iteration key to the existing record read
`ihke3206s01/SP_IHKE3206S08`. Accordingly, the bounded `dietRecordList` parser
accepts arrays and keyed objects: array entries bind their numeric index, while
object entries bind the exact string key. The `{ "DATA01_SEQ": ... }` read body
preserves that number/string distinction. A similarly named property inside a
row cannot replace the observed selector, and duplicate object keys are rejected.
The response must report boolean `ResultStatus: true` and include
`IsSelectFoodList`; source dates, hours, minutes and each selected `CODE` remain
separate exact strings. The general `foodList` catalog is not evidence of actual
intake and is not used to recalculate calories, nutrients or portions. None of the
adjacent save, delete or date-edit operations is called.

Osteoporosis detail `IHKE3411S02/page_load?crid=...&ctype=...` reads report fields
and delegates any source-provided image preparation/download references to the
document connector.

Screening projections retain source `lung_RADS`, breast `diagnosis_CODE`, HPV
`hpv_diagnosis_code_rpt`, and source `assay_RESULT` text. COVID entries retain
`inspect_MODE`, `assay_DISEA`, and `recv_DATE` alongside separate collection/report
dates. These are source strings, not newly inferred diagnoses or formula inputs.
A detail must contain recognized source evidence before parent date/institution
context is inherited; an unknown or empty row cannot become a successful event.

## Preservation, privacy and verification

Known clinical field strings retain exact decimal spellings, inequalities,
source flags, duplicate rows, units and distinct dates. The connector projects
documented clinical fields rather than forwarding arbitrary source JSON. Unknown
fields, raw personal IDs, JWTs, QR credentials, arbitrary URLs and unrelated menu
metadata are excluded. This is not a claim that the projected display is a
byte-for-byte copy of the entire upstream response. Original user-requested
official archives/documents are a separate download channel.

All upstream JSON rejects duplicate keys, invalid UTF-8 and isolated escaped
surrogates before decoding. Typed authentication/laboratory envelopes also reject
case-fold aliases. Case-sensitive source data maps keep their key distinctions.
Projected row/field/text limits are checked after combining all response roots
and before replacing live references; overflow never silently truncates data.
Explicit source expiry, including case variants of its error field and bounded
HTTP 200 error bodies with a JSON Content-Type on document routes, clears the
connector. A wrong-MIME non-document response is rejected as invalid content,
without inferring authentication state. Invalid date
inputs are rejected before any token-refresh request.

Text remains text; source HTML is never executed. No third-party scripts, trackers,
runtime schema services or new dependencies are added. Imports and downloaded
datasets remain unverified documents and cannot authenticate a subject.

Focused synthetic tests cover source paths/payloads, original precision and
duplicates, source versus self-entry provenance, explicit range search despite
empty recent defaults, nested device-day retrieval, dispensing versus suggestion,
stale/wrong-category/wrong-parent references, atomic failed refresh, size bounds,
null/malformed/error responses and expiry disposal. These tests do not establish
authenticated official data availability, completeness, retention or eligibility
for an individual account.

## Public-source evidence

The public SPA files were retrieved as inert text at **2026-10-28T02:10:55Z**
(10:10:55 Taiwan). No official script was executed and no credentials or medical
records were supplied. Public component presence proves an observed shipped
contract, not current menu eligibility or successful authenticated server behavior.
RC9 independently re-fetched ten relevant bundles on the same date, including
exercise detail, breast/lung/HPV/HPSA screening, COVID and original-file download
components. Their listed hashes matched; the additional download component hashes
are recorded in [provider-documents.md](provider-documents.md). This does not
relabel the earlier all-bundle snapshot as a new live-account verification.
Each file below is available under
`https://myhealthbank.nhi.gov.tw/IHKE3000/js/`.

| Official file | SHA-256 |
| --- | --- |
| [IHKE3202S01.6ccd65a9.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3202S01.6ccd65a9.js) | `bfdf609da4001b4c6e93cc499effb1c6a65b8967c4ca6f4cd6349d27a3ed85f6` |
| [IHKE3203S01.6f76972a.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3203S01.6f76972a.js) | `bef9a07d592ce43dbd4892126f62d89bdc20a4ec594b92862d000765bfe7256b` |
| [IHKE3204S01.a537a150.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3204S01.a537a150.js) | `1779a022ad2b3d31477867cfa7089606c36cba8f714ccca68b31eeadbccd1ce8` |
| [IHKE3205S01.73832712.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3205S01.73832712.js) | `66ec6f19e079b3a82f20d2eb5c233feee63ff3eb5eb618907c1b1adf6ffe54e1` |
| [IHKE3205S02.4e3c4e04.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3205S02.4e3c4e04.js) | `cfee70c0eee6f4baa5e1b6410255f73d6fcbc4b94010df57e7b20dabf8b4a7bb` |
| [IHKE3205S04.d96d0d1a.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3205S04.d96d0d1a.js) | `a595e6448c51a3411427e1aa89afe4b79c18bb56ab0d82f7a1eaade7aaa5aefc` |
| [IHKE3206S01.a6fa8842.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3206S01.a6fa8842.js) | `e8665d846e7033b08dfbf267c197bc83de41827a2621c1376ea921c46c45c3ec` |
| [IHKE3207S01.c7276474.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3207S01.c7276474.js) | `8a79b2261d56fe24300fe439bdb18900681b98ae429dc4f24b21ca584e2350f1` |
| [IHKE3207S02.ecc284ce.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3207S02.ecc284ce.js) | `c96b75e1228929685b9bc1d60371fced981ef9c7bdc09ac84fb5e3b514d4604f` |
| [IHKE3207S05.ca63f283.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3207S05.ca63f283.js) | `a973ad371030507f7a33362bd603792ce95e485870f0db94641dbefce3ed640c` |
| [IHKE3208S01.fdea6059.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3208S01.fdea6059.js) | `456679d4d703f3a5674ef51d0329781da00d7ddc3d9bc4156cefec11421b2b44` |
| [IHKE3209S01.b24d9c4d.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3209S01.b24d9c4d.js) | `ce8bbf2c774a28a3296e6fa04889b54d43fcd3254df63f373f9ba519995f4232` |
| [IHKE3211S01.2aa8a3a2.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3211S01.2aa8a3a2.js) | `82bfd1baa1959ca08b96ba331a2bf7565891a3142c9d713716f3f60d17fd82f6` |
| [IHKE3212S01.647a1fc8.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3212S01.647a1fc8.js) | `4a31db2e715245c2db4152461bfd49148518be60ee3516d1e508059e5efb152d` |
| [IHKE3212S02.6ed8b6f5.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3212S02.6ed8b6f5.js) | `7112f6aa027f4b9fb1ffa5cf27483007af503520c6a01087af189fb36746052b` |
| [IHKE3213S01.4e42c419.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3213S01.4e42c419.js) | `d3bd93f3e15afc4c4fd636979e89d86d243c0b85938b40aa345976d8af6adec3` |
| [IHKE3220S01.4a9c35f1.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3220S01.4a9c35f1.js) | `1b9d29e53170f30f4a7219b8893db140fbfacb72784096fc0cad46fd83940754` |
| [IHKE3220S02.bd47083e.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3220S02.bd47083e.js) | `8d54789c1b6be0009592294160ac8dd5cdb45fb52044f50cf4cd6d46ff3692af` |
| [IHKE3221S01.4424f5d8.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3221S01.4424f5d8.js) | `db53b14104a43c4eab289216c80f69e89f98e2035c21891106019286a0252978` |
| [IHKE3221S02.6d51f2f5.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3221S02.6d51f2f5.js) | `9c34680bb080f4476dfa88894ccfca03af0b04b22320a675c67c4e5f3f1028fb` |
| [IHKE3404S02.6d465ff6.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3404S02.6d465ff6.js) | `77377e52854d885b890022c97b0eba7e433b49f60be10b4750d1f1b7b2d7b066` |
| [IHKE3404S03.0f94c5cd.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3404S03.0f94c5cd.js) | `37eb38534430418b36c4d36bb0e89993ff47f1f42bcd4274382ec299542fcae6` |
| [IHKE3404S04.e067c757.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3404S04.e067c757.js) | `87073930634bae2bc3e9571c5c1474b8279ee072caa52cb051e69b7d949898ee` |
| [IHKE3404S05.5af01eec.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3404S05.5af01eec.js) | `25250f9f3411d5fc1d1dd0c738d9a8bfc4e64e2f547684e8f7b40d5367c5eac7` |
| [IHKE3404S06.f883a2cf.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3404S06.f883a2cf.js) | `f77b532b6a4b5c06b9b4f39ebb23ab0d1bd58157ef36f3689bd16854fc7e93a4` |
| [IHKE3404S07.9698d7e5.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3404S07.9698d7e5.js) | `bcda768fa85f98ab0473f46817b8835508cfdd6181a7fe834bd366cd86eca6f8` |
| [IHKE3404S08.4f024a4c.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3404S08.4f024a4c.js) | `b58326da4aca51166225212ac6e52edbf71c46653d2e00d46d069986326b53ec` |
| [IHKE3405S01.eaad4192.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3405S01.eaad4192.js) | `d6a689116845bdd6e8c85c126483c874b44fdb199c3bae2ea8b0800c7b9b57eb` |
| [IHKE3406S01.674eac69.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3406S01.674eac69.js) | `88a98e59bf1a8f58a1bed700056ba8644866c41d96188fd891288d7180f658a1` |
| [IHKE3407S01.5f2d9f07.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3407S01.5f2d9f07.js) | `7c81354d0a2a7df868324f64ca3e11cc0d971081b7c3afc4e1aa6c8871281f4c` |
| [IHKE3407S02.17d29a96.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3407S02.17d29a96.js) | `fd64060b81dd6154c43e02a54aad6611d3fda6b4a706725e52e34a3ef7b6dbe4` |
| [IHKE3410S01.96611c9c.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3410S01.96611c9c.js) | `087caf08a72ce7cb85cb5aa4da0e161a657e7274c80dc1575cc2b476967b26d3` |
| [IHKE3411S01.6e442f87.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3411S01.6e442f87.js) | `6bd629d09576a155d100eba2950ff6501ee96b8d21821075e93b7426332072ce` |
| [IHKE3411S02.cc4529c5.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3411S02.cc4529c5.js) | `d70a6a67b2ce1c2ab3ad0df6fc54d4e211c100a910207febfb02d09c7118861c` |
| [IHKE3504S01.46f8fe96.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3504S01.46f8fe96.js) | `236c3a7668b9116e2ac7223b3831d6dae0ac5579d7533aeba915eb18a89a06e4` |
| [IHKE3504S03.62771fcb.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3504S03.62771fcb.js) | `5651b6ed181dfab95e3ebbc2b17b5e541eaa196303ebf469a3b47aad5198deff` |
| [IHKE3505S01.6ea6a451.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3505S01.6ea6a451.js) | `b3e529271e0c1145f4daa079c8e303b35735ffe6511755c75aade8f41d1d2c77` |
| [IHKE3505S03.cb77998a.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3505S03.cb77998a.js) | `c00f66f7cb901881bb4a5031d3de74d269f12ebf32d175bf521c23b88a5f5699` |
| [IHKE3507S01.23bdf7f0.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3507S01.23bdf7f0.js) | `d1f369a25b49569ca35e76a7f8fa63d859285e28cbdaadf1627ac87bc9c47852` |
| [IHKE3507S03.d3bab70e.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3507S03.d3bab70e.js) | `cc80dfa44d84c5c3b079e3cdf8f4cc26c8fcf3aba559bfb1473db2e28acb607d` |
| [IHKE3512S01.936037f9.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3512S01.936037f9.js) | `9e2e474edf1472bb3872ca981b052b9c250aad16b084033db31d6b1a234804c8` |
| [IHKE3512S02.37443700.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3512S02.37443700.js) | `9abf8082227ec292547fb43ecf5eff17ae63e7f7bc3219fdc30fe7bb84ee4ee2` |
| [IHKE3513S01.d8bc1c84.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3513S01.d8bc1c84.js) | `8a009b95ed32d9d774eab55cb9b85dcabf72872bc71e1ebfe648d01b65c2c346` |
| [IHKE3513S03.9998993b.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3513S03.9998993b.js) | `fc3e8c461efeb61525aea46921dd31aa34b6b06e65f712d4066b4217e8d4b87f` |
| [IHKE3514S01.30153756.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3514S01.30153756.js) | `54e500b8e7ceaf968db46f00e3f9f32e99fb90f130647999dc38d70710abb6cd` |
| [IHKE3516S01.c7c30eaf.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3516S01.c7c30eaf.js) | `3db512f861a860b5fe3ba4f0c6b2a1e50e49a136edb29ed5a5717d86215aaf1d` |
| [IHKE3516S02.cb47bb20.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3516S02.cb47bb20.js) | `d36afba4af5c9d5e9646d8666bc2bfbca27e97dba151c69921873b044d6216f8` |
| [IHKE3516S03.758f87ac.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3516S03.758f87ac.js) | `935a1e64e9ef4c7354bb9087f0c44ea887deb9473479ed0dfcb3118e752c6bce` |
| [IHKE3517S01.794f02ff.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3517S01.794f02ff.js) | `bfd9ec45cba899fb1069085f891846f7b2727187a53ef5cb4573522e0277e7b7` |
| [IHKE3517S02.362e4ca8.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3517S02.362e4ca8.js) | `91aa6f3d91199255aee49854a8c49b7769918b3a451824b06c9a6e5e1e6d08e9` |
| [IHKE3517S03.da1169c3.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3517S03.da1169c3.js) | `49672dc9293088ca6dcc0b9b6521307247e03ba279c373f2648608e5ebebd4fb` |
| [IHKE3712S01.2e601c04.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3712S01.2e601c04.js) | `a5bbe1cd15a204129fe5df627309c3fe9097ecc73c57944064628ab52eb10584` |
| [IHKE3720S01.68eb7b6a.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3720S01.68eb7b6a.js) | `f8c8e956bd0aae88e2a50d42fb94448fccd1ab10db9879c4e4727730cbe71a42` |
| [IHKE3721S01.7d20d235.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3721S01.7d20d235.js) | `6cdccca01c038eb59257cd237c2de1f2b67e135093e3af9ff6bf01986aa8a0ce` |
| [IHKE3721S03.90898f2c.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3721S03.90898f2c.js) | `ea3343bf60923a83987f302f6e11031f3857687e203fe9ff024e48d9d74b44c2` |
