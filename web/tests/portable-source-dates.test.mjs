// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import assert from 'node:assert/strict';
import test from 'node:test';
import { portableRows, portableReportJSON, portableReportXML } from '../src/portable-report.ts';

test('portable reports retain source ROC dates, timestamps and unrecognized date text', () => {
  for (const rawDate of ['115/10/28 08:31:12', '2026-10-28T09:32:13+08:00', '日期待確認', '']) {
    const row = { exam_date: '2026-10-28', visit_date: '2025-01-03', raw_check_date: rawDate,
      raw_visit_date: '114/01/03 11:11:11', order_code: 'SYNTHETIC', order_name: '合成',
      item_name: '合成資料', result: '>400.000000000000000001', unit: 'mg/dL', reference_text: '[][200]',
      institution: 'SYNTHETIC', source: 'synthetic' };
    const [portable] = portableRows([row]);
    assert.equal(portable.check_date, rawDate);
    assert.equal(portable.visit_date, row.raw_visit_date);
    assert.equal(JSON.parse(portableReportJSON([row])).records[0].check_date, rawDate);
    assert.ok(portableReportXML([row]).includes(`<check_date>${rawDate}</check_date>`));
    const legacy = { ...row };
    delete legacy.raw_check_date;
    delete legacy.raw_visit_date;
    assert.equal(portableRows([legacy])[0].check_date, legacy.exam_date);
    assert.equal(portableRows([legacy])[0].visit_date, legacy.visit_date);
  }
});
