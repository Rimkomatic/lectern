import type { Item, Kind, ResumeMode } from './types';

/** Under this many seconds in (or from the end) there is nothing worth resuming. */
export const MIN_RESUME_SECONDS = 5;

/** The ?t= / ?p= part of an item link. */
export interface Spot {
  t?: number;
  p?: number;
}

export interface Start {
  /** Seconds to start playing at (audio/video). */
  at: number;
  /** Page to open at (PDF). */
  page: number;
  /** True when the saved spot should be offered to the user, not applied. */
  offer: boolean;
  savedAt: number;
  savedPage: number;
}

const isMedia = (k: Kind) => k === 'video' || k === 'audio';

/** Where an unfinished item was left, or null if there is nothing worth resuming. */
export function savedSpot(item: Item): { seconds: number; page: number } | null {
  const pr = item.progress;
  if (!pr || pr.state === 'done') return null;
  if (isMedia(item.kind)) {
    const dur = pr.duration ?? item.duration ?? 0;
    const nearEnd = dur > 0 && pr.position >= dur - MIN_RESUME_SECONDS;
    return pr.position > MIN_RESUME_SECONDS && !nearEnd ? { seconds: pr.position, page: 1 } : null;
  }
  if (item.kind === 'pdf' && (pr.page ?? 1) > 1) return { seconds: 0, page: pr.page ?? 1 };
  return null;
}

/**
 * Decides how an item opens. A link that names a time or page (the course's
 * "Continue" button, a shared link) always wins. Otherwise the user's resume
 * mode applies: "auto" jumps to the saved spot, "ask" offers it, "never"
 * starts from the beginning.
 */
export function decideStart(item: Item, mode: ResumeMode, explicit: Spot): Start {
  const none: Start = { at: 0, page: 1, offer: false, savedAt: 0, savedPage: 1 };
  if (explicit.t !== undefined || explicit.p !== undefined) {
    return { ...none, at: explicit.t ?? 0, page: explicit.p ?? 1 };
  }
  const spot = mode === 'never' ? null : savedSpot(item);
  if (!spot) return none;
  const saved = { savedAt: spot.seconds, savedPage: spot.page };
  if (mode === 'auto') return { at: spot.seconds, page: spot.page, offer: false, ...saved };
  return { ...none, offer: true, ...saved };
}

/** Link parameters for a "Continue" click on an item that was left unfinished. */
export function continueSpot(
  kind: Kind,
  state: 'started' | 'done',
  position: number,
  page: number | null,
  mode: ResumeMode,
): Spot {
  if (mode === 'never' || state === 'done') return {};
  if (isMedia(kind)) return position > MIN_RESUME_SECONDS ? { t: position } : {};
  if (kind === 'pdf' && page !== null && page > 1) return { p: page };
  return {};
}
