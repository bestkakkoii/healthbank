# Changelog

Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT.

## v1.0.3-rc.4 — 2026-10-27 (Asia/Taipei)

This prerelease refreshes verified upstream sources and adds strict checks, substantive regressions and real MySQL verification. Authenticated live NHI retrieval and physical iOS/BaoTa acceptance remain outstanding.

- Updated the Go compiler, MySQL adapter and all linked Go runtime modules to verified upstream HEADs. Pinned current Node/npm, ESLint source, typed-lint alpha and React Hooks canary; inventoried every installed npm dependency and explicitly documented upstream compatibility limits.
- Added strict typed ESLint with zero-warning failure gates, current Hooks rules, stronger TypeScript checks and seven focused model tests. Corrected unsafe local preference parsing, asynchronous request ownership, stale session responses, optional-value handling and settings edit entry points.
- Fixed CSV formula-prefix handling while retaining valid negative numeric values. Replaced spread-based chart extrema with an iteration that handles the supported large report sizes without stack overflow.
- Corrected visible/accessibility label mismatches, generic ARIA misuse and interactive SVG semantics. Contained absolutely positioned screen-reader text within the comparison scroller so large mobile text cannot create whole-page horizontal overflow.
- Shared complete CAPTCHA image decoding and bounded image dimensions between production and diagnostics, rejecting truncated and oversized synthetic images before issuing a usable challenge.
- Exercised SQLite-to-MySQL-to-SQLite settings transfer, identity isolation, revisions and concurrent writes against an isolated official MySQL 26.10.0 server. Both full Go and full race runs passed 261 test/subtest events with zero skips.
- Hardened source-pinned bootstrap downloads and compiler-cache provenance. Added source/artifact integrity manifests and a release gate that rejects stale, changed or extra frontend files; nine isolated gate checks passed.
- Extended CI/release checks with ESLint, frontend model tests, known-vulnerability scans and a digest-pinned MySQL service. Documented the development Go standard-library scan limitation and incomplete live/browser/device acceptance separately.
- Added a two-part user/developer quality audit and retained bestkakkoii llc. copyright and single-executable BaoTa deployment.
- A credentialless rc.4 QA diagnostic retrieved and fully decoded an official 100×36 PNG on this run; the restricted web preview still encountered DNS_FAILED. Neither result proves authenticated retrieval or user-host availability.

## v1.0.3-rc.3 — 2026-10-27 (Asia/Taipei)

- Removed frozen date columns and annual labels in compact portrait and short-landscape layouts; all columns now scroll together. Kept readable compact dates and desktop freezing.
- Removed the reserved mobile date inset and made page-arrow distance follow the actual CSS state. Corrected the accessible scroller description.
- Refreshed the Go master source pin to 6c0b2c39e3cbaaf44061959a352bd6c7214bf621 after official upstream verification; other directly pinned sources are unchanged.

## v1.0.3-rc.2 — 2026-10-27 (Asia/Taipei)

This prerelease completes built-in settings visibility and researched warning profiles. Official CAPTCHA, authenticated login and complete live retrieval remain unverified.

- Exposed all 76 unique built-in measurement configurations before import, including source categories, named reference templates, units, aliases and independent alert explanations. No placeholder results or observed-item records are created.
- Added CK-MB activity, unspecified-context blood glucose, capillary postprandial glucose, D-dimer with unresolved FEU/DDU, homocysteine, intact PTH, vitamin B12 and a separate urine-protein qualitative source channel. Added the verified `Creatinine(健檢)` alias.
- Added unit-checked independent potassium, total-calcium, sodium, chloride, hemoglobin, hematocrit, platelet and WBC reminders from the explicitly named VGH notification policy. Extended existing low-glucose alerts to the two added blood-glucose contexts. Conditional age, specialty, outpatient and specimen-dependent thresholds remain inactive.
- Kept Trace/± findings unclassified pending original-method context instead of universally labelling them abnormal. Original flags remain visible.
- Kept method-dependent reference templates as visible previews; they do not silently replace missing original references or invent universal five-level danger cutoffs.
- Added per-item and confirmed reset-all actions that remove personal thresholds while preserving appearance, reading size, favorites and original report contents. Validated saves and portable settings now accept unobserved built-ins.
- Reconciled verified legacy dynamic aliases without losing conflicting personal settings. Persisted conflict state prevents a reset canonical value from being silently replaced by an older override.
- Normalized fullwidth comparison symbols while preserving original text, exact interval endpoints and the absence of invented chart points.
- Refreshed pinned Go and SQLite upstream snapshots and React declaration provenance, keeping remaining direct sources unchanged after verification.

## v1.0.3-rc.1 — 2026-10-27 (Asia/Taipei)

This prerelease adds actionable connection diagnostics. It is not a verified fix for official CAPTCHA or login availability.

