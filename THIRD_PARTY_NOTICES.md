# Third-party notices

Copyright (c) 2026 bestkakkoii llc. for the Healthbank application. Third-party works retain their own copyrights and licenses.

## Runtime components

Go standard library/runtime license and patents are under `licenses/go/`. React, React DOM and Scheduler use the React MIT license under `licenses/react/` and within the vendored npm archives. Their upstream source is https://github.com/react/react/tree/fd51cb10dd154aee73e4e7948005c02968651c3d.

The Go runtime dependencies below are unmodified. Complete license texts are bundled at the listed locations. Source is available at each named module repository and the exact versions recorded in `go.mod`, `go.sum` and `dependencies.lock.json`.

| Module | Version | License files |
| --- | --- | --- |
| filippo.io/edwards25519 | v1.2.0 | `licenses/filippo.io__edwards25519/LICENSE` |
| github.com/dustin/go-humanize | v1.1.0 | `licenses/github.com__dustin__go-humanize/LICENSE` |
| github.com/go-sql-driver/mysql | v1.10.2-0.20261016105952-4b12a5b6e7fb | `licenses/github.com__go-sql-driver__mysql/AUTHORS`, `licenses/github.com__go-sql-driver__mysql/LICENSE` |
| github.com/google/uuid | v1.6.0 | `licenses/github.com__google__uuid/LICENSE` |
| github.com/remyoudompheng/bigfft | v0.0.0-20230129092748-24d4a6f8daec | `licenses/github.com__remyoudompheng__bigfft/LICENSE` |
| golang.org/x/net | v0.61.1-0.20261022225259-05c76ee9adc5 | `licenses/golang.org__x__net/LICENSE` |
| golang.org/x/sys | v0.49.0 | `licenses/golang.org__x__sys/LICENSE` |
| modernc.org/libc | v1.79.0 | `licenses/modernc.org__libc/AUTHORS`, `licenses/modernc.org__libc/LICENSE-3RD-PARTY.md`, `licenses/modernc.org__libc/LICENSE` |
| modernc.org/mathutil | v1.7.1 | `licenses/modernc.org__mathutil/AUTHORS`, `licenses/modernc.org__mathutil/LICENSE` |
| modernc.org/memory | v1.12.1 | `licenses/modernc.org__memory/AUTHORS`, `licenses/modernc.org__memory/LICENSE-GO`, `licenses/modernc.org__memory/LICENSE-MMAP-GO`, `licenses/modernc.org__memory/LICENSE`, `licenses/modernc.org__memory/LICENSE-LOGO` |
| modernc.org/sqlite | v1.61.1-0.20261026192429-e6990422658b | `licenses/modernc.org__sqlite/AUTHORS`, `licenses/modernc.org__sqlite/LICENSE-3RD-PARTY.md`, `licenses/modernc.org__sqlite/LICENSE-SQLITE_VEC`, `licenses/modernc.org__sqlite/LICENSE-SQLITE`, `licenses/modernc.org__sqlite/LICENSE` |

The MySQL driver is licensed under MPL-2.0. Its unmodified covered source is available at https://github.com/go-sql-driver/mysql/tree/4b12a5b6e7fb8951a847f1b61485a59f337ada18. The application does not alter those covered files.

## Build-time types and tools

React type declarations are vendored from DefinitelyTyped at the pinned commit; only packaging metadata is normalized. Their MIT license is in `licenses/definitelytyped/`. `csstype` is a type-only dependency with its license in `licenses/csstype/`. TypeScript (Apache-2.0) and esbuild (MIT) compilers are downloaded/built during development and are not required or distributed as application runtime executables.

Medical reference citations are attribution and provenance, not an assertion that guideline organizations endorse this software. See `docs/clinical-reference.zh-TW.md`.
