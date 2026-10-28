// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import test from 'node:test';
import { acquisitionSummary, capturePreparations, datasetDigest, nextPreparationPolls, packageFileGroups, preparationRefreshKey, preparationRefreshTargets, preparationTasks,
  protectPolledPreparations, recordSavedOriginals, remainingOriginals, retryableOriginals,
  selectedSnapshot, submitPreparations, waitForAcquisition } from '../src/full-acquisition.ts';
import { emptyHealthData } from '../src/health-data.ts';

const id = digit => digit.repeat(48);
const source = (entries = []) => ({ category: 'imaging', entries, fetched_at: '2026-10-28T00:00:00Z', coverage: '合成來源，總數未知', complete: false });
const row = actions => ({ date: '115/10/28', institution: '合成院所', title: '合成影像', source: 'SYNTHETIC', fields: [], document_actions: actions });
const action = (digit, kind = 'prepare_jpeg') => ({ id: id(digit), kind, name: '合成產製要求' });
function fixture() {
  return { ...emptyHealthData(), reports: { imaging: source([{ ...row([]), detail_id: id('a') }]), hospital: source([row([action('f')])]) },
    details: { imaging: { [id('a')]: source([row([action('b'), action('c', 'prepare_dicom')]), row([action('b')])]) } },
    states: { imaging: { status: 'ready', detail_errors: {} }, hospital: { status: 'failed', detail_errors: {}, error: '合成更新失敗，保留舊內容' } } };
}
const classifyFailure = error => ({ message: error.message, expired: error.message === 'expired', retryable: true });
const requested = category => ({ status: 'requested', message: '合成來源僅回覆已受理', refresh: category === 'imaging'
  ? { category, detail: id('a'), correlation: 'same_record' } : { category: 'official_files', correlation: 'unavailable' } });

test('initial preparation capture preserves distinct operations but ignores repeated handles and failed stale sources', () => {
  const tasks = preparationTasks(fixture(), ['imaging', 'hospital']);
  assert.equal(tasks.length, 2);
  assert.deepEqual(tasks.map(task => task.id), [id('b'), id('c')]);
  assert.ok(tasks.every(task => task.parent === id('a') && task.state === 'queued'));
  const invalid = fixture(); invalid.details.imaging[id('a')].entries[0].document_actions[0].id = 'archive_1';
  assert.throws(() => preparationTasks(invalid, ['imaging']), /不屬於目前官方工作階段/);
});

test('resume captures previously missing details without replaying refreshed operation or nested handles', () => {
  const snapshot = fixture();
  snapshot.reports.imaging.entries.push({ ...row([]), detail_id: id('d') });
  const first = capturePreparations(snapshot, ['imaging'], [], new Set());
  snapshot.details.imaging[id('d')] = source([row([action('e')])]);
  const resumed = capturePreparations(snapshot, ['imaging'], first.tasks, first.captured);
  assert.deepEqual(resumed.tasks.map(task => task.id), [id('b'), id('c'), id('e')]);
  snapshot.details.imaging[id('a')] = source([{ ...row([action('1')]), detail_id: id('2') }]);
  const protectedReports = protectPolledPreparations(snapshot, 'imaging', id('a'), resumed.captured);
  snapshot.details.imaging[id('2')] = source([row([action('3')])]);
  const afterPolling = capturePreparations(snapshot, ['imaging'], resumed.tasks,
    protectPolledPreparations(snapshot, 'imaging', id('a'), protectedReports));
  assert.deepEqual(afterPolling.tasks, resumed.tasks);
  assert.ok(snapshot.details.imaging[id('2')].entries[0].document_actions.length, 'uncorrelated refreshed actions remain in the raw source');
});

test('a lost or cancelled preparation is never retried when the same plan resumes', async () => {
  const controller = new AbortController();
  const tasks = preparationTasks(fixture(), ['imaging']);
  const calls = [];
  const first = await submitPreparations({ tasks, signal: controller.signal, classifyFailure, onUpdate() {},
    submit: async task => { calls.push(task.id); controller.abort(); throw new DOMException('synthetic cancellation after dispatch', 'AbortError'); } });
  assert.equal(first[0].state, 'indeterminate');
  assert.equal(first[1].state, 'queued');
  const second = await submitPreparations({ tasks: first, signal: new AbortController().signal, classifyFailure, onUpdate() {},
    submit: async task => { calls.push(task.id); return requested('official_files'); } });
  assert.deepEqual(calls, [id('b'), id('c')]);
  assert.equal(second[0].state, 'indeterminate');
  assert.equal(second[1].state, 'correlation_unavailable');
});

