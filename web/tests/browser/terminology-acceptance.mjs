// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import assert from 'node:assert/strict';
import path from 'node:path';

// Synthetic vocabulary and observations only; this exercises the real local
// importer/catalog/UI without authentication to any official health service.
export async function runTerminologyAcceptance({ createContext, baseURL, evidenceDir, importLaboratory }) {
  const context = await createContext(), page = await context.newPage();
  page.setDefaultTimeout(20000);
  const rawOrder = 'SYNTHETIC_ORDER_NT_DETAIL <img src=x onerror="globalThis.syntheticTermInjection=1">';
  const rawSource = '合成來源名稱 <script>globalThis.syntheticTermInjection=1</script>';
  const row = (item_name, order_code, unit, result, extra = {}) => ({
    check_date: '109/02/03', visit_date: '109/02/01', upload_date: '109/02/06',
    item_name, order_code, unit, result, order_name: `合成醫令 ${item_name}`,
    reference: '', institution: '合成檢驗機構', source: 'SYNTHETIC_SOURCE_CODE',
    source_name: '合成來源名稱', official_flag: '', ...extra,
  });
  const nt = row('NT-ProBNP', '12193C', 'pg/mL', '123.000000000000001', { order_name: rawOrder, source_name: rawSource, official_flag: 'SYNTHETIC_FLAG_H' });
  const records = [row('A/C ratio', '06012C', 'mg/gCr', '<10.000000000000001'),
    row('ACR', '09016C', 'mg/g', '12.000000000000001'), nt, { ...nt },
    row('PO2', '09041B', 'mmHg', '33.000000000000001'),
    row('是否人工閱片', 'SYNTHETIC_REVIEW', '無單位', '')];
  const navigate = async (full, short) => {
    const button = page.getByRole('button', { name: full, exact: true });
    if (await button.isVisible()) await button.click();
    else await page.getByRole('navigation', { name: '行動版主要導覽' }).getByRole('button', { name: short, exact: true }).click();
  };
  try {
    const imported = await importLaboratory(context, baseURL, { schema: 'healthbank.lab.v2', records });
    assert.equal(imported.report.records.length, records.length);
    const ntRows = imported.report.records.filter(record => record.item_name === 'NT-ProBNP');
    assert.equal(ntRows.length, 2); assert.notEqual(ntRows[0].id, ntRows[1].id);
    assert.equal(ntRows[0].raw_upload_date, '109/02/06'); assert.equal(ntRows[0].upload_date, '2020-02-06');
    assert.equal(ntRows[0].exam_date, '2020-02-03'); assert.equal(ntRows[0].source_name, rawSource);
    await page.goto(baseURL, { waitUntil: 'networkidle' });
    await navigate('年度比較', '比較');
    const screen = page.locator('.comparison-screen'), search = page.locator('.filter-bar input[type="search"]');
    const headers = screen.locator('thead .measurement-column');
    await search.fill('albumin-to-creatinine');
    await headers.getByRole('button', { name: '尿白蛋白／肌酸酐比值（半定量）', exact: true }).waitFor();
    await headers.getByRole('button', { name: '尿白蛋白／肌酸酐比值（定量）', exact: true }).waitFor();
    assert.equal(await headers.count(), 2, 'Shared settings must not merge distinct result channels.');
    assert.equal(await headers.locator('.terminology-english').count(), 2);
    await headers.getByText('原名：A/C ratio', { exact: true }).waitFor();
    await headers.getByText('原名：ACR', { exact: true }).waitFor();

    await search.fill('N-terminal pro-B-type natriuretic peptide');
    await headers.getByText('N-terminal pro-B-type natriuretic peptide (NT-proBNP)', { exact: true }).waitFor();
    assert.equal(await headers.count(), 1);
    await headers.getByRole('button', { name: 'N端B型利鈉肽前驅物', exact: true }).waitFor();
    await headers.getByText('原名：NT-ProBNP', { exact: true }).waitFor();
    assert.equal(await screen.locator('button.result-cell').count(), 2, 'Duplicate observations remain separately selectable.');
    await search.fill('SYNTHETIC_ORDER_NT_DETAIL');
    await screen.locator('button.result-cell').first().waitFor();
    assert.equal(await headers.count(), 1, 'Original order names participate in search.');
    await screen.locator('button.result-cell').first().click();
    const detail = page.getByRole('dialog');
    await detail.getByRole('heading', { name: 'N端B型利鈉肽前驅物', exact: true }).waitFor();
    const field = name => detail.getByText(name, { exact: true }).locator('..').locator('dd');
    assert.equal(await field('原始檢驗名稱').textContent(), 'NT-ProBNP');
    assert.equal(await field('原始單位').textContent(), 'pg/mL');
    assert.equal(await field('原院旗標原文').textContent(), 'SYNTHETIC_FLAG_H', 'Unknown source flag spellings remain inspectable without being interpreted as the special official 1 flag.');
    assert.equal(await field('檢驗檢查醫囑名稱').textContent(), rawOrder);
    assert.equal(await field('來源名稱').textContent(), rawSource);
    assert.equal(await field('資料來源').textContent(), 'SYNTHETIC_SOURCE_CODE');
    assert.ok((await field('上傳日期').textContent()).includes('2020/02/06'));
    assert.ok((await field('上傳日期').textContent()).includes('109/02/06'));
    assert.ok((await field('檢查日期').textContent()).includes('2020/02/03'));
    assert.ok((await detail.locator('.detail-result').textContent()).includes(nt.result));
    await detail.getByText('名稱來源與整理方式', { exact: true }).click();
    assert.ok(await detail.locator('.source-list a').count() > 0, 'Name attribution remains independently inspectable.');
    assert.equal(await page.evaluate(() => globalThis.syntheticTermInjection), undefined, 'Source names/order labels render only as inert text.');
    await detail.getByRole('button', { name: '關閉視窗', exact: true }).click();

    await search.fill('Manual slide review status');
    await headers.getByText('Manual slide review status', { exact: true }).waitFor();
    await headers.getByText('原名：是否人工閱片', { exact: true }).waitFor();
    assert.equal(await headers.count(), 1, 'An empty source-status row keeps its exact-name glossary without becoming a measurement.');
    await screen.locator('button.result-cell').first().click();
    assert.equal(await field('原始單位').textContent(), '無單位', 'Literal missing-unit source text must not disappear from the detail.');
    assert.equal(await field('原院旗標原文').textContent(), '未提供');
    await detail.getByRole('button', { name: '關閉視窗', exact: true }).click();
    await search.fill('Blood-gas oxygen partial pressure');
    await headers.getByRole('button', { name: '血氣氧分壓', exact: true }).waitFor();
    assert.ok(!(await headers.textContent()).includes('靜脈血氧分壓'), 'Generic source gas names do not assert venous collection.');

    await navigate('個人設定', '設定');
    const scale = page.getByRole('button', { name: /200%/ });
    if (await scale.getAttribute('aria-pressed') !== 'true') {
      const [response] = await Promise.all([page.waitForResponse(response => new URL(response.url()).pathname === '/api/settings' && response.request().method() === 'PUT'), scale.click()]);
      assert.ok(response.ok());
    }
    const layouts = [];
    for (const size of [{ width: 320, height: 740 }, { width: 844, height: 390 }]) {
      await page.setViewportSize(size); await navigate('年度比較', '比較');
      await search.fill('N-terminal pro-B-type natriuretic peptide');
      const english = headers.locator('.terminology-english');
      await english.waitFor(); await english.scrollIntoViewIfNeeded();
      await page.evaluate(() => globalThis.document.fonts.ready);
      const measured = await english.evaluate(element => {
        const style = globalThis.getComputedStyle(element);
        return { width: globalThis.document.documentElement.scrollWidth, font_scale: globalThis.document.documentElement.style.getPropertyValue('--font-scale'),
          clipped: element.scrollWidth > element.clientWidth + 1, wraps: element.getBoundingClientRect().height > Number.parseFloat(style.lineHeight) * 1.5,
          overflow_wrap: style.overflowWrap, language: element.getAttribute('lang') };
      });
      assert.equal(measured.font_scale, '2'); assert.equal(measured.language, 'en');
      assert.ok(measured.width <= size.width + 1, 'Bilingual headings must not overflow the mobile document.');
      assert.equal(measured.clipped, false); assert.equal(measured.overflow_wrap, 'anywhere');
      assert.equal(measured.wraps, true, 'Long English names must wrap visibly rather than being clamped or omitted.');
      await screen.locator('button.result-cell').first().click();
      await detail.getByRole('button', { name: '關閉視窗', exact: true }).click();
      await english.scrollIntoViewIfNeeded();
      await page.screenshot({ path: path.join(evidenceDir, `synthetic-terminology-${size.width}x${size.height}.png`) });
      layouts.push({ ...size, font_scale: 2, english_wrapped: true, detail_operable: true });
    }
    return { synthetic_only: true, official_login_tested: false, source_rows: records.length, acr_channels: 2,
      duplicate_rows_retained: true, raw_item_order_result_source_and_dates_retained: true, raw_unit_and_uninterpreted_flags_visible: true,
      attributed_and_literal_names_separate: true, english_and_order_name_search: true, layouts };
  } catch (failure) {
    if (!page.isClosed()) await page.screenshot({ path: path.join(evidenceDir, 'synthetic-terminology-failure.png') }).catch(() => {});
    throw failure;
  } finally { await context.close(); }
}
