// Chapters of a long video ("<name>.chapters.txt" next to it): which one is
// playing, when one has been watched to its end, and getting that to the server.

import { ApiError, api, messageOf, url } from './api';
import type { Chapter, ChaptersReply, Item, Progress } from './types';

/** How the server names a chapter: its start in whole seconds. */
export const chapterKey = (c: Pick<Chapter, 'start'>): number => Math.floor(c.start);

export const chaptersOf = (item: Item): Chapter[] => item.chapters ?? [];

export const hasChapters = (item: Item): boolean => (item.chapters?.length ?? 0) > 0;

export const doneChapters = (chapters: readonly Chapter[]): number => chapters.filter((c) => c.done).length;

/** Index of the chapter playing at time t; -1 before the first one starts (or when there are none). */
export function chapterIndexAt(chapters: readonly Chapter[], t: number): number {
  let lo = 0;
  let hi = chapters.length - 1;
  let found = -1;
  while (lo <= hi) {
    const mid = (lo + hi) >> 1;
    if (chapters[mid].start <= t) {
      found = mid;
      lo = mid + 1;
    } else {
      hi = mid - 1;
    }
  }
  return found;
}

/** Where chapter i ends: where the next begins, else the end of the video (Infinity if that is unknown). */
export function chapterEnd(chapters: readonly Chapter[], i: number, duration?: number | null): number {
  if (i + 1 < chapters.length) return chapters[i + 1].start;
  const start = chapters[i]?.start ?? 0;
  return duration && Number.isFinite(duration) && duration > start ? duration : Infinity;
}

/** Length of chapter i in seconds, or 0 when it cannot be told (the last one of a video of unknown length). */
export function chapterLength(chapters: readonly Chapter[], i: number, duration?: number | null): number {
  const end = chapterEnd(chapters, i, duration);
  return Number.isFinite(end) ? Math.max(0, end - chapters[i].start) : 0;
}

/** How far through chapter i the time t is, 0..1. */
export function chapterFraction(chapters: readonly Chapter[], i: number, t: number, duration?: number | null): number {
  const len = chapterLength(chapters, i, duration);
  if (len <= 0) return 0;
  return Math.min(1, Math.max(0, (t - chapters[i].start) / len));
}

/**
 * Notices when playback runs through the end of a chapter. A chapter counts as
 * watched when the video plays (or is nudged forward a few seconds, as with the
 * arrow keys) past its end. Jumping around does not count: dragging the bar
 * over a chapter, or clicking one further on, leaves the ones skipped alone.
 */
export class ChapterTracker {
  private last: number | null = null;
  private jumped = false;
  private stepped = false;

  /** @param maxStep the most one time update may move and still be playing
   *  @param maxSkip the most a seek may move forward and still be a nudge */
  constructor(
    private readonly maxStep = 3,
    private readonly maxSkip = 10,
  ) {}

  /** The player is seeking. */
  seeking(): void {
    this.jumped = true;
  }

  /** The playback time moved to `time`. Returns the chapters whose end was passed on the way. */
  update(chapters: readonly Chapter[], time: number): Chapter[] {
    if (!Number.isFinite(time)) return [];
    const prev = this.last;
    const jumped = this.jumped;
    this.last = time;
    this.jumped = false;
    if (prev === null) return [];
    const dt = time - prev;
    if (dt === 0) return [];
    if (!(dt > 0) || dt > (jumped ? this.maxSkip : this.maxStep)) {
      this.stepped = false; // backwards, or a jump: start over from here
      return [];
    }
    this.stepped = true;
    const out: Chapter[] = [];
    for (let i = 0; i + 1 < chapters.length; i++) {
      const end = chapters[i + 1].start;
      if (prev < end && end <= time) out.push(chapters[i]);
    }
    return out;
  }

  /** The video ended. Returns the last chapter if it got there by playing and not by being dragged to the end. */
  ended(chapters: readonly Chapter[], duration?: number): Chapter | null {
    const lengthKnown = duration !== undefined && Number.isFinite(duration);
    const close = this.last !== null && (!lengthKnown || duration - this.last <= this.maxStep);
    const ok = this.stepped && close && chapters.length > 0;
    this.stepped = false;
    return ok ? chapters[chapters.length - 1] : null;
  }
}

/**
 * The server's progress for an item after a chapter change, folded into what the
 * page already has. The position is only taken when the reply is not older.
 */
export function mergeProgress(cur: Progress | null | undefined, next: Progress): Progress {
  if (!cur) return next;
  const newer = Date.parse(next.updated_at) >= Date.parse(cur.updated_at);
  return newer ? next : { ...cur, state: next.state, done_at: next.done_at };
}

/**
 * Copies the server's chapters onto a list, except for chapters the person has
 * just changed and the server has not confirmed yet.
 */
export function paintChapters(
  target: Chapter[] | undefined,
  fresh: readonly Chapter[],
  pending: (key: number) => boolean | undefined = () => undefined,
): void {
  if (!target) return;
  for (const f of fresh) {
    const key = chapterKey(f);
    const t = target.find((c) => chapterKey(c) === key);
    if (t) t.done = pending(key) ?? f.done;
  }
}

// ---- telling the server ----

export type MarkResult = { reply: ChaptersReply } | { error: 'retry' | 'fatal'; message: string };

