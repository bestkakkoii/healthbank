// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
// Device-local convenience only. These values never establish authentication.
export const LOGIN_STORAGE_KEY = 'bestkakkoii.healthbank.login-fields.v2';
export const LEGACY_LOGIN_STORAGE_KEY = 'bestkakkoi.healthbank.login-fields';
export const MAX_REMEMBERED_USERS = 8;
export interface RememberedUser { identity: string; card: string }
export interface RememberedLogins { users: RememberedUser[]; lastIdentity: string }
export interface LoginStorageResult extends RememberedLogins { warning: string }
export type LoginStorage = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>;

function object(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}
export function normalizeIdentity(value: string): string { return value.trim().toUpperCase(); }
export function normalizeCard(value: string): string | null {
  const compact = value.replace(/\s/g, '');
  if (/^\d{8}$/.test(compact)) return `0000${compact}`;
  return /^\d{12}$/.test(compact) ? compact : null;
}
function user(value: unknown): RememberedUser | null {
  if (!object(value) || Object.keys(value).some(key => key !== 'identity' && key !== 'card')) return null;
  if (typeof value['identity'] !== 'string' || typeof value['card'] !== 'string') return null;
  const identity = normalizeIdentity(value['identity']);
  const card = normalizeCard(value['card']);
  return /^[A-Z][A-Z0-9]\d{8}$/.test(identity) && card ? { identity, card } : null;
}
export function parseRememberedLogins(raw: string | null, legacy = false): RememberedLogins {
  if (raw === null) return { users: [], lastIdentity: '' };
  const value: unknown = JSON.parse(raw);
  if (legacy) {
    const entry = user(value);
    if (!entry) throw new Error('LOGIN_STORAGE_INVALID');
    return { users: [entry], lastIdentity: entry.identity };
  }
  if (!object(value) || value['schema'] !== 2 || !Array.isArray(value['users']) ||
      typeof value['lastIdentity'] !== 'string' || value['users'].length > MAX_REMEMBERED_USERS ||
      Object.keys(value).some(key => !['schema', 'users', 'lastIdentity'].includes(key))) {
    throw new Error('LOGIN_STORAGE_INVALID');
  }
  const users: RememberedUser[] = [];
  for (const candidate of value['users']) {
    const entry = user(candidate);
    if (!entry || users.some(current => current.identity === entry.identity)) throw new Error('LOGIN_STORAGE_INVALID');
    users.push(entry);
  }
  const lastIdentity = value['lastIdentity'];
  if (lastIdentity !== '' && !users.some(entry => entry.identity === lastIdentity)) throw new Error('LOGIN_STORAGE_INVALID');
  return { users, lastIdentity };
}
export function readRememberedLogins(storage: LoginStorage): LoginStorageResult {
  try {
    const current = storage.getItem(LOGIN_STORAGE_KEY);
    const result = current !== null
      ? parseRememberedLogins(current)
      : parseRememberedLogins(storage.getItem(LEGACY_LOGIN_STORAGE_KEY), true);
    return { ...result, warning: '' };
  } catch {
    return { users: [], lastIdentity: '', warning: '無法讀取此裝置的登入欄位，請重新輸入；登入功能仍可使用。[LOGIN_STORAGE_READ]' };
  }
}
export function writeRememberedUser(storage: LoginStorage, entry: RememberedUser): void {
  const normalized = user(entry);
  if (!normalized) throw new Error('登入欄位格式不正確，未保存。[LOGIN_STORAGE_INVALID]');
  // Read inside the write operation so another tab's newly added user is not lost.
  const current = readRememberedLogins(storage);
  if (current.warning) throw new Error(current.warning);
  const users = current.users.filter(existing => existing.identity !== normalized.identity);
  if (users.length >= MAX_REMEMBERED_USERS) throw new Error('此裝置已記住八位使用者，請先清除不需要的紀錄。[LOGIN_STORAGE_FULL]');
  users.push(normalized);
  try {
    // Serialize an explicit allowlist, never a credential/request object.
    storage.setItem(LOGIN_STORAGE_KEY, JSON.stringify({ schema: 2, users, lastIdentity: normalized.identity }));
    storage.removeItem(LEGACY_LOGIN_STORAGE_KEY);
  } catch {
    throw new Error('登入成功，但此裝置無法保存登入欄位。[LOGIN_STORAGE_WRITE]');
  }
}
export function forgetRememberedUser(storage: LoginStorage, identity: string): RememberedLogins {
  const current = readRememberedLogins(storage);
  if (current.warning) throw new Error(current.warning);
  const users = current.users.filter(entry => entry.identity !== normalizeIdentity(identity));
  const lastIdentity = users.some(entry => entry.identity === current.lastIdentity)
    ? current.lastIdentity : users.at(-1)?.identity ?? '';
  try {
    if (users.length) storage.setItem(LOGIN_STORAGE_KEY, JSON.stringify({ schema: 2, users, lastIdentity }));
    else storage.removeItem(LOGIN_STORAGE_KEY);
    storage.removeItem(LEGACY_LOGIN_STORAGE_KEY);
  } catch {
    throw new Error('此裝置無法移除登入欄位，請使用瀏覽器網站資料設定清除。[LOGIN_STORAGE_REMOVE]');
  }
  return { users, lastIdentity };
}
export function forgetAllRememberedUsers(storage: LoginStorage): void {
  try {
    storage.removeItem(LOGIN_STORAGE_KEY);
    storage.removeItem(LEGACY_LOGIN_STORAGE_KEY);
  } catch {
    throw new Error('此裝置無法移除登入欄位，請使用瀏覽器網站資料設定清除。[LOGIN_STORAGE_REMOVE]');
  }
}
export function maskedIdentity(value: string): string {
  return value.length > 5 ? `${value.slice(0, 2)}${'•'.repeat(value.length - 5)}${value.slice(-3)}` : '未記住使用者';
}
