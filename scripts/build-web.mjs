// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { readFileSync, mkdirSync, writeFileSync, rmSync, copyFileSync, readdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const web = path.join(root, 'web');
const out = path.join(root, 'internal/webui/dist');
const manifestName = 'frontend-build.json';
const digest = (value) => createHash('sha256').update(value).digest('hex');

function inputDigest() {
  const files = ['scripts/build-web.mjs', 'dependencies.lock.json', 'web/index.html',
    'web/package.json', 'web/package-lock.json', 'web/tsconfig.json'];
  function collect(directory) {
    for (const entry of readdirSync(path.join(root, directory), { withFileTypes: true })) {
      const relative = `${directory}/${entry.name}`;
      if (entry.isSymbolicLink()) throw new Error(`Frontend build input must not be a symlink: ${relative}`);
      if (entry.isDirectory()) collect(relative);
      else if (entry.isFile()) files.push(relative);
    }
  }
  for (const directory of ['web/src', 'web/public', 'web/vendor']) collect(directory);
  return digest(files.sort().map(file => `${file}\0${digest(readFileSync(path.join(root, file)))}\n`).join(''));
}

function artifactDigests() {
  return readdirSync(out, { withFileTypes: true })
    .filter(entry => entry.name !== manifestName)
    .sort((left, right) => left.name.localeCompare(right.name, 'en'))
    .map(entry => {
      if (!entry.isFile()) throw new Error(`Unexpected frontend artifact: ${entry.name}`);
      return { file: entry.name, sha256: digest(readFileSync(path.join(out, entry.name))) };
    });
}

function verifyBuild() {
  const manifest = JSON.parse(readFileSync(path.join(out, manifestName), 'utf8'));
  const pinned = JSON.parse(readFileSync(path.join(root, 'dependencies.lock.json'), 'utf8'));
  if (manifest.schema_version !== 1 || manifest.input_sha256 !== inputDigest())
    throw new Error('Compiled frontend does not match its current source/dependency inputs. Rebuild before releasing.');
  if (manifest.node_version !== `v${pinned.build_runtime.node}`)
    throw new Error('Frontend was not built with the pinned Node.js runtime. Bootstrap the tools and rebuild.');
  if (JSON.stringify(manifest.artifacts) !== JSON.stringify(artifactDigests()))
    throw new Error('Compiled frontend artifacts changed after the build. Rebuild before releasing.');
  console.log('Verified compiled frontend source and artifact hashes.');
}

function run(binary, args) {
  const result = spawnSync(binary, args, { cwd: web, stdio: 'inherit' });
  if (result.error) throw new Error('Cannot start build tool: ' + result.error.message);
  if (result.status !== 0) process.exit(result.status ?? 1);
}
if (process.argv.includes('--verify')) {
  verifyBuild();
} else {
  const inputsBeforeBuild = inputDigest();
  run(process.env.TSC || path.join(root, '.tools/bin/tsc'), ['--project', 'tsconfig.json']);
  if (!process.argv.includes('--check')) {
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
    if (inputDigest() !== inputsBeforeBuild)
      throw new Error('Frontend source/dependency inputs changed during compilation. Rebuild before releasing.');
    writeFileSync(path.join(out, manifestName), JSON.stringify({
      schema_version: 1,
      input_sha256: inputsBeforeBuild,
      node_version: process.version,
      artifacts: artifactDigests(),
    }, null, 2) + '\n');
    console.log('Built embedded production frontend.');
  }
}
