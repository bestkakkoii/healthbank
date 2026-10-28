// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import { parseStrictJSON } from './strict-json.ts';

export function invalidArchive(message = '保存索引的格式、關聯或容量不符合規格；目前工作區未被取代。'): never { throw new Error(message); }

export function archiveObject(value: unknown, allowed: readonly string[]): Record<string, unknown> {
  if (typeof value !== 'object' || value === null || Array.isArray(value) ||
    Object.keys(value).some(key => !allowed.includes(key) || ['__proto__', 'constructor', 'prototype'].includes(key))) return invalidArchive();
  return value as Record<string, unknown>;
}
export function archiveText(value: unknown, maximum: number): string {
  if (typeof value !== 'string' || value.length > maximum || /[\uD800-\uDFFF]/u.test(value) ||
    new TextEncoder().encode(value).length > maximum) return invalidArchive();
  return value;
}
export function archiveInteger(value: unknown, maximum: number, minimum = 0): number {
  if (typeof value !== 'number' || !Number.isSafeInteger(value) || value < minimum || value > maximum) return invalidArchive();
  return value;
}
export function archiveBoolean(value: unknown): boolean { if (typeof value !== 'boolean') return invalidArchive(); return value; }
export function archiveList(value: unknown, maximum: number): unknown[] {
  if (!Array.isArray(value) || value.length > maximum) return invalidArchive();
  return value;
}
export function archiveChoice<T extends string>(value: unknown, allowed: readonly T[]): T {
  const found = allowed.find(item => item === value); if (found === undefined) return invalidArchive(); return found;
}
export function archiveDigest(value: unknown): string {
  const digest = archiveText(value, 64); if (!/^[a-f0-9]{64}$/.test(digest)) return invalidArchive(); return digest;
}
export function archiveCategory(value: unknown): string {
  const category = archiveText(value, 100);
  if (!/^[a-z][a-z0-9_]{0,99}$/.test(category) || ['constructor', 'prototype'].includes(category)) return invalidArchive();
  return category;
}
export function archiveReference(value: unknown): string {
  const id = archiveText(value, 32); if (!/^archive_[1-9][0-9]{0,8}$/.test(id)) return invalidArchive(); return id;
}
export function archiveFilename(value: unknown): string {
  const name = archiveText(value, 240);
  if (!name || /[/\\<>:"|?*]/.test(name) || name === '.' || name === '..') return invalidArchive();
  for (const character of name) if (character.charCodeAt(0) < 32 || character.charCodeAt(0) === 127) return invalidArchive();
  return name;
}
export function archiveTime(value: unknown): string {
  const time = archiveText(value, 100); if (!Number.isFinite(Date.parse(time))) return invalidArchive(); return time;
}
export function archiveJSON(bytes: Uint8Array, maximum: number): unknown {
  if (!bytes.length || bytes.length > maximum) return invalidArchive();
  return parseStrictJSON(new TextDecoder('utf-8', { fatal: true }).decode(bytes).replace(/^\uFEFF/, ''));
}
