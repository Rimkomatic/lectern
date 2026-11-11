// The player's volume is remembered per device (localStorage), not per account:
// speakers and headphones differ from one device to the next.

export const VOLUME_KEY = 'lectern-volume';

export interface SavedVolume {
  volume: number; // 0–1
  muted: boolean;
}

/** Reads what was saved; anything malformed is ignored. */
export function parseVolume(raw: string | null): SavedVolume | null {
  if (!raw) return null;
  try {
    const v: unknown = JSON.parse(raw);
    if (typeof v !== 'object' || v === null) return null;
    const { volume, muted } = v as { volume?: unknown; muted?: unknown };
    if (typeof volume !== 'number' || !Number.isFinite(volume)) return null;
    return { volume: Math.min(1, Math.max(0, volume)), muted: muted === true };
  } catch {
    return null;
  }
}

export function loadVolume(storage?: Pick<Storage, 'getItem'>): SavedVolume | null {
  try {
    return parseVolume((storage ?? localStorage).getItem(VOLUME_KEY));
  } catch {
    return null; // storage blocked
  }
}

export function saveVolume(v: SavedVolume, storage?: Pick<Storage, 'setItem'>): void {
  try {
    (storage ?? localStorage).setItem(VOLUME_KEY, JSON.stringify({ volume: v.volume, muted: v.muted }));
  } catch {
    // storage blocked or full: the volume just won't carry over
  }
}
