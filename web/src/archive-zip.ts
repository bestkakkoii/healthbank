// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
// Restricted reader for Healthbank acquisition packages. It accepts the ZIP
// and ZIP64 records written by Go archive/zip, not arbitrary backup formats.
// https://pkware.cachefly.net/webdocs/casestudies/APPNOTE.TXT
import { ARCHIVE_CHUNK_BYTES, ArchiveCRC32, ArchiveSHA256 } from './archive-integrity.ts';
import { archiveFilename, invalidArchive } from './archive-structure.ts';
import { MAX_DATASET_BYTES } from './health-dataset.ts';

export const MAX_PACKAGE_ZIP_BYTES = 9 * 1024 ** 3;
export const MAX_PACKAGE_ORIGINAL_BYTES = 500 * 1024 ** 2;
export const MAX_PACKAGE_MANIFEST_BYTES = 4 * 1024 ** 2;
const MAX_DIRECTORY_BYTES = 4 * 1024 ** 2;
const UINT32_MAX = 0xffffffff;
const decoder = new TextDecoder('utf-8', { fatal: true });
export interface ArchiveZIPEntry {
  name: string; method: 0 | 8; flags: number; crc32: number;
  compressed_size: number; size: number; offset: number; data_offset: number;
}
export interface ArchiveZIP { source: Blob; entries: Map<string, ArchiveZIPEntry> }

function invalidZIP(): never { return invalidArchive('ZIP 結構、路徑或校驗不符合 Healthbank 原始檔包；沒有載入不完整或不明格式的檔案。'); }
function view(bytes: Uint8Array): DataView { return new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength); }
function integer64(data: DataView, offset: number): number {
  const value = data.getBigUint64(offset, true);
  if (value > BigInt(Number.MAX_SAFE_INTEGER)) return invalidZIP();
  return Number(value);
}
async function read(source: Blob, offset: number, length: number, signal: AbortSignal): Promise<Uint8Array> {
  if (!Number.isSafeInteger(offset) || !Number.isSafeInteger(length) || offset < 0 || length < 0 || offset + length > source.size) return invalidZIP();
  signal.throwIfAborted();
  const result = new Uint8Array(await source.slice(offset, offset + length).arrayBuffer());
  signal.throwIfAborted(); if (result.length !== length) return invalidZIP(); return result;
}
function extras(bytes: Uint8Array): Map<number, Uint8Array> {
  const result = new Map<number, Uint8Array>(), data = view(bytes);
  for (let offset = 0; offset < bytes.length;) {
    if (offset + 4 > bytes.length) return invalidZIP();
    const id = data.getUint16(offset, true), size = data.getUint16(offset + 2, true); offset += 4;
    if (offset + size > bytes.length || result.has(id) || ![0x0001, 0x5455].includes(id)) return invalidZIP();
    if (id === 0x5455 && (size !== 5 || bytes[offset] !== 1)) return invalidZIP();
    result.set(id, bytes.subarray(offset, offset + size)); offset += size;
  }
  return result;
}
function memberLimit(name: string): number {
  if (name === 'healthbank.dataset.v2.json') return MAX_DATASET_BYTES;
  if (name === 'manifest.json') return MAX_PACKAGE_MANIFEST_BYTES;
  if (name === 'README.txt') return 65_536;
  const original = /^originals\/(archive_[1-9][0-9]{0,8})\/([^/]+)$/.exec(name);
  if (!original?.[2]) return invalidZIP();
  archiveFilename(original[2]); return MAX_PACKAGE_ORIGINAL_BYTES;
}

