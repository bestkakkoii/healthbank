# NHI connector protocol and verification boundary

Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT.

This document describes the source-derived connector in `internal/nhi`. It is
an implementation of the official web application's observed requests, not a
claim that NHI provides or supports these endpoints as a public API.

## Evidence

The user supplied a browser-saved copy of the official My Health Bank site.
Only the public JavaScript bundles were read as text. No saved script was
executed, and no patient observations or saved authenticated HTML are included
in this repository or the release.

The relevant saved public bundles have these SHA-256 hashes:

| Bundle | SHA-256 |
| --- | --- |
| `IHKE3099S01.195e6838.js` | `dea1f5e323cfb1ea1d4d033f34215b0ec12e3ee3cccef487abd7cb3854092ab1` |
| `IHKE3409S01.4adbbcd2.js` | `a1ff2a8bd316ce59086df2a93c27e811f2f8df187f00726d2b95989c02619e90` |
| `app.64b5ac08.js` | `4bb9558b37a75f0e04532be86a4f39f968e77bdf96335e122c2c5df5dbf3d6b7` |
| `master.59cf95f2.js` | `cbaee1317f8d6ac1264e52d7d32881a0f0ea08c68582e4de70838df342f6332c` |
| `chunk-vendors.5ebb76d6.js` | `a508bcee3dbbaacbd8a653baf361c32ddaf3f5040eda4af8a31da7ce522d9a8b` |

On 2026-10-27 at 10:28:24 Asia/Taipei, credentialless requests re-fetched the
[current public report bundle](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3409S01.4adbbcd2.js)
and [usage-guide bundle](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3115S01.be579f6a.js),
using paths exposed by the freshly retrieved application manifest. Both
returned HTTP 200. The report bundle was byte-identical to the hash above;
the 65,548-byte guide has SHA-256
`3ec6745a8319c63b4a35e4f4eddf4768b4f30e0718dd5ce04532eb430b151222`.
These are public-source observations, not authenticated report verification.

On 2026-10-26 UTC, read-only requests from the build environment to the public
login route and to a newly generated public CAPTCHA URL each returned HTTP
503. A separate web retrieval of the login route timed out. This establishes
an environment/upstream availability limitation; it does not establish that
the site is unavailable to all users.

Those earlier build-environment checks used no real credentials. On 2026-10-28,
the user completed an authorized sign-in in the official site's browser. Read-only
inspection confirmed the official laboratory range controls, all displayed result
pages, distinct date/source labels and several personal-data pages. A later resumed
session also checked the medical overview, dental detail fields and language toggle,
the medication list's expanded date range, and a subset of medication details.
The medication default interval returned fewer rows than the explicit maximum-range
query; its list and detail responses must remain separate acquisition steps. Source
field wording and editorial English translations are separately attributed.
No credentials,
cookies, tokens or individual observations were transferred into this repository,
synthetic fixtures or CI. The official browser session later returned to the sign-in
screen during active reading, leaving additional medication details and remaining
medical/examination pages unverified in that session. No timeout cause or exact
official session lifetime was inferred from that observation.

**Official-site browser inspection is not authenticated execution of Healthbank's
Go connector.** The connector still requires its own authorized deployment acceptance
for account identity, full-source retrieval and logout. Synthetic protocol fixtures,
public-source review and live browser observations are separate evidence classes;
none certifies complete lifetime history.

### REA-assisted source review

