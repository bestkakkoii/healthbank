// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';

export async function runGradingContextAcceptance({ createContext, baseURL, evidenceDir, importLaboratory }) {
  const context = await createContext(), page = await context.newPage();
  page.setDefaultTimeout(20000);
  const raw = (check_date, result) => ({ check_date, visit_date: check_date, item_name: 'Triglyceride',
    order_code: '09004C', order_name: '合成分級條件測試', result, unit: 'mg/dL', reference: '',
    institution: '合成分級測試院所', source: 'SYNTHETIC_GRADING_ONLY', official_flag: '1' });
  const download = async (button, name) => {
    const [file] = await Promise.all([page.waitForEvent('download'), button.click()]);
    const output = path.join(evidenceDir, name); await file.saveAs(output);
    assert.equal(await file.failure(), null); return readFile(output, 'utf8');
  };
  try {
    const imported = await importLaboratory(context, baseURL, { schema: 'healthbank.lab.v2', records: [
      raw('112/03/07', '245.000000000000000001'), raw('113/04/09', '565.000000000000000001'),
    ] });
    const records = imported.report.records;
    assert.equal(records.length, 2);
    assert.ok(records.every(record => record.grading_contexts?.some(value => value.rule_id === 'adult_triglycerides')),
      'The real Go engine must return the applied qualified triglyceride profile.');
    const contexts = [...new Map(records.flatMap(record => record.grading_contexts).map(value => [JSON.stringify(value), value])).values()];
    assert.ok(contexts.every(value => value.label && value.note));
    await page.goto(baseURL, { waitUntil: 'networkidle' });
    await page.getByRole('button', { name: '年度比較', exact: true }).click();
    await page.locator('.comparison-screen .grading-context-label').first().waitFor();
    const qualifiers = await page.locator('.comparison-screen').innerText();
    assert.ok(qualifiers.includes('條件未確認'));
    assert.equal(await page.locator('.comparison-screen .official-flag-badge').filter({ hasText: '原院異常' }).count(), 2);
    await page.getByRole('button', { name: '數值趨勢', exact: true }).click();
    await page.locator('.trend-svg .chart-point-target').first().waitFor();
    const displayedNotes = page.locator('.trend-section > .grading-context-notes').first();
    await displayedNotes.locator(':scope > summary').click();
    const noteText = await displayedNotes.innerText();
    for (const value of contexts) assert.ok(noteText.includes(value.note), 'The interactive chart must expose full notes.');
    const csv = await download(page.getByRole('button', { name: '匯出篩選的 2 筆 CSV', exact: true }), 'synthetic-qualified-levels.csv');
    for (const value of contexts) assert.ok(csv.includes(value.note));
    for (const record of records) assert.ok(csv.includes(record.result));
    const svg = await download(page.getByRole('button', { name: '下載圖表', exact: true }).first(), 'synthetic-qualified-chart.svg');
    const inspected = await page.evaluate(text => {
      const parsed = new globalThis.DOMParser().parseFromString(text, 'image/svg+xml');
      return { error: Boolean(parsed.querySelector('parsererror')), description: parsed.querySelector('desc')?.textContent ?? '',
        legend: parsed.querySelector('g[aria-label="分級依據與適用限制"]')?.textContent ?? '',
        points: parsed.querySelectorAll('.chart-point-target').length };
    }, svg);
    assert.equal(inspected.error, false); assert.equal(inspected.points, 2);
    for (const value of contexts) {
      assert.ok(inspected.description.includes(value.note));
      assert.ok(inspected.legend.replace(/\s+/g, '').includes(value.note.replace(/\s+/g, '')), 'The downloaded SVG must also visibly retain full qualifications.');
    }
    const preview = await context.newPage();
    await preview.setContent(svg);
    const bounds = await preview.locator('svg').evaluate(element => {
      const legend = element.querySelector('g[aria-label="分級依據與適用限制"]');
      const box = legend.getBBox(), view = element.viewBox.baseVal;
      return { bottom: box.y + box.height, right: box.x + box.width, height: view.height, width: view.width };
    });
    assert.ok(bounds.bottom <= bounds.height && bounds.right <= bounds.width, 'Exported qualification text must fit inside the SVG bounds.');
    await preview.screenshot({ path: path.join(evidenceDir, 'synthetic-qualified-chart.png'), fullPage: true });
    await preview.close();

    await page.getByRole('button', { name: '列印／PDF', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: '製作列印／PDF 報表', exact: true });
    const html = await download(dialog.getByRole('button', { name: '下載專用報表 HTML', exact: true }), 'synthetic-qualified-report.html');
    for (const value of contexts) assert.ok(html.includes(value.note));
    for (const record of records) assert.ok(html.includes(record.result));
    assert.equal((html.match(/data-laboratory-row=/g) ?? []).length, 2);
    assert.ok(!html.includes(imported.session.csrf_token), 'Derived presentation files must not contain the local session token.');
    return { synthetic_only: true, official_login_tested: false, real_go_grading_contexts: true,
      compact_visible_labels: true, source_flags_preserved: true, full_notes_accessible: true,
      downloaded_csv_and_print_qualifications: true, downloaded_svg_visible_and_accessible_qualifications: true,
      svg_qualification_text_inside_bounds: true, original_exact_results_preserved: true };
  } catch (failure) {
    if (!page.isClosed()) await page.screenshot({ path: path.join(evidenceDir, 'synthetic-grading-context-failure.png') }).catch(() => {});
    throw failure;
  } finally { await context.close(); }
}
