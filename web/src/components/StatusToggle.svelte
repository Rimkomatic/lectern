<script lang="ts">
  import { fraction, isDone } from '../lib/actions';
  import type { Item } from '../lib/types';

  interface Props {
    item: Item;
    /** True while this item's change is being saved. */
    busy?: boolean;
    /** Diameter of the ring in px. */
    size?: number;
    onToggle: (item: Item) => void;
  }

  let { item, busy = false, size = 24, onToggle }: Props = $props();

  const done = $derived(isDone(item));
  const frac = $derived(fraction(item));
  const label = $derived(done ? 'Mark not done' : 'Mark done');
  const C = 2 * Math.PI * 9.5; // circumference of the ring
</script>

<!-- The ring shows progress; pressing it marks the item done or not done. -->
<button
  type="button"
  class="st"
  class:done
  class:started={!done && frac > 0}
  disabled={busy}
  title={label}
  aria-label={label}
  onclick={(e) => {
    e.preventDefault();
    e.stopPropagation();
    onToggle(item);
  }}
>
  <svg width={size} height={size} viewBox="0 0 24 24" aria-hidden="true" focusable="false">
    {#if done}
      <circle class="disc" cx="12" cy="12" r="11" />
      <path class="tick" d="m7.4 12.6 3.1 3.1 6.2-6.6" />
    {:else}
      <circle class="ring" cx="12" cy="12" r="9.5" />
      {#if frac > 0}
        <circle
          class="arc"
          cx="12"
          cy="12"
          r="9.5"
          stroke-dasharray="{(frac * C).toFixed(2)} {C.toFixed(2)}"
          transform="rotate(-90 12 12)"
        />
      {/if}
      <path class="hint" d="m7.4 12.6 3.1 3.1 6.2-6.6" />
    {/if}
  </svg>
</button>

<style>
  .st {
    flex: none;
    display: grid;
    place-items: center;
    width: 36px;
    height: 36px;
    padding: 0;
    border: 0;
    border-radius: 50%;
    background: transparent;
    color: var(--text-3);
    cursor: pointer;
    transition:
      background-color var(--dur),
      transform var(--dur) var(--ease);
  }
  .st:hover {
    background: var(--surface-3);
  }
  .st:active {
    transform: scale(0.9);
  }
  .st:disabled {
    cursor: progress;
    opacity: 0.6;
  }
  svg {
    overflow: visible;
  }
  .ring {
    fill: none;
    stroke: var(--border-input);
    stroke-width: 2;
  }
  .arc {
    fill: none;
    stroke: var(--accent);
    stroke-width: 2.6;
    stroke-linecap: round;
  }
  /* the unfinished part of a started item: lighter than the empty ring, but visible on every row colour */
  .started .ring {
    stroke: color-mix(in srgb, var(--border-input) 55%, transparent);
  }
  .disc {
    fill: var(--ok);
  }
  .tick {
    fill: none;
    stroke: var(--surface);
    stroke-width: 2.6;
    stroke-linecap: round;
    stroke-linejoin: round;
  }
  /* a faint tick appears on hover to say "this completes it" */
  .hint {
    fill: none;
    stroke: var(--text-3);
    stroke-width: 2.2;
    stroke-linecap: round;
    stroke-linejoin: round;
    opacity: 0;
    transition: opacity var(--dur);
  }
  .st:hover .hint,
  .st:focus-visible .hint {
    opacity: 1;
  }
</style>
