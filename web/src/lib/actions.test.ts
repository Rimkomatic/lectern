import { describe, expect, it } from 'vitest';
import {
  doneCount,
  followItem,
  fraction,
  hasTracking,
  isFlat,
  itemMeta,
  kindIcon,
  lessons,
  lessonsDone,
  sectionStats,
  statusKey,
  statusLabel,
} from './actions';
import type { Chapter, CourseTree, Item, Progress, Section } from './types';

function item(over: Partial<Item> = {}, progress: Partial<Progress> | null = null): Item {
  return {
    id: 1,
    course_id: 1,
    section_id: 1,
    name: 'x',
    title: 'x',
    kind: 'video',
    ext: '.webm',
    mime: 'video/webm',
    size: 1,
    duration: 100,
    pages: null,
    ord: 1,
    subtitles: [],
    progress:
      progress === null
        ? null
        : {
            state: 'started',
            position: 0,
            furthest: 0,
            duration: null,
            page: null,
            pages: null,
            done_at: null,
            updated_at: '',
            ...progress,
          },
    ...over,
  };
}

describe('statusKey / statusLabel', () => {
  it('tells never opened, started and done apart', () => {
    expect(statusKey(item())).toBe('new');
    expect(statusKey(item({}, { state: 'started' }))).toBe('started');
    expect(statusKey(item({}, { state: 'done' }))).toBe('done');
    expect(statusLabel(item({}, { state: 'done' }))).toBe('done');
    expect(statusLabel(item())).toBe('not started');
  });
});

describe('fraction', () => {
  it('is 0 when never opened and 1 when done', () => {
    expect(fraction(item())).toBe(0);
    expect(fraction(item({}, { state: 'done' }))).toBe(1);
  });
  it('follows the playback position of media', () => {
    expect(fraction(item({ duration: 100 }, { position: 50 }))).toBeCloseTo(0.5);
    expect(fraction(item({ duration: null }, { position: 30, duration: 60 }))).toBeCloseTo(0.5);
  });
  it('never shows an empty or full ring for an unfinished item', () => {
    expect(fraction(item({ duration: 100 }, { position: 0 }))).toBeCloseTo(0.05);
    expect(fraction(item({ duration: 100 }, { position: 100 }))).toBeCloseTo(0.97);
  });
  it('follows the page of a pdf', () => {
    expect(fraction(item({ kind: 'pdf', duration: null, pages: 10 }, { page: 5 }))).toBeCloseTo(0.5);
  });
  it('shows a small stub for things without a length', () => {
    expect(fraction(item({ kind: 'image', duration: null }, { state: 'started' }))).toBeCloseTo(0.1);
  });
});

describe('sectionStats', () => {
  it('counts done items and sums media time only', () => {
    const s: Section = {
      id: 1,
      title: 's',
      ord: 1,
      items: [
        item({ id: 1, duration: 60 }, { state: 'done' }),
        item({ id: 2, duration: 30 }),
        item({ id: 3, kind: 'pdf', duration: null }, { state: 'done' }),
      ],
    };
    expect(sectionStats(s)).toEqual({ done: 2, total: 3, seconds: 90 });
  });
});

describe('kindIcon', () => {
  it('has an icon for every kind', () => {
    for (const k of ['video', 'audio', 'pdf', 'image', 'markdown', 'text', 'csv', 'sheet', 'link', 'other'] as const) {
      expect(kindIcon(k)).toBeTruthy();
    }
  });
});

// ---- videos with chapters ----

const chaps = (...done: boolean[]): Chapter[] =>
  done.map((d, i) => ({ start: i * 600, title: `Part ${i + 1}`, done: d }));

