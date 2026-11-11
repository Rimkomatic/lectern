import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ProgressReporter, type SendResult, type Transport } from './progress';
import type { Progress, ProgressBody } from './types';

const prog = (position: number): Progress => ({
  state: 'started',
  position,
  furthest: position,
  duration: 100,
  page: null,
  pages: null,
  done_at: null,
  updated_at: '2024-01-01T00:00:00Z',
});

/** A transport whose requests stay pending until the test resolves them. */
function fakeTransport() {
  const sent: ProgressBody[] = [];
  const beacons: ProgressBody[] = [];
  const pending: Array<(r: SendResult) => void> = [];
  const t: Transport = {
    send: (_id, body) =>
      new Promise<SendResult>((resolve) => {
        sent.push(body);
        pending.push(resolve);
      }),
    beacon: (_id, body) => {
      beacons.push(body);
      return true;
    },
  };
  return {
    t,
    sent,
    beacons,
    ok: async (i = 0) => {
      pending[i]({ progress: prog(sent[i].position ?? 0) });
      await vi.advanceTimersByTimeAsync(0);
    },
    fail: async (i: number, error: 'retry' | 'fatal') => {
      pending[i]({ error });
      await vi.advanceTimersByTimeAsync(0);
    },
  };
}

describe('ProgressReporter', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it('sends heartbeats on the interval with increasing timestamps', async () => {
    const f = fakeTransport();
    let pos = 10;
    const r = new ProgressReporter(7, { transport: f.t, intervalMs: 10_000, now: () => 1000 });
    r.startHeartbeat(() => ({ position: pos, duration: 100 }));
    await vi.advanceTimersByTimeAsync(10_000);
    expect(f.sent).toHaveLength(1);
    expect(f.sent[0]).toMatchObject({ event: 'heartbeat', position: 10, duration: 100 });
    await f.ok(0);
    pos = 20;
    await vi.advanceTimersByTimeAsync(10_000);
    expect(f.sent).toHaveLength(2);
    expect(f.sent[1].position).toBe(20);
    expect(f.sent[1].client_ts).toBeGreaterThan(f.sent[0].client_ts); // even though the clock did not move
    r.destroy();
  });

  it('never has two requests in flight and sends only the latest queued state', async () => {
    const f = fakeTransport();
    const r = new ProgressReporter(1, { transport: f.t });
    r.report('seek', { position: 10 });
    r.report('seek', { position: 20 });
    r.report('seek', { position: 30 });
    expect(f.sent).toHaveLength(1);
    await f.ok(0);
    expect(f.sent).toHaveLength(2);
    expect(f.sent[1].position).toBe(30);
    r.destroy();
  });

  it('does not lose an "end" to a later heartbeat', async () => {
    const f = fakeTransport();
    const r = new ProgressReporter(1, { transport: f.t });
    r.report('heartbeat', { position: 5 });
    r.report('end', { position: 100, duration: 100 });
    r.report('heartbeat', { position: 1 });
    await f.ok(0);
    expect(f.sent[1].event).toBe('end');
    r.destroy();
  });

  it('keeps the done flag when merged', async () => {
    const f = fakeTransport();
    const r = new ProgressReporter(1, { transport: f.t });
    r.report('heartbeat', { position: 5 });
    r.report('done', { done: true });
    r.report('heartbeat', { position: 6 });
    await f.ok(0);
    expect(f.sent[1].done).toBe(true);
    r.destroy();
  });

  it('retries an "end" that failed, then gives up after a few tries', async () => {
    const f = fakeTransport();
    const r = new ProgressReporter(1, { transport: f.t });
    r.report('end', { position: 100, duration: 100 });
    await f.fail(0, 'retry');
    expect(f.sent).toHaveLength(1);
    await vi.advanceTimersByTimeAsync(3000);
    expect(f.sent).toHaveLength(2);
    expect(f.sent[1].client_ts).toBeGreaterThan(f.sent[0].client_ts);
    await f.fail(1, 'retry');
    await vi.advanceTimersByTimeAsync(6000);
    expect(f.sent).toHaveLength(3);
    await f.fail(2, 'retry');
    await vi.advanceTimersByTimeAsync(9000);
    expect(f.sent).toHaveLength(4);
    await f.fail(3, 'retry');
    await vi.advanceTimersByTimeAsync(60_000);
    expect(f.sent).toHaveLength(4); // MAX_RETRIES reached
    r.destroy();
  });

  it('does not retry fatal errors or ordinary heartbeats', async () => {
    const f = fakeTransport();
    const r = new ProgressReporter(1, { transport: f.t });
    r.report('end', { position: 100 });
    await f.fail(0, 'fatal');
    await vi.advanceTimersByTimeAsync(60_000);
    expect(f.sent).toHaveLength(1);
    r.report('heartbeat', { position: 3 });
    await f.fail(1, 'retry');
    await vi.advanceTimersByTimeAsync(60_000);
    expect(f.sent).toHaveLength(2);
    r.destroy();
  });

  it('reports server progress to onProgress', async () => {
    const f = fakeTransport();
    const seen: number[] = [];
    const r = new ProgressReporter(1, { transport: f.t, onProgress: (p) => seen.push(p.position) });
    r.report('pause', { position: 42 });
    await f.ok(0);
    expect(seen).toEqual([42]);
    r.destroy();
  });

  it('flush() beacons the current position', () => {
    const f = fakeTransport();
    const r = new ProgressReporter(1, { transport: f.t });
    r.flush(); // nothing known yet: nothing sent
    expect(f.beacons).toHaveLength(0);
    r.startHeartbeat(() => ({ position: 55, duration: 100 }));
    r.flush();
    expect(f.beacons).toHaveLength(1);
    expect(f.beacons[0]).toMatchObject({ event: 'close', position: 55, duration: 100 });
    r.destroy();
  });

  it('destroy() sends a final close, stops the timer and ignores later calls', async () => {
    const f = fakeTransport();
    const r = new ProgressReporter(1, { transport: f.t, intervalMs: 1000 });
    r.startHeartbeat(() => ({ position: 9 }));
    r.destroy();
    expect(f.sent).toHaveLength(1);
    expect(f.sent[0]).toMatchObject({ event: 'close', position: 9 });
    await f.ok(0);
    await vi.advanceTimersByTimeAsync(5000);
    r.report('pause', { position: 99 });
    expect(f.sent).toHaveLength(1);
  });

  describe('hold()', () => {
    it('keeps heartbeats, beacons and the final close quiet', async () => {
      const f = fakeTransport();
      const r = new ProgressReporter(1, { transport: f.t, intervalMs: 1000 });
      r.startHeartbeat(() => ({ position: 99 }));
      r.hold();
      await vi.advanceTimersByTimeAsync(5000);
      r.flush();
      r.destroy();
      expect(f.sent).toHaveLength(0);
      expect(f.beacons).toHaveLength(0);
    });

    it('drops a queued heartbeat but not a queued explicit report', async () => {
      const f = fakeTransport();
      const r = new ProgressReporter(1, { transport: f.t });
      r.report('seek', { position: 1 }); // in flight
      r.report('heartbeat', { position: 99 }); // queued behind it
      r.hold();
      await f.ok(0);
      expect(f.sent).toHaveLength(1);

      r.report('seek', { position: 2 }); // in flight again (held was cleared by the seek)
      r.report('done', { done: false }); // queued
      r.hold();
      await f.ok(1);
      expect(f.sent).toHaveLength(3);
      expect(f.sent[2]).toMatchObject({ event: 'done', done: false });
      r.destroy();
    });

    it('cancels a pending retry of an old "end"', async () => {
      const f = fakeTransport();
      const r = new ProgressReporter(1, { transport: f.t });
      r.report('end', { position: 100, duration: 100 });
      await f.fail(0, 'retry');
      r.hold();
      await vi.advanceTimersByTimeAsync(60_000);
      expect(f.sent).toHaveLength(1);
      r.destroy();
    });

    it('resumes when the user plays, acts or marks done again', async () => {
      const f = fakeTransport();
      const r = new ProgressReporter(1, { transport: f.t, intervalMs: 1000 });
      r.hold();
      r.startHeartbeat(() => ({ position: 3 }));
      await vi.advanceTimersByTimeAsync(1000);
      expect(f.sent).toHaveLength(1);
      await f.ok(0);

      r.hold();
      r.report('pause', { position: 4 });
      expect(f.sent).toHaveLength(2);
      await f.ok(1);

      r.hold();
      r.report('done', { done: true });
      expect(f.sent).toHaveLength(3);
      r.destroy();
    });
  });

  it('drops invalid numbers instead of sending them', async () => {
    const f = fakeTransport();
    const r = new ProgressReporter(1, { transport: f.t });
    r.report('heartbeat', { position: NaN, duration: Infinity });
    expect(f.sent[0].position).toBeUndefined();
    expect(f.sent[0].duration).toBeUndefined();
    r.destroy();
  });
});
