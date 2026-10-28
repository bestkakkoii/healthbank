# Contributing

Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT.

Start with [README_AI.md](README_AI.md), then read the clinical and protocol documents required for the affected behavior. Keep patches focused on a concrete user problem. Explain the resulting behavior, the evidence used, and any remaining verification boundary.

## Development environment

The bootstrap targets Linux x86_64. It requires Bash, Git, curl, Python 3, and tar with xz support; Go race checks also need a working C compiler. Network access is required to obtain the pinned official sources and package dependencies. End-user Linux releases do not require these build tools.

The authoritative versions and revisions are in [dependencies.lock.json](dependencies.lock.json), the package/module lockfiles, and [DEPENDENCIES.md](DEPENDENCIES.md). Use the official upstream sources recorded there; update provenance and integrity records together. Do not allow dependency drift during one build or force incompatible upstream peer/API requirements. The native TypeScript compiler and the JavaScript API used by typed linting are intentionally separate tools.

```bash
bash scripts/bootstrap-upstream.sh
export PATH="$PWD/.tools/bin:$PWD/.tools/go/bin:$PATH"
unset GOROOT
export GOTOOLCHAIN=local GOENV=off GOWORK=off
npm --prefix web ci
```

## Quality checks

Use the project's effective strict settings. Fix the cause of diagnostics; do not lower severity, hide output, broadly suppress rules, or add unsafe assertions to obtain a passing result. Run the relevant gates while developing; before a release, complete the full verification required by README_AI.md.

```bash
npm --prefix web run lint
npm --prefix web run build
npm --prefix web test
node scripts/build-web.mjs --verify
python3 -W error scripts/test_release.py
.tools/go/bin/go test -count=1 ./...
.tools/go/bin/go test -count=1 -race ./...
.tools/go/bin/go vet ./...
.tools/go/bin/go mod verify
```

Format changed Go code with `gofmt`. The lint runner enforces zero warnings. Run existing directly affected tests and add focused tests for substantive risks; do not add tests that merely repeat styling or implementation details. A clinical change requires a primary source, exact units/endpoints, applicability rules, and meaningful boundary cases. UI checks must distinguish desktop viewport tests from actual device/browser validation.

MySQL integration requires `HEALTHBANK_TEST_MYSQL_DSN` pointing to a dedicated empty synthetic test database. Without it, the integration test is skipped, not passed. CI provides a digest-pinned disposable service. Never use a production database for tests.

Run `npm --prefix web audit --audit-level=low` and the pinned `govulncheck` when assessing dependency advisories. Record the date and applicable scanner limits; a development Go standard-library version may not be comparable with published advisory version ranges.

## Data and diagnostics

Provider changes must include the exact observed source route, fields, date
semantics and capability limits in `docs/provider-data.md` or
`docs/provider-documents.md`. Use synthetic fixtures for list/detail/session
isolation and truncation failures. The portable dataset must round-trip every
accepted source field and preserve missing details/attachments without claiming
completeness; validate identical export/import bounds before emitting a file.
Document application-facing changes in `docs/data-workspace.zh-TW.md`.

Use only synthetic fixtures. Do not commit patient observations, report HTML, identifiers, credentials, official tokens/cookies, databases, or `identity.key`. Report errors with the operation, safe code, relevant stage, and outcome. Preserve the reason and result of recovered failures without logging private request/response bodies.

Official image retrieval, synthetic protocol tests, authenticated login, and complete live historical retrieval are separate acceptance claims. Keep historical evidence in [docs/verification.zh-TW.md](docs/verification.zh-TW.md); do not reuse it as proof of a new build or erase earlier failures.

## Release preparation

Build the frontend before Go embedding. Its manifest checks source/dependency and artifact hashes; compiler provenance is checked separately by bootstrap. Changed sources, modified output, or extra generated files require a rebuild. Never edit the manifest to conceal stale assets.

Use a clean reviewed commit. Set `HEALTHBANK_RELEASE_TAG` to the intended semantic release tag, then package with that explicit version and the commit's timestamp:

```bash
: "${HEALTHBANK_RELEASE_TAG:?Set the intended semantic release tag first}"
export SOURCE_DATE_EPOCH="$(git show -s --format=%ct HEAD)"
GO="$PWD/.tools/go/bin/go" bash scripts/release.sh "$HEALTHBANK_RELEASE_TAG"
```

Review both Linux architecture archives, the source archive, `SHA256SUMS`, licenses, and provenance. Source packaging uses an explicit allowlist and must exclude private data and build caches. Check that relative README images and shipped documentation remain usable inside the binary package.

The scripts package locally. Git pushes, workflow execution, and publication are separate actions. A pushed version tag preserves an existing release or prepares a draft. An explicitly changed RELEASE_CANDIDATE on main publishes that RC only after the release workflow's full checks, MySQL/race tests, build and checksum gates pass; it refuses to replace an existing release. Preserve the user's explicit publication authorization. Review the actual [Actions results](https://github.com/bestkakkoii/healthbank/actions) and downloadable release assets before claiming completion.

## Submitting a change

Describe the problem, behavior change, and relevant verification in the pull request. Include evidence for intentional exceptions and explicitly identify necessary checks that could not be performed. Update the canonical document for the changed contract rather than duplicating specifications. Keep current product/tool versions out of evergreen prose; exact versions belong in the changelog, release evidence, and dependency provenance.

Use Taiwan Traditional Chinese for end-user copy and concise technical English for internal guidance. Preserve `bestkakkoii llc.` attribution and third-party notices. See [SECURITY.md](SECURITY.md) before sharing any sensitive issue details.
