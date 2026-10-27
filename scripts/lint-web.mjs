// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const result = spawnSync(process.execPath, [
  path.join(root, 'web/node_modules/eslint/bin/eslint.js'),
  '--config', 'web/eslint.config.mjs', '--max-warnings', '0',
  'web/src', 'web/tests', 'scripts', 'web/eslint.config.mjs', ...process.argv.slice(2),
], { cwd: root, stdio: 'inherit' });
if (result.error) throw result.error;
process.exitCode = result.status ?? 1;
