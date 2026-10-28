// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
import { crc32, deflateRawSync } from 'node:zlib';

// Synthetic packages only. Matches streaming Go ZIP headers and descriptors.
export function syntheticZIP(members, { descriptor = true, zip64 = false } = {}) {
  const locals = [], directories = []; let offset = 0;
  for (const member of members) {
    const name = Buffer.from(member.name), content = Buffer.from(member.content), method = member.method ?? 0;
    const compressed = method === 8 ? deflateRawSync(content) : content;
    const crc = crc32(content), flags = 0x0800 | (descriptor ? 8 : 0);
    const header = Buffer.alloc(30); header.writeUInt32LE(0x04034b50); header.writeUInt16LE(20, 4);
    header.writeUInt16LE(flags, 6); header.writeUInt16LE(method, 8); header.writeUInt16LE(name.length, 26);
    if (!descriptor) { header.writeUInt32LE(crc, 14); header.writeUInt32LE(compressed.length, 18); header.writeUInt32LE(content.length, 22); }
    const trailer = Buffer.alloc(descriptor ? 16 : 0);
    if (descriptor) { trailer.writeUInt32LE(0x08074b50); trailer.writeUInt32LE(crc, 4); trailer.writeUInt32LE(compressed.length, 8); trailer.writeUInt32LE(content.length, 12); }
    locals.push(header, name, compressed, trailer);
    const central = Buffer.alloc(46); central.writeUInt32LE(0x02014b50); central.writeUInt16LE(20, 4); central.writeUInt16LE(20, 6);
    central.writeUInt16LE(flags, 8); central.writeUInt16LE(method, 10); central.writeUInt32LE(crc, 16);
    central.writeUInt32LE(compressed.length, 20); central.writeUInt32LE(content.length, 24); central.writeUInt16LE(name.length, 28); central.writeUInt32LE(offset, 42);
    directories.push(central, name); offset += header.length + name.length + compressed.length + trailer.length;
  }
  const directory = Buffer.concat(directories), tail = [];
  if (zip64) {
    const record = Buffer.alloc(56), locator = Buffer.alloc(20); record.writeUInt32LE(0x06064b50); record.writeBigUInt64LE(44n, 4);
    record.writeUInt16LE(45, 12); record.writeUInt16LE(45, 14); record.writeBigUInt64LE(BigInt(members.length), 24); record.writeBigUInt64LE(BigInt(members.length), 32);
    record.writeBigUInt64LE(BigInt(directory.length), 40); record.writeBigUInt64LE(BigInt(offset), 48);
    locator.writeUInt32LE(0x07064b50); locator.writeBigUInt64LE(BigInt(offset + directory.length), 8); locator.writeUInt32LE(1, 16); tail.push(record, locator);
  }
  const eocd = Buffer.alloc(22); eocd.writeUInt32LE(0x06054b50); eocd.writeUInt16LE(members.length, 8); eocd.writeUInt16LE(members.length, 10);
  eocd.writeUInt32LE(directory.length, 12); eocd.writeUInt32LE(offset, 16);
  return Buffer.concat([...locals, directory, ...tail, eocd]);
}
