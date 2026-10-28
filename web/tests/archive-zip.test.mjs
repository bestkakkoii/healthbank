// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import test from 'node:test';
import { openArchiveZIP, readZIPEntry, visitZIPEntry } from '../src/archive-zip.ts';
import { syntheticZIP } from './fixtures/acquisition-zip.mjs';
const signal = () => new AbortController().signal;
const members = [
  { name: 'healthbank.dataset.v2.json', content: '合成 UTF-8 資料\n', method: 8 },
  { name: 'README.txt', content: '合成 README', method: 8 },
  { name: 'originals/archive_1/合成原始檔.txt', content: '合成原始檔\n' },
  { name: 'manifest.json', content: '{"synthetic":true}\n', method: 8 },
];

test('restricted ZIP reads streamed descriptors, deflate metadata, stored originals and ZIP64 end records', async () => {
  for (const descriptor of [true, false]) for (const zip64 of [true, false]) {
    const source = new Blob([syntheticZIP(members, { descriptor, zip64 })]);
    const checked = { size: source.size, slice(from, to) { assert.ok(to - from <= 4 * 1024 ** 2); return source.slice(from, to); },
      arrayBuffer() { throw new Error('Whole package buffering is forbidden'); } };
    const zip = await openArchiveZIP(checked, signal());
    for (const member of members) {
      const restored = await readZIPEntry(zip, zip.entries.get(member.name), 1024, signal());
      assert.equal(new TextDecoder().decode(restored.content), member.content);
      assert.equal(restored.sha256, createHash('sha256').update(member.content).digest('hex'));
    }
  }
});

test('ZIP rejects unsafe entries, duplicate members, inconsistent local headers, truncation and CRC changes', async () => {
  for (const list of [members.concat(members[3]), [...members.slice(0, -1), { name: 'originals/archive_2/../secret', content: '' }, members[3]],
    [...members.slice(0, -1), { name: 'originals/archive_2/file.txt', content: 'x', method: 8 }, members[3]]]) {
    await assert.rejects(openArchiveZIP(new Blob([syntheticZIP(list)]), signal()));
  }
  for (const alter of [bytes => bytes.subarray(0, bytes.length - 1), bytes => Buffer.concat([bytes, Buffer.from('junk')]),
    bytes => { bytes.writeUInt16LE(1, 6); return bytes; },
    bytes => { bytes[30] ^= 1; return bytes; },
  ]) await assert.rejects(openArchiveZIP(new Blob([alter(syntheticZIP(members))]), signal()));
  const bytes = syntheticZIP(members), initial = await openArchiveZIP(new Blob([bytes]), signal());
  const original = initial.entries.get(members[2].name); bytes[original.data_offset] ^= 1;
  const corrupted = await openArchiveZIP(new Blob([bytes]), signal());
  await assert.rejects(visitZIPEntry(corrupted, corrupted.entries.get(members[2].name), signal()));
  const abort = new AbortController(); abort.abort(); await assert.rejects(openArchiveZIP(new Blob([bytes]), abort.signal), { name: 'AbortError' });
});

test('ZIP64 offsets beyond 4 GiB are read with sparse random access without allocating original payloads', async () => {
  const pieces = [], central = []; let offset = 0;
  const names = ['README.txt', ...Array.from({ length: 9 }, (_, index) => `originals/archive_${index + 1}/synthetic.bin`), 'manifest.json'];
  const add = (position, bytes) => pieces.push({ position, bytes });
  for (const name of names) {
    const size = name.startsWith('originals/') ? 500 * 1024 ** 2 : 0, encoded = Buffer.from(name);
    const header = Buffer.alloc(30); header.writeUInt32LE(0x04034b50); header.writeUInt16LE(20, 4); header.writeUInt16LE(8, 6); header.writeUInt16LE(encoded.length, 26);
    add(offset, header); add(offset + 30, encoded);
    const trailer = Buffer.alloc(16); trailer.writeUInt32LE(0x08074b50); trailer.writeUInt32LE(size, 8); trailer.writeUInt32LE(size, 12);
    add(offset + 30 + encoded.length + size, trailer);
    const extra = Buffer.alloc(offset >= 0xffffffff ? 12 : 0);
    if (extra.length) { extra.writeUInt16LE(1); extra.writeUInt16LE(8, 2); extra.writeBigUInt64LE(BigInt(offset), 4); }
    const directory = Buffer.alloc(46); directory.writeUInt32LE(0x02014b50); directory.writeUInt16LE(extra.length ? 45 : 20, 6); directory.writeUInt16LE(8, 8);
    directory.writeUInt32LE(size, 20); directory.writeUInt32LE(size, 24); directory.writeUInt16LE(encoded.length, 28); directory.writeUInt16LE(extra.length, 30);
    directory.writeUInt32LE(Math.min(offset, 0xffffffff), 42); central.push(directory, encoded, extra);
    offset += 30 + encoded.length + size + 16;
  }
  const directoryOffset = offset, directory = Buffer.concat(central); add(offset, directory); offset += directory.length;
  const zip64 = Buffer.alloc(56); zip64.writeUInt32LE(0x06064b50); zip64.writeBigUInt64LE(44n, 4); zip64.writeUInt16LE(45, 12); zip64.writeUInt16LE(45, 14);
  zip64.writeBigUInt64LE(BigInt(names.length), 24); zip64.writeBigUInt64LE(BigInt(names.length), 32); zip64.writeBigUInt64LE(BigInt(directory.length), 40); zip64.writeBigUInt64LE(BigInt(directoryOffset), 48); add(offset, zip64);
  const locator = Buffer.alloc(20); locator.writeUInt32LE(0x07064b50); locator.writeBigUInt64LE(BigInt(offset), 8); locator.writeUInt32LE(1, 16); add(offset + 56, locator); offset += 76;
  const end = Buffer.alloc(22); end.writeUInt32LE(0x06054b50); end.writeUInt16LE(names.length, 8); end.writeUInt16LE(names.length, 10); end.writeUInt32LE(directory.length, 12); end.writeUInt32LE(0xffffffff, 16); add(offset, end); offset += 22;
  let totalRead = 0;
  const source = { size: offset, slice(start, finish) {
    assert.ok(finish - start <= 65_557, 'Only tail/header/directory slices may be read while indexing.');
    const bytes = Buffer.alloc(finish - start); totalRead += bytes.length;
    for (const piece of pieces) {
      const from = Math.max(start, piece.position), to = Math.min(finish, piece.position + piece.bytes.length);
      if (from < to) bytes.set(piece.bytes.subarray(from - piece.position, to - piece.position), from - start);
    }
    return new Blob([bytes]);
  }, arrayBuffer() { throw new Error('Sparse whole-file read prohibited'); } };
  const zip = await openArchiveZIP(source, signal());
  assert.ok(zip.entries.get('manifest.json').offset > 2 ** 32); assert.equal(zip.entries.size, names.length);
  assert.ok(totalRead < 100_000, `Indexed the >4 GiB ZIP in ${totalRead} actual bytes.`);
});
