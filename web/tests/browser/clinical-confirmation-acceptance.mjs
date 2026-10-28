// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import assert from 'node:assert/strict';
import path from 'node:path';

// Uses real Go normalization/session generation and a narrowly mocked official
// refresh boundary. No credentials, personal API or real observations are used.
export async function runClinicalConfirmationAcceptance({ createContext, baseURL, evidenceDir, importLaboratory }) {
  const context = await createContext(), page = await context.newPage();
  page.setDefaultTimeout(20000);
  const raw = (result, check_date) => ({ check_date, visit_date: check_date,
    item_name: 'Creatinine', order_code: '09015C', order_name: '合成肌酸酐測試', result,
    unit: 'mg/dL', reference: '[0.5][1.5]', institution: '合成確認測試院所', source: 'SYNTHETIC_ONLY', official_flag: '' });
  const failures = [];
  let closing = false, refreshes = 0;
  const guard = handler => async route => {
    try { await handler(route); }
    catch (failure) { if (!closing) failures.push(failure); await route.abort().catch(() => {}); }
  };
  try {
    const imported = await importLaboratory(context, baseURL, { schema: 'healthbank.lab.v2', records: [raw('1.0000000001', '109/02/03'), raw('1.4000000001', '110/03/04')] });
    const [first, replacement] = imported.report.records;
    assert.ok(first && replacement && first.id !== replacement.id, 'Different complete source observations must have different normalized IDs.');
    const token = imported.session.csrf_token;
    const report = record => ({ ...imported.report, records: [record], session_token: token, complete: false });
    await context.route('**/api/session', guard(async route => {
      const response = await route.fetch(), session = await response.json();
      assert.ok(session.csrf_token === token, 'The synthetic browser case must keep the same actual local session generation.');
      await route.fulfill({ response, json: { ...session, mode: 'official', authenticated: true, profile_key: 'c'.repeat(64) } });
    }));
    await context.route('**/api/reports/current', guard(async route => {
      assert.ok(route.request().headers()['x-csrf-token'] === token, 'Current reads must retain the actual synthetic session token.');
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(report(first)) });
    }));
    await context.route('**/api/reports/cache', guard(async route => {
      assert.equal(route.request().method(), 'DELETE', 'This fixture never consents to device caching or restores cached data.');
      assert.ok(route.request().headers()['x-csrf-token'] === token, 'Cache initialization must preserve the actual synthetic session.');
      const input = route.request().postDataJSON();
      assert.equal(input.expected_profile_key, 'c'.repeat(64));
      assert.equal(input.scope, 'cache_only', 'Cache initialization must not clear the fresh source report.');
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ status: 'cleared' }) });
    }));
    await context.route('**/api/reports?*', guard(async route => {
      assert.ok(route.request().headers()['x-csrf-token'] === token, 'Refresh must not rotate or replace the synthetic session.');
      refreshes++;
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(report(replacement)) });
    }));
    await page.goto(baseURL, { waitUntil: 'networkidle' });
    await page.getByRole('button', { name: '年度比較', exact: true }).click();
    const clinical = page.locator('.clinical-comparison');
    const disclosure = page.locator('details.compact-disclosure').filter({ has: clinical });
    if (!await disclosure.evaluate(element => element.open)) await disclosure.locator(':scope > summary').click();
    const person = clinical.locator('.scenario-fields').first();
    await person.getByRole('spinbutton').fill('40');
    await person.getByRole('combobox').selectOption('male');
    const confirmation = clinical.getByRole('checkbox', { name: /另作 CKD-EPI 2021 情境估算/ });
    const estimated = clinical.locator('.scenario-result').first().locator('.scenario-estimate b');
    assert.equal(await estimated.textContent(), 'eGFR 未計算');
    await confirmation.check();
    assert.match(await estimated.textContent(), /情境 eGFR 約/);
    await clinical.evaluate(element => { element.dataset.syntheticConfirmationInstance = 'same-mounted-instance'; });

    const response = page.waitForResponse(value => new URL(value.url()).pathname === '/api/reports');
    await page.getByRole('button', { name: '取得全部歷史檢驗', exact: true }).click();
    assert.ok((await response).ok());
    await clinical.getByText(`原始結果：${replacement.result} mg/dL`, { exact: false }).waitFor();
    assert.equal(await clinical.evaluate(element => element.dataset.syntheticConfirmationInstance), 'same-mounted-instance', 'The comparison component must remain mounted for this regression.');
    assert.equal(await confirmation.isChecked(), false, 'Confirmation must not silently transfer to the replacement observation.');
    assert.equal(await estimated.textContent(), 'eGFR 未計算');
    await confirmation.check();
    assert.match(await estimated.textContent(), /情境 eGFR 約/, 'The explicitly reconfirmed replacement can produce its own scenario estimate.');
    assert.equal(refreshes, 1);
    if (failures.length) throw failures[0];
    await page.screenshot({ path: path.join(evidenceDir, 'synthetic-record-bound-clinical-confirmation.png') });
    return { synthetic_only: true, official_login_tested: false, real_go_normalization: true,
      same_session_generation: true, component_remained_mounted: true,
      replacement_required_reconfirmation: true, reconfirmed_estimate_available: true };
  } catch (failure) {
    if (!page.isClosed()) await page.screenshot({ path: path.join(evidenceDir, 'synthetic-clinical-confirmation-failure.png') }).catch(() => {});
    throw failure;
  } finally { closing = true; await context.close(); }
}
