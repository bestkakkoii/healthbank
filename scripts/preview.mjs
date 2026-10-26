// Copyright (c) 2026 bestkakkoi llc. SPDX-License-Identifier: MIT
import { spawn } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const portIndex = process.argv.indexOf('--port');
const previewPort = process.env.PORT || (portIndex >= 0 ? process.argv[portIndex + 1] : '3000');
const child = spawn(process.env.HEALTHBANK_PREVIEW_BINARY || path.join(root, '.tools/preview/healthbank'), [], {
  cwd: root, stdio: 'inherit',
  env: { ...process.env, HEALTHBANK_ADDR: '0.0.0.0:' + previewPort, HEALTHBANK_DATA_DIR: path.join(root, '.tools/preview/data') },
});
for (const signal of ['SIGINT', 'SIGTERM']) process.on(signal, () => child.kill(signal));
child.on('error', error => { console.error(error.message); process.exitCode = 1; });
child.on('exit', code => { process.exitCode = code ?? 0; });
