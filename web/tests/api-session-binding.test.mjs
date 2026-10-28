// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import ts from '../node_modules/typescript/lib/typescript.js';

// The API error class uses TypeScript parameter properties, which Node's
// erase-only loader does not support. Compile with the project's pinned TS.
const source = await readFile(new URL('../src/api.ts', import.meta.url), 'utf8');
const compiled = ts.transpileModule(source, {
  compilerOptions: { target: ts.ScriptTarget.ESNext, module: ts.ModuleKind.ESNext },
}).outputText;
const { api, ApiError, isSessionInvalidation, setCsrfToken } = await import(
  `data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`
);
const tokenA = 'a'.repeat(64);
const tokenB = 'b'.repeat(64);
const emptyReport = { records: [], fetched_at: '', complete: false, warnings: [] };

test('stateful reads and downloaded attachment bytes stay bound to the page generation', async t => {
  const calls = [];
  t.mock.method(globalThis, 'fetch', async (path, options) => {
    calls.push({ path, token: new Headers(options.headers).get('X-CSRF-Token'), options });
    return Response.json({ ...emptyReport, session_token: tokenA });
  });
  t.after(() => setCsrfToken());
  setCsrfToken(tokenA);
  await api.currentReports();
  await api.catalog();
  await api.supplementary('medicine', '', '', '', new AbortController().signal);
  await api.document('medicine', 'c'.repeat(48), new AbortController().signal);
  await api.session();
  assert.deepEqual(calls.map(call => call.token), [tokenA, tokenA, tokenA, tokenA, null]);
  for (const call of calls) {
    assert.equal(call.options.credentials, 'same-origin');
    assert.equal(call.options.cache, 'no-store');
  }
});

test('all laboratory response paths require an explicit valid generation proof', async t => {
  let responseToken;
  t.mock.method(globalThis, 'fetch', async () => Response.json({ ...emptyReport, session_token: responseToken }));
  const operations = [
    () => api.reports('', ''),
    () => api.currentReports(),
    () => api.demo(),
    () => api.importReports(new File(['{"records":[]}'], 'synthetic.json')),
  ];
  for (responseToken of [undefined, '', tokenA.slice(1), 'G'.repeat(64), 123]) {
    for (const operation of operations) {
      await assert.rejects(operation(), error => error instanceof ApiError &&
        error.code === 'invalid_report_session' && isSessionInvalidation(error));
    }
  }
  responseToken = tokenB;
  for (const operation of operations) assert.equal((await operation()).session_token, tokenB);
});

test('a changed browser cookie rejects an old page read without treating ordinary conflicts as logout', async t => {
  t.mock.method(globalThis, 'fetch', async (_path, options) => {
    if (new Headers(options.headers).get('X-CSRF-Token') !== tokenB)
      return Response.json({ error: { code: 'session_changed', message: '合成工作階段已更換' } }, { status: 409 });
    return Response.json({ ...emptyReport, session_token: tokenB });
  });
  t.after(() => setCsrfToken());
  setCsrfToken(tokenA);
  await assert.rejects(api.currentReports(), error => error instanceof ApiError && isSessionInvalidation(error));
  setCsrfToken(tokenB);
  assert.equal((await api.currentReports()).session_token, tokenB);
  assert.equal(isSessionInvalidation(new ApiError('合成設定版本衝突', 'settings_conflict', 409)), false);
  assert.equal(isSessionInvalidation(new ApiError('合成來源限制', 'origin_rejected', 403)), false);
});
