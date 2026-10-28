// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { once } from 'node:events';
import { mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { createServer } from 'node:net';
import os from 'node:os';
import path from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import { chromium } from 'playwright';
import { replacementLaboratory, syntheticDataset } from './fixtures.mjs';
import { browserExecutable, fontConfig, root, sha256, toolchain, verifyBrowserSetup } from './toolchain.mjs';

async function unusedPort() {
  const socket = createServer();
  socket.listen(0, '127.0.0.1');
  await once(socket, 'listening');
  const port = socket.address().port;
  await new Promise((resolve, reject) => socket.close(error => error ? reject(error) : resolve()));
  return port;
}

async function stopServer(child) {
  if (!child || child.exitCode !== null || child.signalCode !== null) return;
  const exited = once(child, 'exit');
  child.kill('SIGTERM');
  const timeout = setTimeout(() => child.kill('SIGKILL'), 10000);
  try { await exited; } finally { clearTimeout(timeout); }
}

async function openDisclosure(locator) {
  if (!await locator.evaluate(element => element.open)) await locator.locator(':scope > summary').click();
}

async function saveDownload(page, button, filename, directory) {
  assert.equal(path.basename(filename), filename, 'Evidence download name must be a basename.');
  const [download] = await Promise.all([page.waitForEvent('download'), button.click()]);
  const output = path.join(directory, filename);
  await download.saveAs(output);
  assert.equal(await download.failure(), null, 'Synthetic file download failed.');
  return readFile(output);
}

async function session(context, baseURL) {
  const response = await context.request.get(`${baseURL}/api/session`);
  assert.ok(response.ok(), 'Local session API must succeed.');
  return response.json();
}

async function currentReport(context, baseURL, token) {
  const response = await context.request.get(`${baseURL}/api/reports/current`, { headers: { 'X-CSRF-Token': token } });
  assert.ok(response.ok(), 'Current report API must succeed.');
  const report = await response.json();
  // Do not place actual tokens in assertion diffs or the evidence document.
  assert.ok(report.session_token === token, 'Report generation marker must match the verified session.');
  return report;
}

async function importLaboratory(context, baseURL, document) {
  const before = await session(context, baseURL);
  const response = await context.request.post(`${baseURL}/api/reports/import`, {
    headers: { Origin: baseURL, 'X-CSRF-Token': before.csrf_token },
    multipart: { file: { name: 'synthetic-browser-report.json', mimeType: 'application/json', buffer: Buffer.from(JSON.stringify(document)) } },
  });
  assert.ok(response.ok(), 'Real Go synthetic laboratory import must succeed.');
  const report = await response.json(), after = await session(context, baseURL);
  assert.equal(after.mode, 'import');
  assert.equal(after.authenticated, false);
  assert.ok(report.session_token === after.csrf_token && before.csrf_token !== after.csrf_token,
    'Successful import must rotate its real session generation.');
  return { report, session: after };
}

export async function runBrowserAcceptance({ binary, evidenceDir }) {
  await verifyBrowserSetup();
  await mkdir(evidenceDir, { recursive: true });
  const runtime = await mkdtemp(path.join(os.tmpdir(), 'healthbank-browser-'));
  const baseURL = `http://127.0.0.1:${await unusedPort()}`;
  const childEnv = Object.fromEntries(Object.entries(process.env).filter(([name]) => !name.startsWith('HEALTHBANK_')));
  const result = {
    schema: 'healthbank.synthetic-browser-acceptance.v1', synthetic_only: true, official_login_tested: false,
    binary_sha256: sha256(await readFile(binary)), browser_version: toolchain.chromium.version,
    playwright_version: toolchain.playwright.version, chinese_font_sha256: toolchain.font.sha256,
    checks: [], layouts: [], prohibited_requests: [], page_errors: 0, passed: false,
  };
  const record = (name, detail = {}) => {
    result.checks.push({ name, ...detail, passed: true });
    console.log(`PASS browser: ${name}`);
  };
  let child, browser, page;
  let phase = 'startup';
  try {
    child = spawn(binary, ['serve'], { cwd: runtime, env: { ...childEnv, HEALTHBANK_ADDR: new URL(baseURL).host,
      HEALTHBANK_DATA_DIR: runtime, HEALTHBANK_DB_DRIVER: 'sqlite', HEALTHBANK_MYSQL_DSN: '' }, stdio: 'ignore' });
    let startError;
    child.once('error', error => { startError = error; });
    let ready = false;
    for (let attempt = 0; attempt < 100; attempt++) {
      if (startError) throw startError;
      if (child.exitCode !== null) throw new Error(`Isolated Go process exited during startup (${child.exitCode}).`);
      try { ready = (await fetch(`${baseURL}/api/health`, { signal: AbortSignal.timeout(500) })).ok; }
      catch { /* The isolated loopback listener is still starting. */ }
      if (ready) break;
      await delay(100);
    }
    assert.ok(ready, 'Compiled Go application did not become healthy.');
    browser = await chromium.launch({ headless: true, executablePath: browserExecutable, env: { ...process.env, FONTCONFIG_FILE: fontConfig } });
    assert.equal(browser.version(), toolchain.chromium.version);
    const createContext = async ({ syntheticOrigin = baseURL } = {}) => {
      const fixtureOrigin = new URL(syntheticOrigin);
      assert.ok(fixtureOrigin.protocol === 'http:' && fixtureOrigin.hostname === '127.0.0.1' && fixtureOrigin.pathname === '/',
        'A synthetic fixture may only use an isolated loopback origin.');
      const context = await browser.newContext({ viewport: { width: 1280, height: 900 }, locale: 'zh-TW', timezoneId: 'Asia/Taipei', serviceWorkers: 'block' });
      context.on('page', opened => opened.on('pageerror', () => { result.page_errors++; }));
      await context.route(/^https?:/, route => {
        const url = new URL(route.request().url());
        if (url.origin !== syntheticOrigin || ['/api/auth/captcha', '/api/auth/login', '/api/reports', '/api/supplementary', '/api/documents', '/api/documents/prepare'].includes(url.pathname)) {
          result.prohibited_requests.push(url.origin === syntheticOrigin ? url.pathname : 'external-origin');
          return route.abort();
        }
        return route.continue();
      });
      return context;
    };
    const context = await createContext();
    page = await context.newPage();
    page.setDefaultTimeout(15000);
    await page.goto(baseURL, { waitUntil: 'networkidle' });
    await page.locator('.boot-screen').waitFor({ state: 'hidden' });
    result.application_version = (await session(context, baseURL)).version;
    result.frontend_sha256 = {};
    for (const asset of ['app.js', 'app.css']) {
      const response = await context.request.get(`${baseURL}/${asset}`), bytes = await response.body();
      assert.ok(response.ok());
      assert.equal(sha256(bytes), sha256(await readFile(path.join(root, 'internal/webui/dist', asset))), `Executable embeds the current ${asset}.`);
      result.frontend_sha256[asset] = sha256(bytes);
    }
    const navigate = async (full, short) => {
      const desktop = page.getByRole('button', { name: full, exact: true });
      if (await desktop.isVisible()) await desktop.click();
      else await page.getByRole('navigation', { name: '行動版主要導覽' }).getByRole('button', { name: short, exact: true }).click();
      await page.locator('#main-content h1').filter({ hasText: full }).waitFor();
    };
    const downloadJSON = async (button, filename) => JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(await saveDownload(page, button, filename, evidenceDir)));
    const restore = async fixture => {
      await navigate('醫療紀錄', '紀錄');
      await openDisclosure(page.locator('#dataset-tools'));
      const [response] = await Promise.all([
        page.waitForResponse(response => new URL(response.url()).pathname === '/api/reports/import' && response.request().method() === 'POST'),
        page.getByLabel('選擇 Healthbank 完整資料包', { exact: true }).setInputFiles({ name: 'synthetic-dataset.json', mimeType: 'application/json', buffer: Buffer.from(JSON.stringify(fixture)) }),
      ]);
      assert.ok(response.ok(), 'Archive restoration must import its labs through real Go.');
      await page.getByText('完整資料包已還原至獨立匯入模式', { exact: false }).waitFor();
      await page.locator('.busy-strip').waitFor({ state: 'hidden' });
      await page.getByRole('status').filter({ hasText: '顯示 621／621 筆來源紀錄' }).waitFor();
      return response.json();
    };
    const setFont = async (scale) => {
      await navigate('個人設定', '設定');
      const button = page.getByRole('button', { name: scale === 2 ? /200%/ : /Aa 標準/ });
      if (await button.getAttribute('aria-pressed') === 'true') return;
      const [response] = await Promise.all([page.waitForResponse(response => new URL(response.url()).pathname === '/api/settings' && response.request().method() === 'PUT'), button.click()]);
      assert.ok(response.ok(), 'Font preference must save through real Go.');
      await page.waitForFunction(value => globalThis.document.documentElement.style.getPropertyValue('--font-scale') === String(value), scale);
    };

    phase = 'archive restoration';
    const fixture = syntheticDataset(), imported = await restore(fixture), verified = await session(context, baseURL);
    assert.equal(imported.records.length, 500);
    assert.ok(imported.session_token === verified.csrf_token, 'Import response must belong to the current session.');
    const loaded = await currentReport(context, baseURL, verified.csrf_token);
    assert.equal(loaded.records.length, 500);
    const first = loaded.records.find(row => row.source === 'SYNTHETIC_LAB_0000');
    assert.equal(first.raw_check_date, '104/01/01');
    assert.equal(first.raw_visit_date, '103/12/31');
    assert.equal(first.exam_date, '2015-01-01');
    const cookie = (await context.cookies()).find(cookie => cookie.name === 'healthbank_session');
    assert.ok(cookie?.httpOnly && cookie.sameSite === 'Strict' && cookie.expires === -1, 'Session cookie must be HttpOnly, Strict, and browser-session scoped.');
    await navigate('檢驗總覽', '總覽');
    await page.getByText('資料範圍', { exact: true }).click();
    await page.getByText('匯入資料保留 500 筆；本次未向官方重新核對資料或身分。', { exact: true }).waitFor();
    record('real archive import, raw dates, generation marker and imported coverage', { laboratory_rows: 500, clinical_rows: 121 });

    phase = '200 percent layouts';
    await setFont(2);
    await page.locator('.toast').waitFor({ state: 'hidden' });
    const sizes = [{ width: 320, height: 900 }, { width: 390, height: 900 }, { width: 844, height: 390 }, { width: 1280, height: 900 }];
    for (const size of sizes) {
      await page.setViewportSize(size);
      for (const [view, short, key] of [['檢驗總覽', '總覽', 'overview'], ['醫療紀錄', '紀錄', 'records'], ['年度比較', '比較', 'comparison'], ['數值趨勢', '趨勢', 'trends'], ['個人設定', '設定', 'settings']]) {
        await navigate(view, short);
        await page.evaluate(() => globalThis.document.fonts.ready);
        const measurement = await page.evaluate(({ size, key }) => ({ view: key, ...size,
          font_scale: globalThis.document.documentElement.style.getPropertyValue('--font-scale'),
          document_width: globalThis.document.documentElement.scrollWidth,
          heading_width: globalThis.document.querySelector('#main-content h1').getBoundingClientRect().width,
        }), { size, key });
        assert.ok(measurement.document_width <= size.width + 1, `200% ${key} at ${size.width}×${size.height} must not overflow the page.`);
        assert.equal(measurement.font_scale, '2');
        result.layouts.push(measurement);
        const workSurface = { overview: '.summary-grid', records: '.health-data-search', comparison: '.comparison-screen', trends: '.chart-viewport', settings: '.font-choices' }[key];
        await page.locator(workSurface).first().scrollIntoViewIfNeeded();
        await page.screenshot({ path: path.join(evidenceDir, `${key}-200-${size.width}x${size.height}.png`) });
      }
    }
    await navigate('醫療紀錄', '紀錄');
    const cdp = await context.newCDPSession(page);
    await cdp.send('DOM.enable'); await cdp.send('CSS.enable');
    const document = await cdp.send('DOM.getDocument'), heading = await cdp.send('DOM.querySelector', { nodeId: document.root.nodeId, selector: '#main-content h1' });
    const fonts = await cdp.send('CSS.getPlatformFontsForNode', { nodeId: heading.nodeId });
    assert.ok(fonts.fonts.some(font => font.familyName === toolchain.font.family && font.glyphCount > 0), 'Actual Chinese heading glyphs must use the pinned CJK font.');
    await cdp.detach();
    await restore(fixture); // A real restored-archive notice, not an inserted DOM toast.
    for (const size of sizes) {
      await page.setViewportSize(size);
      const bounds = await page.locator('.toast').boundingBox();
      assert.ok(bounds && bounds.x >= -1 && bounds.x + bounds.width <= size.width + 1 && bounds.y >= -1 && bounds.y + bounds.height <= size.height + 1,
        `200% toast must fit the ${size.width}×${size.height} viewport.`);
      await page.screenshot({ path: path.join(evidenceDir, `notice-200-${size.width}x${size.height}.png`) });
    }
    const hierarchy = await page.evaluate(() => {
      const top = selector => globalThis.document.querySelector(selector).getBoundingClientRect().top + globalThis.scrollY;
      return { search: top('.health-data-search'), first_record: top('.health-data-event'), archive: top('#dataset-tools') };
    });
    assert.ok(hierarchy.search < hierarchy.first_record && hierarchy.first_record < hierarchy.archive, 'Reading/search workflow precedes archive management.');
    assert.equal(await page.evaluate(() => globalThis.syntheticInjection), undefined, 'Source markup must remain inert.');
    record('200% layouts, real notice bounds, task order and actual CJK glyphs', { viewport_checks: result.layouts.length, hierarchy });
    await setFont(1); await page.setViewportSize({ width: 1280, height: 900 });

    phase = 'detail navigation';
    await navigate('醫療紀錄', '紀錄');
    await openDisclosure(page.locator('.health-data-display-options'));
    await page.getByRole('combobox', { name: '顯示類別', exact: true }).selectOption('medicine');
    await page.getByRole('searchbox', { name: '搜尋紀錄', exact: true }).fill('合成回診');
    const upper = page.getByRole('navigation', { name: '醫療紀錄分頁（上方）' });
    await upper.getByRole('button', { name: '下一頁', exact: true }).click();
    assert.match(await upper.innerText(), /第 2／3 頁/);
    assert.match(await page.getByRole('navigation', { name: '醫療紀錄分頁（下方）' }).innerText(), /第 2／3 頁/);
    const parent = page.locator('.health-data-event').filter({ has: page.getByText('合成回診 025', { exact: true }) });
    await openDisclosure(parent);
    const detailButton = parent.getByRole('button', { name: '查看已取得明細', exact: true });
    await detailButton.click();
    await page.getByRole('heading', { name: '所選紀錄的明細', exact: true }).waitFor();
    assert.ok(await page.locator('.health-data-events > h3').evaluate(element => element === globalThis.document.activeElement), 'Opening detail focuses its heading.');
    await page.getByRole('button', { name: '返回上一層紀錄', exact: true }).click();
    assert.equal(await page.getByRole('searchbox', { name: '搜尋紀錄', exact: true }).inputValue(), '合成回診');
    assert.equal(await page.getByRole('combobox', { name: '顯示類別', exact: true }).inputValue(), 'medicine');
    assert.match(await upper.innerText(), /第 2／3 頁/);
    assert.ok(await detailButton.evaluate(element => element === globalThis.document.activeElement), 'Returning restores focus to the originating detail button.');
    const backBounds = await detailButton.boundingBox();
    assert.ok(backBounds.y >= 0 && backBounds.y + backBounds.height <= 900, 'Restored detail trigger must be visible.');
    await parent.getByRole('button', { name: '在檢驗趨勢標示日期', exact: true }).click();
    await parent.getByRole('status').filter({ hasText: '已標示' }).waitFor();
    assert.match(await parent.locator('.health-data-date-feedback').innerText(), /目前未取得當日檢驗結果/);
    await parent.getByRole('button', { name: '前往檢驗趨勢', exact: true }).click();
    await page.locator('#main-content h1').filter({ hasText: '數值趨勢' }).waitFor();
    record('detail return preserves filter, page, visible focus; date marker gives feedback and navigation');

    phase = 'large chart keyboard navigation';
    await page.locator('.chart-point-target').first().waitFor();
    assert.equal(await page.locator('.chart-point-target').count(), 500);
    assert.equal(await page.locator('.chart-point-target[tabindex="0"]').count(), 1);
    assert.equal(await page.locator('.chart-records tbody tr').count(), 50);
    const points = page.locator('.chart-point-target');
    await points.first().focus(); await page.keyboard.press('ArrowRight');
    assert.ok(await points.nth(1).evaluate(element => element === globalThis.document.activeElement), 'ArrowRight reaches the second original observation, including duplicates.');
    assert.equal(await points.nth(0).getAttribute('cx'), await points.nth(1).getAttribute('cx'));
    assert.equal(await points.nth(0).getAttribute('cy'), await points.nth(1).getAttribute('cy'));
    await page.keyboard.press('Enter');
    const detail = page.getByRole('dialog');
    await detail.waitFor();
    const sources = await detail.innerText();
    const duplicateSources = loaded.records.filter(row => row.exam_date === '2015-01-01').sort((left, right) => left.id.localeCompare(right.id)).map(row => row.source);
    assert.equal(duplicateSources.length, 2);
    assert.ok(sources.includes(duplicateSources[1]) && !sources.includes(duplicateSources[0]), 'Keyboard activation opens the exact second duplicate observation.');
    await detail.getByRole('button', { name: '關閉視窗', exact: true }).click();
    await points.nth(1).focus(); await page.keyboard.press('End');
    assert.ok(await points.last().evaluate(element => element === globalThis.document.activeElement));
    await page.keyboard.press('Home');
    assert.ok(await points.first().evaluate(element => element === globalThis.document.activeElement));
    await page.keyboard.press('Tab');
    assert.ok(await page.evaluate(() => !globalThis.document.activeElement?.classList.contains('chart-point-target')), 'Tab leaves the roving chart rather than walking all 500 points.');
    await page.getByRole('button', { name: '跳到完整檢查紀錄（500 筆）', exact: true }).click();
    const recordHeading = page.locator('.chart-records > h3');
    assert.ok(await recordHeading.evaluate(element => element === globalThis.document.activeElement));
    await page.getByRole('group', { name: '檢查紀錄分頁（上方）' }).getByRole('button', { name: '下一頁檢查紀錄', exact: true }).click();
    assert.equal(await page.locator('.chart-records tbody tr').count(), 50);
    record('500 chart observations retain duplicates, one tab stop, arrow/Home/End navigation and paged records');

    phase = 'actual downloaded scope';
    await navigate('年度比較', '比較');
    await page.getByRole('combobox', { name: '比較年度', exact: true }).selectOption('2016');
    const expectedYear = fixture.laboratory.records.filter(row => row.check_date.startsWith('2016-')).length;
    await page.getByRole('button', { name: `匯出篩選的 ${expectedYear} 筆 CSV`, exact: true }).waitFor();
    const portableButton = page.getByRole('button', { name: '匯出全部 500 筆檢驗 JSON（UTF-8）', exact: true });
    const portable = await downloadJSON(portableButton, 'all-laboratory.json');
    assert.equal(portable.records.length, 500);
    assert.deepEqual(portable.records, fixture.laboratory.records);
    await openDisclosure(page.locator('.portable-export-options'));
    await page.getByRole('checkbox', { name: 'JSON 加 UTF-8 BOM（文字檢視器亂碼時使用）', exact: true }).check();
    const bom = await saveDownload(page, portableButton, 'all-laboratory-bom.json', evidenceDir);
    assert.ok(bom.subarray(0, 3).equals(Buffer.from([239, 187, 191])));
    assert.deepEqual(JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(bom)).records, portable.records);
    const xml = await saveDownload(page, page.getByRole('button', { name: '匯出全部 500 筆檢驗 XML', exact: true }), 'all-laboratory.xml', evidenceDir);
    const parsedXML = await page.evaluate(text => {
      const document = new globalThis.DOMParser().parseFromString(text, 'application/xml');
      return { errors: document.querySelectorAll('parsererror').length, records: document.querySelectorAll('record').length,
        dates: [...document.querySelectorAll('check_date')].slice(0, 2).map(element => element.textContent) };
    }, new TextDecoder('utf-8', { fatal: true }).decode(xml));
    assert.equal(parsedXML.errors, 0); assert.equal(parsedXML.records, 500);
    assert.deepEqual(parsedXML.dates, fixture.laboratory.records.slice(0, 2).map(row => row.check_date));
    const csv = await saveDownload(page, page.getByRole('button', { name: `匯出篩選的 ${expectedYear} 筆 CSV`, exact: true }), 'selected-year.csv', evidenceDir);
    assert.equal(new TextDecoder('utf-8', { fatal: true }).decode(csv).trimEnd().split(/\r?\n/).length - 1, expectedYear);
    await navigate('醫療紀錄', '紀錄'); await openDisclosure(page.locator('#dataset-tools'));
    const archived = await downloadJSON(page.getByRole('button', { name: '匯出全部已取得資料', exact: true }), 'complete-dataset.json');
    assert.equal(archived.schema, 'healthbank.dataset.v2');
    assert.deepEqual(archived.laboratory.records, fixture.laboratory.records);
    assert.equal(archived.data.reports.medicine.entries.length, 120);
    assert.equal(Object.values(archived.data.details.medicine)[0].entries.length, 1);
    assert.ok(!JSON.stringify(archived).includes((await session(context, baseURL)).csrf_token), 'Session tokens must never enter portable archives.');
    record('real downloads preserve all raw rows and dates; CSV uses the labeled selected year', { portable_rows: 500, selected_csv_rows: expectedYear, dataset_schema: archived.schema });

    phase = 'data-generated report and PDF';
    const { runPrintAcceptance } = await import('./print-acceptance.mjs');
    record('data-generated report and PDF', await runPrintAcceptance({ page, context, baseURL, evidenceDir, fixture, navigate, downloadJSON, record }));

    phase = 'cross-tab generation and stale state';
    await navigate('醫療紀錄', '紀錄');
    const old = await session(context, baseURL), replacement = await importLaboratory(context, baseURL, replacementLaboratory());
    assert.equal(replacement.report.records.length, 1);
    const staleRead = await context.request.get(`${baseURL}/api/reports/current`, { headers: { 'X-CSRF-Token': old.csrf_token } });
    assert.equal(staleRead.status(), 409); assert.equal((await staleRead.json()).error.code, 'session_changed');
    assert.ok(!staleRead.headers()['set-cookie'], 'Stale reads must not reissue a retired cookie.');
    await navigate('個人設定', '設定');
    const [staleSave] = await Promise.all([page.waitForResponse(response => new URL(response.url()).pathname === '/api/settings' && response.request().method() === 'PUT'), page.getByRole('button', { name: /200%/ }).click()]);
    assert.equal(staleSave.status(), 403);
    await page.getByText('使用階段已到期或更換，先前報告與醫療資料已清除。請確認目前使用者後重新取得資料。', { exact: true }).waitFor();
    await navigate('醫療紀錄', '紀錄');
    assert.equal(await page.locator('.health-data-event').count(), 0);
    assert.equal(await page.getByText('SYNTHETIC_REPLACEMENT_SESSION', { exact: true }).count(), 0);
    await page.reload({ waitUntil: 'networkidle' });
    await navigate('醫療紀錄', '紀錄');
    await page.getByRole('status').filter({ hasText: '顯示 1／1 筆來源紀錄' }).waitFor();
    record('another tab rotates the real session; stale read/write are rejected and old workspace is cleared');

    phase = 'delayed same-mode hydration';
    const delayed = await context.newPage();
    delayed.setDefaultTimeout(15000);
    const reached = Promise.withResolvers(), released = Promise.withResolvers();
    await delayed.route('**/api/reports/current', async route => {
      const response = await route.fetch();
      reached.resolve();
      await released.promise;
      await route.fulfill({ response }); // Delay the real response unchanged; no synthetic API payload.
    });
    try {
      await delayed.goto(baseURL, { waitUntil: 'domcontentloaded' });
      const requestTimeout = setTimeout(() => reached.reject(new Error('The delayed hydration request did not arrive.')), 15000);
      try { await reached.promise; } finally { clearTimeout(requestTimeout); }
      const latest = await importLaboratory(context, baseURL, { schema: 'healthbank.lab.v1', records: fixture.laboratory.records.slice(0, 3) });
      released.resolve();
      await delayed.getByText('使用階段已到期或更換，先前報告與醫療資料已清除。請確認目前使用者後重新取得資料。', { exact: true }).waitFor();
      assert.equal(await delayed.locator('.chart-point-target').count(), 0);
      assert.equal(await delayed.locator('.health-data-event').count(), 0);
      assert.equal((await currentReport(context, baseURL, latest.session.csrf_token)).records.length, 3);
    } finally { released.resolve(); await delayed.close(); }
    await page.reload({ waitUntil: 'networkidle' });
    const beforeLogout = await session(context, baseURL);
    await page.getByRole('button', { name: '結束瀏覽', exact: true }).click();
    await page.getByText('已結束使用階段並清除目前報告。', { exact: true }).waitFor();
    const loggedOut = await session(context, baseURL);
    assert.equal(loggedOut.mode, 'anonymous');
    assert.equal(loggedOut.authenticated, false);
    assert.ok(beforeLogout.csrf_token !== loggedOut.csrf_token, 'Logout rotates its generation.');
    assert.equal((await currentReport(context, baseURL, loggedOut.csrf_token)).records.length, 0);
    await navigate('醫療紀錄', '紀錄'); assert.equal(await page.locator('.health-data-event').count(), 0);
    record('delayed real response cannot hydrate a newer same-mode import; safe logout clears records');
    await context.close();

    phase = 'explicit synthetic count presentation fixture';
    const counts = await createContext();
    await importLaboratory(counts, baseURL, { schema: 'healthbank.lab.v1', records: fixture.laboratory.records.slice(0, 2) });
    page = await counts.newPage(); page.setDefaultTimeout(15000);
    await page.goto(baseURL, { waitUntil: 'networkidle' });
    await page.getByText('資料範圍', { exact: true }).click();
    await page.getByText('匯入資料保留 2 筆；本次未向官方重新核對資料或身分。', { exact: true }).waitFor();
    // Isolated payload override: a local count inconsistency presentation.
    // The backend remains an unverified imported session; its token/settings/rows
    // are unchanged. This never exercises or represents a successful NHI login.
    await page.route('**/api/session', async route => {
      const response = await route.fetch();
      await route.fulfill({ response, json: { ...await response.json(), mode: 'official', authenticated: true } });
    });
    await page.route('**/api/reports/current', async route => {
      const response = await route.fetch(), report = await response.json();
      await route.fulfill({ response, json: { ...report, complete: false,
        official_query: { mode: 'all_history', start_date: '1912-01-01', end_date: '2026-10-28', reported_count: 1 },
        warnings: ['合成核對差異：官方回報 1 筆，但實際取得 2 筆。'],
      } });
    });
    await page.reload({ waitUntil: 'networkidle' });
    await page.getByText('本次取得 2 筆原始結果', { exact: true }).waitFor();
    await page.getByText('資料範圍', { exact: true }).click();
    const mismatch = '官方回報 1 筆，實際保留 2 筆；筆數不一致，所有已回傳紀錄均保留，請查看來源警告。';
    assert.ok(await page.getByText(mismatch, { exact: false }).filter({ visible: true }).count() >= 2, 'Both visible count summaries must expose the mismatch.');
    const visibleText = await page.locator('body').innerText();
    assert.ok(!visibleText.includes('已完成筆數核對') && !visibleText.includes('未取得同期間的獨立官方總數'), 'Contradictory count states must not be displayed.');
    await page.screenshot({ path: path.join(evidenceDir, 'synthetic-count-mismatch.png') });
    record('isolated synthetic official-presentation count mismatch', { fixture_only: true, actual_official_login: false, rows: 2, reported_count: 1, complete: false, real_session_token_preserved: true });
    await counts.close();
    phase = 'explicit synthetic batch-controller fixture';
    const { runBatchAcceptance } = await import('./batch-acceptance.mjs');
    record('isolated synthetic date-batch controller', await runBatchAcceptance({ createContext, baseURL, evidenceDir, fixture, importLaboratory, session }));
    phase = 'explicit synthetic full-acquisition controller fixture';
    const { runFullAcquisitionAcceptance } = await import('./full-acquisition-acceptance.mjs');
    record('isolated synthetic full acquisition and original-file saving', await runFullAcquisitionAcceptance({ createContext, baseURL, evidenceDir, fixture, importLaboratory, session }));
    const { runCollectionRestoreAcceptance } = await import('./collection-restore-acceptance.mjs');
    record('batch and ZIP restoration with sticky mobile comparison dates', await runCollectionRestoreAcceptance({ createContext, baseURL, evidenceDir, fixture, importLaboratory, session }));
    phase = 'device cache atomic storage';
    const { runDeviceCacheStorageAcceptance } = await import('./device-cache-storage-acceptance.mjs');
    record('device cache opt-in, isolation and atomic revocation', await runDeviceCacheStorageAcceptance({ createContext, baseURL, fixture, importLaboratory }));
    phase = 'synthetic verified-user cache UI';
    const { runDeviceCacheAcceptance } = await import('./device-cache-acceptance.mjs');
    record('cached login, clear-before-refresh and cross-tab UI', await runDeviceCacheAcceptance({ createContext, baseURL, evidenceDir, fixture, importLaboratory }));
    phase = 'sourced bilingual terminology';
    const { runTerminologyAcceptance } = await import('./terminology-acceptance.mjs');
    record('bilingual labels, raw source dates and mobile wrapping', await runTerminologyAcceptance({ createContext, baseURL, evidenceDir, importLaboratory }));
    assert.equal(result.page_errors, 0, 'No unhandled browser errors are allowed.');
    assert.deepEqual(result.prohibited_requests, [], 'No official acquisition/login or external browser requests are allowed.');
    result.passed = true;
  } catch (failure) {
    result.failed_phase = phase;
    if (page && !page.isClosed()) await page.screenshot({ path: path.join(evidenceDir, 'synthetic-failure.png') }).catch(() => {});
    throw failure;
  } finally {
    try { await browser?.close(); }
    finally {
      try { await stopServer(child); }
      finally {
        await rm(runtime, { recursive: true, force: true });
        result.runtime_removed = true;
        await writeFile(path.join(evidenceDir, 'summary.json'), JSON.stringify(result, null, 2) + '\n');
      }
    }
  }
}
