# Contributing

Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT.

Read README_AI.md and the clinical/protocol documents first. Keep changes small and explain the user-visible problem, resulting behavior, and verification performed. Submit only synthetic fixtures; sanitize identifiers, institution-specific patient rows, cookies, tokens, report HTML and logs before sharing an issue.

Use the pinned tool bootstrap, npm lockfile and Go module checksums. Build the frontend before compiling Go. Run gofmt, the strict TypeScript build, relevant tests and go vet. A clinical rule change needs a primary source, exact units/boundaries, an applicability statement and meaningful boundary tests.

Do not claim production official-login, MySQL, BaoTa or iPhone validation without performing it. Include source/license notices when changing dependencies. Repository CI and draft-release workflows are provided; maintainers control publication.
