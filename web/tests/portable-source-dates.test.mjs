// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import assert from 'node:assert/strict';
import test from 'node:test';
import { portableRows, portableReportJSON, portableReportXML } from '../src/portable-report.ts';
import { exportDataset, importDataset } from '../src/health-dataset.ts';
import { emptyHealthData } from '../src/health-data.ts';
import { toCsv } from '../src/model.ts';
import { createPrintReport, DEFAULT_PRINT_OPTIONS, renderPrintReport } from '../src/print-report.ts';

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

test('lab v2 retains separate raw upload date and source name across JSON, XML, CSV, datasets and both print layouts', async () => {
  const row = { id: 'synthetic-upload', item_id: 'synthetic', configuration_key: 'synthetic', display_name: '合成檢驗', category: 'other',
    exam_date: '2024-02-03', raw_check_date: '113/02/03', visit_date: '2024-01-19', raw_visit_date: '113/01/19', upload_date: '2024-03-20',
    raw_upload_date: '113/03/20 09:41:22', source_name: '合成上傳來源名稱', source: 'SYNTHETIC_CODE', item_name: '合成項目', order_code: 'SYNTHETIC',
    order_name: '合成醫令', result: '17.000000000000000001', unit: 'U/L', reference_text: '', institution: '合成院所', official_flag: '',
    interpretation: '', rule_source: '', reference_status: 'unknown', level: null, numeric_value: 17, value_kind: 'number' };
  const report = { records: [row, { ...row }], complete: false, fetched_at: '', warnings: [] };
  const portable = JSON.parse(portableReportJSON(report.records));
  assert.equal(portable.schema, 'healthbank.lab.v2');
  assert.equal(portable.records[0].upload_date, row.raw_upload_date);
  assert.equal(portable.records[0].source_name, row.source_name);
  assert.equal(portable.records[0].check_date, row.raw_check_date);
  assert.ok(portableReportXML(report.records).includes(`<upload_date>${row.raw_upload_date}</upload_date>`));
  assert.ok(toCsv(report.records).includes(row.raw_upload_date)); assert.ok(toCsv(report.records).includes(row.source_name));
  const content = await exportDataset(report, emptyHealthData(), [], []), bytes = new TextEncoder().encode(content);
  const restored = await importDataset(bytes), restoredRows = JSON.parse(restored.laboratoryJSON).records;
  assert.deepEqual(restoredRows, portable.records);
  const invalidV1 = JSON.parse(content); invalidV1.laboratory.schema = 'healthbank.lab.v1';
  await assert.rejects(importDataset(new TextEncoder().encode(JSON.stringify(invalidV1))));
  for (const layout of ['records', 'comparison']) {
    const html = renderPrintReport(createPrintReport({ report, snapshot: emptyHealthData(), categories: [], selectedLaboratory: [], mode: 'import',
      version: 'synthetic', generatedAt: '2026-10-28T00:00:00Z', options: { ...DEFAULT_PRINT_OPTIONS, layout } }));
    assert.equal(html.split(row.raw_upload_date).length - 1, 2); assert.equal(html.split(row.source_name).length - 1, 2);
  }
  const explicitEmpty = { ...row, raw_upload_date: '', upload_date: '', source_name: '' };
  assert.equal(portableRows([explicitEmpty])[0].upload_date, ''); assert.equal(portableRows([explicitEmpty])[0].source_name, '');
});
