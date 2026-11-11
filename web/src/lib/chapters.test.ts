import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  ChapterMarks,
  ChapterTracker,
  chapterEnd,
  chapterFraction,
  chapterIndexAt,
  chapterKey,
  chapterLength,
  doneChapters,
  mergeProgress,
  paintChapters,
  type MarkResult,
  type MarkTransport,
} from './chapters';
import type { Chapter, ChaptersReply, Progress } from './types';

const chap = (start: number, title = `at ${start}`, done = false): Chapter => ({ start, title, done });
// three chapters: 0–600, 600–1200, 1200–end
const three = () => [chap(0, 'Intro'), chap(600, 'Middle'), chap(1200, 'End')];

const prog = (over: Partial<Progress> = {}): Progress => ({
  state: 'started',
  position: 0,
  furthest: 0,
  duration: null,
  page: null,
  pages: null,
  done_at: null,
  updated_at: '2026-01-01T00:00:10Z',
  ...over,
});

describe('chapterKey', () => {
  it('is the start in whole seconds, the way the server names a chapter', () => {
    expect(chapterKey(chap(754.9))).toBe(754);
    expect(chapterKey(chap(0))).toBe(0);
  });
});

describe('chapterIndexAt', () => {
  it('finds the chapter playing at a time', () => {
    const c = three();
    expect(chapterIndexAt(c, 0)).toBe(0);
    expect(chapterIndexAt(c, 599.9)).toBe(0);
    expect(chapterIndexAt(c, 600)).toBe(1);
    expect(chapterIndexAt(c, 5000)).toBe(2);
  });
  it('is -1 before the first chapter and when there are none', () => {
    expect(chapterIndexAt([chap(30), chap(90)], 10)).toBe(-1);
    expect(chapterIndexAt([], 10)).toBe(-1);
    expect(chapterIndexAt(three(), Number.NaN)).toBe(-1);
  });
});

describe('chapter lengths', () => {
  it('ends where the next one starts, and the last at the end of the video', () => {
    const c = three();
    expect(chapterEnd(c, 0)).toBe(600);
    expect(chapterEnd(c, 2, 1800)).toBe(1800);
    expect(chapterEnd(c, 2)).toBe(Infinity);
    expect(chapterEnd(c, 2, 900)).toBe(Infinity); // a length from before the chapter cannot be right
    expect(chapterLength(c, 1)).toBe(600);
    expect(chapterLength(c, 2, 1800)).toBe(600);
    expect(chapterLength(c, 2)).toBe(0);
  });
  it('says how far into a chapter a time is', () => {
    const c = three();
    expect(chapterFraction(c, 1, 900)).toBeCloseTo(0.5);
    expect(chapterFraction(c, 1, 100)).toBe(0);
    expect(chapterFraction(c, 1, 5000)).toBe(1);
    expect(chapterFraction(c, 2, 1500)).toBe(0); // length unknown
    expect(chapterFraction(c, 2, 1500, 1800)).toBeCloseTo(0.5);
  });
});

