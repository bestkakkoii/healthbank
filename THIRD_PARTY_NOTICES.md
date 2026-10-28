# Third-party notices

Copyright (c) 2026 bestkakkoii llc. for the Healthbank application. Third-party works retain their own copyrights and licenses.

## Runtime components

Go standard library/runtime license and patents are under `licenses/go/`. React, React DOM and Scheduler use the React MIT license under `licenses/react/` and within the vendored npm archives. Their upstream source is https://github.com/facebook/react/tree/e9df86ca6991b779ee50bc744ad8bbdeefa9bae6.

The Go runtime dependencies below are unmodified. Complete license texts are bundled at the listed locations. Source is available at each named module repository and the exact versions recorded in `go.mod`, `go.sum` and `dependencies.lock.json`.

| Module | Version | License files |
| --- | --- | --- |
| filippo.io/edwards25519 | v1.2.0 | `licenses/filippo.io__edwards25519/LICENSE` |
| github.com/dustin/go-humanize | v1.1.1-0.20261013172158-f5b54ae13923 | `licenses/github.com__dustin__go-humanize/LICENSE` |
| github.com/go-sql-driver/mysql | v1.10.2-0.20261027070611-c9e0863414a0 | `licenses/github.com__go-sql-driver__mysql/AUTHORS`, `licenses/github.com__go-sql-driver__mysql/LICENSE` |
| github.com/google/uuid | v1.6.1-0.20241114170450-2d3c2a9cc518 | `licenses/github.com__google__uuid/LICENSE` |
| github.com/remyoudompheng/bigfft | v0.0.0-20230129092748-24d4a6f8daec | `licenses/github.com__remyoudompheng__bigfft/LICENSE` |
| golang.org/x/net | v0.62.0 | `licenses/golang.org__x__net/LICENSE` |
| golang.org/x/sys | v0.49.1-0.20261026092948-908ab5927565 | `licenses/golang.org__x__sys/LICENSE` |
| modernc.org/libc | v1.79.2-0.20261026191927-bc04fe32d833 | `licenses/modernc.org__libc/AUTHORS`, `licenses/modernc.org__libc/LICENSE-3RD-PARTY.md`, `licenses/modernc.org__libc/LICENSE` |
| modernc.org/mathutil | v1.7.1 | `licenses/modernc.org__mathutil/AUTHORS`, `licenses/modernc.org__mathutil/LICENSE` |
| modernc.org/memory | v1.12.1 | `licenses/modernc.org__memory/AUTHORS`, `licenses/modernc.org__memory/LICENSE-GO`, `licenses/modernc.org__memory/LICENSE-MMAP-GO`, `licenses/modernc.org__memory/LICENSE`, `licenses/modernc.org__memory/LICENSE-LOGO` |
| modernc.org/sqlite | v1.61.1-0.20261026192429-e6990422658b | `licenses/modernc.org__sqlite/AUTHORS`, `licenses/modernc.org__sqlite/LICENSE-3RD-PARTY.md`, `licenses/modernc.org__sqlite/LICENSE-SQLITE_VEC`, `licenses/modernc.org__sqlite/LICENSE-SQLITE`, `licenses/modernc.org__sqlite/LICENSE` |

The MySQL driver is licensed under MPL-2.0. Its unmodified covered source is available at https://github.com/go-sql-driver/mysql/tree/c9e0863414a0e47bd708fcbc93d4adc611d15a5e. The application does not alter those covered files.

## Build-time types and tools

React type declarations are vendored from DefinitelyTyped at the pinned commit; only packaging metadata is normalized. Their MIT license is in `licenses/definitelytyped/`. `csstype` is a type-only dependency with its license in `licenses/csstype/`. TypeScript (Apache-2.0) and esbuild (MIT) compilers are downloaded/built during development and are not required or distributed as application runtime executables.

ESLint and @eslint/js are included as unmodified source npm archives under `web/vendor/`; their MIT license is also in `licenses/eslint/LICENSE`. React Hooks, typescript-eslint, globals and the supported TypeScript JavaScript API are development-only dependencies installed from the locked npm graph. Their own package distributions preserve their upstream licenses. Node.js, npm, govulncheck and Staticcheck are verified build/analysis tools downloaded by the bootstrap; they are not application runtime requirements or bundled executables. Staticcheck's unmodified MIT-licensed source is pinned to https://github.com/dominikh/go-tools/tree/54455593555daf6bfcf3c480faaada7de3145357. See `web/dependency-audit.json` for every resolved frontend dependency and its upstream constraints.

## Browser and PDF verification tools

Playwright and playwright-core **1.64.1** are unmodified Apache-2.0 test-only npm
packages from https://github.com/microsoft/playwright/tree/cfe490d941f992655948eee8fd4fd94e1c4234bf.
Their locked package distributions include their upstream license and notices.
The matching Chromium Headless Shell is downloaded from Google's official Chrome
for Testing distribution; the archive's upstream notices remain in the isolated
test installation. No browser executable is included in a Healthbank deployment.

The synthetic browser test uses **Noto Sans CJK TC Regular**, from
https://github.com/notofonts/noto-cjk/tree/f8d157532fbfaeda587e826d4cd5b21a49186f7c,
under the **SIL Open Font License 1.1**. Setup downloads and separately verifies the
original font and its unmodified license into `.tools/browser-acceptance/fonts/`.
It does not add a font download or remote font requirement to the application.
The checksums and exact sources are in `web/tests/browser/toolchain.json`.

Poppler's `pdftotext` and `pdfinfo` are CI/system command-line tools used to verify
generated synthetic PDFs. They retain their system-package licenses and are not
linked into or bundled with the application. Browser verification files, fonts,
caches and synthetic evidence are excluded from the executable/source archives
except for the first-party test scripts and their source metadata.

Medical reference citations are attribution and provenance, not an assertion that guideline organizations endorse this software. See `docs/clinical-reference.zh-TW.md`.
