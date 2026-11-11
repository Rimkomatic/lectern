<script lang="ts">
  import { tick } from 'svelte';
  import { chapterKey, chapterLength } from '../lib/chapters';
  import { formatDuration, formatTime } from '../lib/format';
  import type { Chapter } from '../lib/types';
  import Icon from './Icon.svelte';

  interface Props {
    chapters: Chapter[];
    /** Name for screen readers, e.g. "Chapters of Intro to Rust". */
    label: string;
    /** Where a chapter's link goes. */
    href: (c: Chapter) => string;
    /**
     * Handles a plain click on a chapter (the page seeks in place). Without it
     * the link is followed. Modified clicks always open the link as usual.
     */
    onOpen?: (c: Chapter) => void;
    /** The tick at the start of a row. */
    onToggle: (c: Chapter) => void;
    /** Index of the chapter being played, or -1. */
    current?: number;
    /** Length of the video; gives the last chapter its length. */
    duration?: number | null;
    /** Scroll inside the list (instead of growing) and keep the playing chapter in view. */
    follow?: boolean;
  }

  let { chapters, label, href, onOpen, onToggle, current = -1, duration = null, follow = false }: Props = $props();

  let list: HTMLElement | undefined = $state();

  const plain = (e: MouseEvent) => e.button === 0 && !e.metaKey && !e.ctrlKey && !e.shiftKey && !e.altKey;

  // Keep the chapter being played in view, without moving the page itself.
  $effect(() => {
    void current;
    if (!follow) return;
    void tick().then(() => {
      const box = list;
      const row = box?.querySelector<HTMLElement>('[data-now="true"]');
      if (!box || !row) return;
      const b = box.getBoundingClientRect();
      const r = row.getBoundingClientRect();
      if (r.top < b.top + 4 || r.bottom > b.bottom - 4) {
        box.scrollTop += r.top - b.top - (b.height - r.height) / 2;
      }
    });
  });
</script>

<ol class="chapters" class:follow aria-label={label} bind:this={list}>
  {#each chapters as c, i (chapterKey(c))}
    {@const len = chapterLength(chapters, i, duration)}
    {@const now = i === current}
    <li class="ch" class:now class:done={c.done} data-now={now ? 'true' : undefined}>
      <button
        type="button"
        class="tick"
        aria-pressed={c.done}
        aria-label="{c.done ? 'Mark not done' : 'Mark done'}: {c.title}"
        title={c.done ? 'Mark not done' : 'Mark done'}
        onclick={() => onToggle(c)}
      >
        <Icon name="check" size={13} stroke={3} />
      </button>
      <a
        class="go"
        href={href(c)}
        aria-current={now ? 'true' : undefined}
        onclick={(e) => {
          if (onOpen && plain(e)) {
            e.preventDefault();
            onOpen(c);
          }
        }}
      >
        <span class="time">{formatTime(c.start)}</span>
        <span class="name">{c.title}</span>
        {#if len > 0}<span class="len">{formatDuration(len)}</span>{/if}
      </a>
    </li>
  {/each}
</ol>

<style>
  .chapters {
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .chapters.follow {
    max-height: min(46dvh, 440px);
    overflow-y: auto;
    overscroll-behavior: contain;
  }
  .ch {
    display: flex;
    align-items: flex-start;
    gap: 4px;
    padding: 1px 6px 1px 4px;
    border-radius: var(--r-md);
    transition: background-color var(--dur);
  }
  .ch:hover {
    background: var(--surface-2);
  }
  .ch.now {
    background: var(--accent-soft);
    box-shadow: inset 3px 0 0 var(--accent);
  }

  /* the tick: an empty circle that fills when the chapter is done */
  .tick {
    flex: none;
    display: grid;
    place-items: center;
    width: 36px;
    height: 36px;
    padding: 0;
    border: 0;
    border-radius: 50%;
    background: transparent;
    cursor: pointer;
  }
  .tick :global(svg) {
    box-sizing: content-box;
    width: 13px;
    height: 13px;
    padding: 3px;
    border: 2px solid var(--border-input);
    border-radius: 50%;
    color: transparent;
    transition:
      background-color var(--dur),
      border-color var(--dur),
      color var(--dur);
  }
  .tick:hover :global(svg),
  .tick:focus-visible :global(svg) {
    border-color: var(--accent);
    color: var(--text-3);
  }
  .tick[aria-pressed='true'] :global(svg) {
    border-color: var(--ok);
    background: var(--ok);
    color: var(--surface);
  }

  .go {
    flex: 1;
    min-width: 0;
    display: flex;
    align-items: baseline;
    gap: 10px;
    padding: 9px 2px;
    color: var(--text);
    text-decoration: none;
  }
  .go:hover {
    text-decoration: none;
  }
  .time {
    flex: none;
    min-width: 3.4em;
    color: var(--text-2);
    font-size: 0.75rem;
    font-variant-numeric: tabular-nums;
  }
  .name {
    flex: 1;
    min-width: 0;
    font-size: 0.875rem;
    font-weight: 550;
    line-height: 1.35;
    overflow-wrap: anywhere;
  }
  .len {
    flex: none;
    color: var(--text-2);
    font-size: 0.75rem;
    font-variant-numeric: tabular-nums;
  }
  .ch.done .name {
    color: var(--text-2);
  }
  .ch.now .name {
    color: var(--text);
    font-weight: 700;
  }
</style>