- Added bounded per-request DNS/TCP/proxy/TLS/HTTP phase diagnostics to dependency errors and sanitized logs, without changing the official origin, TLS configuration, proxy selection, headers, retry behavior or authentication payload.
- Added `healthbank diagnose-nhi`, which runs the same document-to-CAPTCHA flow without opening a database or disturbing the running application. It validates the complete raster image and prints only safe connection facts; it never accepts credentials, solves CAPTCHA, logs in, or prints cookies, image bytes or challenge identifiers.
- Displayed the observed failure phase beside existing error and request codes.
- Corrected the empty-report welcome action for an already authenticated session to open the existing update controls rather than requesting another CAPTCHA.
- Reproduced `CONNECTION_RESET` through the deployed v1.0.2 web UI. The exact v1.0.2 executable was also started and exercised as a real web application in the preview environment, where outbound DNS failed. A separate approved public request completed its configured proxy/TLS path but received HTTP 503. These distinct results do not establish the deployed host's reset stage or prove a global outage.

## v1.0.2 — 2026-10-27 (Asia/Taipei)

- Added the Healthbank heart/pulse SVG favicon, a 16/32/48px ICO fallback and a 180px Apple touch icon; all icons are embedded in the Go executable.
- Added a visible horizontal slider and left/right buttons above the comparison table, synchronized with native scrolling and resizing. Desktop controls remain available while reading long tables; narrow and short layouts retain their normal page flow.
- Fitted trend charts to the available width on phones, including enlarged reading sizes. Dates and titles wrap or use fewer labels while every plottable observation keeps its original date and value; pointer selection resolves to the nearest visible point.
- Removed artificial comparison-row height and excess padding. Values and status badges share a wrapping line where possible, complete metadata remains visible, and touch targets stay usable.
- Narrowed the fixed date column on phones and short landscape layouts independently of enlarged text. Dates stack as year/month-day while complete accessible, desktop and printed labels remain available.
- Removed artificial chart-option height at enlarged reading sizes and contained the full-record table's hidden label within its local scroller, eliminating an additional whole-page horizontal overflow.
- Corrected project-owned copyright and company attribution to bestkakkoii llc. without changing persisted configuration identifiers.
- Replaced blanket upstream HTTP 502 responses with structured dependency errors and safe DNS/TLS/timeout/connection causes. Anonymous CAPTCHA refusals are no longer reported as expired logins; numeric upstream status and recognized codes remain available for diagnosis.
- Strengthened synthetic HTTPS tests to decode the CAPTCHA image and verify the exact login payload, matching challenge ID and cookie continuity. A bounded real CAPTCHA request from the build environment still received HTTP 503; live login is not claimed as verified.
- Exposed the effective official query interval and reported total beside the received count, with bounded expected/received diagnostics for incomplete responses. Replaced the arbitrary initial four-year query with the official default interval, while keeping explicit date selection and clearly limiting completeness claims to the returned interval.
- Strengthened the full-search fixture with an initial 50-row page and a separate 52-row complete response, preserving replacement semantics and exact effective query parameters.
- Added a bounded login-document bootstrap before the first CAPTCHA in each connector session, preserving document cookies through image retrieval and login. Document requests use HTML semantics without XHR or Authorization headers; failures do not silently continue or retry. This corrects a browser-sequence difference, not a proven cause of the observed HTTP 503.
- Used REA 6.8.0 for inert analysis of six saved public JavaScript bundles and recorded its partial-coverage limits alongside the direct source review.

## v1.0.1 — 2026-10-27 (Asia/Taipei)

- Changed the comparison table to inspection-date rows and laboratory-item columns, with a fixed date column and item headers.
- Kept every same-date observation in its corresponding cell, including original flags, units, institutions and detail access; missing results remain missing.
- Changed annual mode to group actual date rows by year without aggregating observations; continuous-date mode remains available.
- Added readable print column groups with repeated dates and headings for wide comparisons.
- Fixed clipped focus highlights, fixed-height controls, narrow mobile forms and short landscape navigation; added safe-area spacing and a relative base text size.
- Added 150% and 200% reading sizes, retaining earlier choices and synchronizing the same validated setting across devices.
- Documented the existing automatic CAPTCHA-image loading and an origin-local diagnostic for image requests hidden behind a proxy HTTP 502. This release does not claim to resolve an unverified deployment-to-NHI connection failure.

## v1.0.0 — 2026-10-27 (Asia/Taipei)

- Added the self-hosted Go application with embedded React/TypeScript frontend and automatic SQLite initialization.
- Added source-derived NHI manual-CAPTCHA login, complete-array laboratory search, isolated sessions and bounded upstream errors.
- Added in-memory saved-HTML/ZIP import, annual/date comparisons, exact-time SVG trends, personal baselines, source flags, CSV and print/PDF.
- Added dynamic settings keyed to verified pseudonymous identities, cross-device optimistic updates, MySQL adapter and portable migration.
- Added a versioned 72-definition clinical catalog with exact interval rules and explicit applicability limits.
- Added light/dark/system appearance, accessible large controls and adjustable reading size.
- Added Linux amd64/arm64 release packaging, source package, checksums, pinned dependencies, third-party notices and CI/draft-release workflows.

### Validation limits

Authenticated live NHI login, a real MySQL server, the user's BaoTa/Nginx installation and physical iOS Safari were not exercised in this build environment. Local protocol fixtures, application/store/parser/grading tests, compiled browser preview and Linux builds are documented separately.

主要部署目標：Debian 13（trixie）x64，使用 linux_amd64 靜態執行包。
