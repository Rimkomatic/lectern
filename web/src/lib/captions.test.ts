import { describe, expect, it } from 'vitest';
import { CAPTIONS_KEY, loadCaptions, parseCaptions, pickTrack, saveCaptions, trackForKey } from './captions';

function fakeStorage(initial: Record<string, string> = {}) {
  const data = new Map(Object.entries(initial));
  return {
    getItem: (k: string) => data.get(k) ?? null,
    setItem: (k: string, v: string) => void data.set(k, v),
    data,
  };
}

const t = (...langs: string[]) => langs.map((lang) => ({ lang }));

describe('parseCaptions', () => {
  it('reads a saved choice', () => {
    expect(parseCaptions('{"on":true,"lang":"en"}')).toEqual({ on: true, lang: 'en' });
    expect(parseCaptions('{"on":false,"lang":"es"}')).toEqual({ on: false, lang: 'es' });
    expect(parseCaptions('{"on":true}')).toEqual({ on: true, lang: '' });
  });
  it('ignores anything malformed', () => {
    for (const raw of [null, '', 'nope', '[]', 'null', '42', '{}', '{"on":"yes"}', '{"lang":"en"}']) {
      expect(parseCaptions(raw), String(raw)).toBeNull();
    }
  });
  it('does not keep a silly language value', () => {
    expect(parseCaptions('{"on":true,"lang":7}')?.lang).toBe('');
    expect(parseCaptions(JSON.stringify({ on: true, lang: 'x'.repeat(500) }))?.lang.length).toBe(35);
  });
});

describe('load and save', () => {
  it('round-trips', () => {
    const s = fakeStorage();
    expect(loadCaptions(s)).toBeNull();
    saveCaptions({ on: true, lang: 'fr' }, s);
    expect(s.data.has(CAPTIONS_KEY)).toBe(true);
    expect(loadCaptions(s)).toEqual({ on: true, lang: 'fr' });
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
    expect(loadCaptions(broken)).toBeNull();
    expect(() => saveCaptions({ on: true, lang: 'en' }, broken)).not.toThrow();
  });
});

describe('pickTrack', () => {
  it('shows nothing until captions have been turned on', () => {
    expect(pickTrack(t('en'), null)).toBe(-1);
    expect(pickTrack(t('en', 'fr'), { on: false, lang: 'en' })).toBe(-1);
  });
  it('shows the remembered language', () => {
    expect(pickTrack(t('en', 'fr', 'es'), { on: true, lang: 'fr' })).toBe(1);
    expect(pickTrack(t('en', 'FR'), { on: true, lang: 'fr' })).toBe(1);
  });
  it('treats a region as the same language when it has to', () => {
    expect(pickTrack(t('de', 'en-US'), { on: true, lang: 'en' })).toBe(1);
    expect(pickTrack(t('de', 'en'), { on: true, lang: 'en-GB' })).toBe(1);
    expect(pickTrack(t('en-GB', 'en-US'), { on: true, lang: 'en-US' })).toBe(1); // the exact one first
  });
  it('shows the only track there is, in any language', () => {
    expect(pickTrack(t('en'), { on: true, lang: 'es' })).toBe(0);
    expect(pickTrack(t(''), { on: true, lang: 'en' })).toBe(0);
  });
  it('leaves captions off when several tracks and none is the language', () => {
    expect(pickTrack(t('en', 'fr'), { on: true, lang: 'es' })).toBe(-1);
  });
  it('finds a track with no language when that is what was on', () => {
    expect(pickTrack(t('en', ''), { on: true, lang: '' })).toBe(1);
  });
  it('has nothing to show on a lesson without tracks', () => {
    expect(pickTrack([], { on: true, lang: 'en' })).toBe(-1);
  });
});

describe('trackForKey', () => {
  it('uses the remembered language, even while captions are off', () => {
    expect(trackForKey(t('en', 'fr'), { on: false, lang: 'fr' })).toBe(1);
  });
  it('falls back to the first track', () => {
    expect(trackForKey(t('en', 'fr'), null)).toBe(0);
    expect(trackForKey(t('en', 'fr'), { on: false, lang: 'es' })).toBe(0);
  });
  it('has nothing without tracks', () => {
    expect(trackForKey([], null)).toBe(-1);
  });
});