The official npm release of [REA 6.8.0](https://github.com/morluto/rea) was run
locally with `analyze-javascript-application`, using inert text only. The six
selected public bundles were the login component, `app`, `loginmaster`,
`master`, `fmMaster~master`, and the laboratory component. REA parsed all six
without a syntax failure: 195,479 source bytes and 49,142 visited AST nodes.
The retained parent Evidence ID is
`ev_e651d3aa3ea6ad85543d679761e4f752225f9af651ad461351fb7b95af7b6292`.

This is a scoped static analysis. Six other inventoried sources were outside
the selected scope, and REA reported partial application/semantic coverage,
including unresolved dynamic relationships. Its generic request extractor
does not establish the complete Vue-resource request flow. The source-level
checks below were therefore reviewed directly against the saved bundles;
successful parsing is not successful authentication. Captured code was never
executed, and no browser profile, credentials or patient report was supplied
to REA. The tool and its large analysis outputs are development-only and are
not part of this application's executable or release.

| Question | Source finding and boundary |
| --- | --- |
| CAPTCHA transport | The login component renders an ordinary image URL. Its image request does not traverse the Vue-resource Authorization interceptor. |
| Image identifier | The component combines the current millisecond with a six-digit random number and uses that millisecond as the cache parameter. Login submits the same identifier. |
| Login submission | A direct same-origin JSON POST contains the five fields documented below. The selected source does not establish an additional CSRF-token field. |
| Misleading helper name | The component's `getCookie` helper triggers CAPTCHA audio; its name is not evidence of a required cookie-initialization request. |
| Other pre-login requests | The login shell loads visit counts and announcements. No observed client-side path waits for their response or copies it into the login payload. This does not prove that their servers have no hidden side effects. |
| Post-login shell | Menu, identity-list and home-page requests are separate from the report component's token-authenticated `page_load` and `search`. No required response value from those requests was observed in the report payload. Dependent-family switching remains an explicit user action, not part of this connector. |
| Initial document cookies | A browser loads the login document before its image. Saved JavaScript cannot establish whether that document's live HTTP response sets a required Cookie. The connector now preserves this normal request order and captures any such Cookie. |
| Live acceptance | Server-side cookie requirements, current edge behavior, a valid image, successful account login and complete live report retrieval remain runtime questions. Static analysis cannot settle them. |

## Runtime connection diagnostics (v1.0.3-rc.1)

The deployed v1.0.2 UI was exercised on 2026-10-26 UTC and reproduced
`CONNECTION_RESET` while loading the login document. The same released binary
was started as a real web application in the managed preview and exercised
through its visible login dialog; that environment returned `DNS_FAILED`.
A separate public HTTPS check with the configured egress proxy completed
CONNECT and the connected-peer TLS handshake, then received HTTP 503. A direct
public TLS diagnostic could not get past DNS. These are distinct network paths;
none establishes where the deployed host's connection was reset.

Request diagnostics use `net/http/httptrace` and retain only allowlisted
operation/phase labels, bounded elapsed time, whether a configured proxy was
selected, and observed TLS version/protocol. They do not retain hostnames,
addresses, URLs, query strings, headers, credentials, cookies, tokens,
certificate bodies or raw errors. Redirected attempts update the active phase;
concurrent callbacks are synchronized. API output and logs use independent
sanitized copies. Transport settings, routing and request counts are unchanged.

`healthbank diagnose-nhi` runs the same fresh-session document and CAPTCHA
sequence before any database/configuration/instance-lock setup. It validates
the complete bounded raster image and reports a nonzero exit status on failure.
It does not return the image or challenge ID, authenticate a person, or read
reports. A successful CAPTCHA check cannot establish successful login.

Negotiated TLS fields refer to the connection that delivered the request.
With an egress proxy they do not prove an independently authenticated origin
handshake. A `tls_handshake` phase can motivate a controlled compatibility
investigation; it does not by itself prove a specific TLS extension is at fault.
No automatic TLS downgrade, fingerprint imitation or retry was added.

## Fixed origin and API

The only production origin is `https://myhealthbank.nhi.gov.tw`. The starting
page is
[the card-number login](https://myhealthbank.nhi.gov.tw/IHKE3000/IHKE3099S01).
Production code has no configurable upstream URL, arbitrary proxy endpoint or
caller-selected host. A distinct `http.Client` and cookie jar belong to every
`Session`. Cookies received from the CAPTCHA or subsequent requests are
returned only according to standard cookie rules. They are never shared
between application users or written to disk by this package.

| Operation | Method and path | Request or response |
| --- | --- | --- |
| Login document | `GET /IHKE3000/IHKE3099S01` | One successful HTML bootstrap per fresh connector session, before CAPTCHA; the same cookie jar is retained. |
| CAPTCHA | `GET /api/ihke3000/IHKELOGIN/captcha` | Query `id`, `r`; raster image response. |
| Login | `POST /api/ihke3000/ihkelogin/Login` | JSON fields below; response includes `error_code` and `jwt_token`. |
| Initial report metadata | `GET /api/ihke3000/ihke3409s01/page_load` | Date/filter query; wrapper includes `sdate`, `edate`, `counts`, `sP_IHKE3409S02_Data`. |
| Complete report search | `GET /api/ihke3000/ihke3409s01/search` | Date/filter query; response is the complete row array for that query. |
| Refresh JWT | `GET /api/ihke3000/ihkelogin/RefreshJWT` | Bearer token; response `error_code`, `jwt_token`. |
| Logout | `GET /api/ihke3000/ihkelogin/Logout` | Bearer token. Local state is discarded even if the request fails. |

The observed Vue resource transport sends JSON with
`Content-Type: application/json;charset=utf-8` and
`X-Requested-With: XMLHttpRequest`. Its interceptor supplies
`Authorization: Bearer <jwt_token>`, or the literal `Bearer null` during login.
The connector mirrors those semantics. Standard HTTPS certificate validation
remains enabled. Redirects outside the exact HTTPS official host are rejected.

## CAPTCHA and credential transformations

A fresh connector first requests the fixed HTTPS login document with an HTML
Accept header and no XHR or Authorization header. The response is bounded and
must be HTML; HTTP/network failures retain their typed cause, while a
successful non-HTML response yields `LOGIN_PAGE_INVALID`. Only a successful
document load marks the bootstrap complete. Later manual CAPTCHA refreshes
reuse it and the same cookie jar. A failed bootstrap does not proceed to the
image and is not automatically retried. A later explicit user request can
start it again. The complete document-plus-image operation shares a 35-second
context budget.

This adds the normal browser document-to-image ordering that the previous
direct-image connector omitted. No live evidence establishes that an initial
document Cookie caused the observed HTTP 503, so this compatibility change
must not be described as a confirmed availability fix. JavaScript challenges
in the returned document are not executed or bypassed.

The saved login component generates `codeID` from the current millisecond
within the second followed by a random six-digit number. `r` is the current
millisecond used as a cache buster. The connector uses a cryptographic random
source for the same numeric identifier shape.

The image is provided to the user unchanged. This connector never interprets,
solves or submits a generated answer. Production retrieval and `diagnose-nhi`
share `nhi.InspectChallengeImage`: only PNG, JPEG or GIF up to 512 KiB with
positive dimensions no larger than 2048 pixels per side are accepted. A complete
raster decode and consistent dimensions are required before a challenge ID is
issued. A signature-only or truncated response is rejected instead of becoming
an unusable CAPTCHA. This validates image usability, not external availability
or successful authentication. A local five-minute challenge expiry bounds use;
the official site's own expiry may be shorter.

The login payload is:

```json
{
  "c_id": "<user-entered ID, uppercased>",
  "c_no": "<12-digit card number>",
  "c_pw": "<password, unchanged>",
  "c_ValidCode": "<user-entered CAPTCHA answer>",
  "c_Codeid": "<current challenge identifier>"
}
```

An eight-digit card input receives the prefix `0000`. A complete twelve-digit
input is accepted directly; spaces and hyphens are presentation separators.
Other lengths and non-ASCII digits are rejected. ID validation accepts the
shape of a Taiwan ID or residence certificate and does not claim to replace
the official identity/checksum checks.

The official component accepts `0000`, `Z003` and `Z003A` as successful login
codes. The connector accepts these only when a nonempty, header-safe token is
also returned. Other responses become a safe failure with an optional
recognized error-code shape. Raw official error messages are not relayed,
because they may contain identifiers or other sensitive data. CAPTCHA/login
attempts are not automatically retried; a submitted challenge is consumed
locally even when the upstream call fails.

The observed success handler demonstrates a token and an error code. It does
not demonstrate an independently returned, verified subject identifier.
The application may derive a stable settings key from the normalized ID that
was submitted **only after** that exact request successfully authenticates
against the official origin. Never create an authenticated settings session
from a client-submitted ID alone. The connector does not decode unverified JWT
claims or switch to a dependent family member. Each family member signs in
with their own official credentials.

## Report range and completeness

The initial metadata query follows the official report component:

```text
s_type=
s_date=
e_date=
s_sort=A1
nowPage=1
pageDataCount=50
```

Empty initialization dates request the official default interval. The official
SPA adopts `sdate` and `edate` on initial page load, but its `btnOK()` date-filter
action calls `/search` directly with the selected dates. It does not repeat
`page_load` and overwrite the selection. Versions through rc.4 incorrectly
applied that initialization behavior to every query, including explicitly
requested historical dates.

From rc.5, empty **application** dates mean `all_history`: resolve once to
`1912-01-01` through today's date in Asia/Taipei, then send both dates explicitly
to `/search`. The start is the application's conservative supported query
floor, **not** a claimed NHI retention boundary or earliest available record.
Two explicit application dates mean `custom` and reach `/search` unchanged.
Incomplete, invalid, inverted or out-of-supported-year inputs fail before an
official request. The supported application years remain 1912 through 2200.

The fresh official date inputs have no `min`/`max`; `btnOK()` checks presence
and ordering only. Unused `one_year`/`two_year`/`three_year` fields do not prove
an active range cap. `remark_SDate` is rendered as availability information
for sources **A/B only**, with no established wire-date format. It must not
truncate C/D history or be treated as an all-source boundary. The official
usage guide generally describes recent-three-year institutional laboratory
data; a broad normal filter cannot recover records the official account no
longer provides. Wider-query server acceptance and actual personal history
remain authenticated deployment verification items.

The authorized official-browser inspection on 2026-10-28 accepted the application's
1912-01-01 floor through the current Taiwan date in its visible filter and allowed
reading every displayed result page. This confirms that UI path for the reviewed
session only; it does not prove that all historical source records were returned,
or authenticate the application's separate connector session.

The complete search uses `s_type`, `s_date`, `e_date`, and `s_sort` only. Source
is left empty to include A/B/C/D, and `A1` means newest inspection date first.
There is no assay/category filter in the connector.

The saved SPA first displays `sP_IHKE3409S02_Data`, then replaces it with the
array returned by `/search`. Its 50-row pagination is a client-side array
slice. Therefore, iterating `page_load` or extracting only the current saved
HTML page would be an incorrect completeness strategy for the live connector.
The connector always performs the complete search and never falls back to the
initial 50 rows when search fails.

The full search returns an array without independent range/count metadata.
`page_load.counts` becomes the reported total only when its date interval
exactly matches the search interval. For a wider search that contains the
entire default interval, that count is only a known **minimum**: returning
fewer rows is still `INCOMPLETE_RESPONSE`, while the history total remains
unavailable. A narrower, overlapping or disjoint query cannot inherit the
default count, including an oversized default count that does not apply to
the requested interval.

The error retains `ExpectedCount` and `ReceivedCount` but no partial
observations; UI wording describes the expected count as "at least" for both
exact-period and minimum guards. More records remain valid because data may
arrive between reads. An unavailable applicable total is `-1` internally and
`null` in the application API, never fabricated from array length. Empty `[]`
is valid only when it does not violate a known minimum; `null`, HTML,
unrelated objects, malformed rows and changed schemas remain failures.

RC11 also parses the initial row array strictly and verifies its multiset against
the full search whenever the requested dates contain the complete default interval.
Every original field value and duplicate occurrence must remain represented. Equal
array lengths can otherwise hide a missing row replaced by another row. A mismatch
is `INCOMPLETE_RESPONSE`, with a source-row inconsistency message and no misleading
expected/received total pair. The application preserves the previous successful
report; it does not merge, deduplicate or invent a reconciliation of two snapshots.

Requests time out after 35 seconds each. JSON bodies are limited to 16 MiB,
authentication responses to 64 KiB, and complete reports to 50,000 records.
Oversized data produces an explicit request to shorten the selected interval.

The application exposes transient `official_query` metadata alongside the
report: `mode` (`all_history` or `custom`), `start_date`, `end_date`, and
`reported_count`. Dates are the exact submitted search interval, not an
independently confirmed server retention interval. Metadata survives the
current-report endpoint and is cleared with the session; imports, demos and
anonymous responses omit it. `complete` still means an applicable independent
count was checked, not lifetime coverage. A successful full-array response
without that count receives a precise neutral explanation in the official UI.
Failed refreshes retain the previous report atomically, except an expired
authenticated session intentionally clears it. Successful refreshes reset
display-only year/category filters so historical rows cannot remain hidden.

For `INCOMPLETE_RESPONSE` only, the API includes `error.expected_count` and
`error.received_count` when `0 <= received < expected <= 50000`. A received
count of zero is retained. Invalid pairs and unrelated errors omit both fields.

## Row mapping

Result values are retained as original strings. Numeric JSON fields are
converted to their original JSON number text, avoiding binary float rounding.
No diagnosis, unit conversion, date substitution or clinical level assignment
happens in this package.

| Official field | Connector field | Meaning |
| --- | --- | --- |
| `reaL_INSPECT_DATE` | `CheckDate` | 檢查日期; the date used for the observation. |
| `ordeR_CODE` | `OrderCode` | 醫令代碼. |
| `ordeR_NAME` | `OrderName` | 檢驗檢查醫囑名稱. |
| `assaY_ITEM_NAME` | `ItemName` | Full 檢驗檢查項目名稱, not a shortened display label. |
| `assaY_VALUE` | `Result` | Full result, including `≥`, ranges, qualitative words, `+` grades or `-`. |
| `uniT_DATA` | `Unit` | Original unit string. |
| `consulT_VALUE` | `Reference` | Original institution reference text. |
| `hosP_ABBR` | `Institution` | Institution display name. |
| `funC_DATE` | `VisitDate` | 就醫日期/入院日期; kept separate from inspection date. |
| `orI_TYPE` | `Source` | Source category A/B/C/D. |
| `orI_TYPE_NAME` | `SourceName` | Original full source label, retained separately from the source code. |
| `assaY_UPLOAD_DATE` | `UploadDate` | Optional upload metadata; never used as inspection date. |
| `assaY_MARK` | `OfficialFlag` | Original upstream flag; not a locally inferred risk grade. |

Unknown extra fields are ignored. Empty item names are rejected to prevent
silent schema changes from producing incorrect rows. Display code must render
all strings as text, not as HTML.

The application retains upload/source metadata through `lab.RawRecord`, normalized
records, CSV, print and `healthbank.lab.v2` JSON/XML. Normalized `upload_date` never
replaces examination/encounter dates; `raw_upload_date` preserves the original text.
Version 1 remains strict and does not accept these newly introduced portable fields.

## Session lifetime

The saved official master component refreshes its JWT on a ten-minute timer
for up to six refreshes. The connector has no background timer: `Fetch`
refreshes first when the last token acquisition is at least eight minutes
old, and `Refresh` is available explicitly. Application-level session expiry
must still be enforced; a refresh does not create an indefinitely persistent
login. HTTP 401/403 on authenticated requests and the observed expired-token
code `9999` clear local authentication. A 401/403 response to an anonymous
CAPTCHA or login transport request is instead `UPSTREAM_ACCESS_DENIED`;
it cannot establish that a previously authenticated session has expired.

`Logout` makes a bounded official logout request and closes local state in all
cases. `Close` is a network-free disposal path for timeouts or process shutdown.
These operations release token/cookie references; Go garbage collection does
not guarantee physical erasure of every copy in memory. Nothing in this
package writes credentials, upstream cookies, JWTs or report data to SQL,
files or logs.

## Targeted validation

`session_test.go` uses a local TLS `httptest` server, a trusted test transport
and completely synthetic data. The test-only transport refuses any unexpected
origin and rewrites official requests exclusively to that local server.
Production TLS settings and the fixed production origin are unchanged.

The tests exercise:

- CAPTCHA retrieval, complete synthetic PNG decoding and cookie continuity.
- Login-document → CAPTCHA → login ordering, shared document/image cookies,
  one successful bootstrap per session and failure without automatic replay.
- Exact five-field login payload, ID uppercase and eight-digit card prefix.
- Accepted official success codes, bearer forwarding and token refresh.
- An initial 50-record page followed by exactly 52 full-search records, with
  original order, no duplicated initial rows, exactly four search parameters
  and a server-adjusted effective date propagated to the full query.
- Separate inspection/visit/upload dates and original qualitative value text.
- Explicit incomplete-result, expired-session, bad-schema and HTTP failures.
- No automatic replay of a failed manual CAPTCHA/login attempt.
- Logout/local disposal, safe errors and redirect-origin restrictions.
- Typed network causes, safe upstream HTTP status, and error/log redaction.
- Four API metadata/logout cases and eleven count-diagnostic validation cases
  in `internal/server/report_metadata_test.go`.

## Current provider-workspace contract (RC11 candidate)

The official service supplies authenticated source data. Healthbank's own source
catalog, acquisition scheduler, timeline and portable dataset are documented in
[data-workspace.zh-TW.md](data-workspace.zh-TW.md). The fixed routes and public
bundle hashes and the complete current public-source inventory are in [provider-data.md](provider-data.md) and [provider-source-inventory.json](provider-source-inventory.json);
original report retrieval and explicit file preparation are in
[provider-documents.md](provider-documents.md).

The catalog counts source/service capabilities, not independent medical datasets.
The versioned registry and inventory record its current exact membership.
Empty application dates request the supported maximum range; sources
without a date-search contract retain the actual available source window. The
workspace retrieves selected lists first, then all reachable nested details, with
bounded sequential requests, cancellation, resume and explicit capacity failures.
Source queries, date drafts and display filters are separate. No independent
source total means completeness remains unverified.

Original file preparation requires an explicit user operation and CSRF protection.
Normal acquisition performs read-only list/detail requests. The explicit full
acquisition workflow can include one-shot preparation, bounded source refresh and
streaming ZIP preservation with per-file and whole-package hashes. Missing job
correlation remains explicit. Original documents
are streamed or handled within stated memory limits; arbitrary browser-supplied
upstream URLs and server file spooling are not accepted. Public source inspection
and synthetic fixtures do not establish live account eligibility or availability.

## Medical categories and detail references (rc.7 source baseline)

On 2026-10-28 (Asia/Taipei), credentialless retrieval of the current public
application manifest and fourteen medical list/detail bundles confirmed the
source contracts below. Only inert public JavaScript was inspected; no login,
patient data or hidden authenticated route was accessed. Each bundle is available
under `https://myhealthbank.nhi.gov.tw/IHKE3000/js/` with the filename recorded below.

| Application category | Source module / detail route | Date and field boundary |
| --- | --- | --- |
| Medication | IHKE3306S01 / IHKE3306S02 | `s_type=A` means all types. Preserve original quantities, days and `act` pharmacology classification. |
| Chronic prescriptions | IHKE3307S01 / IHKE3306S02 | The observed list has no date-search endpoint; it cannot establish all-history coverage. |
| Western encounters | IHKE3303S01 / ihke3303s02 | Keep medicines, non-medication orders and examination orders as distinct source sections. |
| Chinese encounters | IHKE3304S01 / ihke3303s02 | Uses the observed shared detail route; do not infer herbal ingredients or dose equivalence. |
| Dental encounters | IHKE3305S01 / ihke3305s02 | Keep medicine orders and non-medication/tooth-position fields distinct. |
| Procedures | IHKE3308S01 / IHKE3308S02 | Use the original procedure execution date; no fallback to encounter date when absent. |
| Hospitalization | IHKE3309S01 / IHKE3309S02 | `in_DATE`/`out_DATE` are admission/discharge, `appl_S_DATE`/`appl_E_DATE` are claim dates, and item execution dates remain separate. |

The medicine/chronic detail view identifies `func_DATE` as dispensing only when
`type_name_isr` is the boolean true. `cure_E_DATE` is the separately displayed visit
date, not a medication stop date. Hospital orders without known quantity/day
fields do not derive these values from their execution interval. Prescribing,
dispensing and actual administration are separate facts.

Lists preserve duplicates and replace their transient detail-reference map only
on success. Random 48-character detail tokens are bound to the current connector
session and category; raw upstream row identifiers are not accepted from the
browser. Authentication reset, requery and expiry retire references. Both header
and order entries count toward the response limit. An invalid top-level null,
wrong returned row identity, unsupported detail category or oversized result fails
explicitly rather than being reported as a complete empty history.

The rc.7 UI requested at most 24 detail records per explicit batch. The rc.8
workspace replaces that interaction with sequential full-detail acquisition,
including nested references, within the documented explicit limits. Partial
successes remain visible on transient failures; HTTP 401
clears the cached medical data and invokes the application-wide session-expiry
handler. Neither source-contract tests nor a completed batch prove account
completeness. All automated medical fixtures are wholly synthetic.

| Public bundle | SHA-256 |
| --- | --- |
| `IHKE3303S01.3d574db2.js` | `7a0eac5049c1d347f3569611a305947c872f2cca89833358295f1d476eca4073` |
| `IHKE3303S02.07d29ff4.js` | `9e6bdb486644362840664f0d35fb4b921b107ef149b84bbc8ed3c80e365e1048` |
| `IHKE3304S01.7582cdc3.js` | `642a7e523f62431744563816a1d66bc5ec87f1de1f409e01550681991f38dfc0` |
| `IHKE3304S02.4ead7523.js` | `dea2113e9b55c9456103379c31a6eeedf3e16925bd575a397f6d018596a141c8` |
| `IHKE3305S01.1508dc2e.js` | `69d136daf309f60c86ad525cbdb487b0aa4a039497e43768a6a01d9b0d911689` |
| `IHKE3305S02.ff9f2963.js` | `cba59ffa627aa81c80f27ad3ba6ead5c23453bd75fbcd6ba7892fb83edbc8136` |
| `IHKE3306S01.88c16693.js` | `ef62d6a7f5ae6a62a97f31481c65bc6e7d94c187a4e33ef55ad481b9c38739ef` |
| `IHKE3306S02.48917477.js` | `f37ebe84b5b907320af934e8fe09b1f4bb4a0446a8ed7dfa8bfc979e2fc93efe` |
| `IHKE3307S01.73c5913c.js` | `1006181e144a523e0c05b76ecadce0d81d970e0e063762117d424ec384d5d4c4` |
| `IHKE3307S02.d141e2ab.js` | `16e5f7fa031c43deec5685a5b73b56bb169a636ae6f6ed115d86684e94162b1a` |
| `IHKE3308S01.8e1f92e5.js` | `9dea9913968f58c9d4fa192cf32227561fde71bceba843de23e6673d79b5043f` |
| `IHKE3308S02.daccc485.js` | `1e093fa6d4465dd71fa34502117e0c8e8ad5695bfa0822d0774eb80f38f1861e` |
| `IHKE3309S01.f158f613.js` | `e5e9a713ffd077c592075bb77032f43fa7fb03a68723b1dd9f4c58343cfeaabd` |
| `IHKE3309S02.deb370e8.js` | `f09984c1c3f46a0dc3eecf58163f95718295abd0af7d6fea74665b3df51047d0` |

## Application dependency errors (v1.0.2)

The application maps official dependency failures to HTTP 424 with safe
`error.code`, `error.message`, `error.request_id`, optional numeric
`error.upstream_status` and a recognized `error.upstream_code`. Actual input
errors remain HTTP 400, authentication errors remain HTTP 401, and internal
initialization/request-construction failures remain HTTP 500. This replaces
the previous blanket application HTTP 502; proxy custom-error rules can still
replace responses and must be verified on the deployment.

Network causes are classified from Go error types without copying raw error
text, URLs, proxy addresses, cookies, identifiers or official response bodies.
No automatic CAPTCHA/login retry, alternate official origin, disabled TLS
validation or transport fingerprint change was added.

On 2026-10-27 (Asia/Taipei), the earlier direct-CAPTCHA production call received
HTTP 503 after 4.073 seconds. After adding document initialization, a new bounded
production call at 03:28:39 received HTTP 503 after 4.248 seconds while loading
the login document; it did not reach the CAPTCHA request. Its safe message
identified that stage. No image was obtained and no live login was attempted.
The exact login submission remains verified against the local synthetic HTTPS
fixture, not an authenticated live account. These responses do not establish
which upstream component generated the HTTP 503 or its root cause.

These fixtures establish implementation behavior against the observed
protocol. They do not replace a live authorized sign-in acceptance test after
deployment or prove that the official internal protocol has remained stable.

## Direct-device access and official integration options

Reviewed 2026-10-27 (Asia/Taipei). These are architecture findings, not a claim
that NHI approves this application's use of its internal web endpoints.

The user's own browser can open the official site and sign in normally.
However, a page on the self-hosted application origin cannot thereby read the
official tab, its authenticated DOM or its storage. Browser `fetch` calls need
the official origin's applicable CORS permission; credentialed requests have
additional origin/credential and cookie requirements. Adding CORS headers on
the self-hosted server does not grant permission to read official responses.
No usable NHI CORS response was verified in this environment. See
[MDN same-origin policy](https://developer.mozilla.org/en-US/docs/Web/Security/Defenses/Same-origin_policy)
and [credentialed CORS](https://developer.mozilla.org/en-US/docs/Web/HTTP/Guides/CORS).

| Approach | Practical scope for this project |
| --- | --- |
| Existing Go connector | Keeps the no-install family browser workflow. The deployment's actual upstream failure must first be established using the safe error code and status. |
| Official site → saved file → import | Official login originates from the user's device. The existing importer accepts the validated saved HTML/ZIP structure and parses it in Go memory; this is not browser-only processing. |
| Browser-only direct requests | Requires explicit official cross-origin cooperation and compatible session handling; not established by the current evidence. |
| Browser extension | A separate, explicitly installed component can operate with narrowly granted site permission. Both complete-history retrieval and each browser's behavior need validation. |
| Official NHI SDK app | A reviewed native Android/iOS integration with API keys and a different installation/distribution model. It is not a drop-in JavaScript or Go public API. |

[Chrome extension networking](https://developer.chrome.com/docs/extensions/develop/concepts/network-requests)
distinguishes extension-origin access with host permissions from ordinary page
and content-script requests. [Apple Safari extensions](https://developer.apple.com/safari/extensions/)
support iPhone, iPad and Mac, but require installing/enabling the extension;
this changes the original zero-install website requirement.

The [NHI SDK application instructions](https://www.nhi.gov.tw/ch/dl-26951-93ca1150ceb34ba4beaf29b9f9223d70-1.pdf)
describe test identities followed by reviewed production access. The
[management rules revised 2026-10-12](https://www.nhi.gov.tw/ch/dl-26950-70acff31efa540ee939ca186e139b2a4-1.pdf)
set organizational eligibility and review requirements. No application,
contract or new permission was submitted as part of this release.

NHI's [public feature timeline](https://www.nhi.gov.tw/ch/lp-4142-1.html)
lists HTML, PDF, XML and JSON downloads. This does not establish that every
official export matches the supplied saved-page structure: bulk HTML requires
its own format validation. Arbitrary official NHI JSON/XML schemas are not
supported. Healthbank's own versioned JSON/XML portable laboratory-report format
is implemented separately.

Copying a visitor's User-Agent would change one header; the Go connection
would still originate from the server, use its transport and maintain its own
isolated cookie jar. It would not become the visitor's existing Safari session.
The observed HTTP 503 is insufficient to attribute a failure to fingerprinting
or a ban, so no fingerprint cloning, proxy rotation or access-control bypass
was introduced.

If official login moves entirely to the device in a future version, the
current server-verified identity linkage for cross-device settings must also
be redesigned. Importing a file or supplying an identity string does not prove
ownership of an existing saved configuration. Browser-only parsing and a
separate settings-sync identity would be distinct implementation work.
