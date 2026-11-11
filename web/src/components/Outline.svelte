<script lang="ts">
  import { onMount, tick, untrack } from 'svelte';
  import { SvelteSet } from 'svelte/reactivity';
  import { doneCount, isFlat, itemMeta, kindIcon, kindLabel, sectionStats, statusKey } from '../lib/actions';
  import { formatDuration, percent } from '../lib/format';
  import { recall, remember } from '../lib/outline.svelte';
  import { links } from '../lib/router.svelte';
  import type { Chapter, Item, Section } from '../lib/types';
  import ChapterList from './ChapterList.svelte';
  import Icon from './Icon.svelte';
  import ProgressBar from './ProgressBar.svelte';
  import StatusToggle from './StatusToggle.svelte';

  interface Props {
    courseId: number;
    title: string;
    sections: Section[];
    /** The lesson being viewed; it is highlighted and kept in view. */
    currentId: number;
    /** An item whose done-toggle is being saved. */
    busyId?: number | null;
    onToggleDone: (item: Item) => void;
    /** Given on small screens, where the outline is a drawer that needs a close button. */
    onClose?: () => void;
    /** True while the outline can be seen; the current lesson is scrolled into view when it turns true. */
    shown?: boolean;
    /** Index of the chapter playing in the current lesson, or -1. */
    chapterNow?: number;
    /** A plain click on a chapter of the current lesson. */
    onChapterOpen?: (c: Chapter) => void;
    /** The tick on a chapter of the current lesson. */
    onToggleChapter?: (c: Chapter) => void;
  }

  let {
    courseId,
    title,
    sections,
    currentId,
    busyId = null,
    onToggleDone,
    onClose,
    shown = true,
    chapterNow = -1,
    onChapterOpen,
    onToggleChapter,
  }: Props = $props();

  // A course that is only a list of files has no sections worth showing.
  const flat = $derived(isFlat(sections));

  const counts = $derived(doneCount(sections));
  const pct = $derived(percent(counts.done, counts.total));

  const currentSection = (): number | undefined =>
    sections.find((s) => s.items.some((i) => i.id === currentId))?.id;

  // Sections the person opened stay open from lesson to lesson; the one being watched is always open.
  const open = new SvelteSet<number>(untrack(() => recall(courseId)?.expanded ?? []));
  {
    const here = currentSection();
    if (here !== undefined) open.add(here);
  }

  const allOpen = $derived(sections.every((s) => open.has(s.id)));

  function toggle(id: number) {
    if (open.has(id)) open.delete(id);
    else open.add(id);
    remember(courseId, { expanded: [...open] });
  }

  function toggleAll() {
    if (allOpen) {
      open.clear();
      const here = currentSection();
      if (here !== undefined) open.add(here); // keep your place
    } else {
      for (const s of sections) open.add(s.id);
    }
    remember(courseId, { expanded: [...open] });
  }

  let scroller: HTMLElement | undefined = $state();
  let lastTop = 0;

  /** Brings the current lesson (or the chapter being played) to the middle of the list if it is out of sight. */
  function reveal() {
    const el =
      scroller?.querySelector<HTMLElement>('[data-now="true"]') ??
      scroller?.querySelector<HTMLElement>('[aria-current="true"]');
    if (!scroller || !el) return;
    const s = scroller.getBoundingClientRect();
    const r = el.getBoundingClientRect();
    if (r.top < s.top + 8 || r.bottom > s.bottom - 8) {
      scroller.scrollTop += r.top - s.top - (s.height - r.height) / 2;
    }
  }

  onMount(() => {
    const m = recall(courseId);
    if (scroller && m) scroller.scrollTop = m.scrollTop;
    void tick().then(reveal);
    return () => remember(courseId, { scrollTop: lastTop });
  });

  $effect(() => {
    void chapterNow; // follow the video from chapter to chapter
    if (shown) void tick().then(reveal);
  });
</script>

