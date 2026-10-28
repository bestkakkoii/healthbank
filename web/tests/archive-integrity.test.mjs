// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import assert from 'node:assert/strict';
import { createHash, randomBytes } from 'node:crypto';
import { crc32 } from 'node:zlib';
import test from 'node:test';
import { ArchiveCRC32, ArchiveSHA256, hashArchiveBlob, ARCHIVE_CHUNK_BYTES } from '../src/archive-integrity.ts';

test('incremental SHA-256 matches standard vectors and independent Node hashes across padding and chunk boundaries', () => {
  const vectors = ['', 'abc', 'abcdbcdecdefdefgefghfghighijhijkijkljklmklmnlmnomnopnopq', 'a'.repeat(1_000_000)];
  for (const content of vectors) {
    const bytes = Buffer.from(content), hash = new ArchiveSHA256();
    for (let offset = 0; offset < bytes.length; offset += 37) hash.update(bytes.subarray(offset, offset + 37));
    assert.equal(hash.digest(), createHash('sha256').update(bytes).digest('hex'));
    assert.equal(hash.digest(), hash.digest());
    assert.throws(() => hash.update(new Uint8Array([1])), /已結束/);
  }
  for (const length of [1, 55, 56, 57, 63, 64, 65, 119, 120, 127, 128, 129, 65535, 65536, 1048577]) {
    const bytes = randomBytes(length);
    for (const chunk of [1, 64, 16379, 65536]) {
      const hash = new ArchiveSHA256(), crc = new ArchiveCRC32();
      for (let offset = 0; offset < bytes.length; offset += chunk) {
        const part = bytes.subarray(offset, offset + chunk); hash.update(part); crc.update(part);
      }
      assert.equal(hash.digest(), createHash('sha256').update(bytes).digest('hex'));
      assert.equal(crc.digest(), crc32(bytes));
    }
  }
});

test('Blob integrity reads bounded slices and cancellation never continues to another chunk', async () => {
  const bytes = randomBytes(ARCHIVE_CHUNK_BYTES * 2 + 7), blob = new Blob([bytes]);
  const sizes = [], offsets = [];
  const source = { size: blob.size, slice(from, to) { sizes.push(to - from); offsets.push(from); return blob.slice(from, to); },
    arrayBuffer() { throw new Error('Whole-archive buffering is forbidden.'); } };
  const progress = [];
  assert.equal(await hashArchiveBlob(source, new AbortController().signal, read => progress.push(read)), createHash('sha256').update(bytes).digest('hex'));
  assert.ok(sizes.every(size => size <= ARCHIVE_CHUNK_BYTES));
  assert.deepEqual(progress, [ARCHIVE_CHUNK_BYTES, ARCHIVE_CHUNK_BYTES * 2, bytes.length]);
  const controller = new AbortController(); offsets.length = 0;
  await assert.rejects(hashArchiveBlob(source, controller.signal, () => controller.abort()), error => error.name === 'AbortError');
  assert.deepEqual(offsets, [0]);
});

test('incremental SHA-256 preserves the high bit-length word beyond the 512 MiB boundary with constant input memory', () => {
  const block = Buffer.alloc(1024 ** 2, 0x61), hash = new ArchiveSHA256(), independent = createHash('sha256');
  for (let index = 0; index < 513; index++) { hash.update(block); independent.update(block); }
  assert.equal(hash.digest(), independent.digest('hex'));
});
