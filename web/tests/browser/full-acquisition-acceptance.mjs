// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { readFile } from 'node:fs/promises';
import { createServer, request as httpRequest } from 'node:http';
import path from 'node:path';
import { importDataset } from '../../src/health-dataset.ts';

const digest = bytes => createHash('sha256').update(bytes).digest('hex');
const opaque = number => number.toString(16).padStart(48, '0');
const json = body => ({ status: 200, contentType: 'application/json', body: JSON.stringify(body) });
const missing = () => ({ status: 404, contentType: 'application/json', body: JSON.stringify({ error: { code: 'package_unavailable', message: '合成下載包已清除。' } }) });
const entry = (title, extra = {}) => ({ date: '115/10/28', institution: '合成取得院所', title, source: 'SYNTHETIC_FULL_ACQUISITION',
  fields: [{ name: '來源精確原文', value: '400.00000000000000001' }], ...extra });
const source = (category, entries) => ({ category, entries, fetched_at: '2026-10-28T00:00:00Z', complete: false,
  coverage: '合成來源，歷史總數未確認；不屬於任何真實個人。' });
const categories = [
  { id: 'imaging', label: '合成影像來源', group: '合成完整取得', range_mode: 'source', has_details: true },
  { id: 'official_files', label: '合成官方工作檔案', group: '合成完整取得', range_mode: 'source', has_details: true },
  { id: 'exports', label: '合成原始匯出', group: '合成完整取得', range_mode: 'source' },
];

// A deliberately tiny, stored ZIP fixture. Its bytes exercise the actual
// browser download; production ZIP generation is tested by the Go package.
function storedZIP(entries) {
  let offset = 0;
  const locals = [], directory = [];
  for (const [filename, content] of entries) {
    const name = Buffer.from(filename), bytes = Buffer.from(content);
    let crc = 0xffffffff;
    for (const byte of bytes) {
      crc ^= byte;
      for (let bit = 0; bit < 8; bit++) crc = (crc >>> 1) ^ (crc & 1 ? 0xedb88320 : 0);
    }
    crc = (crc ^ 0xffffffff) >>> 0;
    const local = Buffer.alloc(30);
    local.writeUInt32LE(0x04034b50, 0); local.writeUInt16LE(20, 4); local.writeUInt16LE(0x800, 6);
    local.writeUInt32LE(crc, 14); local.writeUInt32LE(bytes.length, 18); local.writeUInt32LE(bytes.length, 22); local.writeUInt16LE(name.length, 26);
    const central = Buffer.alloc(46);
    central.writeUInt32LE(0x02014b50, 0); central.writeUInt16LE(20, 4); central.writeUInt16LE(20, 6); central.writeUInt16LE(0x800, 8);
    central.writeUInt32LE(crc, 16); central.writeUInt32LE(bytes.length, 20); central.writeUInt32LE(bytes.length, 24);
    central.writeUInt16LE(name.length, 28); central.writeUInt32LE(offset, 42);
    locals.push(local, name, bytes); directory.push(central, name); offset += local.length + name.length + bytes.length;
  }
  const central = Buffer.concat(directory), end = Buffer.alloc(22);
  end.writeUInt32LE(0x06054b50, 0); end.writeUInt16LE(entries.length, 8); end.writeUInt16LE(entries.length, 10);
  end.writeUInt32LE(central.length, 12); end.writeUInt32LE(offset, 16);
  return Buffer.concat([...locals, central, end]);
}

async function saveDownload(page, control, output) {
  const [download] = await Promise.all([page.waitForEvent('download'), control.click()]);
  await download.saveAs(output);
  assert.equal(await download.failure(), null, 'Synthetic acquisition download must actually finish.');
  return readFile(output);
}

