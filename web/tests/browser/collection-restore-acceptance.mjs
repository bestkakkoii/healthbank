// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { batchCollectionJSON, describeBatchDataset, newBatchCollection, summarizeBatch, updateBatchDescriptor } from '../../src/batch-collection.ts';
import { beginDateBatch, missingBatchLaboratory } from '../../src/date-batches.ts';
import { exportDataset, exportDatasetPackage } from '../../src/health-dataset.ts';
import { syntheticZIP } from '../fixtures/acquisition-zip.mjs';

const digest = value => createHash('sha256').update(value).digest('hex');
const catalog = [{ id: 'medicine', label: '合成保存來源', group: '合成', range_mode: 'date_search', has_details: false }];
const source = entries => ({ category: 'medicine', entries, coverage: '合成來源沒有獨立總數', fetched_at: '2026-10-28T00:00:00Z', complete: false });
const openDisclosure = async locator => { if (!await locator.evaluate(element => element.open)) await locator.locator(':scope > summary').click(); };

export async function runCollectionRestoreAcceptance({ createContext, baseURL, evidenceDir, fixture, importLaboratory, session }) {
  const context = await createContext(), page = await context.newPage(); page.setDefaultTimeout(20000);
  try {
    const lab = { schema: 'healthbank.lab.v1', records: ['Glucose', 'Triglyceride', 'ALT'].map((name, index) => ({ ...fixture.laboratory.records[0],
      check_date: '2026-02-01', item_name: name, order_name: `合成 ${name}`, order_code: ['09002C', '09004C', '09025C'][index], result: ['103', '142', '27'][index],
      reference: '', unit: index === 2 ? 'U/L' : 'mg/dL', visit_date: '', source: `SYNTHETIC_RESTORE_${index}` })) };
    const imported = await importLaboratory(context, baseURL, lab);
    const navigate = async (full, short) => {
      const button = page.getByRole('button', { name: full, exact: true });
      if (await button.isVisible()) await button.click();
      else await page.getByRole('navigation', { name: '行動版主要導覽' }).getByRole('button', { name: short, exact: true }).click();
    };
    const tools = page.locator('#dataset-tools'), collection = page.locator('.collection-tools');
    const reopen = async () => {
      await page.locator('.busy-strip').waitFor({ state: 'hidden' }); await navigate('醫療紀錄', '紀錄');
      await openDisclosure(tools); await openDisclosure(collection);
    };
    const restoreClick = async button => {
      const [response] = await Promise.all([page.waitForResponse(response => new URL(response.url()).pathname === '/api/reports/import' && response.request().method() === 'POST'), button.click()]);
      assert.ok(response.ok(), 'The restored snapshot imports laboratory rows through real Go.');
      await reopen();
    };
    await page.goto(baseURL, { waitUntil: 'networkidle' }); await reopen();
    const plan = { ranges: [{ from: '2023-01-01', to: '2023-12-31' }, { from: '2024-01-01', to: '2024-12-31' }], categories: ['medicine'], includeLaboratory: false };
    let index = newBatchCollection(plan, 'year'); const members = [];
    for (const [ordinal, query] of plan.ranges.entries()) {
      const snapshot = beginDateBatch(query, plan.categories), row = { date: query.from, institution: '合成院所', title: `SYNTHETIC_BATCH_RESTORE_${ordinal + 1}`,
        source: 'SYNTHETIC_ONLY', fields: [], attachments: [{ id: '1'.repeat(48), name: '合成來源附件.txt', content_type: 'text/plain' }] };
      snapshot.reports.medicine = source([row, { ...row }]); snapshot.states.medicine = { status: 'ready', detail_errors: {}, query };
      const content = await exportDataset(missingBatchLaboratory(query, false), snapshot, [], catalog), name = `synthetic-batch-${ordinal + 1}.json`;
      const descriptor = await describeBatchDataset(content, name, summarizeBatch(snapshot, plan, null));
      index = updateBatchDescriptor(index, ordinal + 1, { ...descriptor, saved_acknowledged: true });
      members.push({ name, mimeType: 'application/json', buffer: Buffer.from(content) });
    }
    await page.getByLabel('選擇分批保存索引 JSON', { exact: true }).setInputFiles({ name: 'synthetic-batches-index.json', mimeType: 'application/json', buffer: Buffer.from(batchCollectionJSON(index)) });
    await collection.getByRole('heading', { name: '分批查詢：2023-01-01 至 2024-12-31', exact: true }).waitFor();
    await page.getByLabel('選擇分批資料 JSON 檔案', { exact: true }).setInputFiles(members);
    await restoreClick(collection.getByRole('button', { name: '開啟此批資料', exact: true }).first());
    assert.equal(await page.locator('.health-data-event').filter({ hasText: 'SYNTHETIC_BATCH_RESTORE_1' }).count(), 2);
    await collection.getByText('目前開啟第 1 批。', { exact: false }).waitFor();
    await restoreClick(collection.getByRole('button', { name: '開啟此批資料', exact: true }));
    assert.equal(await page.locator('.health-data-event').filter({ hasText: 'SYNTHETIC_BATCH_RESTORE_2' }).count(), 2);
    assert.equal(await page.locator('.health-data-event').filter({ hasText: 'SYNTHETIC_BATCH_RESTORE_1' }).count(), 0);
    await page.getByLabel('指定第 1 批 JSON', { exact: true }).setInputFiles(members[1]);
    await collection.getByRole('button', { name: '開啟此批資料', exact: true }).click();
    await collection.getByRole('alert').filter({ hasText: 'SHA-256' }).waitFor();
    assert.equal(await page.locator('.health-data-event').filter({ hasText: 'SYNTHETIC_BATCH_RESTORE_2' }).count(), 2, 'Invalid batch must preserve the current restored workspace.');
    await collection.getByRole('button', { name: '清除選取的本機檔案', exact: true }).click();
    await openDisclosure(collection);

    const row = { date: '2026-02-01', institution: '合成院所', title: 'SYNTHETIC_ZIP_RESTORE', source: 'SYNTHETIC_ONLY', fields: [],
      attachments: [1, 2].map(value => ({ id: String(value).repeat(48), name: `合成提示 ${value}`, content_type: 'application/octet-stream' })) };
    const snapshot = { reports: { medicine: source([row, { ...row }]) }, details: {}, states: { medicine: { status: 'ready', detail_errors: {} } }, query: { from: '', to: '' } };
    const projection = await exportDatasetPackage(imported.report, snapshot, catalog);
    const originalBytes = [Buffer.from('合成中文原始檔，不屬於真實個人。\n'), Buffer.from('BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nSUMMARY:合成建議領藥日期，未自動匯入行事曆\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n')];
    const zipFor = (retry = false) => {
      const originals = projection.files.map((reference, ordinal) => ({ category: reference.category, archive_id: reference.archive_id,
        usable: retry || ordinal === 0, state: retry || ordinal === 0 ? 'completed' : 'failed', bytes: retry || ordinal === 0 ? originalBytes[ordinal].length : 0,
        ...(retry || ordinal === 0 ? { sha256: digest(originalBytes[ordinal]), path: `originals/${reference.archive_id}/合成原始檔${ordinal + 1}.${ordinal === 0 ? 'txt' : 'ics'}`,
          content_type: ordinal === 0 ? 'text/plain' : 'text/calendar' } : { code: 'DOCUMENT_NOT_READY' }) }));
      const files = retry ? originals.slice(1) : originals;
      const manifest = { schema: 'healthbank.acquisition.package.v1', created_at: '2026-10-28T00:00:00Z', state: retry ? 'completed' : 'partial',
        plan: { id: (retry ? 'b' : 'a').repeat(32), index: 1, total: 1, files_total: files.length },
        dataset: { name: 'healthbank.dataset.v2.json', size: Buffer.byteLength(projection.content), sha256: digest(projection.content), included: true },
        acquisition: { scope: 'all_available', started_at: '2026-10-28T00:00:00Z', finished_at: '2026-10-28T00:00:01Z',
          statuses: [{ category: 'medicine', kind: 'source', state: 'ready', count: 1, code: 'source_total_unknown' }] }, files,
        history_completeness: 'preserved_in_dataset_and_source_statuses_not_inferred_from_transfer' };
      return { name: retry ? 'synthetic-retry.zip' : 'synthetic-first.zip', mimeType: 'application/zip', buffer: syntheticZIP([
        { name: 'healthbank.dataset.v2.json', content: projection.content, method: 8 }, { name: 'README.txt', content: '合成 ZIP', method: 8 },
        ...files.filter(file => file.usable).map(file => ({ name: file.path, content: originalBytes[projection.files.findIndex(reference => reference.archive_id === file.archive_id)], method: 0 })),
        { name: 'manifest.json', content: JSON.stringify(manifest) + '\n', method: 8 },
      ]) };
    };
    await page.getByLabel('選擇原始檔 ZIP 部分', { exact: true }).setInputFiles(zipFor());
    await collection.getByRole('heading', { name: '1 個 ZIP 部分／補取包', exact: true }).waitFor();
    await restoreClick(collection.getByRole('button', { name: '驗證原始檔並還原資料', exact: true }));
    await collection.getByText('來源取得失敗／未完成，不能使用', { exact: false }).waitFor();
    assert.equal(await page.locator('.health-data-event').filter({ hasText: 'SYNTHETIC_ZIP_RESTORE' }).count(), 2);
    await page.getByLabel('選擇原始檔 ZIP 部分', { exact: true }).setInputFiles(zipFor(true));
    await collection.getByRole('heading', { name: '2 個 ZIP 部分／補取包', exact: true }).waitFor();
    await restoreClick(collection.getByRole('button', { name: '接續校驗並還原原始資料', exact: true }));
    assert.equal(await collection.getByRole('button', { name: '另存此原始檔', exact: true }).count(), 2);
    const [originalDownload] = await Promise.all([page.waitForEvent('download'), collection.getByRole('button', { name: '另存此原始檔', exact: true }).nth(1).click()]);
    const originalPath = path.join(evidenceDir, 'synthetic-restored-refill.ics'); await originalDownload.saveAs(originalPath);
    assert.deepEqual(await readFile(originalPath), originalBytes[1]);
    const [exported] = await Promise.all([page.waitForEvent('download'), tools.getByRole('button', { name: '匯出全部已取得資料', exact: true }).click()]);
    const archivePath = path.join(evidenceDir, 'synthetic-restored-complete.json'); await exported.saveAs(archivePath);
    const archived = JSON.parse(await readFile(archivePath, 'utf8'));
    assert.equal(archived.attachments.length, 2); assert.equal(archived.data.reports.medicine.entries.length, 2); assert.equal(archived.data.reports.medicine.complete, false);
    const after = await session(context, baseURL); assert.equal(after.mode, 'import'); assert.equal(after.authenticated, false);
    assert.ok(after.csrf_token !== imported.session.csrf_token, 'Each explicit restore rotates real local session generation.');
    await page.locator('.toast').waitFor({ state: 'hidden' });
    await collection.scrollIntoViewIfNeeded();
    await page.screenshot({ path: path.join(evidenceDir, 'synthetic-collection-restoration.png') });

    await navigate('個人設定', '設定');
    const scaleButton = page.getByRole('button', { name: /200%/ });
    if (await scaleButton.getAttribute('aria-pressed') !== 'true') {
      const [response] = await Promise.all([page.waitForResponse(response => new URL(response.url()).pathname === '/api/settings' && response.request().method() === 'PUT'), scaleButton.click()]);
      assert.ok(response.ok());
    }
    const layouts = [];
    for (const size of [{ width: 320, height: 740 }, { width: 390, height: 844 }, { width: 844, height: 390 }]) {
      await page.setViewportSize(size); await navigate('年度比較', '比較');
      const scroll = page.locator('.comparison-scroll'), cell = page.locator('.comparison-table tbody .date-column').first();
      await cell.waitFor();
      await cell.scrollIntoViewIfNeeded();
      await scroll.evaluate(element => { element.scrollLeft = 0; });
      const before = await cell.boundingBox(); assert.ok(before);
      await scroll.evaluate(element => { element.scrollLeft = element.scrollWidth - element.clientWidth; });
      await page.waitForFunction(() => (globalThis.document.querySelector('.comparison-scroll')?.scrollLeft ?? 0) > 100);
      const after = await cell.boundingBox(); assert.ok(after);
      assert.ok(Math.abs(before.x - after.x) < 1, 'Date column remains stationary during real horizontal scrolling.');
      assert.equal(await cell.evaluate(element => globalThis.getComputedStyle(element).position), 'sticky');
      const finalValue = await page.locator('.comparison-screen .comparison-table tbody .result-cell').last().boundingBox();
      const container = await scroll.boundingBox(); assert.ok(finalValue && container);
      assert.ok(finalValue.x >= after.x + after.width - 1 && finalValue.x + finalValue.width <= container.x + container.width + 1,
        'The last measurement remains fully visible beside the date at the horizontal scroll limit.');
      const measured = await page.evaluate(() => ({ width: globalThis.document.documentElement.scrollWidth,
        font: globalThis.document.documentElement.style.getPropertyValue('--font-scale') }));
      assert.equal(measured.font, '2'); assert.ok(measured.width <= size.width + 1, '200% phone/landscape page must not overflow horizontally.');
      layouts.push({ ...size, date_column_fixed: true, font_scale: 2 });
      await page.screenshot({ path: path.join(evidenceDir, `synthetic-sticky-date-${size.width}x${size.height}.png`) });
    }
    return { synthetic_only: true, actual_official_login: false, batch_snapshots_restored: 2, duplicate_source_rows_retained: true,
      wrong_batch_rejected_without_replacing_workspace: true, zip_retry_parts_merged_by_exact_dataset_and_archive_ids: true,
      originals_restored: 2, actual_calendar_download_bytes_verified: true, source_completeness_preserved: true, sticky_mobile_layouts: layouts };
  } catch (failure) {
    if (!page.isClosed()) await page.screenshot({ path: path.join(evidenceDir, 'synthetic-collection-restoration-failure.png') }).catch(() => {});
    throw failure;
  } finally { await context.close(); }
}