test('retryable transport errors do not replay mutations and uncorrelated success never becomes a matched job', async () => {
  const calls = [];
  const result = await submitPreparations({ tasks: preparationTasks(fixture(), ['imaging']), signal: new AbortController().signal,
    classifyFailure, onUpdate() {}, submit: async task => {
      calls.push(task.id);
      if (task.id === id('b')) throw new TypeError('synthetic lost response');
      return requested('official_files');
    } });
  assert.deepEqual(calls, [id('b'), id('c')]);
  assert.deepEqual(result.map(task => task.state), ['indeterminate', 'correlation_unavailable']);
  assert.deepEqual(preparationRefreshTargets(result), [
    { category: 'imaging', detail: id('a'), correlation: 'same_record' },
    { category: 'official_files', correlation: 'unavailable' },
  ]);
  const expired = new Error('expired');
  await assert.rejects(submitPreparations({ tasks: preparationTasks(fixture(), ['imaging']), signal: new AbortController().signal,
    classifyFailure, onUpdate() {}, submit: async () => { throw expired; } }), error => error === expired);
});

test('a parent generation abort without the local signal abort stops subsequent preparations and polling', async () => {
  const calls = [], controller = new AbortController();
  const aborted = new DOMException('synthetic generation change', 'AbortError');
  await assert.rejects(submitPreparations({ tasks: preparationTasks(fixture(), ['imaging']), signal: controller.signal,
    classifyFailure, onUpdate() {}, submit: async task => { calls.push(task.id); throw aborted; } }), error => error === aborted);
  assert.deepEqual(calls, [id('b')]);
  assert.equal(controller.signal.aborted, false);
});

test('cancelled polling resumes unfinished parents first and never restarts completed poll budgets', () => {
  const targets = [1, 2, 3].map(value => ({ category: 'imaging', detail: opaquePollReference(value), correlation: 'same_record' }));
  function opaquePollReference(value) { return value.toString().repeat(48); }
  const attempts = new Map([[preparationRefreshKey(targets[0]), 6], [preparationRefreshKey(targets[1]), 2], [preparationRefreshKey(targets[2]), 1]]);
  assert.deepEqual(nextPreparationPolls(targets, attempts), [targets[2]]);
  attempts.set(preparationRefreshKey(targets[2]), 2);
  assert.deepEqual(nextPreparationPolls(targets, attempts), targets.slice(1));
  for (const target of targets) attempts.set(preparationRefreshKey(target), 6);
  assert.deepEqual(nextPreparationPolls(targets, attempts), []);
  const newlySubmitted = { category: 'official_files', correlation: 'unavailable' };
  assert.deepEqual(nextPreparationPolls([...targets, newlySubmitted], attempts), [newlySubmitted]);
});

test('scope projection excludes unselected sources without changing raw duplicates or the workspace', () => {
  const before = fixture(); const copy = structuredClone(before);
  const result = selectedSnapshot(before, ['imaging']);
  assert.deepEqual(Object.keys(result.reports), ['imaging']);
  assert.equal(result.details.imaging[id('a')].entries.length, 2);
  assert.deepEqual(before, copy);
});

