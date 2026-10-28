// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
// Generated synthetic examples only. Never replace these with patient exports.
const fetchedAt = '2026-10-28T00:00:00Z';
const date = (year, index) => new Date(Date.UTC(year, 0, 1 + index)).toISOString().slice(0, 10);
const padded = index => String(index).padStart(4, '0');

export function syntheticDataset() {
  const sourceReport = entries => ({ category: 'medicine', entries, coverage: 'SYNTHETIC_EXCLUDED_MEDICINE_COVERAGE：合成來源，未取得獨立總數', fetched_at: fetchedAt, complete: false });
  const parent = index => ({
    date: date(2024, index), institution: '合成院所', title: `合成回診 ${String(index).padStart(3, '0')}`,
    source: `SYNTHETIC_VISIT_${padded(index)}`,
    fields: [{ name: '原始說明', value: index === 0 ? '合成文字 <script>globalThis.syntheticInjection=1</script> & 非真實病歷' : '合成測試，非任何人的醫療資料' }],
    ...(index === 25 ? { detail_id: 'archive_1' } : {}),
  });
  return {
    schema: 'healthbank.dataset.v1', exported_at: fetchedAt,
    laboratory: {
      schema: 'healthbank.lab.v1', fetched_at: fetchedAt, complete: false,
      warnings: ['合成資料，非真實個人報告', 'SYNTHETIC_EXCLUDED_GLOBAL_WARNING：合成的完整資料集提醒，不可隨部分分享範圍一併透露。'],
      records: Array.from({ length: 500 }, (_, index) => ({
        check_date: index === 0 ? '104/01/01' : index === 1 ? '2015-01-01T08:30:00+08:00' : date(2015, index),
        order_code: '09004C', order_name: index === 0 ? '合成醫令 <img src=x onerror="globalThis.syntheticInjection=1">' : '合成醫令',
        item_name: 'Triglyceride', result: index < 2 ? '100.00' : String(100 + index % 80),
        unit: 'mg/dL', reference: '[0][150]', institution: '合成院所', visit_date: index === 0 ? '103/12/31' : '',
        source: `SYNTHETIC_LAB_${padded(index)}`, official_flag: '',
      })),
    },
    catalog: [{ id: 'medicine', label: '合成用藥來源', group: '合成測試', range_mode: 'date_search', has_details: true }],
    data: {
      reports: { medicine: sourceReport(Array.from({ length: 120 }, (_, index) => parent(index))) },
      details: { medicine: { archive_1: sourceReport([{ ...parent(25), title: '合成明細 025', source: 'SYNTHETIC_DETAIL_0025', detail_id: undefined }]) } },
      states: { medicine: { status: 'ready', detail_errors: {}, query: { from: '', to: '' } } },
      query: { from: '', to: '' },
    },
    attachments: [],
  };
}

export function replacementLaboratory() {
  return { schema: 'healthbank.lab.v1', records: [{
    check_date: '2026-01-02', order_code: '09002C', order_name: '合成第二工作階段', item_name: 'Glucose',
    result: '91.234', unit: 'mg/dL', reference: '[70][100]', institution: '合成不同來源', visit_date: '',
    source: 'SYNTHETIC_REPLACEMENT_SESSION', official_flag: '',
  }] };
}
