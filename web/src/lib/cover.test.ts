import { describe, expect, it } from 'vitest';
import { COVER_COUNT, coverIndex, initials } from './cover';

describe('coverIndex', () => {
  it('is stable and in range', () => {
    for (const t of ['Go Basics', 'Python Tips', '', 'Ünïcode 课程']) {
      const i = coverIndex(t);
      expect(i).toBe(coverIndex(t));
      expect(i).toBeGreaterThanOrEqual(0);
      expect(i).toBeLessThan(COVER_COUNT);
    }
  });
  it('ignores case and surrounding space', () => {
    expect(coverIndex('  Go Basics ')).toBe(coverIndex('go basics'));
  });
  it('spreads different titles over several covers', () => {
    const seen = new Set(Array.from({ length: 40 }, (_, i) => coverIndex(`Course number ${i}`)));
    expect(seen.size).toBeGreaterThan(4);
  });
});

describe('initials', () => {
  it('takes the first letters of the first two meaningful words', () => {
    expect(initials('Go Basics')).toBe('GB');
    expect(initials('Learn Rust by Building Real Applications')).toBe('LR');
    expect(initials('The Art of Cooking')).toBe('AC');
  });
  it('copes with one word, symbols and other scripts', () => {
    expect(initials('rust')).toBe('R');
    expect(initials('[TutsNode.com] - Learn Rust')).toBe('LR');
    expect(initials('[Tag] (2024)')).toBe('T2'); // nothing else to go on: keep the tag
    expect(initials('日本語 入門')).toBe('日入');
    expect(initials('???')).toBe('•');
    expect(initials('')).toBe('•');
  });
});
