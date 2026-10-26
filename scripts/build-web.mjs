// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import { spawnSync } from 'node:child_process';
import { readFileSync, mkdirSync, writeFileSync, rmSync, copyFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const web = path.join(root, 'web');
function run(binary, args) {
  const result = spawnSync(binary, args, { cwd: web, stdio: 'inherit' });
  if (result.error) throw new Error('Cannot start build tool: ' + result.error.message);
  if (result.status !== 0) process.exit(result.status ?? 1);
}
run(process.env.TSC || path.join(root, '.tools/bin/tsc'), ['--project', 'tsconfig.json']);
if (!process.argv.includes('--check')) {
  const out = path.join(root, 'internal/webui/dist');
  rmSync(out, { recursive: true, force: true });
  mkdirSync(out, { recursive: true });
  run(process.env.ESBUILD || path.join(root, '.tools/bin/esbuild'), [
    'src/main.tsx', '--bundle', '--minify', '--format=esm',
    '--target=es2022,safari16.4', '--jsx=automatic',
    '--define:process.env.NODE_ENV="production"',
    '--legal-comments=linked', '--metafile=' + path.join(out, 'build-meta.json'),
    '--outfile=' + path.join(out, 'app.js'),
  ]);
  let html = readFileSync(path.join(web, 'index.html'), 'utf8');
  html = html.replace(/<script[^>]*src="\/src\/main\.tsx"[^>]*><\/script>/,
    '<link rel="stylesheet" href="/app.css"><script type="module" src="/app.js"></script>');
  if (html.includes('/src/main.tsx')) throw new Error('Source entry not replaced');
  writeFileSync(path.join(out, 'index.html'), html);
  // Include browser and Home Screen icons in the single executable deployment.
  for (const asset of ['favicon.svg', 'favicon.ico', 'apple-touch-icon.png']) {
    copyFileSync(path.join(web, 'public', asset), path.join(out, asset));
  }
  // Absolute build paths are not needed by the production browser.
  rmSync(path.join(out, 'build-meta.json'));
  console.log('Built embedded production frontend.');
}