export async function openArchiveZIP(source: Blob, signal: AbortSignal): Promise<ArchiveZIP> {
  if (!Number.isSafeInteger(source.size) || source.size < 22 || source.size > MAX_PACKAGE_ZIP_BYTES) return invalidZIP();
  const tailOffset = Math.max(0, source.size - 65_557), tail = await read(source, tailOffset, source.size - tailOffset, signal), end = view(tail);
  let relative = -1;
  for (let index = tail.length - 22; index >= 0; index--) {
    if (end.getUint32(index, true) === 0x06054b50 && index + 22 + end.getUint16(index + 20, true) === tail.length) { relative = index; break; }
  }
  if (relative < 0) return invalidZIP();
  const eocd = tailOffset + relative;
  if (end.getUint16(relative + 4, true) || end.getUint16(relative + 6, true)) return invalidZIP();
  let count = end.getUint16(relative + 10, true), size = end.getUint32(relative + 12, true), offset = end.getUint32(relative + 16, true);
  if (end.getUint16(relative + 8, true) !== count) return invalidZIP();
  let directoryEnd = eocd;
  const locator = eocd >= 20 ? view(await read(source, eocd - 20, 20, signal)) : null;
  if (locator?.getUint32(0, true) === 0x07064b50) {
    if (locator.getUint32(4, true) !== 0 || locator.getUint32(16, true) !== 1) return invalidZIP();
    const location = integer64(locator, 8), record = view(await read(source, location, 56, signal));
    if (location + 56 !== eocd - 20 || record.getUint32(0, true) !== 0x06064b50 || integer64(record, 4) !== 44 ||
      record.getUint16(14, true) > 45 || record.getUint32(16, true) || record.getUint32(20, true)) return invalidZIP();
    const total = integer64(record, 32), length = integer64(record, 40), start = integer64(record, 48);
    if (integer64(record, 24) !== total || count !== Math.min(total, 0xffff) || size !== Math.min(length, UINT32_MAX) || offset !== Math.min(start, UINT32_MAX)) return invalidZIP();
    count = total; size = length; offset = start; directoryEnd = location;
  } else if (count === 0xffff || size === UINT32_MAX || offset === UINT32_MAX) return invalidZIP();
  if (count < 2 || count > 1003 || size > MAX_DIRECTORY_BYTES || offset + size !== directoryEnd) return invalidZIP();
  const bytes = await read(source, offset, size, signal), data = view(bytes), entries = new Map<string, ArchiveZIPEntry>();
  let cursor = 0;
  for (let index = 0; index < count; index++) {
    if (cursor + 46 > size || data.getUint32(cursor, true) !== 0x02014b50) return invalidZIP();
    const version = data.getUint16(cursor + 6, true), flags = data.getUint16(cursor + 8, true), method = data.getUint16(cursor + 10, true);
    const nameLength = data.getUint16(cursor + 28, true), extraLength = data.getUint16(cursor + 30, true), commentLength = data.getUint16(cursor + 32, true);
    const endOffset = cursor + 46 + nameLength + extraLength + commentLength;
    const fileMode = data.getUint32(cursor + 38, true) >>> 16 & 0xf000;
    if (endOffset > size || nameLength < 1 || nameLength > 500 || commentLength || flags & ~0x0808 || ![0, 8].includes(method) || version > 45 ||
      data.getUint16(cursor + 34, true) || fileMode !== 0 && fileMode !== 0x8000) return invalidZIP();
    const name = decoder.decode(bytes.subarray(cursor + 46, cursor + 46 + nameLength)), limit = memberLimit(name);
    if (entries.has(name) || name.includes('\\') || name.includes('\0')) return invalidZIP();
    const fields = extras(bytes.subarray(cursor + 46 + nameLength, cursor + 46 + nameLength + extraLength));
    let uncompressed = data.getUint32(cursor + 24, true), compressed = data.getUint32(cursor + 20, true), localOffset = data.getUint32(cursor + 42, true);
    const zip64 = fields.get(1), required = [uncompressed, compressed, localOffset].filter(value => value === UINT32_MAX).length * 8;
    if ((zip64?.length ?? 0) !== required) return invalidZIP();
    let extraOffset = 0;
    const number = () => { if (!zip64) return invalidZIP(); const value = integer64(view(zip64), extraOffset); extraOffset += 8; return value; };
    if (uncompressed === UINT32_MAX) uncompressed = number();
    if (compressed === UINT32_MAX) compressed = number();
    if (localOffset === UINT32_MAX) localOffset = number();
    if (uncompressed > limit || compressed > limit + 65_536 || method === 0 && compressed !== uncompressed || name.startsWith('originals/') && method !== 0 ||
      localOffset >= offset) return invalidZIP();
    entries.set(name, { name, method: method === 0 ? 0 : 8, flags, crc32: data.getUint32(cursor + 16, true),
      compressed_size: compressed, size: uncompressed, offset: localOffset, data_offset: 0 });
    cursor = endOffset;
  }
  if (cursor !== size || !entries.has('manifest.json') || !entries.has('README.txt')) return invalidZIP();
  const ordered = [...entries.values()].sort((first, second) => first.offset - second.offset);
  let nextOffset = 0;
  for (const entry of ordered) {
    if (entry.offset !== nextOffset) return invalidZIP();
    const header = view(await read(source, entry.offset, 30, signal));
    if (header.getUint32(0, true) !== 0x04034b50 || header.getUint16(4, true) > 45 || header.getUint16(6, true) !== entry.flags ||
      header.getUint16(8, true) !== entry.method) return invalidZIP();
    const nameLength = header.getUint16(26, true), extraLength = header.getUint16(28, true);
    const local = await read(source, entry.offset + 30, nameLength + extraLength, signal);
    if (decoder.decode(local.subarray(0, nameLength)) !== entry.name) return invalidZIP();
    const localExtra = extras(local.subarray(nameLength));
    if (localExtra.has(1)) return invalidZIP(); // Each supported member is < 4 GiB.
    entry.data_offset = entry.offset + 30 + local.length;
    nextOffset = entry.data_offset + entry.compressed_size;
    if (entry.flags & 8) {
      if (header.getUint32(14, true) || header.getUint32(18, true) || header.getUint32(22, true)) return invalidZIP();
      const descriptor = view(await read(source, nextOffset, 16, signal)); nextOffset += 16;
      if (descriptor.getUint32(0, true) !== 0x08074b50 || descriptor.getUint32(4, true) !== entry.crc32 ||
        descriptor.getUint32(8, true) !== entry.compressed_size || descriptor.getUint32(12, true) !== entry.size) return invalidZIP();
    } else if (header.getUint32(14, true) !== entry.crc32 || header.getUint32(18, true) !== entry.compressed_size || header.getUint32(22, true) !== entry.size) return invalidZIP();
    if (nextOffset > offset) return invalidZIP();
  }
  if (nextOffset !== offset || ordered.at(-1)?.name !== 'manifest.json') return invalidZIP();
  return { source, entries };
}