<div class="outline">
  <div class="head">
    <div class="head-top">
      <h2 class="label">Course content</h2>
      {#if onClose}
        <button type="button" class="btn btn-ghost btn-icon btn-sm" onclick={onClose}>
          <Icon name="x" size={18} /><span class="sr-only">Close course content</span>
        </button>
      {/if}
    </div>
    <a class="course" href={links.course(courseId)} title="Course overview">{title}</a>
    <div class="prog">
      <ProgressBar value={pct} size="sm" label="Course progress" />
      <small>{pct}% complete · {counts.done}/{counts.total}</small>
    </div>
    {#if !flat && sections.length > 2}
      <button type="button" class="expand" onclick={toggleAll}>
        <Icon name={allOpen ? 'chevron-up' : 'chevron-down'} size={14} />{allOpen ? 'Collapse all' : 'Expand all'}
      </button>
    {/if}
  </div>

  <div class="scroller" bind:this={scroller} onscroll={() => (lastTop = scroller?.scrollTop ?? 0)}>
    {#if flat}
      <ol class="items flat" aria-label="Lessons">
        {@render rows(sections[0].items)}
      </ol>
    {:else}
      {#each sections as section (section.id)}
        {@const st = sectionStats(section)}
        {@const isOpen = open.has(section.id)}
        {@const meta = st.seconds > 0 ? `${st.done}/${st.total} · ${formatDuration(st.seconds)}` : `${st.done}/${st.total}`}
        <section class="sec">
          <h3>
            <button type="button" class="sec-btn" aria-expanded={isOpen} aria-controls="sec-{section.id}" onclick={() => toggle(section.id)}>
              <span class="sec-text">
                <span class="sec-title">{section.title}</span>
                <span class="sec-meta">
                  {#if st.done === st.total && st.total > 0}<Icon name="check" size={13} stroke={3} />{/if}
                  {meta}
                </span>
              </span>
              <Icon name="chevron-down" size={18} class="chev" />
            </button>
          </h3>
          <ol id="sec-{section.id}" class="items" hidden={!isOpen}>
            {@render rows(section.items)}
          </ol>
        </section>
      {/each}
    {/if}
  </div>
</div>

{#snippet rows(items: Item[])}
  {#each items as item (item.id)}
    {@const current = item.id === currentId}
    {@const info = itemMeta(item)}
    <li class="item" class:current class:chaptered={current && !!item.chapters?.length} data-status={statusKey(item)}>
      <div class="row">
        <StatusToggle {item} size={22} busy={busyId === item.id} onToggle={onToggleDone} />
        <a class="open" href={links.item(courseId, item.id)} aria-current={current ? 'true' : undefined}>
          <span class="t">{item.title}</span>
          <span class="m">
            <Icon name={kindIcon(item.kind)} size={13} />{kindLabel(item)}{info ? ` · ${info}` : ''}
          </span>
        </a>
      </div>
      {#if current && item.chapters?.length}
        <div class="nested">
          <ChapterList
            chapters={item.chapters}
            label="Chapters of {item.title}"
            href={(c) => links.chapter(courseId, item.id, c.start)}
            onOpen={onChapterOpen}
            onToggle={(c) => onToggleChapter?.(c)}
            current={chapterNow}
            duration={item.duration ?? item.progress?.duration ?? null}
          />
        </div>
      {/if}
    </li>
  {/each}
{/snippet}

<style>
  .outline {
    display: flex;
    flex-direction: column;
    height: 100%;
    min-height: 0;
  }
  .head {
    display: grid;
    gap: 10px;
    padding: 18px 18px 14px;
    border-bottom: 1px solid var(--border);
  }
  .head-top {
    display: flex;
    align-items: center;
    justify-content: space-between;
    min-height: 28px;
  }
  .label {
    font-size: 0.75rem;
    font-weight: 650;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--text-3);
  }
  .course {
    display: -webkit-box;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    overflow: hidden;
    color: var(--text);
    font-size: 1.0625rem;
    font-weight: 700;
    line-height: 1.3;
    letter-spacing: -0.02em;
    text-decoration: none;
  }
  .course:hover {
    color: var(--accent-text);
    text-decoration: underline;
  }
  .prog {
    display: grid;
    gap: 6px;
  }
  .prog small {
    color: var(--text-2);
    font-variant-numeric: tabular-nums;
  }
  .expand {
    justify-self: start;
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    margin: 0 0 -4px -6px;
    padding: 4px 10px 4px 8px;
    border: 0;
    border-radius: var(--r-pill);
    background: transparent;
    color: var(--text-2);
    font: 600 0.8125rem/1 var(--font);
    cursor: pointer;
  }
  .expand:hover {
    background: var(--surface-2);
    color: var(--text);
  }

  .scroller {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    overscroll-behavior: contain;
    padding-bottom: calc(12px + env(safe-area-inset-bottom));
  }
  .sec + .sec {
    border-top: 1px solid var(--border);
  }
  .sec h3 {
    font-size: inherit;
    letter-spacing: normal;
  }
  .sec-btn {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    width: 100%;
    padding: 14px 18px;
    border: 0;
    background: transparent;
    color: var(--text);
    font: inherit;
    text-align: left;
    cursor: pointer;
    transition: background-color var(--dur);
  }
  .sec-btn:hover {
    background: var(--surface-2);
  }
  .sec-text {
    display: grid;
    gap: 2px;
    min-width: 0;
  }
  .sec-title {
    font-size: 0.9375rem;
    font-weight: 650;
    line-height: 1.3;
    overflow-wrap: anywhere;
  }
  .sec-meta {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    color: var(--text-2);
    font-size: 0.75rem;
    font-variant-numeric: tabular-nums;
  }
  .sec-btn :global(.chev) {
    flex: none;
    color: var(--text-2);
    transition: transform 0.25s var(--ease);
  }
  .sec-btn[aria-expanded='true'] :global(.chev) {
    transform: rotate(180deg);
  }

  .items {
    margin: 0;
    padding: 0 8px 8px;
    list-style: none;
  }
  .items.flat {
    padding-top: 8px;
  }
  .row {
    display: flex;
    align-items: flex-start;
    gap: 2px;
    padding: 2px 6px 2px 2px;
    border-radius: var(--r-md);
    transition: background-color var(--dur);
  }
  .row:hover {
    background: var(--surface-2);
  }
  .item.current > .row {
    background: var(--accent-soft);
    box-shadow: inset 3px 0 0 var(--accent);
  }
  /* with its chapters listed below, the video itself only keeps the bar: the chapter being played gets the fill */
  .item.chaptered > .row:not(:hover) {
    background: transparent;
  }
  /* the chapters of the lesson being watched, tucked under it */
  .nested {
    margin: 2px 0 6px 20px;
    padding-left: 6px;
    border-left: 2px solid var(--border);
  }
  .item :global(.st) {
    margin-top: 5px;
  }
  .open {
    flex: 1;
    min-width: 0;
    display: grid;
    gap: 3px;
    padding: 9px 4px 9px 2px;
    color: var(--text);
    text-decoration: none;
  }
  .open:hover {
    text-decoration: none;
  }
  .t {
    font-size: 0.875rem;
    font-weight: 550;
    line-height: 1.35;
    overflow-wrap: anywhere;
  }
  .current .t {
    font-weight: 700;
  }
  .item[data-status='done'] .t {
    color: var(--text-2);
  }
  .current[data-status='done'] .t {
    color: var(--text);
  }
  .m {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    color: var(--text-2);
    font-size: 0.75rem;
    font-variant-numeric: tabular-nums;
  }
</style>