test('acquisition manifest keeps missing laboratory, pending details, and unknown source totals distinct', () => {
  const snapshot = fixture();
  snapshot.reports.medicine = source([]); snapshot.states.medicine = { status: 'ready', detail_errors: {} };
  snapshot.reports.imaging.entries.push({ ...row([]), detail_id: id('e') });
  const tasks = preparationTasks(snapshot, ['imaging']).map(task => ({ ...task, state: 'correlation_unavailable' }));
  const result = acquisitionSummary({ snapshot, categories: ['imaging', 'hospital', 'medicine'], laboratory: null,
    includeLaboratory: true, tasks, requestPreparation: true, scope: 'all_available', startedAt: '2026-10-28T00:00:00Z',
    finishedAt: '2026-10-28T00:01:00Z', pollingLimited: true });
  assert.ok(result.statuses.some(item => item.category === 'imaging' && item.code === 'pending_details' && item.state === 'partial'));
  assert.ok(result.statuses.some(item => item.category === 'hospital' && item.code === 'request_failed' && item.state === 'partial'));
  assert.ok(result.statuses.some(item => item.category === 'medicine' && item.code === 'source_total_unknown' && item.state === 'ready'));
  assert.ok(result.statuses.some(item => item.category === 'laboratory' && item.state === 'failed'));
  assert.ok(result.statuses.some(item => item.kind === 'prepare' && item.code === 'source_did_not_provide_job_correlation' && item.count === 2));
  assert.ok(!JSON.stringify(result).includes(id('a')));
});

test('package partitions preserve every file identity and provide a dataset-only package when no originals exist', () => {
  const files = Array.from({ length: 2001 }, (_, index) => ({ category: 'imaging', id: index.toString(16).padStart(48, '0'), archive_id: `archive_${index + 1}` }));
  const groups = packageFileGroups(files);
  assert.deepEqual(groups.map(group => group.length), [1000, 1000, 1]);
  assert.deepEqual(groups.flat(), files);
  assert.deepEqual(packageFileGroups([]), [[]]);
});

test('partial package retries keep successful originals and exact archive identities, with bounded failures but resumable budget skips', () => {
  const files = [1, 2, 3, 4].map(index => ({ category: 'imaging', id: index.toString().repeat(48), archive_id: `archive_${index}` }));
  const completed = { archive_id: 'archive_1', state: 'completed', bytes: 12, sha256: 'a'.repeat(64) };
  const failed = { archive_id: 'archive_2', state: 'failed', bytes: 3, code: 'request_failed' };
  const skipped = { archive_id: 'archive_3', state: 'skipped', bytes: 0, code: 'budget_exceeded' };
  let saved = recordSavedOriginals({}, [completed, failed, skipped], 'first.zip');
  assert.deepEqual(retryableOriginals(files, saved), files.slice(1));
  saved = recordSavedOriginals(saved, [{ ...completed, state: 'failed' }, failed, skipped], 'second.zip');
  saved = recordSavedOriginals(saved, [failed, skipped], 'third.zip');
  assert.equal(saved.archive_1.state, 'completed');
  assert.equal(saved.archive_1.part_name, 'first.zip');
  assert.equal(saved.archive_1.failures, 0);
  assert.equal(saved.archive_2.failures, 3);
  assert.equal(saved.archive_3.failures, 0);
  assert.deepEqual(remainingOriginals(files, saved), files.slice(1));
  assert.deepEqual(retryableOriginals(files, saved), files.slice(2));
  assert.equal(JSON.stringify(saved).includes(files[0].id), false);
  const finished = recordSavedOriginals(saved, [{ ...completed, archive_id: 'archive_3' }], 'fourth.zip');
  assert.equal(finished.archive_3.state, 'completed');
  assert.deepEqual(retryableOriginals(files, finished), [files[3]]);
  assert.equal(saved.archive_3.state, 'skipped', 'prior results remain immutable');
});

test('external dataset binding hashes exact UTF-8 bytes including the preserved terminal newline', async () => {
  const content = '{"schema":"synthetic","原文":"正體中文 😀"}\n';
  const result = await datasetDigest(content);
  assert.equal(result.name, 'healthbank.dataset.v2.json');
  assert.equal(result.size, Buffer.byteLength(content));
  assert.equal(result.sha256, createHash('sha256').update(content).digest('hex'));
  assert.notEqual(result.sha256, createHash('sha256').update(content.trim()).digest('hex'));
});

test('bounded wait aborts promptly without running another poll', async () => {
  const controller = new AbortController();
  const waiting = waitForAcquisition(controller.signal, 10_000);
  controller.abort(new DOMException('synthetic stop', 'AbortError'));
  await assert.rejects(waiting, error => error.name === 'AbortError');
  await assert.rejects(waitForAcquisition(controller.signal, 10_000), error => error.name === 'AbortError');
});
