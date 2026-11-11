// Placeholder artwork for courses that have no cover picture: a stable
// gradient picked from the title, plus the title's initials.

const FILLER = new Set(['the', 'a', 'an', 'of', 'to', 'and', 'in', 'for', 'with', 'on', 'by', 'how']);

/** 32-bit FNV-1a, enough to spread titles over a handful of gradients. */
export function hashString(s: string): number {
  let h = 0x811c9dc5;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 0x01000193);
  }
  return h >>> 0;
}

export const COVER_COUNT = 8;

/** Which of the --cover-0 … --cover-7 gradients a course gets. */
export function coverIndex(title: string): number {
  return hashString(title.trim().toLowerCase()) % COVER_COUNT;
}

/** "Learn Rust by Building Real Applications" -> "LR"; "[TutsNode.com] - Learn Rust" -> "LR". */
export function initials(title: string): string {
  // Release-group tags like "[TutsNode.com]" or "(2024)" say nothing about the course.
  const stripped = title.replace(/\[[^\]]*\]|\([^)]*\)/g, ' ');
  const base = /[\p{L}\p{N}]/u.test(stripped) ? stripped : title;
  const words = base
    .split(/[\s._\-–—:/\\]+/)
    .map((w) => [...w].filter((c) => /[\p{L}\p{N}]/u.test(c)).join(''))
    .filter(Boolean);
  const strong = words.filter((w) => !FILLER.has(w.toLowerCase()));
  const pick = (strong.length ? strong : words).slice(0, 2);
  const out = pick.map((w) => [...w][0] ?? '').join('');
  return (out || '•').toLocaleUpperCase();
}
