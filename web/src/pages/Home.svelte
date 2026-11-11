<script lang="ts">
  import { onMount } from 'svelte';
  import { api, messageOf } from '../lib/api';
  import { setTitle } from '../lib/brand';
  import { formatDuration, percent } from '../lib/format';
  import { continueSpot } from '../lib/resume';
  import { links } from '../lib/router.svelte';
  import { session } from '../lib/session.svelte';
  import type { ContinueEntry, CourseSummary } from '../lib/types';
  import CourseCover from '../components/CourseCover.svelte';
  import Icon from '../components/Icon.svelte';
  import ProgressBar from '../components/ProgressBar.svelte';

  type Filter = 'all' | 'progress' | 'new' | 'done';

  let courses = $state<CourseSummary[]>([]);
  let recent = $state<ContinueEntry[]>([]);
  let query = $state('');
  let searchInput: HTMLInputElement | undefined = $state();

  function clearSearch() {
    query = '';
    searchInput?.focus();
  }
  let filter = $state<Filter>('all');
  let loading = $state(true);
  let error = $state('');

  // Only the newest request may update the list (typing fires several).
  let latest = 0;

  async function load(q: string) {
    const mine = ++latest;
    try {
      const list = await api.courses(q);
      if (mine !== latest) return;
      courses = list;
      error = '';
    } catch (e) {
      if (mine === latest) error = messageOf(e);
    } finally {
      if (mine === latest) loading = false;
    }
  }

  onMount(() => {
    setTitle();
    api
      .continue()
      .then((r) => (recent = r))
      .catch(() => undefined); // the row is a convenience; the list below matters more
  });

  let firstRun = true;
  $effect(() => {
    const q = query.trim();
    const delay = firstRun ? 0 : 250;
    firstRun = false;
    const timer = setTimeout(() => void load(q), delay);
    return () => clearTimeout(timer);
  });

  function resumeHref(entry: ContinueEntry): string {
    const it = entry.item;
    // A finished (or missing) last item: the course page works out what is next.
    if (!it || it.state === 'done') return links.course(entry.course.id);
    return links.item(entry.course.id, it.id, continueSpot(it.kind, it.state, it.position, it.page, session.settings.resume_mode));
  }

  const isComplete = (c: CourseSummary) => c.item_count > 0 && c.done_count >= c.item_count;
  const isStarted = (c: CourseSummary) => !isComplete(c) && (c.done_count > 0 || !!c.last_seen_at);

  const counts = $derived({
    all: courses.length,
    progress: courses.filter(isStarted).length,
    new: courses.filter((c) => !isComplete(c) && !isStarted(c)).length,
    done: courses.filter(isComplete).length,
  });

  // "11 courses · 4 in progress · 1 completed". One string so no separator is lost to whitespace trimming.
  const summary = $derived(
    [
      `${counts.all} ${counts.all === 1 ? 'course' : 'courses'}`,
      counts.progress > 0 ? `${counts.progress} in progress` : '',
      counts.done > 0 ? `${counts.done} completed` : '',
    ]
      .filter(Boolean)
      .join(' · '),
  );

  const shown = $derived(
    courses.filter((c) => {
      switch (filter) {
        case 'progress':
          return isStarted(c);
        case 'new':
          return !isComplete(c) && !isStarted(c);
        case 'done':
          return isComplete(c);
        default:
          return true;
      }
    }),
  );

  const filters: Array<{ id: Filter; label: string }> = [
    { id: 'all', label: 'All' },
    { id: 'progress', label: 'In progress' },
    { id: 'new', label: 'Not started' },
    { id: 'done', label: 'Completed' },
  ];

  // The "Continue learning" rail scrolls sideways; the arrows only matter when it overflows.
  let rail: HTMLElement | undefined = $state();
  let canLeft = $state(false);
  let canRight = $state(false);

  function measureRail() {
    if (!rail) return;
    canLeft = rail.scrollLeft > 4;
    canRight = rail.scrollLeft + rail.clientWidth < rail.scrollWidth - 4;
  }

  $effect(() => {
    void recent.length;
    if (!rail) return;
    measureRail();
    const ro = new ResizeObserver(measureRail);
    ro.observe(rail);
    return () => ro.disconnect();
  });

  function scrollRail(dir: -1 | 1) {
    rail?.scrollBy({ left: dir * Math.max(240, rail.clientWidth * 0.8), behavior: 'smooth' });
  }
</script>