/** API controller fixtures are synthetic; local import/logout and downloads are real. */
export async function runFullAcquisitionAcceptance({ createContext, baseURL, evidenceDir, fixture, importLaboratory, session }) {
  const upstreamOrigin = baseURL;
  let nativeDownload = (_request, response) => { response.writeHead(404); response.end(); };
  // Native browser downloads bypass page request interception in Chromium.
  // This isolated loopback proxy serves only synthetic download bytes; every
  // other unmocked endpoint still reaches the compiled Go process unchanged.
  const fixtureServer = createServer((request, response) => {
    if (new URL(request.url, upstreamOrigin).pathname === '/api/acquisition/packages/download') {
      nativeDownload(request, response); return;
    }
    const upstream = httpRequest(new URL(request.url, upstreamOrigin), { method: request.method, headers: request.headers }, incoming => {
      response.writeHead(incoming.statusCode, incoming.headers); incoming.pipe(response);
    });
    upstream.on('error', () => { if (!response.headersSent) response.writeHead(502); response.end(); });
    request.on('aborted', () => upstream.destroy());
    response.on('close', () => { if (!response.writableEnded) upstream.destroy(); });
    request.pipe(upstream);
  });
  await new Promise((resolve, reject) => { fixtureServer.once('error', reject); fixtureServer.listen(0, '127.0.0.1', resolve); });
  baseURL = `http://127.0.0.1:${fixtureServer.address().port}`;
  try {
    const context = await createContext({ syntheticOrigin: baseURL });
    const imported = await importLaboratory(context, baseURL, { schema: 'healthbank.lab.v1', records: fixture.laboratory.records.slice(0, 1) });
    const token = imported.session.csrf_token, template = imported.report.records[0];
    const page = await context.newPage(); page.setDefaultTimeout(15000);
    let closing = false, presentOfficial = true, phase = 'initial', capacityRejected = false, createResponseLost = false;
    let readyJPEG = false, sequence = 0, unsolicitedLogouts = 0;
    const calls = [], jobs = new Map(), planJobs = new Map(), routeFailures = [];
    const generationHold = { reached: Promise.withResolvers(), release: Promise.withResolvers() };
    const guard = handler => async route => {
      try { await handler(route); }
      catch (failure) {
        if (!closing) { routeFailures.push(failure); console.error('Synthetic acquisition route failed:', failure.message); }
        await route.abort().catch(() => {});
      }
    };
    const bound = route => assert.ok(route.request().headers()['x-csrf-token'] === token, 'Synthetic control request preserves the real local generation token.');
    page.on('request', request => { if (new URL(request.url()).pathname === '/api/auth/logout') unsolicitedLogouts++; });
    try {
      await page.route('**/api/session', guard(async route => {
        const response = await route.fetch(), value = await response.json();
        await route.fulfill({ response, json: presentOfficial ? { ...value, mode: 'official', authenticated: true } : value });
      }));
      await page.route('**/api/data/catalog', route => route.fulfill(json({ categories })));
      await page.route('**/api/reports?*', guard(async route => {
        bound(route); const url = new URL(route.request().url());
        assert.equal(url.searchParams.get('from'), ''); assert.equal(url.searchParams.get('to'), '');
        calls.push({ kind: 'laboratory', phase });
        const response = json({ ...imported.report, session_token: token, records: [{ ...template, source: 'SYNTHETIC_FULL_LAB' }],
          complete: false, warnings: ['合成來源範圍未知'], official_query: { mode: 'all_history', start_date: '1912-01-01', end_date: '2026-10-28', reported_count: null } });
        if (phase === 'generation') {
          generationHold.reached.resolve(); await generationHold.release.promise;
          try { await route.fulfill(response); }
          catch (failure) { if (!page.isClosed() && !route.request().failure()) throw failure; }
          return;
        }
        await route.fulfill(response);
      }));
      await page.route('**/api/supplementary?*', guard(async route => {
        bound(route); const url = new URL(route.request().url()), category = url.searchParams.get('category'), detail = url.searchParams.get('detail');
        assert.equal(url.searchParams.get('from'), ''); assert.equal(url.searchParams.get('to'), '');
        calls.push({ kind: 'source', phase, category, detail: Boolean(detail) });
        let entries;
        if (category === 'imaging') {
          if (!detail) entries = [entry('合成影像清單', { detail_id: opaque(1) })];
          else if (detail === opaque(1)) entries = [entry('合成巢狀明細', { detail_id: opaque(2),
            attachments: [{ id: opaque(10), name: '合成既有報告.txt', content_type: 'text/plain' }] })];
          else {
            assert.equal(detail, opaque(2));
            entries = [entry('合成產製資料', { document_actions: [
              { id: opaque(readyJPEG ? 103 : 3), name: '合成 JPEG 產製', kind: 'prepare_jpeg' },
              { id: opaque(readyJPEG ? 104 : 4), name: '合成 DICOM 產製', kind: 'prepare_dicom' },
            ], ...(readyJPEG ? { attachments: [{ id: opaque(11), name: '合成產製原圖.txt', content_type: 'text/plain' }] } : {}) })];
          }
        } else if (category === 'official_files') entries = detail
          ? [entry('合成已存在且無關聯的工作檔', { attachments: [{ id: opaque(12), name: '合成官方工作.txt', content_type: 'text/plain' }] })]
          : [entry('合成官方工作清單', { detail_id: opaque(6) })];
        else {
          assert.equal(category, 'exports'); assert.equal(detail, '');
          entries = [entry('合成原始匯出選項', { document_actions: [{ id: opaque(5), name: '合成 JSON 產製', kind: 'prepare_export' }] })];
        }
        await route.fulfill(json(source(category, entries)));
      }));
      await page.route('**/api/documents/prepare', guard(async route => {
        bound(route); const input = route.request().postDataJSON(); calls.push({ kind: 'prepare', phase, category: input.category, id: input.id });
        assert.ok([opaque(3), opaque(4), opaque(5)].includes(input.id), 'Polling must not authorize rotated operation handles.');
        assert.equal(calls.filter(call => call.kind === 'prepare' && call.id === input.id).length, 1, 'Each source mutation must be attempted at most once.');
        if (input.id === opaque(5)) { await route.abort('failed'); return; }
        readyJPEG = true;
        await route.fulfill(json({ status: 'requested', message: '合成官方只回覆已受理', refresh: input.id === opaque(3)
          ? { category: 'imaging', detail: opaque(2), correlation: 'same_record' }
          : { category: 'official_files', correlation: 'unavailable' } }));
      }));
      await page.route('**/api/acquisition/packages', guard(async route => {
        bound(route); assert.equal(route.request().method(), 'POST');
        const input = route.request().postDataJSON();
        calls.push({ kind: 'package', phase, input });
        assert.equal(Boolean(input.dataset), !input.external_dataset);
        if (input.dataset) {
          assert.equal(typeof input.dataset, 'object');
          assert.equal(input.dataset.schema, 'healthbank.dataset.v2');
          assert.ok(!JSON.stringify(input.dataset).includes(token));
          for (const file of input.files) assert.ok(!JSON.stringify(input.dataset).includes(file.id), 'Dataset cannot contain live source capabilities.');
        }
        if (phase === 'external' && !input.external_dataset && !capacityRejected) {
          capacityRejected = true;
          await route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ error: { code: 'package_retained_capacity', message: '合成容量拒絕；尚未建立下載包。' } }) });
          return;
        }
        const id = opaque(1000 + ++sequence), key = `${input.plan.id}:${input.plan.index}`;
        assert.ok(!planJobs.has(key), 'A lost create response must be reconciled by lookup, not another mutation.');
        const status = { id, state: 'ready', total: input.files.length, completed: 0, failed: 0, bytes: 0,
          files: input.files.map(file => ({ archive_id: file.archive_id, state: 'queued', bytes: 0 })),
          expires_at: '2200-01-01T00:00:00Z', package_bytes: 0 };
        const job = { id, input, status, phase, downloaded: false };
        jobs.set(id, job); planJobs.set(key, id);
        if (!createResponseLost || phase === 'ambiguous_resume' && calls.filter(call => call.kind === 'package' && call.phase === phase).length === 1) {
          createResponseLost = true; await route.abort('failed'); return;
        }
        await route.fulfill(json(status));
      }));
      await page.route('**/api/acquisition/packages?*', guard(async route => {
        bound(route); assert.equal(route.request().method(), 'DELETE');
        const id = new URL(route.request().url()).searchParams.get('id');
        if (!jobs.has(id)) return route.fulfill(missing());
        jobs.delete(id); await route.fulfill(json({ status: 'cancelled' }));
      }));
      await page.route('**/api/acquisition/packages/status?*', guard(async route => {
        bound(route); const query = new URL(route.request().url()).searchParams;
        const id = query.get('id') ?? planJobs.get(`${query.get('plan')}:${query.get('part')}`);
        calls.push({ kind: 'status', phase, plan_lookup: query.has('plan') });
        if (phase === 'ambiguous_resume' && query.has('plan') && calls.filter(call => call.kind === 'status' && call.phase === phase).length === 1) {
          await route.abort('failed'); return;
        }
        const job = jobs.get(id); await route.fulfill(job ? json(job.status) : missing());
      }));
      nativeDownload = (request, response) => {
        try {
          assert.equal(request.method, 'GET');
          assert.ok(request.headers.cookie, 'Native download retains the real local session cookie.');
          assert.equal(request.headers['x-csrf-token'], undefined, 'Native download uses its bounded handle and cookie, not a whole-archive fetch.');
          const job = jobs.get(new URL(request.url, baseURL).searchParams.get('id'));
          assert.ok(job && !job.downloaded, 'A native download handle must not be replayed.'); job.downloaded = true;
          const firstPartial = job.phase === 'initial' && job.input.files.length === 3;
          const files = job.input.files.map((file, index) => {
            if (firstPartial && index === 2) return { archive_id: file.archive_id, state: 'skipped', bytes: 0, code: 'budget_exceeded' };
            const bytes = Buffer.from(`合成原始檔 ${file.archive_id}\n`);
            return { archive_id: file.archive_id, state: 'completed', bytes: bytes.length, sha256: digest(bytes) };
          });
          const members = files.filter(file => file.state === 'completed').map(file => [`originals/${file.archive_id}/synthetic.txt`, `合成原始檔 ${file.archive_id}\n`]);
          if (job.input.dataset) members.unshift(['dataset.v2.json', JSON.stringify(job.input.dataset) + '\n']);
          members.push(['manifest.json', JSON.stringify({ synthetic_only: true, plan: job.input.plan, files,
            ...(job.input.external_dataset ? { external_dataset: job.input.external_dataset } : {}) }) + '\n']);
          const bytes = storedZIP(members);
          job.status = { ...job.status, state: firstPartial ? 'partial' : 'completed', files,
            completed: files.filter(file => file.state === 'completed').length, bytes: files.reduce((sum, file) => sum + file.bytes, 0),
            package_sha256: digest(bytes), package_bytes: bytes.length };
          calls.push({ kind: 'download', phase, files: job.input.files.map(file => file.archive_id), hash: digest(bytes) });
          response.writeHead(200, { 'Content-Type': 'application/zip', 'Content-Length': bytes.length, 'Cache-Control': 'no-store',
            'Content-Disposition': `attachment; filename="healthbank-acquisition-${job.input.plan.id}-part${job.input.plan.index}.zip"` });
          response.end(bytes);
        } catch (failure) {
          routeFailures.push(failure); response.writeHead(500); response.end();
        }
      };
      await page.goto(baseURL, { waitUntil: 'networkidle' });
      await page.getByRole('button', { name: '醫療紀錄', exact: true }).click();
      const full = page.getByRole('region', { name: '完整取得與原始檔保存', exact: true });
      await full.getByRole('button', { name: '完整取得並保存原始檔', exact: true }).click();
      const zip = full.getByRole('link', { name: '下載第 1／1 部分 ZIP', exact: true });
      await zip.waitFor({ timeout: 90000 });
      assert.equal(calls.filter(call => call.kind === 'prepare').length, 3);
      assert.equal(calls.filter(call => call.kind === 'source' && call.category === 'imaging' && call.detail).length, 8, 'Initial nested detail reads and six bounded parent polls must all execute.');
      assert.equal(calls.filter(call => call.kind === 'package').length, 1);
      assert.ok(calls.some(call => call.kind === 'status' && call.plan_lookup));
      const firstInput = calls.find(call => call.kind === 'package').input;
      assert.equal(firstInput.files.length, 3);
      assert.equal(firstInput.acquisition.scope, 'all_available');
      assert.ok(firstInput.acquisition.statuses.some(item => item.code === 'preparation_outcome_unknown'));
      assert.ok(firstInput.acquisition.statuses.some(item => item.code === 'source_did_not_provide_job_correlation'));
      const acknowledgement = full.getByRole('checkbox', { name: '我已確認第 1 部分 ZIP 已保存', exact: true });
      assert.equal(await acknowledgement.isEnabled(), false, 'A prepared package is not a downloaded file.');
      const firstZIP = await saveDownload(page, zip, path.join(evidenceDir, 'synthetic-acquisition-partial.zip'));
      await acknowledgement.check();
      await full.getByRole('button', { name: '確認最後一部分已保存', exact: true }).click();
      await full.getByRole('button', { name: '重新下載未保存的原始檔（不重送產製）', exact: true }).click();
      await zip.waitFor();
      const secondInput = calls.filter(call => call.kind === 'package')[1].input;
      assert.notEqual(secondInput.plan.id, firstInput.plan.id);
      assert.deepEqual(secondInput.dataset, firstInput.dataset, 'File-only retries must keep the exact initial dataset projection.');
      assert.deepEqual(secondInput.files, firstInput.files.slice(2));
      assert.equal(calls.filter(call => call.kind === 'prepare').length, 3);
      const secondZIP = await saveDownload(page, zip, path.join(evidenceDir, 'synthetic-acquisition-remainder.zip'));
      await acknowledgement.check();
      await full.getByRole('button', { name: '確認最後一部分已保存', exact: true }).click();
      const indexBytes = await saveDownload(page, full.getByRole('button', { name: /^下載整組檔案索引與校驗碼/ }), path.join(evidenceDir, 'synthetic-acquisition-index.json'));
      const index = JSON.parse(indexBytes);
      assert.equal(index.all_known_originals_saved, true); assert.equal(index.all_parts_saved_acknowledged, true);
      assert.equal(index.all_required_files_saved_acknowledged, true);
      assert.ok(index.parts.every(item => item.dataset_included));
      assert.equal(index.original_files_total, 3); assert.equal(index.originals.length, 3); assert.equal(index.parts.length, 2);
      assert.deepEqual(index.parts.map(item => item.sha256), [digest(firstZIP), digest(secondZIP)]);
      assert.ok(index.originals.every(file => file.state === 'completed' && file.sha256 && file.part_name));
      assert.equal(index.dataset.sha256, digest(Buffer.from(JSON.stringify(firstInput.dataset) + '\n')));
      assert.ok(!indexBytes.includes(Buffer.from(token)), 'Collection index must not export the live session token.');
      for (const file of firstInput.files) assert.ok(!indexBytes.includes(Buffer.from(file.id)), 'Collection index must not export document capabilities.');
      await page.setViewportSize({ width: 390, height: 900 });
      await page.evaluate(() => { globalThis.document.documentElement.style.setProperty('--font-scale', '2'); });
      assert.ok(await full.evaluate(element => element.scrollWidth <= element.clientWidth + 1), 'Full acquisition controls must wrap at 390 px and 200% font scale.');
      await full.getByRole('heading', { name: '完整取得與原始檔保存', exact: true }).scrollIntoViewIfNeeded();
      await page.screenshot({ path: path.join(evidenceDir, 'synthetic-acquisition-mobile.png') });
      await full.getByRole('button', { name: /^下載整組檔案索引與校驗碼/ }).scrollIntoViewIfNeeded();
      await page.screenshot({ path: path.join(evidenceDir, 'synthetic-acquisition-mobile-saved-files.png') });
      await page.setViewportSize({ width: 1280, height: 900 });
      await page.evaluate(() => { globalThis.document.documentElement.style.removeProperty('--font-scale'); });
      await full.getByRole('button', { name: '結束完整取得計畫', exact: true }).click();

      phase = 'external'; readyJPEG = false;
      await full.getByRole('checkbox', { name: /^一併要求官方產製/ }).uncheck();
      await full.getByRole('button', { name: '完整取得並保存原始檔', exact: true }).click();
      const externalDownload = full.getByRole('button', { name: '下載完整資料 JSON', exact: true });
      await externalDownload.waitFor();
      const externalBytes = await saveDownload(page, externalDownload, path.join(evidenceDir, 'synthetic-acquisition-external-dataset.json'));
      assert.equal((await importDataset(externalBytes)).snapshot.reports.imaging.entries.length, 1);
      const prepare = full.getByRole('button', { name: '準備第 1 部分原始檔包', exact: true });
      assert.equal(await prepare.isEnabled(), false, 'Device save acknowledgement is required before an external dataset is referenced.');
      const externalAcknowledgement = full.getByRole('checkbox', { name: '我已確認完整資料 JSON 已保存，會與原始檔包一起保留', exact: true });
      await externalAcknowledgement.check();
      await prepare.click(); await zip.waitFor();
      const externalInput = calls.filter(call => call.kind === 'package').at(-1).input;
      assert.ok(!Object.hasOwn(externalInput, 'dataset'));
      assert.deepEqual(externalInput.external_dataset, { name: 'healthbank.dataset.v2.json', size: externalBytes.length, sha256: digest(externalBytes) });
      await externalAcknowledgement.uncheck();
      assert.equal(await zip.getAttribute('aria-disabled'), 'true');
      await zip.click({ force: true });
      assert.equal(calls.filter(call => call.kind === 'download').length, 2, 'Revoking the external dataset acknowledgement blocks native package download.');
      await externalAcknowledgement.check();
      await saveDownload(page, zip, path.join(evidenceDir, 'synthetic-acquisition-external-originals.zip'));
      await acknowledgement.check();
      const finalConfirmation = full.getByRole('button', { name: '確認最後一部分已保存', exact: true });
      await externalAcknowledgement.uncheck();
      assert.equal(await finalConfirmation.isEnabled(), false, 'The complete collection cannot be acknowledged while its external dataset is unconfirmed.');
      await externalAcknowledgement.check(); await finalConfirmation.click();
      assert.equal(calls.filter(call => call.kind === 'prepare').length, 3, 'Unselected preparation remains disabled in a later acquisition.');
      await full.getByRole('button', { name: '結束完整取得計畫', exact: true }).click();

      phase = 'ambiguous_resume';
      await full.getByRole('button', { name: '完整取得並保存原始檔', exact: true }).click();
      await full.getByRole('alert').waitFor();
      const originalPlan = calls.find(call => call.kind === 'package' && call.phase === phase).input.plan.id;
      const originalJob = planJobs.get(`${originalPlan}:1`);
      assert.ok(jobs.has(originalJob), 'The synthetic lost response leaves a real fixture receipt to reconcile.');
      await full.getByRole('button', { name: '接續未完成項目與更新產製狀態', exact: true }).click();
      await zip.waitFor();
      const resumedPlans = calls.filter(call => call.kind === 'package' && call.phase === phase);
      assert.equal(resumedPlans.length, 2);
      assert.notEqual(resumedPlans[1].input.plan.id, originalPlan, 'Source resume must never associate a newly exported dataset with the ambiguous previous package body.');
      assert.equal(jobs.has(originalJob), false, 'The previous ambiguous receipt is retired before rebuilding sources.');
      assert.equal(calls.filter(call => call.kind === 'prepare').length, 3);
      await full.getByRole('button', { name: '結束完整取得計畫', exact: true }).click();

      phase = 'generation';
      await full.getByRole('button', { name: '完整取得並保存原始檔', exact: true }).click();
      const timer = setTimeout(() => generationHold.reached.reject(new Error('Expected held synthetic laboratory request.')), 15000);
      try { await generationHold.reached.promise; } finally { clearTimeout(timer); }
      const sourcesBefore = calls.filter(call => call.kind === 'source').length;
      const logout = await context.request.post(`${baseURL}/api/auth/logout`, { headers: { Origin: baseURL, 'X-CSRF-Token': token }, data: {} });
      assert.ok(logout.ok()); presentOfficial = false;
      const anonymous = await session(context, baseURL);
      assert.ok(anonymous.csrf_token !== token && anonymous.mode === 'anonymous');
      generationHold.release.resolve();
      await full.waitFor({ state: 'hidden' });
      assert.equal(calls.filter(call => call.kind === 'source').length, sourcesBefore, 'Late laboratory data after generation change cannot start source acquisition.');
      assert.equal(unsolicitedLogouts, 0, 'The stale component must not replay logout into the new generation.');
      assert.deepEqual(routeFailures, []);
      return { fixture_only: true, actual_official_login: false, source_preparations: 3, ambiguous_mutations_replayed: 0,
        bounded_parent_polls: 6, native_zip_downloads: 3, partial_package_retry_only_remaining: true,
        exact_dataset_hash_preserved: true, external_dataset_saved_acknowledgement: true,
        ambiguous_source_resume_plan_rotated: true, late_generation_source_requests: 0 };
    } catch (failure) {
      if (!page.isClosed()) await page.screenshot({ path: path.join(evidenceDir, 'synthetic-full-acquisition-failure.png') }).catch(() => {});
      throw failure;
    } finally {
      closing = true; generationHold.release.resolve(); await context.close();
    }
  } finally {
    fixtureServer.closeAllConnections();
    await new Promise((resolve, reject) => fixtureServer.close(error => error ? reject(error) : resolve()));
  }
}