describe('ChapterTracker', () => {
  const play = (t: ChapterTracker, c: Chapter[], from: number, to: number, step = 0.25) => {
    const out: Chapter[] = [];
    for (let x = from + step; x <= to + 1e-9; x += step) out.push(...t.update(c, x));
    return out;
  };

  it('reports a chapter when playback runs through its end', () => {
    const t = new ChapterTracker();
    const c = three();
    t.update(c, 590);
    const done = play(t, c, 590, 610);
    expect(done.map((x) => x.title)).toEqual(['Intro']);
  });

  it('reports each chapter once as the video plays on', () => {
    const t = new ChapterTracker();
    const c = three();
    t.update(c, 595);
    const done = play(t, c, 595, 1210);
    expect(done.map((x) => x.title)).toEqual(['Intro', 'Middle']);
  });

  it('does not count a chapter that was jumped over', () => {
    const t = new ChapterTracker();
    const c = three();
    t.update(c, 100);
    t.seeking();
    expect(t.update(c, 700)).toEqual([]); // dragged from the first chapter into the second
    expect(play(t, c, 700, 800)).toEqual([]);
  });

  it('treats playing on from the new spot as watching again', () => {
    const t = new ChapterTracker();
    const c = three();
    t.update(c, 100);
    t.seeking();
    t.update(c, 1190); // clicked a chapter near the end
    const done = play(t, c, 1190, 1210);
    expect(done.map((x) => x.title)).toEqual(['Middle']);
  });

  it('counts a small nudge forward over the end of a chapter (the arrow keys)', () => {
    const t = new ChapterTracker();
    const c = three();
    t.update(c, 597);
    t.seeking();
    expect(t.update(c, 602).map((x) => x.title)).toEqual(['Intro']); // +5 s
  });

  it('does not count a nudge that is not small, or one backwards', () => {
    const c = three();
    let t = new ChapterTracker();
    t.update(c, 595);
    t.seeking();
    expect(t.update(c, 625)).toEqual([]); // +30 s is a jump
    t = new ChapterTracker();
    t.update(c, 605);
    t.seeking();
    expect(t.update(c, 595)).toEqual([]);
    expect(play(t, c, 595, 603)).toEqual([c[0]]); // but playing on forward from there does
  });

  it('ignores repeated updates at the same time (paused)', () => {
    const t = new ChapterTracker();
    const c = three();
    t.update(c, 599.9);
    expect(t.update(c, 599.9)).toEqual([]);
    expect(t.update(c, 600.1).map((x) => x.title)).toEqual(['Intro']);
  });

  it('finishes the last chapter when the video plays to its end', () => {
    const t = new ChapterTracker();
    const c = three();
    t.update(c, 1795);
    play(t, c, 1795, 1799.75);
    t.update(c, 1800);
    expect(t.ended(c, 1800)?.title).toBe('End');
  });

  it('does not finish the last chapter when the bar was dragged to the end', () => {
    const t = new ChapterTracker();
    const c = three();
    t.update(c, 300);
    t.seeking();
    t.update(c, 1800); // a jump
    expect(t.ended(c, 1800)).toBeNull();
  });

  it('does not finish the last chapter from a stray end event far from the end', () => {
    const t = new ChapterTracker();
    const c = three();
    t.update(c, 100);
    play(t, c, 100, 105);
    expect(t.ended(c, 1800)).toBeNull();
  });

  it('has nothing to say without chapters', () => {
    const t = new ChapterTracker();
    expect(t.update([], 10)).toEqual([]);
    expect(t.update([], 11)).toEqual([]);
    expect(t.ended([], 12)).toBeNull();
  });

  it('ignores times that are not numbers', () => {
    const t = new ChapterTracker();
    expect(t.update(three(), Number.NaN)).toEqual([]);
  });
});

describe('mergeProgress', () => {
  it('takes a newer reply as it is', () => {
    const cur = prog({ position: 50, updated_at: '2026-01-01T00:00:10Z' });
    const next = prog({ position: 10, state: 'done', updated_at: '2026-01-01T00:00:20Z' });
    expect(mergeProgress(cur, next)).toBe(next);
    expect(mergeProgress(null, next)).toBe(next);
  });
  it('keeps the position of an older reply but takes its state', () => {
    const cur = prog({ position: 500, updated_at: '2026-01-01T00:00:30Z' });
    const next = prog({ position: 10, state: 'done', done_at: 'x', updated_at: '2026-01-01T00:00:20Z' });
    expect(mergeProgress(cur, next)).toMatchObject({ position: 500, state: 'done', done_at: 'x' });
  });
});

describe('paintChapters', () => {
  it('copies the server state by chapter, leaving alone what is still on its way', () => {
    const target = three();
    const fresh = [chap(0, 'Intro', true), chap(600, 'Middle', true), chap(1200, 'End', false)];
    paintChapters(target, fresh, (k) => (k === 600 ? false : undefined));
    expect(target.map((c) => c.done)).toEqual([true, false, false]);
    expect(doneChapters(target)).toBe(1);
  });
  it('copes with nothing to paint and with chapters that do not match', () => {
    paintChapters(undefined, three());
    const target = three();
    paintChapters(target, [chap(5, 'Other', true)]);
    expect(target.every((c) => !c.done)).toBe(true);
  });
});

// ---- ChapterMarks ----

function fakeMarks() {
  const sent: Array<{ done: number[]; undone: number[] }> = [];
  const beacons: Array<{ done: number[]; undone: number[] }> = [];
  const pending: Array<(r: MarkResult) => void> = [];
  const t: MarkTransport = {
    send: (_id, done, undone) =>
      new Promise<MarkResult>((resolve) => {
        sent.push({ done, undone });
        pending.push(resolve);
      }),
    beacon: (_id, done, undone) => {
      beacons.push({ done, undone });
      return true;
    },
  };
  const reply = (done: number[]): ChaptersReply => ({
    chapters: three().map((c) => ({ ...c, done: done.includes(c.start) })),
    progress: prog(),
  });
  return {
    t,
    sent,
    beacons,
    reply,
    ok: async (i: number, done: number[]) => {
      pending[i]({ reply: reply(done) });
      await vi.advanceTimersByTimeAsync(0);
    },
    fail: async (i: number, error: 'retry' | 'fatal') => {
      pending[i]({ error, message: 'nope' });
      await vi.advanceTimersByTimeAsync(0);
    },
  };
}