describe('chapters as lessons', () => {
  it('count one lesson per chapter, and one for everything else', () => {
    expect(lessons(item())).toBe(1);
    expect(lessons(item({ chapters: chaps(false, false, false) }))).toBe(3);
    expect(lessons(item({ chapters: [] }))).toBe(1);
  });

  it('count the finished chapters, or all of them once the item is done', () => {
    expect(lessonsDone(item({ chapters: chaps(true, false, true) }, { state: 'started' }))).toBe(2);
    expect(lessonsDone(item({ chapters: chaps(false, false, false) }, { state: 'done' }))).toBe(3);
    expect(lessonsDone(item({}, { state: 'done' }))).toBe(1);
    expect(lessonsDone(item())).toBe(0);
  });

  it('add up in a course and in a section', () => {
    const s: Section = {
      id: 1,
      title: 's',
      ord: 1,
      items: [
        item({ id: 1, duration: 1800, chapters: chaps(true, true, false) }, { state: 'started' }),
        item({ id: 2, duration: 60 }, { state: 'done' }),
        item({ id: 3, kind: 'pdf', duration: null }),
      ],
    };
    expect(sectionStats(s)).toEqual({ done: 3, total: 5, seconds: 1860 });
    expect(doneCount([s, s])).toEqual({ done: 6, total: 10 });
  });

  it('follow the item when it is marked done or not done', () => {
    const it = item({ chapters: chaps(true, false) });
    followItem(it, true);
    expect(it.chapters?.map((c) => c.done)).toEqual([true, true]);
    followItem(it, false);
    expect(it.chapters?.map((c) => c.done)).toEqual([false, false]);
    followItem(item(), true); // nothing to follow, nothing breaks
  });

  it('show the finished chapters plus how far into the current one the video is', () => {
    const it = item(
      { duration: 1800, chapters: chaps(true, false, false) },
      { state: 'started', position: 900, duration: 1800 },
    );
    // one done + half of the second, out of three
    expect(fraction(it)).toBeCloseTo(1.5 / 3);
  });

  it('do not count a finished chapter twice when the video is inside it', () => {
    const it = item(
      { duration: 1800, chapters: chaps(true, true, false) },
      { state: 'started', position: 900, duration: 1800 },
    );
    expect(fraction(it)).toBeCloseTo(2 / 3);
  });

  it('never show an empty or full ring for a video that is not done', () => {
    const none = item({ duration: 1800, chapters: chaps(false, false) }, { state: 'started', position: 0 });
    expect(fraction(none)).toBeCloseTo(0.05);
    const almost = item({ duration: 1800, chapters: chaps(true, true) }, { state: 'started', position: 1790 });
    expect(fraction(almost)).toBeCloseTo(0.97);
    expect(fraction(item({ chapters: chaps(true, true) }, { state: 'done' }))).toBe(1);
  });

  it('are mentioned next to the length', () => {
    expect(itemMeta(item({ duration: 5400, chapters: chaps(false, false, false) }))).toBe('1h 30m · 3 chapters');
    expect(itemMeta(item({ duration: 5400, chapters: chaps(false) }))).toBe('1h 30m · 1 chapter');
    expect(itemMeta(item({ duration: 5400 }))).toBe('1h 30m');
    expect(itemMeta(item({ duration: null, chapters: chaps(false, false) }))).toBe('2 chapters');
  });
});

describe('isFlat', () => {
  const sec = (over: Partial<Section> = {}): Section => ({ id: 1, title: 'Course files', ord: 1, items: [], ...over });
  it('is true for a course made only of loose files', () => {
    expect(isFlat([sec({ loose: true })])).toBe(true);
  });
  it('is false when there are real sections, or none', () => {
    expect(isFlat([sec({ loose: true }), sec({ id: 2, title: 'Part 1' })])).toBe(false);
    expect(isFlat([sec({ title: 'Part 1' })])).toBe(false);
    expect(isFlat([sec()])).toBe(false);
    expect(isFlat([])).toBe(false);
  });
});

describe('hasTracking', () => {
  const chapter = (start: number, done: boolean): Chapter => ({ start, title: `c${start}`, done });
  const tree = (items: Item[], lastSeen: string | null = null): CourseTree => ({
    course: {
      id: 1,
      title: 'Course',
      description: '',
      library: 'Main',
      has_cover: false,
      cover_v: 0,
      item_count: items.length,
      done_count: 0,
      duration_s: 0,
      last_seen_at: lastSeen,
    },
    sections: [{ id: 1, title: 'Part 1', ord: 1, items }],
    resume: null,
    completed: false,
  });

  it('is false for a course nobody has touched', () => {
    expect(hasTracking(tree([item({ id: 1 }), item({ id: 2 })]))).toBe(false);
    expect(hasTracking(tree([]))).toBe(false);
  });
  it('is true once the person has a place to continue from', () => {
    expect(hasTracking(tree([item()], '2026-11-13T10:00:00Z'))).toBe(true);
  });
  it('is true for a lesson that was opened, or marked done by hand', () => {
    expect(hasTracking(tree([item({ id: 1 }), item({ id: 2 }, { state: 'started' })]))).toBe(true);
    expect(hasTracking(tree([item({ id: 1 }, { state: 'done' })]))).toBe(true);
  });
  it('is true for a ticked chapter, false for chapters nobody ticked', () => {
    expect(hasTracking(tree([item({ chapters: [chapter(0, false), chapter(60, true)] })]))).toBe(true);
    expect(hasTracking(tree([item({ chapters: [chapter(0, false), chapter(60, false)] })]))).toBe(false);
  });
});
