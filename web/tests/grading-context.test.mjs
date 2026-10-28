// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import assert from 'node:assert/strict';
import test from 'node:test';
import { gradingContextLabel, gradingContextText, recordGradingContexts, summarizeGradingContexts } from '../src/grading-context.ts';

const profile = { kind: 'profile', rule_id: 'adult_triglycerides', label: '成人空腹 TG 範本（條件未確認）', note: '年齡與空腹狀態未確認。' };
const personal = { kind: 'personal', rule_id: 'personal', label: '個人顯示基準', note: '未驗證為個人治療目標。' };
const alert = { ...profile, kind: 'independent_alert', label: '成人空腹 TG 範本（條件未確認）・獨立提醒' };
const row = extra => ({ id: 'synthetic-row', result: '1000.000000000000001', level: 5, official_flag: 'SYNTHETIC', ...extra });

test('actual grading contexts remain distinct from source flags and numerical observations', () => {
  const record = row({ grading_contexts: [personal, alert] });
  const before = structuredClone(record);
  const contexts = recordGradingContexts(record);
  assert.deepEqual(contexts.map(context => context.label), [personal.label, alert.label]);
  assert.match(gradingContextLabel(record), /個人顯示基準.*獨立提醒/);
  const exported = gradingContextText(record);
  assert.ok(exported.includes(personal.note));
  assert.ok(exported.includes(alert.note));
  assert.deepEqual(record, before);
  assert.notEqual(contexts, record.grading_contexts);
});

test('legacy color has explicit unknown provenance and never guesses from item or scenario metadata', () => {
  const record = row({ item_id: 'serum_triglycerides', rule_source: 'nhlbi_triglycerides', scenario_age: 9 });
  assert.equal(gradingContextLabel(record), '分級依據未附註');
  assert.doesNotMatch(gradingContextText(record), /成人空腹/);
  assert.deepEqual(recordGradingContexts(row({ level: null })), []);
  assert.equal(gradingContextText(row({ level: null })), '');
});

test('ambiguous profile results keep their explanatory context without inventing a numeric level', () => {
  const record = row({ result: '>=300', level: null, grading_contexts: [{ ...profile, rule_id: 'acr_categories', label: 'KDIGO A 範本（條件未確認）' }] });
  assert.equal(recordGradingContexts(record).length, 1);
  assert.match(gradingContextText(record), /KDIGO A/);
  assert.equal(record.level, null);
  assert.equal(record.result, '>=300');
});

test('section summaries consolidate exact explanatory metadata while retaining all source occurrences', () => {
  const records = [row({ grading_contexts: [profile] }), row({ grading_contexts: [profile] }), row({ id: 'other', grading_contexts: [personal, alert] })];
  const before = structuredClone(records);
  assert.deepEqual(summarizeGradingContexts(records).map(context => context.label), [profile.label, personal.label, alert.label]);
  assert.deepEqual(records, before);
  assert.equal(records.length, 3);
});
