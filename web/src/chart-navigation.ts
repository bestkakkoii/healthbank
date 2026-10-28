// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

/** Navigate observation indices, never date buckets: same-date duplicates remain reachable. */
export function nextChartPointIndex(key: string, current: number, count: number): number | null {
  if (count < 1) return null;
  const index = Math.max(0, Math.min(count - 1, current));
  switch (key) {
    case 'ArrowRight':
    case 'ArrowDown': return Math.min(count - 1, index + 1);
    case 'ArrowLeft':
    case 'ArrowUp': return Math.max(0, index - 1);
    case 'Home': return 0;
    case 'End': return count - 1;
    default: return null;
  }
}