<header class="head rise">
  <div class="title">
    <h1>Courses</h1>
    {#if !loading && !error && courses.length > 0 && !query.trim()}
      <p class="muted">{summary}</p>
    {/if}
  </div>

  <form role="search" class="search" onsubmit={(e) => e.preventDefault()}>
    <div class="search-box">
      <label>
        <span class="sr-only">Search courses</span>
        <Icon name="search" size={18} class="lens" />
        <input class="input" type="search" placeholder="Search courses" bind:value={query} bind:this={searchInput} />
      </label>
      {#if query}
        <button type="button" class="clear" onclick={clearSearch}>
          <Icon name="x" size={16} /><span class="sr-only">Clear the search text</span>
        </button>
      {/if}
    </div>
  </form>
</header>

{#if recent.length > 0 && query.trim() === ''}
  <section class="recent rise" aria-label="Continue learning">
    <div class="section-head">
      <h2>Continue learning</h2>
      {#if canLeft || canRight}
        <div class="arrows">
          <button type="button" class="btn btn-outline btn-icon btn-sm" disabled={!canLeft} onclick={() => scrollRail(-1)}>
            <Icon name="chevron-left" size={18} /><span class="sr-only">Scroll left</span>
          </button>
          <button type="button" class="btn btn-outline btn-icon btn-sm" disabled={!canRight} onclick={() => scrollRail(1)}>
            <Icon name="chevron-right" size={18} /><span class="sr-only">Scroll right</span>
          </button>
        </div>
      {/if}
    </div>
    <ul class="rail" bind:this={rail} onscroll={measureRail}>
      {#each recent as entry, i (entry.course.id)}
        {@const pct = percent(entry.course.done_count, entry.course.item_count)}
        <li>
          <a class="resume" href={resumeHref(entry)}>
            <div class="thumb wide">
              <CourseCover course={entry.course} eager={i < 4} />
              <span class="play big" aria-hidden="true"><Icon name="play" size={22} stroke={2.5} /></span>
            </div>
            <div class="body">
              <strong class="name" title={entry.course.title}>{entry.course.title}</strong>
              {#if entry.item}
                <span class="lesson" title={entry.item.title}>{entry.item.title}</span>
              {/if}
              <div class="prog">
                <ProgressBar value={pct} size="sm" label="{entry.course.title} progress" />
                <small>{pct}% complete</small>
              </div>
            </div>
          </a>
        </li>
      {/each}
    </ul>
  </section>
{/if}

{#if error}
  <p class="alert" role="alert"><Icon name="circle-alert" size={18} />{error}</p>
{:else if loading}
  <div class="grid" aria-hidden="true">
    {#each Array(6) as _, i (i)}
      <div class="ghost">
        <div class="skeleton ghost-thumb"></div>
        <div class="skeleton ghost-line"></div>
        <div class="skeleton ghost-line short"></div>
      </div>
    {/each}
  </div>
  <p class="sr-only" role="status">Loading courses…</p>
{:else if courses.length === 0}
  <div class="empty card rise">
    <span class="empty-icon"><Icon name="courses" size={30} /></span>
    {#if query.trim()}
      <p class="empty-title">No courses match "{query.trim()}".</p>
      <button type="button" class="btn btn-outline" onclick={() => (query = '')}>Clear search</button>
    {:else}
      <p class="empty-title">No courses yet</p>
      <p class="muted">
        Put course folders in the library folder on the server; the server rescans on a timer, or an administrator can start a scan.
      </p>
    {/if}
  </div>
{:else}
  <section class="all" aria-labelledby="all-h">
    <div class="section-head">
      <h2 id="all-h" class="sr-only">All courses</h2>
      <div class="filters" role="group" aria-label="Filter courses">
        {#each filters as f (f.id)}
          <button type="button" class="pill" aria-pressed={filter === f.id} onclick={() => (filter = f.id)}>
            {f.label}<span class="count">{counts[f.id]}</span>
          </button>
        {/each}
      </div>
    </div>

    {#if shown.length === 0}
      <div class="empty card">
        <p class="empty-title">Nothing in this view.</p>
        <button type="button" class="btn btn-outline" onclick={() => (filter = 'all')}>Show all courses</button>
      </div>
    {:else}
      <ul class="grid" aria-label="All courses">
        {#each shown as c, i (c.id)}
          {@const pct = percent(c.done_count, c.item_count)}
          <li>
            <a class="course" href={links.course(c.id)}>
              <div class="thumb">
                <CourseCover course={c} eager={i < 8} />
                {#if isComplete(c)}
                  <span class="chip chip-ok badge"><Icon name="check" size={14} stroke={3} />Completed</span>
                {/if}
                <span class="play" aria-hidden="true"><Icon name="play" size={20} stroke={2.5} /></span>
                {#if pct > 0}
                  <span class="line" aria-hidden="true"><span class:done={pct >= 100} style:width="{pct}%"></span></span>
                {/if}
              </div>
              <div class="body">
                <strong class="name" title={c.title}>{c.title}</strong>
                <span class="meta">
                  <span class="pct">{pct}%</span>
                  <span class="dot" aria-hidden="true">·</span>
                  <span>{c.done_count}/{c.item_count} items</span>
                  {#if c.duration_s > 0}
                    <span class="dot" aria-hidden="true">·</span>
                    <span>{formatDuration(c.duration_s)}</span>
                  {/if}
                </span>
              </div>
            </a>
          </li>
        {/each}
      </ul>
    {/if}
  </section>
{/if}

<style>
  .head {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-end;
    justify-content: space-between;
    gap: 1.25rem 2rem;
    margin-bottom: clamp(22px, 3vw, 36px);
  }
  .title p {
    margin-top: 0.4rem;
  }

  /* search */
  .search-box {
    position: relative;
    width: min(100vw - 2 * var(--pad), 380px);
  }
  .search label {
    position: relative;
    display: block;
  }
  .search :global(.lens) {
    position: absolute;
    top: 50%;
    left: 1.05rem;
    margin-top: -9px;
    color: var(--text-3);
    pointer-events: none;
  }
  .search .input {
    min-height: 48px;
    padding-left: 2.8rem;
    padding-right: 2.9rem;
    border-radius: var(--r-pill);
  }
  /* The browser's own clear mark ignores the palette; ours sits in the same place. */
  .search .input::-webkit-search-cancel-button,
  .search .input::-webkit-search-decoration {
    -webkit-appearance: none;
    appearance: none;
    display: none;
  }
  .clear {
    position: absolute;
    top: 50%;
    right: 0.6rem;
    display: grid;
    place-items: center;
    width: 30px;
    height: 30px;
    margin-top: -15px;
    padding: 0;
    border: 0;
    border-radius: 50%;
    background: var(--surface-2);
    color: var(--text-2);
    cursor: pointer;
    transition:
      background-color var(--dur),
      color var(--dur);
  }
  .clear:hover {
    background: var(--surface-3);
    color: var(--text);
  }

  .section-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    margin-bottom: 14px;
  }
  .arrows {
    display: flex;
    gap: 0.4rem;
  }

  /* continue learning rail */
  .recent {
    margin-bottom: clamp(28px, 4vw, 48px);
  }
  .rail {
    display: flex;
    gap: 16px;
    margin: 0 calc(var(--pad) * -1);
    padding: 4px var(--pad) 14px;
    list-style: none;
    overflow-x: auto;
    scroll-snap-type: x proximity;
    scroll-padding-inline: var(--pad);
    overscroll-behavior-x: contain;
  }
  .rail li {
    flex: none;
    width: min(82vw, 340px);
    scroll-snap-align: start;
  }
  .resume,
  .course {
    display: flex;
    flex-direction: column;
    height: 100%;
    overflow: hidden;
    border: 1px solid var(--border);
    border-radius: var(--r-lg);
    background: var(--surface);
    box-shadow: var(--shadow-sm);
    color: var(--text);
    text-decoration: none;
    transition:
      transform 0.3s var(--ease),
      box-shadow 0.3s var(--ease),
      border-color var(--dur);
  }
  .resume:hover,
  .course:hover {
    transform: translateY(-4px);
    border-color: color-mix(in srgb, var(--accent) 45%, var(--border));
    box-shadow: var(--shadow-md);
    text-decoration: none;
  }
  .resume:active,
  .course:active {
    transform: translateY(-1px) scale(0.99);
  }

  .thumb {
    position: relative;
    aspect-ratio: 16 / 10;
    overflow: hidden;
    background: var(--surface-3);
  }
  .thumb.wide {
    aspect-ratio: 16 / 9;
  }
  .thumb :global(.cover > img),
  .thumb :global(.cover > .art) {
    transition: transform 0.6s var(--ease);
  }
  .resume:hover .thumb :global(.cover > img),
  .resume:hover .thumb :global(.cover > .art),
  .course:hover .thumb :global(.cover > img),
  .course:hover .thumb :global(.cover > .art) {
    transform: scale(1.06);
  }

  .play {
    position: absolute;
    right: 12px;
    bottom: 12px;
    display: grid;
    place-items: center;
    width: 44px;
    height: 44px;
    border-radius: 50%;
    background: var(--accent);
    color: var(--on-accent);
    box-shadow: 0 8px 20px -6px rgb(0 0 0 / 0.5);
    opacity: 0;
    transform: translateY(8px) scale(0.9);
    transition:
      opacity 0.25s var(--ease),
      transform 0.25s var(--ease);
  }
  .play :global(svg) {
    margin-left: 2px;
  }
  .course:hover .play,
  .course:focus-visible .play {
    opacity: 1;
    transform: none;
  }
  /* In the "continue" rail the button is always there, in the corner so the cover art stays readable. */
  .play.big {
    right: 14px;
    bottom: 14px;
    width: 52px;
    height: 52px;
    opacity: 1;
    transform: none;
    box-shadow:
      0 0 0 5px rgb(255 255 255 / 0.22),
      0 10px 28px -6px rgb(0 0 0 / 0.55);
  }
  .resume:hover .play.big {
    transform: scale(1.08);
  }

  .badge {
    position: absolute;
    top: 12px;
    left: 12px;
    box-shadow: var(--shadow-sm);
  }
  .line {
    position: absolute;
    left: 0;
    right: 0;
    bottom: 0;
    height: 5px;
    background: rgb(0 0 0 / 0.35);
  }
  .line span {
    display: block;
    height: 100%;
    background: var(--accent);
  }
  .line span.done {
    background: var(--ok);
  }

  .body {
    display: grid;
    gap: 6px;
    padding: 14px 16px 16px;
  }
  .name {
    display: -webkit-box;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    overflow: hidden;
    font-size: 1rem;
    font-weight: 650;
    line-height: 1.3;
    letter-spacing: -0.015em;
  }
  .lesson {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--text-2);
    font-size: 0.875rem;
  }
  .meta {
    display: flex;
    flex-wrap: wrap;
    gap: 0 0.4rem;
    color: var(--text-2);
    font-size: 0.8125rem;
    font-variant-numeric: tabular-nums;
  }
  .pct {
    color: var(--text);
    font-weight: 650;
  }
  .prog {
    display: grid;
    gap: 6px;
    margin-top: 6px;
  }
  .prog small {
    color: var(--text-2);
    font-variant-numeric: tabular-nums;
  }

  /* filters */
  .filters {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
  }
  .pill {
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
    min-height: 38px;
    padding: 0 0.95rem;
    border: 1px solid var(--border);
    border-radius: var(--r-pill);
    background: var(--surface);
    color: var(--text-2);
    font: 600 0.875rem/1 var(--font);
    cursor: pointer;
    transition:
      background-color var(--dur),
      border-color var(--dur),
      color var(--dur);
  }
  .pill:hover {
    border-color: var(--border-input);
    color: var(--text);
  }
  .pill[aria-pressed='true'] {
    border-color: var(--accent);
    background: var(--accent-soft);
    color: var(--text);
  }
  .count {
    min-width: 1.4em;
    padding: 0.18em 0.5em;
    border-radius: var(--r-pill);
    background: var(--surface-2);
    font-size: 0.75rem;
    font-variant-numeric: tabular-nums;
    text-align: center;
  }
  .pill[aria-pressed='true'] .count {
    background: var(--accent);
    color: var(--on-accent);
  }

  /* the gallery */
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(min(100%, 272px), 1fr));
    gap: clamp(16px, 2vw, 26px);
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .grid li {
    min-width: 0;
  }
  .ghost {
    display: grid;
    gap: 12px;
  }
  .ghost-thumb {
    aspect-ratio: 16 / 10;
    border-radius: var(--r-lg);
  }
  .ghost-line {
    height: 16px;
    width: 85%;
  }
  .ghost-line.short {
    width: 50%;
  }

  /* empty states */
  .empty {
    display: grid;
    justify-items: center;
    gap: 0.75rem;
    padding: clamp(32px, 7vw, 72px) 24px;
    text-align: center;
  }
  .empty p {
    max-width: 46ch;
  }
  .empty-title {
    font-size: 1.125rem;
    font-weight: 650;
  }
  .empty-icon {
    display: grid;
    place-items: center;
    width: 72px;
    height: 72px;
    margin-bottom: 0.25rem;
    border-radius: 50%;
    background: var(--accent-soft);
    color: var(--accent-text);
  }
</style>
