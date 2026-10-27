// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import assert from 'node:assert/strict';
import test from 'node:test';
import {
  LOGIN_STORAGE_KEY, LEGACY_LOGIN_STORAGE_KEY, normalizeCard, maskedIdentity,
  readRememberedLogins, writeRememberedUser, forgetRememberedUser,
  forgetAllRememberedUsers, parseRememberedLogins,
} from '../src/remembered-login.ts';
function memoryStorage() {
  const entries = new Map();
  return { getItem: key => entries.get(key) ?? null,
    setItem: (key, value) => entries.set(key, value), removeItem: key => entries.delete(key) };
}
const first = { identity: 'A000000000', card: '000000000001' };
const second = { identity: 'B000000000', card: '000000000002' };
test('new device has no remembered user and card canonicalization preserves leading zeros', () => {
  assert.deepEqual(readRememberedLogins(memoryStorage()), { users: [], lastIdentity: '', warning: '' });
  assert.equal(normalizeCard('1234 5678'), '000012345678');
  assert.equal(normalizeCard('000012345678'), '000012345678');
  for (const raw of ['1234567', '123456789', '12345678901', '1234567890123', 'a1234567']) assert.equal(normalizeCard(raw), null);
});
test('explicit remembered profiles retain matching cards and erase one user independently', () => {
  const storage = memoryStorage();
  writeRememberedUser(storage, first); writeRememberedUser(storage, second);
  assert.deepEqual(readRememberedLogins(storage).users, [first, second]);
  assert.equal(readRememberedLogins(storage).lastIdentity, second.identity);
  writeRememberedUser(storage, { ...first, card: '000000000003' });
  assert.equal(readRememberedLogins(storage).users.find(user => user.identity === first.identity).card, '000000000003');
  forgetRememberedUser(storage, first.identity);
  assert.deepEqual(readRememberedLogins(storage).users, [second]);
  forgetAllRememberedUsers(storage);
  assert.equal(storage.getItem(LOGIN_STORAGE_KEY), null);
  assert.equal(storage.getItem(LEGACY_LOGIN_STORAGE_KEY), null);
});
test('legacy known fields migrate only on a successful-write caller, never carrying credential fields', () => {
  const storage = memoryStorage(); storage.setItem(LEGACY_LOGIN_STORAGE_KEY, JSON.stringify(first));
  assert.deepEqual(readRememberedLogins(storage).users, [first]);
  assert.equal(storage.getItem(LOGIN_STORAGE_KEY), null);
  writeRememberedUser(storage, first);
  assert.equal(storage.getItem(LEGACY_LOGIN_STORAGE_KEY), null);
  assert.throws(() => writeRememberedUser(storage, { ...second, password: 'synthetic-do-not-save' }), /LOGIN_STORAGE_INVALID/);
  assert.ok(!storage.getItem(LOGIN_STORAGE_KEY).includes('password'));
});
test('malformed, oversized, duplicate and credential-bearing storage is rejected rather than trusted', () => {
  for (const text of ['null', '{', '{}', JSON.stringify({ ...first, password: 'synthetic' }),
    JSON.stringify({ schema: 2, users: [first, first], lastIdentity: first.identity }),
    JSON.stringify({ schema: 2, users: [first], lastIdentity: second.identity }),
    JSON.stringify({ schema: 2, users: [first], lastIdentity: first.identity, token: 'synthetic' }),
    JSON.stringify({ schema: 2, users: Array(9).fill(first), lastIdentity: first.identity })]) {
    assert.throws(() => parseRememberedLogins(text));
  }
  const storage = memoryStorage(); storage.setItem(LOGIN_STORAGE_KEY, '{');
  assert.match(readRememberedLogins(storage).warning, /LOGIN_STORAGE_READ/);
  assert.throws(() => writeRememberedUser(storage, first), /LOGIN_STORAGE_READ/);
});
test('storage failures expose safe diagnostics instead of claiming a successful save or erase', () => {
  const storage = { getItem() { throw new Error('private synthetic value'); }, setItem() { throw new Error('private synthetic value'); }, removeItem() { throw new Error('private synthetic value'); } };
  const result = readRememberedLogins(storage);
  assert.match(result.warning, /LOGIN_STORAGE_READ/);
  assert.ok(!result.warning.includes('private synthetic value'));
  assert.throws(() => forgetAllRememberedUsers(storage), /LOGIN_STORAGE_REMOVE/);
  assert.equal(maskedIdentity(first.identity), 'A0•••••000');
});
