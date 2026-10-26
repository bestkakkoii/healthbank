# Dependency provenance

Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT.

Direct dependencies were resolved against their official upstream default branches on 2026-10-26 UTC, then pinned to immutable commits. Build reproducibility uses these commits, not a moving branch. Transitive versions follow the resolved upstream dependency graph and are locked in go.sum / web/package-lock.json; they are not all claimed to be upstream HEAD.

| Component | Pinned upstream source | Build result |
| --- | --- | --- |
| Go | golang/go master `dac9fe5ea9bed1733cb27fbd91aa43457de980b4` | Source-built devel go1.28; application Linux builds use this compiler |
| React / React DOM / Scheduler | react/react main `fd51cb10dd154aee73e4e7948005c02968651c3d` | Source-built packages 19.3.0 / 19.3.0 / 0.28.0 |
| TypeScript | microsoft/TypeScript main `49e69830214c55a8cc361aa3c4e7412d0afd02fe` | Source-built native compiler reports7.1.0-dev |
| esbuild | evanw/esbuild main `f6058f8364fe7ab91ca57a83e02577ed74c9cae4` | Source-built compiler reports0.28.2; Go module pseudoversion records the actual commit |
| SQLite driver | cznic/sqlite master `683b7278591f31956cee8ed4602cd3fe275ff620` | Pure Go, no external SQLite shared library |
| MySQL driver | go-sql-driver/mysql master `4b12a5b6e7fb8951a847f1b61485a59f337ada18` | Optional runtime adapter |
| HTML tokenizer | golang/net master `05c76ee9adc59087cf3ba77ee9c8fcfab61bf61c` | Streaming memory-only saved-report parser |
| React type declarations | DefinitelyTyped master `28db822755d04098fc7cab2013b15ca9f0c58c7f` | Vendored declaration content; workspace-only packaging metadata normalized |

Full source URLs, archive SHA-256 values, package hashes and runtime module license paths are in `dependencies.lock.json`.

The Go bootstrap compiler1.27.3 was downloaded from go.dev and matched against the official SHA-256 manifest before building the pinned Go source. The build host used Node24.22.1 and npm11.9.0. These bootstrap/runtime tools are distinct from the pinned application-source dependencies.

## React source build

The vendored npm packages were generated from the pinned React archive using its frozen upstream yarn.lock with yarn1.22.22. The source command was:

```bash
node scripts/rollup/build.js react/index,react/jsx,react-dom/index,react-dom/client,scheduler --type=NODE_DEV,NODE_PROD
```

The resulting `build/node_modules/{react,react-dom,scheduler}` directories were packaged with standard `package/` prefixes, sorted files, zero timestamps and normalized ownership/permissions. No React source was changed. Package byte hashes are checked by the project bootstrap; npm ci additionally checks package-lock integrity.

React packages are vendored so application builds do not need to rebuild the entire React repository. The original source and build instructions remain available at the recorded commit. Type declaration package.json files drop repository-only devDependencies/scripts and replace workspace dependency specifications with consumable package constraints; .d.ts files are unchanged.

## Build policy

These are pinned development-branch snapshots, not a claim of stable-release certification. Update them deliberately, rerun the meaningful application gates, and update provenance/checksums together. Do not replace a source revision silently merely to make a dependency install succeed. End users need none of these compilers on the BaoTa host.

Go module and npm integrity checks are retained. Third-party license text is included under `licenses/`; see `THIRD_PARTY_NOTICES.md`.
