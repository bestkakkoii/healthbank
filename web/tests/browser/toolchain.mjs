// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { createHash } from 'node:crypto';
import { access, mkdir, readFile, rename, rm, writeFile } from 'node:fs/promises';
import { createRequire } from 'node:module';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

export const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../../..');
export const toolDirectory = path.join(root, '.tools/browser-acceptance');
export const browserDirectory = path.join(toolDirectory, 'browsers');
export const browserExecutable = path.join(browserDirectory, 'chrome-headless-shell-linux64/chrome-headless-shell');
export const fontConfig = path.join(toolDirectory, 'fonts.conf');
export const sha256 = bytes => createHash('sha256').update(bytes).digest('hex');
export const toolchain = JSON.parse(await readFile(new URL('./toolchain.json', import.meta.url), 'utf8'));
export const engineExecutables = {
  chromium: browserExecutable,
  firefox: path.join(toolDirectory, 'engines', `firefox-${toolchain.firefox.revision}`, 'firefox/firefox'),
  webkit: path.join(toolDirectory, 'engines', `webkit-${toolchain.webkit.revision}`, 'pw_run.sh'),
};
const require = createRequire(import.meta.url);

export async function verifyPackages() {
  for (const name of ['playwright', 'playwright-core']) {
    const installed = JSON.parse(await readFile(require.resolve(`${name}/package.json`), 'utf8'));
    assert.equal(installed.version, toolchain.playwright.version, `Unexpected ${name} version; run npm --prefix web ci.`);
  }
  const core = path.dirname(require.resolve('playwright-core/package.json'));
  const manifestBytes = await readFile(path.join(core, 'browsers.json'));
  assert.equal(sha256(manifestBytes), toolchain.playwright.browser_manifest_sha256, 'Installed browser manifest must match the verified upstream release.');
  const manifest = JSON.parse(manifestBytes);
  for (const engine of ['chromium', 'firefox', 'webkit']) {
    const browser = manifest.browsers.find(item => item.name === (engine === 'chromium' ? 'chromium-headless-shell' : engine));
    assert.equal(browser?.revision, toolchain[engine].revision, `${engine} revision must match the reviewed toolchain.`);
    assert.equal(browser.browserVersion, toolchain[engine].version, `${engine} version must match the reviewed toolchain.`);
  }
}

export async function command(file, args, options = {}) {
  await new Promise((resolve, reject) => {
    const child = spawn(file, args, { cwd: root, stdio: 'inherit', ...options });
    child.once('error', reject);
    child.once('exit', code => code === 0 ? resolve() : reject(new Error(`${path.basename(file)} exited with ${code}.`)));
  });
}

async function downloadVerified(url, destination, expected, maxSize = 20000000) {
  try { if (sha256(await readFile(destination)) === expected) return; }
  catch (failure) { if (failure.code !== 'ENOENT') throw failure; }
  const temporary = `${destination}.download`;
  try {
    await command('curl', ['--fail', '--silent', '--show-error', '--location', '--retry', '2',
      '--connect-timeout', '20', '--max-time', '240', '--max-filesize', String(maxSize), '--output', temporary, url]);
    assert.equal(sha256(await readFile(temporary)), expected, 'Pinned development asset checksum mismatch.');
    await rename(temporary, destination);
  } finally { await rm(temporary, { force: true }); }
}

export async function setupBrowser(withDependencies) {
  assert.equal(`${process.platform}-${process.arch}`, 'linux-x64', 'The pinned acceptance browser targets the Linux x64 CI runner.');
  await verifyPackages();
  await mkdir(path.join(toolDirectory, 'fonts'), { recursive: true });
  const cli = path.join(path.dirname(require.resolve('playwright/package.json')), 'cli.js');
  if (withDependencies) await command(process.execPath, [cli, 'install-deps', 'chromium', 'firefox', 'webkit']);
  const archive = path.join(toolDirectory, 'chromium-headless-shell.zip');
  await downloadVerified(toolchain.chromium.linux_x64_url, archive, toolchain.chromium.linux_x64_sha256, 200000000);
  await rm(browserDirectory, { recursive: true, force: true });
  await mkdir(browserDirectory, { recursive: true });
  await command('unzip', ['-q', archive, '-d', browserDirectory]);
  for (const engine of ['firefox', 'webkit']) {
    const pin = toolchain[engine], archive = path.join(toolDirectory, `${engine}-${pin.revision}-ubuntu24.zip`);
    await downloadVerified(pin.linux_x64_url, archive, pin.linux_x64_sha256, 200000000);
    const directory = path.join(toolDirectory, 'engines', `${engine}-${pin.revision}`);
    await rm(directory, { recursive: true, force: true });
    await mkdir(directory, { recursive: true });
    await command('unzip', ['-q', archive, '-d', directory]);
  }
  await Promise.all([
    downloadVerified(toolchain.font.url, path.join(toolDirectory, 'fonts', toolchain.font.file), toolchain.font.sha256),
    downloadVerified(toolchain.font.license_url, path.join(toolDirectory, 'fonts/LICENSE'), toolchain.font.license_sha256),
  ]);
  const escaped = value => value.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;');
  // Use only the reviewed font directory. A preinstalled same-family CJK font
  // on a runner must not silently replace the pinned glyphs/layout metrics.
  await writeFile(fontConfig, `<?xml version="1.0"?>\n<!DOCTYPE fontconfig SYSTEM "urn:fontconfig:fonts.dtd">\n<fontconfig>\n<dir>${escaped(path.join(toolDirectory, 'fonts'))}</dir>\n<cachedir>${escaped(path.join(toolDirectory, 'font-cache'))}</cachedir>\n</fontconfig>\n`);
  console.log(`Verified Playwright ${toolchain.playwright.version}; Linux Chromium ${toolchain.chromium.version}, Firefox ${toolchain.firefox.version}, WebKit ${toolchain.webkit.version}; pinned Traditional Chinese font.`);
}

export async function verifyBrowserSetup() {
  await verifyPackages();
  assert.equal(sha256(await readFile(path.join(toolDirectory, 'fonts', toolchain.font.file))), toolchain.font.sha256,
    'Pinned Traditional Chinese font is missing or changed; run browser acceptance --setup.');
  await readFile(fontConfig);
  for (const executable of Object.values(engineExecutables)) await access(executable);
}
