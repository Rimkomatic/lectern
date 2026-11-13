// Which subtitle track is showing is remembered per device (localStorage), like
// the volume: choose English once and the next lessons start with English on;
// switch captions off and they stay off.

export const CAPTIONS_KEY = 'lectern-captions';

export interface SavedCaptions {
  /** Captions were on when the person last chose. */
  on: boolean;
  /** The language of the track they had on ("" if it has none); kept while captions are off. */
  lang: string;
}

/** Reads what was saved; anything malformed is ignored. */
export function parseCaptions(raw: string | null): SavedCaptions | null {
  if (!raw) return null;
  try {
    const v: unknown = JSON.parse(raw);
    if (typeof v !== 'object' || v === null) return null;
    const { on, lang } = v as { on?: unknown; lang?: unknown };
    if (typeof on !== 'boolean') return null;
    return { on, lang: typeof lang === 'string' ? lang.slice(0, 35) : '' };
  } catch {
    return null;
  }
}

export function loadCaptions(storage?: Pick<Storage, 'getItem'>): SavedCaptions | null {
  try {
    return parseCaptions((storage ?? localStorage).getItem(CAPTIONS_KEY));
  } catch {
    return null; // storage blocked
  }
}

export function saveCaptions(v: SavedCaptions, storage?: Pick<Storage, 'setItem'>): void {
  try {
    (storage ?? localStorage).setItem(CAPTIONS_KEY, JSON.stringify({ on: v.on, lang: v.lang }));
  } catch {
    // storage blocked or full: the choice just won't carry over
  }
}

/** What the choice needs to know about a track. */
export interface TrackInfo {
  lang: string;
}

const primary = (lang: string) => lang.toLowerCase().split(/[-_]/)[0];

/**
 * The track to show for what the person chose before, or -1 for none.
 * The same language wins ("en" also matches "en-US"); a lesson with a single
 * track shows it whatever its language, since they asked for captions.
 */
export function pickTrack(tracks: readonly TrackInfo[], saved: SavedCaptions | null): number {
  if (!saved?.on || tracks.length === 0) return -1;
  const want = saved.lang.toLowerCase();
  let i = tracks.findIndex((t) => t.lang.toLowerCase() === want);
  if (i < 0 && want) i = tracks.findIndex((t) => primary(t.lang) === primary(want));
  if (i < 0 && tracks.length === 1) i = 0;
  return i;
}

/** The track the C key switches on: the remembered language if there is one, else the first. */
export function trackForKey(tracks: readonly TrackInfo[], saved: SavedCaptions | null): number {
  if (tracks.length === 0) return -1;
  const i = pickTrack(tracks, { on: true, lang: saved?.lang ?? '' });
  return i >= 0 ? i : 0;
}
