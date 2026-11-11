// Keyboard shortcuts for the audio/video player: Space plays or pauses, ←/→
// skip back and forward, ↑/↓ change the volume. They work wherever the focus is
// on the lesson page; only text fields and the controls that use those keys
// themselves keep them. The logic lives here so it can be tested without a
// browser; VideoPlayer.svelte does the actual listening.

export const SKIP_SECONDS = 5;
/** One arrow-key press moves the volume by this fraction (5 %). */
export const VOLUME_STEP = 0.05;

export type PlayerAction =
  | { kind: 'toggle' }
  | { kind: 'skip'; seconds: number } // negative = back
  | { kind: 'volume'; delta: number };

/** The parts of a keyboard event that decide what it means. */
export interface KeyFacts {
  key: string;
  ctrlKey: boolean;
  metaKey: boolean;
  altKey: boolean;
  shiftKey: boolean;
  isComposing?: boolean;
}

/** What had the focus when the key was pressed. */
export interface TargetFacts {
  tag: string; // lower case; '' when nothing is focused
  type: string; // the type attribute (inputs)
  role: string; // the role attribute
  editable: boolean; // contenteditable
}

const NOTHING: TargetFacts = { tag: '', type: '', role: '', editable: false };

export function targetFacts(t: EventTarget | null): TargetFacts {
  if (!(t instanceof Element)) return NOTHING;
  const ce = t.getAttribute('contenteditable');
  return {
    tag: t.localName,
    type: (t.getAttribute('type') ?? '').toLowerCase(),
    role: (t.getAttribute('role') ?? '').toLowerCase(),
    editable: (t as HTMLElement).isContentEditable === true || (ce !== null && ce.toLowerCase() !== 'false'),
  };
}

// Widgets that use the arrow keys (and Space/Enter) for their own business.
const OWNS_KEYS = new Set([
  'slider', 'spinbutton', 'scrollbar', 'tab', 'tablist', 'menu', 'menubar', 'menuitem', 'menuitemcheckbox',
  'menuitemradio', 'listbox', 'option', 'radio', 'radiogroup', 'tree', 'treeitem', 'grid', 'gridcell',
  'combobox', 'textbox', 'searchbox',
]);
// Controls that Space presses.
const PRESSABLE = new Set(['button', 'checkbox', 'switch']);

function steppedAside(t: TargetFacts, key: string, keyboardDriven: boolean): boolean {
  if (t.editable || t.tag === 'textarea' || t.tag === 'select' || t.tag === 'input') return true;
  if (OWNS_KEYS.has(t.role)) return true;
  // A button you reached with Tab keeps Space. One you clicked with the mouse
  // does not: Space then means "play", as everyone expects.
  if (key === ' ' && keyboardDriven && (t.tag === 'button' || t.tag === 'summary' || PRESSABLE.has(t.role))) {
    return true;
  }
  return false;
}

/**
 * What a key press should do to the player, or null to leave it alone.
 * `keyboardDriven` is false when the last thing the person did was click or tap.
 */
export function actionFor(k: KeyFacts, target: TargetFacts, keyboardDriven: boolean): PlayerAction | null {
  if (k.ctrlKey || k.metaKey || k.altKey || k.shiftKey || k.isComposing) return null;
  let action: PlayerAction;
  switch (k.key) {
    case ' ':
      action = { kind: 'toggle' };
      break;
    case 'ArrowLeft':
      action = { kind: 'skip', seconds: -SKIP_SECONDS };
      break;
    case 'ArrowRight':
      action = { kind: 'skip', seconds: SKIP_SECONDS };
      break;
    case 'ArrowUp':
      action = { kind: 'volume', delta: VOLUME_STEP };
      break;
    case 'ArrowDown':
      action = { kind: 'volume', delta: -VOLUME_STEP };
      break;
    default:
      return null;
  }
  return steppedAside(target, k.key, keyboardDriven) ? null : action;
}

/** Moves a volume (0–1) by delta, staying on whole percents. */
export function stepVolume(current: number, delta: number): number {
  const v = Math.round((current + delta) * 100) / 100;
  return Math.min(1, Math.max(0, v));
}

/** Where a skip lands: never before the start, never past the end. */
export function skipTarget(current: number, seconds: number, duration: number): number {
  const end = Number.isFinite(duration) && duration > 0 ? duration : Infinity;
  return Math.min(end, Math.max(0, current + seconds));
}

// ---- keyboard or pointer? ----
// The browser's :focus-visible flips to "keyboard" on the very key press we are
// judging, so it cannot tell a clicked button from a tabbed-to one. Remember
// it ourselves: a click or tap says "pointer", Tab says "keyboard".

let usingPointer = false;
let watching = false;

/** Call once at startup; the listeners live as long as the page. */
export function watchInputModality(): void {
  if (watching || typeof window === 'undefined') return;
  watching = true;
  window.addEventListener('pointerdown', () => (usingPointer = true), true);
  window.addEventListener(
    'keydown',
    (e) => {
      if (e.key === 'Tab') usingPointer = false;
    },
    true,
  );
}

export function keyboardDriven(): boolean {
  return !usingPointer;
}
