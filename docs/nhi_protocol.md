# NHI connector protocol and verification boundary

Copyright (c) 2026 bestkakkoi llc. SPDX-License-Identifier: MIT.

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

On 2026-10-26 UTC, read-only requests from the build environment to the public
login route and to a newly generated public CAPTCHA URL each returned HTTP
503. A separate web retrieval of the login route timed out. This establishes
an environment/upstream availability limitation; it does not establish that
the site is unavailable to all users.

**No real credentials were supplied or submitted. An authenticated live login,
account identity, report completeness and live logout have not been verified.**
The implementation is verified against local synthetic protocol fixtures.
The release must keep this distinction visible until a person completes a
normal authorized sign-in on the deployed application.

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

The saved login component generates `codeID` from the current millisecond
within the second followed by a random six-digit number. `r` is the current
millisecond used as a cache buster. The connector uses a cryptographic random
source for the same numeric identifier shape.

The image is provided to the user unchanged. This connector never interprets,
solves or submits a generated answer. It accepts only PNG, JPEG or GIF image
signatures and limits the response to 512 KiB. A local five-minute challenge
expiry bounds use; the official site's own expiry may be shorter.

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

The initial query follows the saved report component:

```text
s_type=
s_date=<YYYY-MM-DD or empty>
e_date=<YYYY-MM-DD or empty>
s_sort=A1
nowPage=1
pageDataCount=50
```

Empty dates request the official default interval. The returned `sdate` and
`edate` become the interval for the subsequent `/search` call. Explicit dates
are Gregorian dates; ROC year conversion is an application/import concern.
An explicit interval requires both endpoints and an end date not before the
start date.

The complete search uses `s_type`, `s_date`, `e_date`, and `s_sort` only. Source
is left empty to include A/B/C/D, and `A1` means newest inspection date first.
There is no assay/category filter in the connector.

The saved SPA first displays `sP_IHKE3409S02_Data`, then replaces it with the
array returned by `/search`. Its 50-row pagination is a client-side array
slice. Therefore, iterating `page_load` or extracting only the current saved
HTML page would be an incorrect completeness strategy for the live connector.
The connector always performs the complete search and never falls back to the
initial 50 rows when search fails.

If `counts` is present, a complete search returning fewer records is rejected
as `INCOMPLETE_RESPONSE`. The error includes `ExpectedCount` and
`ReceivedCount`, and the returned `Report` contains interval/count metadata
but no partial observations. A greater count is accepted because data may
have arrived between the two reads. Missing `counts` is represented as `-1`,
so callers can distinguish an unavailable total from zero. The complete
array must still be structurally valid. Empty `[]` is valid when not in
conflict with an expected count; `null`, HTML, unrelated objects, malformed
rows and changed schemas are not treated as empty reports.

Requests time out after 35 seconds each. JSON bodies are limited to 16 MiB,
authentication responses to 64 KiB, and complete reports to 50,000 records.
Oversized data produces an explicit request to shorten the selected interval.

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
| `assaY_UPLOAD_DATE` | `UploadDate` | Optional upload metadata; never used as inspection date. |
| `assaY_MARK` | `OfficialFlag` | Original upstream flag; not a locally inferred risk grade. |

Unknown extra fields are ignored. Empty item names are rejected to prevent
silent schema changes from producing incorrect rows. Display code must render
all strings as text, not as HTML.

## Session lifetime

The saved official master component refreshes its JWT on a ten-minute timer
for up to six refreshes. The connector has no background timer: `Fetch`
refreshes first when the last token acquisition is at least eight minutes
old, and `Refresh` is available explicitly. Application-level session expiry
must still be enforced; a refresh does not create an indefinitely persistent
login. HTTP 401/403 and the observed expired-token code `9999` clear local
authentication.

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

- CAPTCHA retrieval, image detection and cookie continuity.
- Exact five-field login payload, ID uppercase and eight-digit card prefix.
- Accepted official success codes, bearer forwarding and token refresh.
- A 52-record full result, demonstrating retrieval beyond the initial page.
- Separate inspection/visit/upload dates and original qualitative value text.
- Explicit incomplete-result, expired-session, bad-schema and HTTP failures.
- No automatic replay of a failed manual CAPTCHA/login attempt.
- Logout/local disposal, safe errors and redirect-origin restrictions.

These fixtures establish implementation behavior against the observed
protocol. They do not replace a live authorized sign-in acceptance test after
deployment or prove that the official internal protocol has remained stable.
