// How the admin page words the scan schedule the server reports.

import type { ScanSchedule } from './types';

/** A length of time for the end of "every …": "hour", "15 minutes", "2 days". */
export function everyText(seconds: number): string {
  const units: [number, string][] = [
    [86400, 'day'],
    [3600, 'hour'],
    [60, 'minute'],
    [1, 'second'],
  ];
  const n = Math.max(1, Math.round(seconds));
  for (const [size, name] of units) {
    if (n % size === 0) {
      const count = n / size;
      return count === 1 ? name : `${count} ${name}s`;
    }
  }
  return `${n} seconds`;
}

/** One sentence on when the server looks for new and changed files by itself. */
export function scheduleText(s: ScanSchedule | undefined): string {
  if (!s) return '';
  switch (s.mode) {
    case 'daily':
      return `Scans by itself every day at ${s.at ?? '03:00'}${s.zone ? ` (${s.zone})` : ''}. A course you add shows up after that scan, or at once with Scan now.`;
    case 'interval':
      return `Scans by itself every ${everyText(s.every_seconds ?? 0)}.`;
    default:
      return 'Automatic scans are off. A course you add shows up when you press Scan now.';
  }
}
