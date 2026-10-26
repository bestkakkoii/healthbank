# Healthbank maintainer entry point

Copyright (c) 2026 bestkakkoi llc. SPDX-License-Identifier: MIT.

## Product and boundaries

Healthbank is a self-hosted Traditional Chinese laboratory-results viewer for older adults with some medical literacy. The release must remain an executable-first BaoTa deployment: upload/extract, select the Go executable, bind the site, start. The primary deployment target is Debian 13 (trixie) x86_64 using the linux_amd64 release (GOAMD64=v1, CGO_ENABLED=0). React assets and the clinical catalog are embedded. SQLite is default; MySQL is selectable through the same store contract.

Read README.md, docs/clinical-reference.zh-TW.md, and docs/nhi_protocol.md before changing behavior. Communicate user-facing work in Taiwan Traditional Chinese; technical collaboration may be English.

Never persist medical observations, original identity/card numbers, passwords, official tokens/cookies, report HTML, or login histories. Only validated configuration documents and pseudonymous verified-subject keys belong in SQL. Keep the stable HMAC key outside SQL. Device-local ID/card memory is explicit and clearable. Imported files and demos never authenticate a person or attach to their persisted profile.

## Source layout and ownership

- cmd/healthbank: CLI, startup, lifecycle, configuration migration.
- internal/config: executable-relative paths, instance lock, random identity key, HMAC subject linkage.
- internal/nhi: fixed-origin official HTTPS protocol. Manual CAPTCHA only; per-person memory-only cookie jars/JWT; safe error messages and bounded responses.
- internal/lab: canonical mapping, exact rational intervals, clinical grading, HTML/ZIP importer, source catalog.
- internal/store: SQL dialects, schema migrations, optimistic revisions, portable settings snapshots.
- internal/server: typed API, transient session/report memory, same-origin/CSRF checks.
- internal/webui: embedded production assets.
- web/src: accessible React/TypeScript interface. Native SVG charts avoid an extra charting runtime.
- scripts: source-pinned tool bootstrap, strict frontend build, deterministic release archive packaging.

## Invariants

1. Link settings only after successful official authentication for that submitted identity. Never accept a browser-supplied pseudonym or treat HTML import as identity proof.
2. Reject unknown fields before persistent writes, including nested portable settings imports. SQL adapters are not schema-level content validators.
3. Refresh/rotate sessions without letting queued old-generation requests access new-generation data or reissue retired cookies. Avoid global-lock/session-lock inversion.
4. Preserve every result, including same-date duplicates. Source check date and visit date remain separate. ROC conversion must not substitute another date.
5. Match names, codes and units together. Never conflate urine/blood Albumin, PH/pH, RBC/WBC, or spot concentration with ratios.
6. Reference flags and official flags survive personal overrides. Reference conformity is not a diagnosis or validated danger score.
7. Compare exact rational decimals; do not round first. Inequalities/ranges stay intervals and never become fabricated chart points.
8. A custom green interval cannot erase independently applicable low-glucose/high-TG alert floors. Preserve guaranteed alerts and uncertain categories separately.
9. A missing eGFR unit cannot be silently confirmed by a personal threshold. Source profile demographics/methods are not inferred.
10. Metadata and never-observed missing results do not create configuration entries. Shared ACR settings do not collapse the source measurement channels.
11. No external scripts/fonts/analytics, report localStorage, service-worker report cache, uploaded script execution, or multipart disk spooling.
12. All human-facing output retains bestkakkoi llc copyright. Synthetic demonstrations must be conspicuously labelled.
13. Comparison rows are inspection dates and columns are item identities. Annual mode groups dates without aggregating values; preserve every same-date observation within its own item cell. Print column batches must use the same lossless matrix.
14. Browser font preferences and in-app reading sizes up to 200% must retain usable controls and readable content. Keep overflow local to two-dimensional tables/charts, protect focus highlights, and do not equate desktop viewport checks with real iOS Dynamic Type or safe-area verification.

## Build and verification

Use scripts/bootstrap-upstream.sh and web/package-lock.json. Direct upstream source commits and archive hashes are in dependencies.lock.json; do not silently replace them with older release packages. Indirect dependencies follow the locked upstream dependency graph.

Run the strict TypeScript production build, focused Go tests, go vet, and Linux amd64/arm64 cross-compilation. The grading/parser, settings isolation/migration, and official-protocol fixture tests protect substantive invariants. Do not add tests that merely mirror styling. Recheck visual changes in a browser and disclose any missing real-device verification.

The authenticated live NHI flow remains unverified unless a user actually completes an authorized login on a reachable deployment. An upstream HTTP503 is an availability result, not evidence of global outage or bot detection. Do not request credentials in chat or solve CAPTCHA automatically. MySQL and BaoTa runtime acceptance require their actual environments; a skipped test is not a pass.

## Release

The source archive is explicitly allowlisted by scripts/release.py. Keep raw reports, upload directories, .tools, databases, identity.key, environment files and logs outside it. Build compiled frontend before Go embedding. Use a clean commit and semantic version, preserve SOURCE_DATE_EPOCH, produce Linux amd64/arm64 tar.gz archives and SHA256SUMS, and include notices/licenses and deployment/clinical/verification documents. Local packaging does not authorize publication to GitHub or a user's server.
