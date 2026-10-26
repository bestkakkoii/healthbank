# Changelog

Copyright (c) 2026 bestkakkoi llc. SPDX-License-Identifier: MIT.

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
