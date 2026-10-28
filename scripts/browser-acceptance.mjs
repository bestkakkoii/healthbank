// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import path from 'node:path';
import { parseArgs } from 'node:util';
import { browserDirectory, setupBrowser } from '../web/tests/browser/toolchain.mjs';

const { values, positionals } = parseArgs({ options: {
  help: { type: 'boolean', short: 'h' }, setup: { type: 'boolean' }, 'with-deps': { type: 'boolean' },
  binary: { type: 'string' }, evidence: { type: 'string' },
}, allowPositionals: false });
if (positionals.length) throw new Error('Unexpected positional argument.');
if (values.help) {
  console.log('Synthetic compiled-Go browser acceptance\n\nSetup: node scripts/browser-acceptance.mjs --setup [--with-deps]\nRun:   node scripts/browser-acceptance.mjs --binary /absolute/healthbank --evidence /absolute/evidence\n\nThe browser is test-only. No live official login or real health data is used.\nCopyright (c) 2026 bestkakkoii llc.');
} else if (values.setup) {
  if (values.binary || values.evidence) throw new Error('Setup and acceptance must be separate commands.');
  await setupBrowser(values['with-deps'] ?? false);
} else {
  if (!values.binary || !values.evidence || values['with-deps']) throw new Error('Specify --binary and --evidence; use --help for setup.');
  process.env.PLAYWRIGHT_BROWSERS_PATH = browserDirectory;
  const { runBrowserAcceptance } = await import('../web/tests/browser/acceptance.mjs');
  await runBrowserAcceptance({ binary: path.resolve(values.binary), evidenceDir: path.resolve(values.evidence) });
}
