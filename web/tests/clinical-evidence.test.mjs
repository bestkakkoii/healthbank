// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { egfrCreatinine2021, scenarioInterval, COMPARISON_ANALYTES, CLINICAL_SOURCES } from '../src/clinical-comparison.ts';
import { assessLipids } from '../src/lipid-analysis.ts';

const repository = new URL('../../', import.meta.url);
const evidence = JSON.parse(readFileSync(new URL('internal/lab/evidence-registry.json', repository), 'utf8'));

test('clinical provenance hashes bind the reviewed implementation inputs', () => {
  assert.equal(evidence.schema_version, 1);
  assert.equal(evidence.catalog_sha256, createHash('sha256').update(readFileSync(new URL('internal/lab/catalog.json', repository))).digest('hex'));
  for (const [path, digest] of Object.entries(evidence.implementation_files)) {
    assert.equal(createHash('sha256').update(readFileSync(new URL(path, repository))).digest('hex'), digest, `Clinical implementation changed without evidence review: ${path}`);
  }
});

test('versioned named templates preserve reviewed source loci and independent age boundaries', () => {
  assert.deepEqual(evidence.reference_templates.map(item => item.id).sort(), Object.keys(COMPARISON_ANALYTES).sort());
  for (const item of evidence.reference_templates) {
    assert.equal(item.unit, COMPARISON_ANALYTES[item.id].unit);
    assert.equal(evidence.sources[item.source_ids[0]].url, CLINICAL_SOURCES[item.id]);
    assert.ok(item.population && item.exclusions.length);
    for (const vector of item.test_vectors) {
      const actual = scenarioInterval(item.id, { age: vector.age, sex: vector.sex });
      if (vector.unavailable) assert.equal(actual, null);
      else {
        assert.equal(actual.low, vector.low);
        assert.equal(actual.high, vector.high);
        assert.equal(actual.unit, item.unit);
      }
    }
  }
});

test('versioned CKD-EPI and Sampson evidence vectors execute independently of source observations', () => {
  for (const item of evidence.calculations) {
    for (const vector of item.test_vectors) {
      let actual;
      if (item.id === 'ckd_epi_2021_creatinine') actual = egfrCreatinine2021(Number(vector.creatinine_mg_dl), { age: vector.age, sex: vector.sex }, true).value;
      else if (item.id === 'sampson_nih_2020') {
        const record = (code, name, raw) => ({ id: code, order_code: code, item_name: name, result: raw, numeric_value: Number(raw), value_kind: 'numeric', unit: 'mg/dL', exam_date: '2000-01-01', visit_date: '2000-01-01', institution: '合成證據向量' });
        actual = assessLipids(record('09001C', 'Cholesterol', vector.tc), record('09043C', 'HDL-Cholesterol', vector.hdl), record('09004C', 'Triglyceride', vector.tg), true, true).sampson;
      } else assert.fail(`Missing evidence vector runner: ${item.id}`);
      assert.notEqual(actual, null);
      assert.ok(Math.abs(actual - Number(vector.expected)) < 1e-10);
    }
  }
});
