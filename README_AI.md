# Healthbank maintainer entry point

Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT.

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
- web/public: branded SVG/ICO favicon and Apple touch icon, copied into the embedded frontend by the build script.
- scripts: source-pinned tool bootstrap, typed ESLint runner, strict frontend build/manifest verification, deterministic release archive packaging.
- web/tests: seven synthetic data-model cases for CSV, lossless grouping, exact chart points, rule previews, and large-history chart domains.
- web/dependency-audit.json: every resolved frontend dependency instance, incoming upstream ranges, latest compatible versions, and explicit retained constraints.

## Invariants

1. Link settings only after successful official authentication for that submitted identity. Never accept a browser-supplied pseudonym or treat HTML import as identity proof.
2. Reject unknown fields before persistent writes, including nested portable settings imports. SQL adapters are not schema-level content validators.
3. Refresh/rotate sessions without letting queued old-generation requests access new-generation data or reissue retired cookies. Avoid global-lock/session-lock inversion.
4. Preserve every result, including same-date duplicates. Source check date and visit date remain separate. ROC conversion must not substitute another date.
5. Match names, codes and units together. Never conflate urine/blood Albumin, PH/pH, RBC/WBC, or spot concentration with ratios.
6. Reference flags and official flags survive personal overrides. Reference conformity is not a diagnosis or validated danger score.
7. Compare exact rational decimals; do not round first. Inequalities/ranges stay intervals and never become fabricated chart points.
8. A custom green interval cannot erase independently applicable low-glucose, high-TG or explicitly enabled named-laboratory alert floors. Preserve exact endpoints, units, guaranteed alerts and uncertain categories separately; conditional institutional rules remain inactive without their required context.
9. A missing eGFR unit cannot be silently confirmed by a personal threshold. Source profile demographics/methods are not inferred.
10. Expose all built-in measurement configurations before import, without populating observed items or creating results. Metadata remains excluded; unknown dynamic entries require an observation. Shared ACR and urine-protein settings never collapse source channels. Named reference profiles are previews unless explicitly enabled for fallback. Reset removes overrides and cannot resurrect a preserved conflicting legacy override; retain unrelated appearance/favorites.
11. No external scripts/fonts/analytics, report localStorage, service-worker report cache, uploaded script execution, or multipart disk spooling.
12. All human-facing output retains bestkakkoii llc. copyright. Synthetic demonstrations must be conspicuously labelled.
13. Comparison rows are inspection dates and columns are item identities. Annual mode groups dates without aggregating values; preserve every same-date observation within its own item cell. Print column batches must use the same lossless matrix. Compact portrait and short-landscape layouts scroll every column and annual label; desktop dates remain frozen. Scroll padding and page-arrow steps must reflect the actual CSS freezing state.
14. Browser font preferences and in-app reading sizes up to 200% must retain usable controls and readable content. Keep overflow local to two-dimensional tables/charts, protect focus highlights, and do not equate desktop viewport checks with real iOS Dynamic Type or safe-area verification.

## Build and verification

Use scripts/bootstrap-upstream.sh, dependencies.lock.json and web/package-lock.json. The current source snapshot is recorded in Asia/Taipei time; direct application sources and all linked Linux Go modules were checked against official upstream HEAD. Other tools use verified official sources/releases/prereleases. Preserve immutable commits and hashes. Do not silently substitute older release packages or claim every graph-only/transitive package is newest when upstream API/peer constraints retain another version. DEPENDENCIES.md and web/dependency-audit.json contain the exact exceptions.

Bootstrap installs Node.js 26.12.1, npm 12.2.1 and the pinned Go/compiler tools. Add .tools/bin and .tools/go/bin to PATH after it returns; unset an inherited GOROOT and use GOTOOLCHAIN=local, GOENV=off and GOWORK=off for reproducible verification. Production compilation uses native TypeScript 7.1.0-dev; typescript-eslint separately uses the supported TypeScript 6.0.3 JavaScript compiler API. Do not suppress an unsupported-version warning or bypass peer requirements to merge those two roles. Compiler caches are keyed by source and Go revisions and validated against actual embedded Go metadata.

Run npm --prefix web run lint (typed strictTypeChecked rules, current React Hooks recommendations and --max-warnings 0), the strict TypeScript production build, and npm --prefix web test. The seven model tests cover substantive data invariants rather than CSS implementation. Fix defects behind diagnostics; do not lower severities, hide warnings or introduce broad assertions solely to obtain a clean report. Review any intentional lint-policy options in context.

Run gofmt, the full Go suite, the race suite, go vet, go mod verify, and Linux amd64/arm64 cross-compilation. The grading/parser, settings isolation/migration and official-protocol fixture tests protect substantive invariants. MySQL integration is opt-in locally via HEALTHBANK_TEST_MYSQL_DSN to an isolated empty synthetic database; CI provisions a digest-pinned MySQL 26.10.0 service. rc.4 was tested against an actual disposable MySQL 26.10.0 instance, including SQLite→MySQL→SQLite, isolation and optimistic revisions. An absent DSN means a skip, not a pass. Do not use a production database for integration tests.

A production frontend build clears its output directory and writes frontend-build.json with input hashes, artifact hashes and the Node version. Run node scripts/build-web.mjs --verify before Go embedding or packaging; scripts/release.sh also requires it. A changed input, modified output or extra artifact requires a rebuild. This manifest does not independently establish TSC/esbuild executable provenance: retain the pinned-tool metadata checks, especially when using explicit TSC/ESBUILD overrides. Do not edit the manifest to conceal stale artifacts.

npm audit and govulncheck use live advisory databases and may legitimately fail a later run. The current govulncheck cannot semantically classify the development Go standard-library version; do not claim that a zero-finding result clears the development standard library or substitute a fake GOVERSION.

At 2026-10-27 09:51:27 Asia/Taipei, the local rc.4 QA executable built with Go 6c49ac completed one credentialless live diagnose-nhi attempt: PNG 100×36, 790 bytes, fully decoded, 2625 ms overall, exit 0, login_attempted=false. This is actual official CLI image-retrieval evidence, not a fixture result. The QA binary reported commit=unknown; keep the precise executable/evidence boundary in the verification record. Preserve earlier HTTP503/reset observations. Do not infer their cause was fixed, global official availability, authenticated login, report completeness, or the user's BaoTa browser image display from this one success.

Recheck visual changes in a browser and disclose missing real-device verification. The authenticated live NHI flow remains unverified unless a user actually completes an authorized login on a reachable deployment. An upstream HTTP503 is an availability result, not evidence of global outage or bot detection. Do not request credentials in chat or solve CAPTCHA automatically. The synthetic MySQL test does not certify the user's database, Debian 13/BaoTa deployment or physical iOS/Safari environment.

The `diagnose-nhi` command uses the production connector without loading configuration, opening SQL, or acquiring the serving instance lock. It must not accept an upstream override or credentials, retry authentication, print image/challenge contents, or downgrade TLS. Diagnostic phases identify observed client-side progress, not the responsible remote component; a negotiated TLS peer may be a configured proxy.

## Release

The source archive is explicitly allowlisted by scripts/release.py. Keep raw reports, upload directories, .tools, databases, identity.key, environment files and logs outside it. Build and verify the compiled frontend before Go embedding. Review and stage every new allowlisted source file; the archive selection is not limited to already tracked files. Use a clean commit and semantic version (current preview v1.0.3-rc.4), set SOURCE_DATE_EPOCH to that commit timestamp, produce Linux amd64/arm64 tar.gz archives and SHA256SUMS, and include notices/licenses and deployment/clinical/verification documents. Local packaging does not authorize publication to GitHub or a user's server.
