// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

function invalid(): never { throw new Error('JSON 格式、重複欄位或字元不符合規格，未套用資料。'); }

// JSON.parse alone silently accepts repeated object keys. Check the structure
// first so coverage, source values and attachments cannot have ambiguous values.
export function parseStrictJSON(source: string): unknown {
  let position = 0;
  function space() { while (/\s/.test(source[position] ?? '') && position < source.length) position++; }
  function string(): string {
    const start = position++;
    while (position < source.length) {
      const character = source[position++];
      if (character === '\\') position++;
      else if (character === '"') {
        const value: unknown = JSON.parse(source.slice(start, position));
        if (typeof value !== 'string' || /[\uD800-\uDFFF]/u.test(value)) return invalid();
        return value;
      }
    }
    return invalid();
  }
  function scan(depth: number): void {
    if (depth > 24) return invalid();
    space();
    if (source[position] === '"') { string(); return; }
    const open = source[position];
    if (open !== '{' && open !== '[') {
      const start = position;
      while (position < source.length && !/[\s,}\]]/.test(source[position] ?? '')) position++;
      if (position === start) return invalid();
      return;
    }
    position++; space();
    const close = open === '{' ? '}' : ']';
    const keys = new Set<string>();
    if (source[position] === close) { position++; return; }
    for (;;) {
      if (open === '{') {
        if (source[position] !== '"') return invalid();
        const key = string();
        if (keys.has(key) || ['__proto__', 'constructor', 'prototype'].includes(key)) return invalid();
        keys.add(key); space();
        if (source[position++] !== ':') return invalid();
      }
      scan(depth + 1); space();
      if (source[position] === close) { position++; return; }
      if (source[position++] !== ',') return invalid();
      space();
    }
  }
  scan(0); space();
  if (position !== source.length) return invalid();
  return JSON.parse(source) as unknown;
}

