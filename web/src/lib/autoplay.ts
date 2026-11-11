// "Play the next lesson" crosses a page change, so the wish to start playing
// is handed over here: set by the page that finished, taken once by the next.

let pending: number | null = null;

export function requestAutoplay(itemId: number): void {
  pending = itemId;
}

/** True once if autoplay was requested for exactly this item. */
export function takeAutoplay(itemId: number): boolean {
  const hit = pending === itemId;
  pending = null;
  return hit;
}
