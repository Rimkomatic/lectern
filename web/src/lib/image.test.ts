import { describe, expect, it } from 'vitest';
import {
  COVER_ACCEPT,
  COVER_MAX_H,
  COVER_MAX_W,
  CoverError,
  KEEP_BYTES,
  fitWithin,
  needsShrink,
  pictureType,
  prepareCover,
} from './image';

describe('fitWithin', () => {
  it('shrinks a big picture to fit, keeping its shape', () => {
    expect(fitWithin(4000, 3000, 1600, 1000)).toEqual({ width: 1333, height: 1000 });
    expect(fitWithin(3200, 1000, 1600, 1000)).toEqual({ width: 1600, height: 500 });
    expect(fitWithin(1000, 4000, 1600, 1000)).toEqual({ width: 250, height: 1000 });
  });
  it('never enlarges', () => {
    expect(fitWithin(800, 500, 1600, 1000)).toEqual({ width: 800, height: 500 });
    expect(fitWithin(1, 1, 1600, 1000)).toEqual({ width: 1, height: 1 });
  });
  it('never returns an empty size', () => {
    expect(fitWithin(5000, 2, 1600, 1000)).toEqual({ width: 1600, height: 1 });
  });
});

describe('needsShrink', () => {
  it('lets small pictures through untouched', () => {
    expect(needsShrink(200_000, 1200, 800)).toBe(false);
    expect(needsShrink(KEEP_BYTES, COVER_MAX_W, COVER_MAX_H)).toBe(false);
  });
  it('shrinks heavy or large ones', () => {
    expect(needsShrink(KEEP_BYTES + 1, 800, 600)).toBe(true);
    expect(needsShrink(100_000, COVER_MAX_W + 1, 600)).toBe(true);
    expect(needsShrink(100_000, 800, COVER_MAX_H + 1)).toBe(true);
  });
});

describe('pictureType', () => {
  it('trusts the reported type', () => {
    expect(pictureType('x.bin', 'image/png')).toBe('image/png');
  });
  it('falls back to the extension when the system reports none', () => {
    expect(pictureType('cover.AVIF', '')).toBe('image/avif');
    expect(pictureType('a.b.jpeg', '')).toBe('image/jpeg');
    expect(pictureType('photo.jpg', '')).toBe('image/jpeg');
  });
  it('knows nothing about other files', () => {
    expect(pictureType('notes.txt', '')).toBe('');
    expect(pictureType('noextension', '')).toBe('');
  });
});

describe('prepareCover', () => {
  it('refuses anything that is not a supported picture, before reading it', async () => {
    for (const f of [
      new File(['<svg/>'], 'logo.svg', { type: 'image/svg+xml' }),
      new File(['x'], 'anim.gif', { type: 'image/gif' }),
      new File(['x'], 'notes.txt', { type: 'text/plain' }),
      new File(['x'], 'mystery', { type: '' }),
    ]) {
      await expect(prepareCover(f), f.name).rejects.toBeInstanceOf(CoverError);
    }
  });

  it('the file picker offers exactly the types the server accepts', () => {
    expect(COVER_ACCEPT.split(',').sort()).toEqual(['image/avif', 'image/jpeg', 'image/png', 'image/webp']);
  });
});
