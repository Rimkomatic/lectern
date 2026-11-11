import { describe, expect, it } from 'vitest';
import { continueSpot, decideStart, savedSpot } from './resume';
import type { Item, Progress } from './types';

function item(kind: Item['kind'], progress: Partial<Progress> | null, extra: Partial<Item> = {}): Item {
  return {
    id: 1,
    course_id: 1,
    section_id: 1,
    name: 'x',
    title: 'x',
    kind,
    ext: '',
    mime: '',
    size: 1,
    duration: 600,
    pages: 10,
    ord: 1,
    subtitles: [],
    progress: progress
      ? {
          state: 'started',
          position: 0,
          furthest: 0,
          duration: null,
          page: null,
          pages: null,
          done_at: null,
          updated_at: '',
          ...progress,
        }
      : null,
    ...extra,
  };
}

describe('savedSpot', () => {
  it('finds a resumable video position', () => {
    expect(savedSpot(item('video', { position: 120 }))).toEqual({ seconds: 120, page: 1 });
  });
  it('ignores the first seconds and the last seconds', () => {
    expect(savedSpot(item('video', { position: 3 }))).toBeNull();
    expect(savedSpot(item('video', { position: 597 }))).toBeNull(); // duration 600 from the item
    expect(savedSpot(item('video', { position: 597, duration: 1000 }))).toEqual({ seconds: 597, page: 1 }); // progress duration wins
  });
  it('ignores finished and never-opened items', () => {
    expect(savedSpot(item('video', { state: 'done', position: 300 }))).toBeNull();
    expect(savedSpot(item('video', null))).toBeNull();
  });
  it('resumes a PDF only past page 1', () => {
    expect(savedSpot(item('pdf', { page: 1 }))).toBeNull();
    expect(savedSpot(item('pdf', { page: null }))).toBeNull();
    expect(savedSpot(item('pdf', { page: 7 }))).toEqual({ seconds: 0, page: 7 });
  });
  it('has nothing to resume for other kinds', () => {
    expect(savedSpot(item('image', { position: 50 }))).toBeNull();
    expect(savedSpot(item('markdown', { position: 50 }))).toBeNull();
  });
});

describe('decideStart', () => {
  const v = item('video', { position: 120 });
  it('offers in ask mode, jumps in auto mode, ignores in never mode', () => {
    expect(decideStart(v, 'ask', {})).toMatchObject({ at: 0, offer: true, savedAt: 120 });
    expect(decideStart(v, 'auto', {})).toMatchObject({ at: 120, offer: false });
    expect(decideStart(v, 'never', {})).toMatchObject({ at: 0, offer: false });
  });
  it('lets an explicit time or page win in every mode', () => {
    for (const mode of ['ask', 'auto', 'never'] as const) {
      expect(decideStart(v, mode, { t: 30 })).toMatchObject({ at: 30, offer: false });
      expect(decideStart(item('pdf', { page: 7 }), mode, { p: 2 })).toMatchObject({ page: 2, offer: false });
      expect(decideStart(v, mode, { t: 0 })).toMatchObject({ at: 0, offer: false });
    }
  });
  it('starts normally when there is nothing saved', () => {
    expect(decideStart(item('video', null), 'ask', {})).toEqual({ at: 0, page: 1, offer: false, savedAt: 0, savedPage: 1 });
  });
  it('resumes a PDF page', () => {
    const pdf = item('pdf', { page: 7 });
    expect(decideStart(pdf, 'auto', {})).toMatchObject({ page: 7, offer: false });
    expect(decideStart(pdf, 'ask', {})).toMatchObject({ page: 1, offer: true, savedPage: 7 });
  });
});

describe('continueSpot', () => {
  it('gives a time for media and a page for PDFs', () => {
    expect(continueSpot('video', 'started', 90, null, 'ask')).toEqual({ t: 90 });
    expect(continueSpot('audio', 'started', 90, null, 'auto')).toEqual({ t: 90 });
    expect(continueSpot('pdf', 'started', 0, 4, 'ask')).toEqual({ p: 4 });
  });
  it('gives nothing when there is nothing to resume or the user opted out', () => {
    expect(continueSpot('video', 'started', 2, null, 'ask')).toEqual({});
    expect(continueSpot('video', 'started', 90, null, 'never')).toEqual({});
    expect(continueSpot('video', 'done', 90, null, 'ask')).toEqual({});
    expect(continueSpot('pdf', 'started', 0, 1, 'ask')).toEqual({});
    expect(continueSpot('image', 'started', 90, null, 'ask')).toEqual({});
  });
});
