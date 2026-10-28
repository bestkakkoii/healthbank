// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
// SHA-256 follows FIPS 180-4 section 6.2. This incremental implementation is
// used for local archive integrity, never authentication or credential storage.
// https://nvlpubs.nist.gov/nistpubs/FIPS/NIST.FIPS.180-4.pdf

const ROUND = new Uint32Array([
  0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
  0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
  0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
  0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
  0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
  0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
  0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
  0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2,
]);
const rotate = (value: number, count: number) => value >>> count | value << (32 - count);

export class ArchiveSHA256 {
  private readonly state = new Uint32Array([0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19]);
  private readonly words = new Uint32Array(64);
  private readonly pending = new Uint8Array(64);
  private pendingLength = 0;
  private length = 0;
  private result: string | null = null;

  update(bytes: Uint8Array): void {
    if (this.result !== null) throw new Error('校驗碼已結束，不能再加入資料。');
    if (!Number.isSafeInteger(this.length + bytes.length)) throw new Error('校驗資料長度超過可安全處理範圍。');
    this.length += bytes.length;
    let offset = 0;
    if (this.pendingLength) {
      const count = Math.min(64 - this.pendingLength, bytes.length);
      this.pending.set(bytes.subarray(0, count), this.pendingLength);
      this.pendingLength += count; offset += count;
      if (this.pendingLength === 64) { this.compress(this.pending, 0); this.pendingLength = 0; }
    }
    while (offset + 64 <= bytes.length) { this.compress(bytes, offset); offset += 64; }
    if (offset < bytes.length) {
      this.pending.set(bytes.subarray(offset), 0); this.pendingLength = bytes.length - offset;
    }
  }

  digest(): string {
    if (this.result !== null) return this.result;
    const padding = new Uint8Array(this.pendingLength < 56 ? 64 : 128);
    padding.set(this.pending.subarray(0, this.pendingLength)); padding[this.pendingLength] = 0x80;
    new DataView(padding.buffer).setBigUint64(padding.length - 8, BigInt(this.length) * 8n, false);
    this.compress(padding, 0); if (padding.length === 128) this.compress(padding, 64);
    this.result = [...this.state].map(value => value.toString(16).padStart(8, '0')).join('');
    this.pending.fill(0); this.words.fill(0);
    return this.result;
  }

  private compress(bytes: Uint8Array, offset: number): void {
    const input = new DataView(bytes.buffer, bytes.byteOffset + offset, 64), words = this.words;
    for (let index = 0; index < 16; index++) words[index] = input.getUint32(index * 4, false);
    for (let index = 16; index < 64; index++) {
      const first = words[index - 15] ?? 0, second = words[index - 2] ?? 0;
      const low = rotate(first, 7) ^ rotate(first, 18) ^ first >>> 3;
      const high = rotate(second, 17) ^ rotate(second, 19) ^ second >>> 10;
      words[index] = ((words[index - 16] ?? 0) + low + (words[index - 7] ?? 0) + high) >>> 0;
    }
    let [a = 0, b = 0, c = 0, d = 0, e = 0, f = 0, g = 0, h = 0] = this.state;
    for (let index = 0; index < 64; index++) {
      const sum1 = rotate(e, 6) ^ rotate(e, 11) ^ rotate(e, 25), choice = e & f ^ ~e & g;
      const first = (h + sum1 + choice + (ROUND[index] ?? 0) + (words[index] ?? 0)) >>> 0;
      const sum0 = rotate(a, 2) ^ rotate(a, 13) ^ rotate(a, 22), majority = a & b ^ a & c ^ b & c;
      h = g; g = f; f = e; e = (d + first) >>> 0; d = c; c = b; b = a; a = (first + sum0 + majority) >>> 0;
    }
    const next = [a, b, c, d, e, f, g, h];
    for (let index = 0; index < 8; index++) this.state[index] = ((this.state[index] ?? 0) + (next[index] ?? 0)) >>> 0;
  }
}

const CRC_TABLE = Uint32Array.from({ length: 256 }, (_, value) => {
  let crc = value;
  for (let bit = 0; bit < 8; bit++) crc = crc >>> 1 ^ (crc & 1 ? 0xedb88320 : 0);
  return crc >>> 0;
});

export class ArchiveCRC32 {
  private value = 0xffffffff;
  update(bytes: Uint8Array): void {
    for (const byte of bytes) this.value = this.value >>> 8 ^ (CRC_TABLE[(this.value ^ byte) & 255] ?? 0);
  }
  digest(): number { return (this.value ^ 0xffffffff) >>> 0; }
}

export const ARCHIVE_CHUNK_BYTES = 1024 * 1024;

export async function hashArchiveBlob(blob: Blob, signal: AbortSignal, onProgress?: (read: number, total: number) => void): Promise<string> {
  const hash = new ArchiveSHA256();
  for (let offset = 0; offset < blob.size; offset += ARCHIVE_CHUNK_BYTES) {
    signal.throwIfAborted();
    const bytes = new Uint8Array(await blob.slice(offset, Math.min(blob.size, offset + ARCHIVE_CHUNK_BYTES)).arrayBuffer());
    signal.throwIfAborted(); hash.update(bytes); onProgress?.(offset + bytes.length, blob.size);
  }
  signal.throwIfAborted(); return hash.digest();
}
