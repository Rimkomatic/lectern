import { describe, expect, it } from 'vitest';
import { requestAutoplay, takeAutoplay } from './autoplay';

describe('autoplay hand-over', () => {
  it('is taken once, by the requested item only', () => {
    expect(takeAutoplay(5)).toBe(false);
    requestAutoplay(5);
    expect(takeAutoplay(6)).toBe(false); // a different item clears the request
    expect(takeAutoplay(5)).toBe(false);
    requestAutoplay(5);
    expect(takeAutoplay(5)).toBe(true);
    expect(takeAutoplay(5)).toBe(false);
  });
});
