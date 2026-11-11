import { ApiError, api } from './api';
import type { Progress, ProgressBody, ProgressEventName } from './types';

/** What the viewer knows about its position right now. */
export interface Patch {
  position?: number;
  duration?: number;
  page?: number;
  pages?: number;
  done?: boolean;
}

/** Outcome of one report: the merged progress, or whether trying again could help. */
export type SendResult = { progress: Progress } | { error: 'retry' | 'fatal' };

/** How reports reach the server; replaced by a fake in tests. */
export interface Transport {
  send(itemId: number, body: ProgressBody): Promise<SendResult>;
  /** Fire-and-forget delivery that survives the page closing. */
  beacon(itemId: number, body: ProgressBody): boolean;
}

export const httpTransport: Transport = {
  async send(itemId, body) {
    try {
      return { progress: (await api.progress(itemId, body)).progress };
    } catch (e) {
      // 4xx (item gone, signed out, bad input) will not get better by retrying
      const fatal = e instanceof ApiError && e.status >= 400 && e.status < 500 && e.status !== 429;
      return { error: fatal ? 'fatal' : 'retry' };
    }
  },
  beacon(itemId, body) {
    try {
      const blob = new Blob([JSON.stringify(body)], { type: 'application/json' });
      return navigator.sendBeacon(`/api/v1/items/${itemId}/progress`, blob);
    } catch {
      return false;
    }
  },
};

export interface ReporterOptions {
  /** Heartbeat period while playing. Default 10 s. */
  intervalMs?: number;
  transport?: Transport;
  now?: () => number;
  /** Called with the server's merged progress after each successful report. */
  onProgress?: (p: Progress) => void;
}

// Higher wins when two queued reports are merged: an "end" must not be lost
// because a later heartbeat took its place.
const priority: Record<ProgressEventName, number> = {
  heartbeat: 0,
  open: 1,
  close: 1,
  seek: 2,
  pause: 2,
  page: 2,
  done: 3,
  end: 3,
};

const important = (b: ProgressBody) => b.event === 'end' || b.done === true;
const MAX_RETRIES = 3;

/**
 * Sends one item's progress to the server: periodic heartbeats while the
 * viewer is active, immediate reports for pause/seek/end/page changes, and a
 * last-chance beacon when the tab is hidden or closed.
 *
 * Requests never overlap: while one is in flight, newer reports are merged
 * into a single queued request. Every report carries a strictly increasing
 * client timestamp, so the server can order them even if they arrive late.
 */
export class ProgressReporter {
  readonly itemId: number;
  private readonly intervalMs: number;
  private readonly transport: Transport;
  private readonly now: () => number;
  private readonly onProgress?: (p: Progress) => void;

  private lastTs = 0;
  private inFlight = false;
  private queued: ProgressBody | null = null;
  private queuedAttempt = 0;
  private timer: ReturnType<typeof setInterval> | null = null;
  private getPatch: (() => Patch) | null = null;
  private latest: Patch = {};
  private destroyed = false;
  private held = false;
  private retryTimers = new Set<ReturnType<typeof setTimeout>>();

  constructor(itemId: number, opts: ReporterOptions = {}) {
    this.itemId = itemId;
    this.intervalMs = opts.intervalMs ?? 10_000;
    this.transport = opts.transport ?? httpTransport;
    this.now = opts.now ?? Date.now;
    this.onProgress = opts.onProgress;
  }

  /** Sends an event right away (coalesced with any request already in flight). */
  report(event: ProgressEventName, patch: Patch = {}): void {
    if (this.destroyed) return;
    if (this.held) {
      // Passive reports must not undo what the user just did (see hold()).
      if (event === 'heartbeat' || event === 'close') return;
      this.held = false; // a deliberate action: reporting resumes
    }
    this.latest = { ...this.latest, ...patch };
    this.enqueue(this.body(event, patch));
  }

