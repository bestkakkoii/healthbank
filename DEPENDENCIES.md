# Dependency provenance

Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT.

## Targeted security refresh: 2026-10-28

The final RC7 live advisory scan found **GO-2026-6866** and **GO-2026-6860** in
the imported `golang.org/x/net/html` package. It reported **zero reachable affected
symbols**: Healthbank uses the streaming tokenizer, while the published affected
symbols belong to HTML rendering and tree parsing. The findings were still removed
before release by updating only `golang.org/x/net` from
`v0.61.1-0.20261022225259-05c76ee9adc5` to **v0.62.0**.

The [official Go vulnerability records](https://vuln.go.dev/ID/GO-2026-6866.json)
and [the tree-parser advisory](https://vuln.go.dev/ID/GO-2026-6860.json) both identify
v0.62.0 as the fixed release. Official `golang/net` master resolved to
[`3de643db506517b3e652a13cc0b56034cea6b8e6`](https://github.com/golang/net/commit/3de643db506517b3e652a13cc0b56034cea6b8e6),
which is also that release's commit. This selection therefore retains the project's
verified upstream-main policy. The module's requirements and bundled license text
are unchanged; before/after selected module graphs differ only in `x/net`.

With the pinned compiler, `go test ./internal/lab ./internal/nhi ./internal/server
-count=1` and `go mod verify` passed. A new `govulncheck -show=verbose ./...` scan
reported **No vulnerabilities found**, including no remaining imported-package
findings. The real development Go version was retained; this result does not fully
classify the development standard library or establish absence of unknown defects.
Full final CI and release evidence is recorded in `docs/verification.zh-TW.md`.

Actual `go version -m` metadata for TypeScript, esbuild and govulncheck also confirmed
Go commit `41cb39b794bfe81144930746ccf72440a0382adb`. Stale compiler-provenance fields
were corrected to that already-pinned revision; the tools' source revisions and
embedded module/version/checksum graphs were not changed.

This is a targeted security and provenance correction on **2026-10-28, Asia/Taipei**,
not a new complete upstream-HEAD or registry audit. Earlier audit timestamps below
remain historical. Exact checksums and scope are in the lock's `security_refresh`.

## Runtime refresh and current verification scope

The runtime snapshot prepared on 2026-10-27T08:46:19Z updates React/DOM/Scheduler to
`e9df86ca6991b779ee50bc744ad8bbdeefa9bae6` and the MySQL driver to
`c9e0863414a0e47bd708fcbc93d4adc611d15a5e`. The source-build workflow is
[40475265417](https://github.com/bestkakkoii/healthbank/actions/runs/40475265417).
Its immutable archive SHA-256 is
`5865f3446041614b10e0b66285d8c8a5a21ebaabef4626160c20169106c81ef7`.
The lock, local package integrity, bootstrap hash and Go module selections were
updated together. The temporary build isolated React's Closure dependency to a
compatible Java 21 runtime after the newer default Java rejected that build;
this is a build-only upstream compatibility boundary, not a production dependency.

The privacy/comparison batch adds no dependencies and verifies those pinned inputs
offline. Earlier full-graph/registry audit timestamps below remain historical;
this refresh is not a new all-dependency HEAD or vulnerability audit. The npm audit
in that artifact preparation belongs to that run, not to the subsequent code batch.

## Verified snapshot and scope

Official upstream source and release metadata were checked on **2026-10-27, Asia/Taipei**. The initial source snapshot completed at **09:21:04+08:00**; the complete selected Go graph audit completed at **09:26:44+08:00**. These are immutable snapshots, not moving branch dependencies. `dependencies.lock.json` records exact source commits, archive hashes, selected module versions, license paths, compiler build graphs, and the later lint-tool inventory (registry snapshot completed at **09:34:53+08:00**).

At the initial complete audit, every third-party Go module linked into the Linux application was at its verified official upstream HEAD; later runtime-only refresh scope is stated above. Newest compatible source or official release/prerelease versions are used for direct frontend/build/lint tools. This does **not** mean every transitive package in every upstream build graph has been forced to its newest major version: upstream peer/API constraints, graph-only modules, and the separate TypeScript lint API are explicitly retained and documented below.

## Application and compiler sources

| Component | Verified source/version | Result |
| --- | --- | --- |
| Go | golang/go master `41cb39b794bfe81144930746ccf72440a0382adb` | Source-built `devel go1.28`; actual application/tool compiler metadata rechecked 2026-10-28 |
| React / React DOM / Scheduler | facebook/react main `e9df86ca6991b779ee50bc744ad8bbdeefa9bae6` | Source-built packages reporting 19.3.0 / 19.3.0 / 0.28.0 |
| Production TypeScript | microsoft/TypeScript main `49e69830214c55a8cc361aa3c4e7412d0afd02fe` | Source-built native compiler reporting 7.1.0-dev |
| esbuild | evanw/esbuild main `f6058f8364fe7ab91ca57a83e02577ed74c9cae4` | Source-built compiler reporting 0.28.2; source identity remains the recorded commit |
| SQLite Go driver | cznic/sqlite master `e6990422658b8c9b79328fc5bbda69cc892d70ee` | Pure Go; actual `SELECT sqlite_version()` returned **3.54.1** |
| MySQL Go driver | go-sql-driver/mysql master `c9e0863414a0e47bd708fcbc93d4adc611d15a5e` | Updated optional runtime adapter; latest local batch skips live MySQL (no DSN); earlier integration is historical |
| HTML tokenizer | golang/net master `3de643db506517b3e652a13cc0b56034cea6b8e6` / v0.62.0 | Streaming memory-only saved-report parser; targeted security refresh 2026-10-28 |
| React declarations | DefinitelyTyped master `0d5b76188fd585fe187acb6cffbe08071f2bdd16` | Vendored declaration content unchanged from the verified upstream directories |
| ESLint / @eslint/js | eslint/eslint main `161558ae1391a8649a8d69a8b2c2e555b40121b9` | Unmodified source packages reporting 10.13.0 / 10.0.1 |
| govulncheck | golang/vuln master `e672cbe8fbc8c76e28fcb6db0efd9ce354e0dac5` | Official v1.9.0 source, compiled with the pinned development Go |
| Node.js | Official current distribution **26.12.1** | Linux x64 archive verified against nodejs.org SHA-256 manifest |
| npm | Official registry **12.2.1** | Separate pinned npm package, verified by registry integrity and SHA-256; the older npm bundled inside Node is not invoked |

The embedded SQLite source ID is `2026-10-16 14:45:20 555e07ef33f69a0957d24d8e555b702464a131d6f1607f4c3c1e4551e8e6ca76`. It is supplied by the source-pinned driver; no system SQLite shared library is used.

A development branch can contain newer source while still reporting an older package version than a published release. For example, `npm outdated` cannot compare the vendored React/ESLint commit identity with a registry version label. This is recorded as source provenance, not concealed by editing upstream package versions.

GitHub Actions checkout, setup-node and upload-artifact were also checked against their default branch HEADs and remain pinned to full commit SHAs in the workflows and lock document.

## Go runtime versus the full selected graph

The Linux application and its tests import **11 third-party modules**. All 11 are at the verified upstream HEAD, including the updated MySQL driver, `go-humanize`, `uuid`, `x/sys`, and `modernc.org/libc`. Runtime license texts were compared with the selected module directories; the MySQL AUTHORS file was refreshed. Each runtime module version, checksum, source commit and license path is in `go_runtime_modules`.

`go list -m all` also exposes upstream tests, code generators, alternative platforms and dependency metadata that the application does not import. After the current updates and `go mod tidy`, this selected graph contains **50 modules**: 11 linked modules and 39 graph-only modules. **19 graph-only entries are below their current default-branch HEAD**. Their selected versions and newer revisions are individually recorded in `go_module_inventory`.

Unused explicit upgrades disappear under `go mod tidy`; adding unnecessary requirements solely to make a latest-version report green would misrepresent the application dependency graph. One concrete incompatible case is `github.com/yuin/goldmark`: its newest default branch has changed the module path to `github.com/yuin/goldmark/v2`, while an upstream graph still requests v1. Neither module is linked into Healthbank. The failed v1-at-v2-HEAD lookup was retained in the audit record, and no module-path substitution was made.

Native TypeScript, esbuild and govulncheck also have their **own upstream Go module graphs**. The tools are rebuilt with the latest pinned Go, but those upstream source graphs are not silently patched. `compiler_tool_dependencies` records the actual modules embedded into each compiler/scanner; these modules are not linked into the Healthbank server executable.

## Frontend lint compatibility

The production compiler is native TypeScript 7.1. Its implementation does not provide the JavaScript compiler API required by current typescript-eslint. Linting therefore uses the separate official **TypeScript 6.0.3 JavaScript compiler API**, within typescript-eslint's supported `>=4.8.4 <6.1.0` range. Production compilation still uses the pinned native TypeScript 7 compiler. Unsupported-version warnings are not hidden and peer dependencies are not bypassed.

The current react-hooks canary matches the application React source commit. ESLint is packaged from the official main source. Transitive lint dependencies follow their declared compatible ranges; examples include ESLint's AJV 6 API and find-up 5 requirements, and the React Hooks compiler's Babel/Hermes requirements. Forcing an unrelated newest major can break the upstream consumer. The installed tools are typescript-eslint **8.72.2-alpha.1**, react-hooks **7.1.1-canary-fd51cb10-20261026**, and globals **17.13.0**.

`web/dependency-audit.json` inventories **145 installed instances / 141 unique package names**: three browser-runtime instances and 142 build/lint/test-only instances. It records each incoming parent/range, latest stable tag, highest published version including prereleases, newest compatible version, and explicit blockers. At the audit snapshot, 82 instances were the highest published version, 55 the newest compatible version under actual upstream constraints, and eight verified source-HEAD/canary instances. There were zero missing required dependencies, zero selected versions violating an upstream range, and zero unaddressed newer-compatible candidates. Project exact pins are displayed but excluded from compatibility filtering so they cannot conceal an available upgrade. Linked declaration source directories and the application root are not double-counted.

The final npm audit returned **zero known vulnerabilities**. This is a database result at the audit time, not a certification that unknown vulnerabilities cannot exist.

## Source packaging

React archives were generated from the recorded official source using its frozen upstream yarn.lock with Classic Yarn 1.22.22:

```bash
node scripts/rollup/build.js react/index,react/jsx,react-dom/index,react-dom/client,scheduler --type=NODE_DEV,NODE_PROD
```

The resulting `build/node_modules/{react,react-dom,scheduler}` directories were packaged with standard `package/` prefixes, sorted files, zero timestamps and normalized ownership/permissions. No React source was changed. Their hashes are checked before installation, and `npm ci` additionally verifies package-lock integrity. The existing official React build graph is retained; replacing its package manager and all transitive build packages is not part of consuming the unchanged React snapshot.

ESLint and @eslint/js archives were generated with `npm pack --ignore-scripts` from the official ESLint repository root and `packages/js`. No source or peer dependency declarations were modified. The source archive and vendored package hashes are recorded in `dependencies.lock.json`.

Type declaration package.json files drop repository-only devDependencies/scripts and replace workspace dependency specifications with consumable package constraints; declaration files are unchanged.

## Reproducible bootstrap

`scripts/bootstrap-upstream.sh` installs the pinned Node/npm tools and source-built Go/compiler tools into `.tools`. Downloads use immutable version/commit names, temporary files, SHA-256 verification and atomic activation. An interrupted download cannot become a trusted final cache file. A hash mismatch fails with the offending path and retains the evidence.

Compiler binary paths include both the source revision and Go revision. The bootstrap checks the actual embedded Go build metadata before accepting a compiler cache. Older directory-style `.tools/go` installations are preserved with an actionable move-aside error rather than overwritten. A normal CI checkout gets clean immutable tool paths.

The Go bootstrap compiler 1.27.3 is verified against its official go.dev archive hash. It only builds the pinned development compiler; it does not compile the application release. After bootstrap, commands must put `.tools/bin` and `.tools/go/bin` first in PATH. End users need none of these tools on the BaoTa host.

## Historical verification boundaries

At the original 2026-10-27 audit, the application passed its full Go suite and race-enabled suite with the disposable MySQL integration enabled: **261 passing test/subtest events per run, zero failed and zero skipped events**. Go vet and module checksum verification also passed. These are historical application checks, not the RC7 final integration run or upstream certification of all development snapshots.

The official govulncheck source scan returned zero findings using a vulnerability database last modified **2026-10-21T00:27:29Z**. It recorded the real development Go compiler version. However, govulncheck's semantic-version conversion cannot classify that development standard-library version, so **this result does not certify the development Go standard library**. No fake GOVERSION was substituted to make the scan appear complete. A clean scan also does not establish that no undisclosed vulnerability exists.

Browser accessibility checks use local axe-core **4.14.0** only during verification. The disposable integration server uses official MySQL **26.10.0**; CI pins its image to an immutable manifest digest. The local test server's source-built libaio dependency is recorded separately. None of these verification tools is shipped inside, or required to run, the Healthbank release.

Full package hashes, immutable source references and explicit retained dependency constraints are available in `dependencies.lock.json`; third-party license text is included under `licenses/` and listed in `THIRD_PARTY_NOTICES.md`.