describe('ChapterMarks', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it('sends changes that come close together as one request', async () => {
    const f = fakeMarks();
    const replies: ChaptersReply[] = [];
    const m = new ChapterMarks(7, { transport: f.t, onReply: (r) => replies.push(r) });
    m.mark(0, true);
    m.mark(600, true);
    m.mark(1200, false);
    expect(f.sent).toHaveLength(0);
    await vi.advanceTimersByTimeAsync(400);
    expect(f.sent).toEqual([{ done: [0, 600], undone: [1200] }]);
    expect(m.busy).toBe(true);
    expect(m.pending(600)).toBe(true);
    await f.ok(0, [0, 600]);
    expect(replies).toHaveLength(1);
    expect(m.busy).toBe(false);
    expect(m.pending(600)).toBeUndefined();
  });

  it('lets the last wish about a chapter win', async () => {
    const f = fakeMarks();
    const m = new ChapterMarks(7, { transport: f.t });
    m.mark(600, true);
    m.mark(600, false);
    await vi.advanceTimersByTimeAsync(400);
    expect(f.sent).toEqual([{ done: [], undone: [600] }]);
  });

  it('never overlaps requests: a change made while one is out goes after it', async () => {
    const f = fakeMarks();
    const m = new ChapterMarks(7, { transport: f.t });
    m.mark(0, true);
    await vi.advanceTimersByTimeAsync(400);
    m.mark(600, true);
    await vi.advanceTimersByTimeAsync(400);
    expect(f.sent).toHaveLength(1);
    expect(m.pending(600)).toBe(true); // still wanted
    await f.ok(0, [0]);
    await vi.advanceTimersByTimeAsync(0);
    expect(f.sent).toHaveLength(2);
    expect(f.sent[1]).toEqual({ done: [600], undone: [] });
  });

  it('tries again later when the network fails, then gives up with a message', async () => {
    const f = fakeMarks();
    const errors: string[] = [];
    const m = new ChapterMarks(7, { transport: f.t, retryMs: 1000, maxRetries: 2, onError: (e) => errors.push(e) });
    m.mark(0, true);
    await vi.advanceTimersByTimeAsync(400);
    await f.fail(0, 'retry');
    expect(m.pending(0)).toBe(true); // still wanted while waiting to retry
    await vi.advanceTimersByTimeAsync(1000);
    expect(f.sent).toHaveLength(2);
    await f.fail(1, 'retry');
    await vi.advanceTimersByTimeAsync(2000); // doubled
    expect(f.sent).toHaveLength(3);
    await f.fail(2, 'retry');
    expect(errors).toEqual(['nope']);
    expect(m.busy).toBe(false);
    await vi.advanceTimersByTimeAsync(60_000);
    expect(f.sent).toHaveLength(3);
  });

  it('does not retry what cannot succeed', async () => {
    const f = fakeMarks();
    const errors: string[] = [];
    const m = new ChapterMarks(7, { transport: f.t, onError: (e) => errors.push(e) });
    m.mark(0, true);
    await vi.advanceTimersByTimeAsync(400);
    await f.fail(0, 'fatal');
    expect(errors).toEqual(['nope']);
    await vi.advanceTimersByTimeAsync(60_000);
    expect(f.sent).toHaveLength(1);
  });

  it('hands what is unconfirmed to the browser when the page is left', async () => {
    const f = fakeMarks();
    const m = new ChapterMarks(7, { transport: f.t });
    m.mark(0, true);
    await vi.advanceTimersByTimeAsync(400); // on its way
    m.mark(600, true); // not sent yet
    m.leave();
    expect(f.beacons).toEqual([{ done: [0, 600], undone: [] }]);
    await vi.advanceTimersByTimeAsync(10_000);
    expect(f.sent).toHaveLength(1); // the waiting one went by beacon, not again
  });

  it('says nothing when there is nothing to leave behind', () => {
    const f = fakeMarks();
    const m = new ChapterMarks(7, { transport: f.t });
    m.leave();
    expect(f.beacons).toHaveLength(0);
  });

  it('goes quiet once destroyed', async () => {
    const f = fakeMarks();
    const m = new ChapterMarks(7, { transport: f.t });
    m.mark(0, true);
    m.destroy();
    await vi.advanceTimersByTimeAsync(10_000);
    m.mark(600, true);
    await vi.advanceTimersByTimeAsync(10_000);
    expect(f.sent).toHaveLength(0);
  });
});