  /** Starts periodic heartbeats; getPatch returns the current position. */
  startHeartbeat(getPatch: () => Patch): void {
    if (this.destroyed) return;
    this.held = false; // playing again is a deliberate action
    this.getPatch = getPatch;
    if (this.timer) return;
    this.timer = setInterval(() => {
      const p = this.getPatch?.();
      if (p) this.report('heartbeat', p);
    }, this.intervalMs);
  }

  stopHeartbeat(): void {
    if (this.timer) clearInterval(this.timer);
    this.timer = null;
  }

  /**
   * Call right before the user marks the item "not done". A player parked at
   * the end would otherwise re-complete it with its next heartbeat or the
   * final "close" when the page is left. Until the user does something again
   * (play, seek, pause, change page, mark done) only deliberate reports go out.
   */
  hold(): void {
    if (this.destroyed) return;
    this.held = true;
    this.stopHeartbeat();
    const q = this.queued;
    if (q && q.done === undefined && (q.event === 'heartbeat' || q.event === 'close')) this.queued = null;
    for (const t of this.retryTimers) clearTimeout(t); // an old "end" must not come back
    this.retryTimers.clear();
  }

  /** Call when the page is being hidden or closed. */
  flush(): void {
    if (this.destroyed || this.held) return;
    const patch = this.getPatch?.() ?? this.latest;
    if (patch.position === undefined && patch.page === undefined) return;
    this.transport.beacon(this.itemId, this.body('close', patch));
  }

  /** Sends a final report and stops everything. */
  destroy(): void {
    if (this.destroyed) return;
    const patch = this.getPatch?.() ?? this.latest;
    this.stopHeartbeat();
    if (patch.position !== undefined || patch.page !== undefined) {
      this.report('close', patch);
    }
    this.destroyed = true;
    for (const t of this.retryTimers) clearTimeout(t);
    this.retryTimers.clear();
  }

  private nextTs(): number {
    this.lastTs = Math.max(this.now(), this.lastTs + 1);
    return this.lastTs;
  }

  private body(event: ProgressEventName, patch: Patch): ProgressBody {
    const b: ProgressBody = { event, client_ts: this.nextTs() };
    if (patch.position !== undefined && Number.isFinite(patch.position)) b.position = Math.max(0, patch.position);
    if (patch.duration !== undefined && Number.isFinite(patch.duration) && patch.duration > 0) b.duration = patch.duration;
    if (patch.page !== undefined) b.page = patch.page;
    if (patch.pages !== undefined) b.pages = patch.pages;
    if (patch.done !== undefined) b.done = patch.done;
    return b;
  }

  private merge(older: ProgressBody, newer: ProgressBody): ProgressBody {
    const keepOlderEvent = priority[older.event] > priority[newer.event];
    return {
      ...older,
      ...newer,
      event: keepOlderEvent ? older.event : newer.event,
      done: newer.done ?? older.done,
    };
  }

  private enqueue(body: ProgressBody, attempt = 0): void {
    if (this.inFlight) {
      this.queued = this.queued ? this.merge(this.queued, body) : body;
      this.queuedAttempt = Math.max(this.queuedAttempt, attempt);
      return;
    }
    void this.run(body, attempt);
  }

  private async run(body: ProgressBody, attempt: number): Promise<void> {
    this.inFlight = true;
    let result: SendResult;
    try {
      result = await this.transport.send(this.itemId, body);
    } catch {
      result = { error: 'retry' };
    } finally {
      this.inFlight = false;
    }
    if ('progress' in result) {
      this.onProgress?.(result.progress);
    } else if (result.error === 'retry' && important(body) && attempt < MAX_RETRIES) {
      // Never lose "finished": try again a little later, with a fresh timestamp.
      const t = setTimeout(() => {
        this.retryTimers.delete(t);
        if (!this.destroyed) this.enqueue({ ...body, client_ts: this.nextTs() }, attempt + 1);
      }, 3000 * (attempt + 1));
      this.retryTimers.add(t);
    }
    if (this.queued) {
      const next = this.queued;
      const nextAttempt = this.queuedAttempt;
      this.queued = null;
      this.queuedAttempt = 0;
      void this.run(next, nextAttempt);
    }
  }
}
