# Contributing

Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT.

Read README_AI.md and the clinical/protocol documents first. Keep changes small and explain the user-visible problem, resulting behavior, and verification performed. Submit only synthetic fixtures; sanitize identifiers, institution-specific patient rows, cookies, tokens, report HTML and logs before sharing an issue.

Use the pinned bootstrap, dependencies.lock.json, npm lockfile and Go module checksums. Bootstrap installs Node.js 26.12.1, npm 12.2.1 and the recorded source-built tools; add .tools/bin and .tools/go/bin to PATH afterwards. Update official source revisions and integrity records together. Check direct and transitive versions, but respect real upstream API/peer constraints and document retained versions rather than forcing incompatible majors. The native TypeScript 7 production compiler and the supported TypeScript 6 JavaScript lint API are separate tools.

Run the typed strict ESLint gate with npm --prefix web run lint; its runner enforces --max-warnings 0 across frontend code, model tests and JavaScript build scripts. Fix the underlying problem instead of disabling diagnostics, lowering severity or adding unsafe assertions merely to silence a warning. Run the strict native TypeScript build and all seven model tests with npm --prefix web test. Use gofmt and run the full Go suite, race checks, go vet, module verification and Linux amd64/arm64 builds for a release. A clinical rule change needs a primary source, exact units/boundaries, an applicability statement and meaningful boundary tests; do not add tests that merely mirror styling.

Use a dedicated empty synthetic database for HEALTHBANK_TEST_MYSQL_DSN. rc.4 has actual disposable MySQL 26.10.0 coverage for SQLite→MySQL→SQLite migration, isolation and revision conflicts; CI provisions a digest-pinned service. A skipped integration test is not a pass, and this coverage does not certify another server's configuration.

Build the frontend before compiling Go. The build clears old output and records source/dependency and artifact hashes. Run node scripts/build-web.mjs --verify before packaging; release.sh enforces the same check. Source changes, modified assets or extra files require a fresh build. Keep compiler provenance checks separate from this manifest, and never edit hashes to hide stale artifacts. Use a clean reviewed commit and its SOURCE_DATE_EPOCH for reproducible release timestamps.

Include source/license notices and the dependency inventory when changing dependencies. npm audit and govulncheck results describe the database and tool capabilities at the test time; the current scanner cannot fully classify the development Go standard-library version. Keep this limit explicit.

A credentialless official CLI image retrieval did succeed once on the local rc.4 QA executable at 2026-10-27 09:51:27 Asia/Taipei, with complete PNG decoding and no login attempt. Keep that positive evidence separate from authenticated login, complete report retrieval and deployment/browser acceptance; preserve the previous failed connection observations and do not infer their root cause from the successful attempt.

Do not claim authenticated official-login, the user's Debian/BaoTa deployment or physical iPhone/Safari validation without performing it. Preserve previous verification records and distinguish them from new evidence. Repository CI and draft-release workflows are provided; maintainers control publication.