export async function visitZIPEntry(zip: ArchiveZIP, entry: ArchiveZIPEntry, signal: AbortSignal,
  onChunk?: (bytes: Uint8Array, offset: number) => void, onProgress?: (read: number, total: number) => void): Promise<{ sha256: string; bytes: number }> {
  if (zip.entries.get(entry.name) !== entry) return invalidZIP();
  signal.throwIfAborted(); const hash = new ArchiveSHA256(), crc = new ArchiveCRC32(); let count = 0;
  const consume = (bytes: Uint8Array) => {
    signal.throwIfAborted(); if (count + bytes.length > entry.size) return invalidZIP();
    hash.update(bytes); crc.update(bytes); onChunk?.(bytes, count); count += bytes.length; onProgress?.(count, entry.size);
  };
  if (entry.method === 0) {
    for (let offset = 0; offset < entry.size; offset += ARCHIVE_CHUNK_BYTES)
      consume(await read(zip.source, entry.data_offset + offset, Math.min(ARCHIVE_CHUNK_BYTES, entry.size - offset), signal));
  } else {
    let decompressor: DecompressionStream;
    try { decompressor = new DecompressionStream('deflate-raw'); }
    catch { throw new Error('此瀏覽器不支援 ZIP 所需的 deflate-raw 解壓縮；可用系統解壓縮工具取出 healthbank.dataset.v2.json，再以完整資料包還原。'); }
    const reader = zip.source.slice(entry.data_offset, entry.data_offset + entry.compressed_size).stream().pipeThrough(decompressor).getReader();
    const cancel = () => { void reader.cancel().catch(() => undefined); };
    signal.addEventListener('abort', cancel, { once: true });
    try { for (;;) { const item = await reader.read(); signal.throwIfAborted(); if (item.done) break; consume(item.value); } }
    catch (error) { await reader.cancel().catch(() => undefined); throw error; }
    finally { signal.removeEventListener('abort', cancel); reader.releaseLock(); }
  }
  signal.throwIfAborted(); if (count !== entry.size || crc.digest() !== entry.crc32) return invalidZIP();
  return { sha256: hash.digest(), bytes: count };
}

export async function readZIPEntry(zip: ArchiveZIP, entry: ArchiveZIPEntry, maximum: number, signal: AbortSignal): Promise<{ content: Uint8Array; sha256: string }> {
  if (entry.size > maximum) return invalidZIP();
  const content = new Uint8Array(entry.size);
  const result = await visitZIPEntry(zip, entry, signal, (chunk, offset) => { content.set(chunk, offset); });
  return { content, sha256: result.sha256 };
}