/** How chapter changes reach the server; replaced by a fake in tests. */
export interface MarkTransport {
  send(itemId: number, done: number[], undone: number[]): Promise<MarkResult>;
  /** Fire-and-forget delivery that survives the page closing. */
  beacon(itemId: number, done: number[], undone: number[]): boolean;
}

export const httpMarks: MarkTransport = {
  async send(itemId, done, undone) {
    try {
      return { reply: await api.chapters(itemId, { done, undone }) };
    } catch (e) {
      // 4xx (video gone, signed out) will not get better by trying again
      const fatal = e instanceof ApiError && e.status >= 400 && e.status < 500 && e.status !== 429;
      return { error: fatal ? 'fatal' : 'retry', message: messageOf(e) };
    }
  },
  beacon(itemId, done, undone) {
    try {
      const blob = new Blob([JSON.stringify({ done, undone })], { type: 'application/json' });
      return navigator.sendBeacon(url.chapters(itemId), blob);
    } catch {
      return false;
    }
  },
};

export interface MarkOptions {
  transport?: MarkTransport;
  /** How long to wait for more changes before sending. Default 400 ms. */
  delayMs?: number;
  /** Pause before the first retry, doubled each time. Default 3 s. */
  retryMs?: number;
  maxRetries?: number;
  /** The server's answer after each successful send. */
  onReply?: (r: ChaptersReply) => void;
  /** A change that could not be saved, after retrying where that can help. */
  onError?: (message: string) => void;
}

/**
 * Collects "chapter finished / not finished" changes for one video and sends
 * them together a moment later. Requests never overlap; a change that fails for
 * a reason that may pass (offline, server busy) is tried again.
 */
export class ChapterMarks {
  readonly itemId: number;
  private readonly transport: MarkTransport;
  private readonly delayMs: number;
  private readonly retryMs: number;
  private readonly maxRetries: number;
  private readonly onReply?: (r: ChaptersReply) => void;
  private readonly onError?: (message: string) => void;

  private want = new Map<number, boolean>(); // asked for, not sent yet
  private sending = new Map<number, boolean>(); // on its way
  private timer: ReturnType<typeof setTimeout> | null = null;
  private inFlight = false;
  private failures = 0;
  private destroyed = false;

  constructor(itemId: number, opts: MarkOptions = {}) {
    this.itemId = itemId;
    this.transport = opts.transport ?? httpMarks;
    this.delayMs = opts.delayMs ?? 400;
    this.retryMs = opts.retryMs ?? 3000;
    this.maxRetries = opts.maxRetries ?? 3;
    this.onReply = opts.onReply;
    this.onError = opts.onError;
  }

  /** Finish or un-finish the chapter that starts at `key` (whole seconds). */
  mark(key: number, done: boolean): void {
    if (this.destroyed) return;
    this.want.set(key, done);
    this.failures = 0; // a new request from the person starts afresh
    this.schedule(this.delayMs);
  }

  /** What the person asked for that the server has not confirmed; undefined when nothing is pending. */
  pending(key: number): boolean | undefined {
    return this.want.get(key) ?? this.sending.get(key);
  }

  get busy(): boolean {
    return this.want.size > 0 || this.sending.size > 0;
  }

  /** Sends what is waiting, now. */
  async flush(): Promise<void> {
    if (this.destroyed || this.inFlight || this.want.size === 0) return;
    this.clearTimer();
    const batch = this.want;
    this.want = new Map();
    this.sending = batch;
    const done = [...batch].filter(([, d]) => d).map(([k]) => k);
    const undone = [...batch].filter(([, d]) => !d).map(([k]) => k);

    this.inFlight = true;
    let result: MarkResult;
    try {
      result = await this.transport.send(this.itemId, done, undone);
    } catch (e) {
      result = { error: 'retry', message: messageOf(e) };
    }
    this.inFlight = false;
    this.sending = new Map();
    if (this.destroyed) return;

    if ('reply' in result) {
      this.failures = 0;
      this.onReply?.(result.reply);
    } else if (result.error === 'retry' && this.failures < this.maxRetries) {
      this.failures++;
      for (const [k, d] of batch) if (!this.want.has(k)) this.want.set(k, d); // newer wishes win
      this.schedule(this.retryMs * 2 ** (this.failures - 1));
      return;
    } else {
      this.failures = 0;
      this.onError?.(result.message);
    }
    if (this.want.size > 0) this.schedule(0);
  }

  /** The page is being hidden or closed: hand whatever is unconfirmed to the browser to deliver. */
  leave(): void {
    if (this.destroyed) return;
    this.clearTimer();
    const all = new Map([...this.sending, ...this.want]);
    if (all.size === 0) return;
    const done = [...all].filter(([, d]) => d).map(([k]) => k);
    const undone = [...all].filter(([, d]) => !d).map(([k]) => k);
    this.transport.beacon(this.itemId, done, undone);
    this.want = new Map();
  }

  destroy(): void {
    this.destroyed = true;
    this.clearTimer();
  }

  private schedule(ms: number): void {
    this.clearTimer();
    this.timer = setTimeout(() => {
      this.timer = null;
      void this.flush();
    }, ms);
  }

  private clearTimer(): void {
    if (this.timer) clearTimeout(this.timer);
    this.timer = null;
  }
}
