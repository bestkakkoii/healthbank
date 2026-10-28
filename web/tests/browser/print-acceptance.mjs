// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import assert from 'node:assert/strict';
import { readFile, mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';

function pdfText(path) {
  // Reading order preserves wrapped values within their cell. Layout mode can
  // interleave adjacent table columns inside a long raw timestamp.
  const result = spawnSync('pdftotext', ['-raw', path, '-'], { encoding: 'utf8', maxBuffer: 16 * 1024 * 1024 });
  assert.equal(result.status, 0, 'PDF text extraction must succeed; install poppler-utils for browser acceptance');
  return result.stdout.replace(/\s+/g, '');
}
function pdfPages(path) {
  const result = spawnSync('pdfinfo', [path], { encoding: 'utf8', maxBuffer: 1024 * 1024 });
  assert.equal(result.status, 0, 'PDF structure inspection must succeed');
  const match = /^Pages:\s+(\d+)$/m.exec(result.stdout);
  assert.ok(match, 'PDF must expose a valid page count');
  return Number(match[1]);
}

function pdfToolVersions() {
  return Object.fromEntries(['pdftotext', 'pdfinfo'].map(tool => {
    const result = spawnSync(tool, ['-v'], { encoding: 'utf8', maxBuffer: 1024 * 1024 });
    assert.equal(result.status, 0, `${tool} version must be available`);
    return [tool, (result.stderr || result.stdout).split(/\r?\n/)[0]];
  }));
}

/** Actual data-generated report/download/PDF acceptance; every fixture is synthetic. */
export async function runPrintAcceptance({ page, context, evidenceDir, fixture }) {
  await mkdir(evidenceDir, { recursive: true });
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.getByRole('button', { name: '醫療紀錄', exact: true }).click();
  await page.getByRole('button', { name: '製作列印／PDF 報表', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: '製作列印／PDF 報表', exact: true });
  await dialog.getByRole('button', { name: '列印／另存 PDF', exact: true }).waitFor();
  const laboratory = fixture.laboratory.records;
  const clinical = Object.entries(fixture.data.reports).flatMap(([category, report]) => [...report.entries,
    ...Object.values(fixture.data.details[category] ?? {}).flatMap(detail => detail.entries)]);

  function verifyPDF(path, expectedRows, expectedClinical, matrix = false) {
    const pages = pdfPages(path), text = pdfText(path);
    assert.ok(pages > 1, 'Long actual datasets must paginate beyond a viewport-sized page');
    assert.ok(text.includes('健康映覽') && text.includes('bestkakkoiillc.'), 'Chinese report and copyright text must survive PDF extraction');
    for (const row of expectedRows) {
      if (!matrix) assert.ok(text.includes(row.source.replace(/\s+/g, '')), `PDF must include source marker ${row.source}`);
      assert.ok(text.includes(row.check_date.replace(/\s+/g, '')), 'PDF must include every raw source date, including both normalized-date duplicates');
      assert.ok(text.includes(row.result.replace(/\s+/g, '')), 'PDF must include every original value');
    }
    for (const row of expectedClinical) assert.ok(text.includes(row.title.replace(/\s+/g, '')), 'PDF must include every source record title beyond on-screen pagination');
    return { laboratoryRows: expectedRows.length, clinicalRows: expectedClinical.length, pages, pdf: path };
  }
  function verifySelectedScope(path) {
    const text = pdfText(path);
    for (const row of laboratory.filter(row => !row.check_date.startsWith('2016'))) assert.ok(!text.includes(row.source), 'Selected PDF must not contain excluded source markers');
    assert.ok(!text.includes('SYNTHETIC_EXCLUDED_MEDICINE_COVERAGE') && !text.includes('清單120筆'), 'Selected laboratory-only PDF must not disclose excluded medicine coverage or counts');
    assert.ok(!text.includes('SYNTHETIC_EXCLUDED_GLOBAL_WARNING'), 'Selected PDF must not copy out-of-scope global clinical warnings');
  }

  async function saveAndRender(name, expectedRows, expectedClinical, matrix = false) {
    const pending = page.waitForEvent('download');
    await dialog.getByRole('button', { name: '下載專用報表 HTML', exact: true }).click();
    const download = await pending;
    const htmlPath = join(evidenceDir, `${name}.html`);
    await download.saveAs(htmlPath);
    const html = await readFile(htmlPath, 'utf8');
    assert.equal((html.match(matrix ? /class="observation"/g : /data-laboratory-row=/g) ?? []).length, expectedRows.length);
    assert.equal((html.match(/data-clinical-row=/g) ?? []).length, expectedClinical.length);
    assert.ok(!html.includes('<script'), 'Source strings must never become executable report markup');
    assert.ok(!html.includes('session_token') && !html.includes('csrf_token'), 'Report must omit live session capabilities');
    const rendered = await context.newPage();
    try {
      await rendered.setContent(html, { waitUntil: 'load' });
      await rendered.emulateMedia({ media: 'print' });
      const path = join(evidenceDir, `${name}.pdf`);
      await rendered.pdf({ path, format: 'A4', preferCSSPageSize: true, printBackground: true });
      return { ...verifyPDF(path, expectedRows, expectedClinical, matrix), html: htmlPath };
    } finally { await rendered.close(); }
  }

  const full = await saveAndRender('print-complete-records', laboratory, clinical);
  const fullText = pdfText(full.pdf);
  assert.ok(fullText.includes('SYNTHETIC_EXCLUDED_MEDICINE_COVERAGE') && fullText.includes('SYNTHETIC_EXCLUDED_GLOBAL_WARNING'), 'Full report must retain its synthetic global source warnings before scope exclusions are tested');
  await dialog.getByLabel('包含醫療來源紀錄', { exact: true }).uncheck();
  await dialog.getByRole('combobox', { name: '檢驗排版', exact: true }).selectOption('comparison');
  const comparison = await saveAndRender('print-complete-comparison', laboratory, [], true);
  assert.ok(!pdfText(comparison.pdf).includes('SYNTHETIC_EXCLUDED_MEDICINE_COVERAGE'), 'Excluding clinical records also excludes their source-coverage details');
  await dialog.getByRole('button', { name: '關閉', exact: true }).click();

  await page.getByRole('button', { name: '年度比較', exact: true }).click();
  await page.getByRole('combobox', { name: '比較年度', exact: true }).selectOption('2016');
  await page.getByRole('button', { name: '列印／PDF', exact: true }).click();
  await dialog.getByRole('combobox', { name: '報表範圍', exact: true }).selectOption('selection');
  await dialog.getByLabel('包含醫療來源紀錄', { exact: true }).uncheck();
  const selectedRows = laboratory.filter(row => row.check_date.startsWith('2016'));
  assert.ok(selectedRows.length > 0 && selectedRows.length < laboratory.length);
  const selected = await saveAndRender('print-selected-records', selectedRows, []);
  verifySelectedScope(selected.pdf);

  // Chromium's native PDF path invokes beforeprint/afterprint on the actual app.
  // An open sharing dialog must retain its precise selected scope in that path.
  const nativeSelectedPath = join(evidenceDir, 'print-native-selected-records.pdf');
  await page.pdf({ path: nativeSelectedPath, format: 'A4', preferCSSPageSize: true, printBackground: true });
  const nativeSelected = verifyPDF(nativeSelectedPath, selectedRows, []);
  verifySelectedScope(nativeSelectedPath);
  assert.equal(await page.locator('#healthbank-native-print').count(), 0, 'Native print must remove its ephemeral data document after printing.');
  assert.ok(await dialog.isVisible(), 'Native print must preserve the prepared sharing dialog.');

  // Exercise the actual print action, but replace the native dialog with a
  // counter solely inside the isolated generated-document frame.
  const preview = page.frameLocator('iframe[title="專用報表完整預覽"]');
  await preview.locator('body').waitFor();
  await page.locator('iframe[title="專用報表完整預覽"]').evaluate(frame => {
    frame.contentWindow.print = () => { frame.dataset.syntheticPrintCalls = String(Number(frame.dataset.syntheticPrintCalls ?? '0') + 1); };
  });
  await dialog.getByRole('button', { name: '列印／另存 PDF', exact: true }).click();
  assert.equal(await page.locator('iframe[title="專用報表完整預覽"]').getAttribute('data-synthetic-print-calls'), '1');
  await dialog.getByRole('button', { name: '關閉', exact: true }).click();
  assert.equal(await page.getByRole('combobox', { name: '比較年度', exact: true }).inputValue(), '2016');
  const nativeFullPath = join(evidenceDir, 'print-native-complete-records.pdf');
  await page.pdf({ path: nativeFullPath, format: 'A4', preferCSSPageSize: true, printBackground: true });
  const nativeFull = verifyPDF(nativeFullPath, laboratory, clinical);
  assert.equal(await page.locator('#healthbank-native-print').count(), 0, 'Native print must remove its ephemeral data document after printing.');
  assert.equal(await page.getByRole('combobox', { name: '比較年度', exact: true }).inputValue(), '2016', 'Full native print must preserve the on-screen year filter.');
  assert.ok(await page.locator('#main-content').isVisible(), 'The workspace remains usable after native print.');
  return { full, selected, comparison, nativeFull, nativeSelected, pdfToolVersions: pdfToolVersions(), nativePrintTarget: 'generated-document-frame', syntheticPrintDialogReplacement: true };
}
