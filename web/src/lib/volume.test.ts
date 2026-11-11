import { describe, expect, it } from 'vitest';
import { VOLUME_KEY, loadVolume, parseVolume, saveVolume } from './volume';

function fakeStorage(initial: Record<string, string> = {}) {
  const data = new Map(Object.entries(initial));
  return {
    getItem: (k: string) => data.get(k) ?? null,
    setItem: (k: string, v: string) => void data.set(k, v),
    data,
  };
}

describe('parseVolume', () => {
  it('reads a saved level and mute', () => {
    expect(parseVolume('{"volume":0.4,"muted":true}')).toEqual({ volume: 0.4, muted: true });
    expect(parseVolume('{"volume":0.4}')).toEqual({ volume: 0.4, muted: false });
  });
  it('clamps out-of-range levels', () => {
    expect(parseVolume('{"volume":7}')?.volume).toBe(1);
    expect(parseVolume('{"volume":-3}')?.volume).toBe(0);
  });
  it('ignores anything malformed', () => {
    for (const raw of [null, '', 'nope', '[]', 'null', '42', '{}', '{"volume":"loud"}', '{"volume":null}']) {
      expect(parseVolume(raw), String(raw)).toBeNull();
    }
  });
  it('does not treat a non-boolean mute as muted', () => {
    expect(parseVolume('{"volume":1,"muted":"yes"}')?.muted).toBe(false);
  });
});

describe('load and save', () => {
  it('round-trips', () => {
    const s = fakeStorage();
    expect(loadVolume(s)).toBeNull();
    saveVolume({ volume: 0.65, muted: false }, s);
    expect(s.data.has(VOLUME_KEY)).toBe(true);
    expect(loadVolume(s)).toEqual({ volume: 0.65, muted: false });
  });
  it('survives storage that throws', () => {
    const broken = {
      getItem: () => {
        throw new Error('blocked');
      },
      setItem: () => {
        throw new Error('blocked');
      },
    };
    expect(loadVolume(broken)).toBeNull();
    expect(() => saveVolume({ volume: 0.5, muted: true }, broken)).not.toThrow();
  });
});
