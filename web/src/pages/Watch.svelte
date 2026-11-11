<script lang="ts">
  import { onMount, tick, untrack } from 'svelte';
  import { MediaQuery } from 'svelte/reactivity';
  import { findItem, flatten, followItem, isDone, isFlat, kindIcon, kindLabel, setDone } from '../lib/actions';
  import { api, messageOf, url } from '../lib/api';
  import { requestAutoplay, takeAutoplay } from '../lib/autoplay';
  import { setTitle } from '../lib/brand';
  import {
    ChapterMarks,
    chapterIndexAt,
    chapterKey,
    doneChapters,
    mergeProgress,
    paintChapters,
  } from '../lib/chapters';
  import { formatTime } from '../lib/format';
  import { recall, remember, setSidebarOpen, sidebar } from '../lib/outline.svelte';
  import { ProgressReporter } from '../lib/progress';
  import { decideStart, type Start } from '../lib/resume';
  import { go, links } from '../lib/router.svelte';
  import { saveSettings, session } from '../lib/session.svelte';
  import type { Chapter, ChaptersReply, CourseTree, Item, ItemDetail, Progress } from '../lib/types';
  import ChapterList from '../components/ChapterList.svelte';
  import Icon from '../components/Icon.svelte';
  import ImageViewer from '../components/ImageViewer.svelte';
  import LinkCard from '../components/LinkCard.svelte';
  import NoPreview from '../components/NoPreview.svelte';
  import Outline from '../components/Outline.svelte';
  import PdfViewer from '../components/PdfViewer.svelte';
  import TextViewer from '../components/TextViewer.svelte';
  import VideoPlayer from '../components/VideoPlayer.svelte';

  interface Props {
    courseId: number;
    itemId: number;
    /** Start time in seconds, from the link. */
    t?: number;
    /** Start page, from the link. */
    p?: number;
  }

  // The page is re-created for every item (see App.svelte), so these never change.
  let { courseId, itemId, t, p }: Props = $props();

  let detail = $state<ItemDetail | null>(null);
  // The outline of the last lesson page of this course is reused so the sidebar does not blink.
  // (courseId never changes for a given page instance, so reading it once is right.)
  let tree = $state<CourseTree | null>(untrack(() => recall(courseId)?.tree ?? null));
  let error = $state('');
  let start = $state<Start | null>(null);
  let offer = $state(false);
  let autoplay = $state(false);
  let reporter = $state.raw<ProgressReporter | null>(null);
  let player: ReturnType<typeof VideoPlayer> | undefined = $state();
  let pdf: ReturnType<typeof PdfViewer> | undefined = $state();
  let busy = $state(false);
  let otherBusy = $state<number | null>(null);
  let actionError = $state('');
  let countdown = $state(0);
  let time = $state(0); // where the video is now, for the chapter being played
  let marks: ChapterMarks | null = null;

  let countdownTimer: ReturnType<typeof setInterval> | undefined;
  let rateTimer: ReturnType<typeof setTimeout> | undefined;

  const flat = $derived(tree ? flatten(tree.sections) : []);
  const index = $derived(flat.findIndex((i) => i.id === itemId));
  const prev = $derived(index > 0 ? flat[index - 1] : undefined);
  const next = $derived(index >= 0 && index < flat.length - 1 ? flat[index + 1] : undefined);

  // Chapters of a long video: each is a lesson of its own, with its own progress.
  const chapters = $derived(detail?.item.chapters ?? []);
  const nowIndex = $derived(chapterIndexAt(chapters, time));

  // The course-content sidebar sits beside the lesson on wide screens
  // (open or closed, remembered per device) and is a slide-in drawer on narrow ones.
  const wide = new MediaQuery('min-width: 1024px');
  let drawer = $state(false);
  const sideOpen = $derived(wide.current ? sidebar.open : drawer);
  let toggleButton: HTMLButtonElement | undefined = $state();
  let sideEl: HTMLElement | undefined = $state();

  function toggleSide() {
    if (wide.current) {
      setSidebarOpen(!sidebar.open);
      return;
    }
    drawer = !drawer;
    if (drawer) void tick().then(() => sideEl?.focus());
  }

  function closeDrawer() {
    drawer = false;
    // The lesson is inert while the drawer is open; focus can only go back to its button once that is lifted.
    void tick().then(() => toggleButton?.focus());
  }

  onMount(() => {
    let cancelled = false;
    const rep = new ProgressReporter(itemId, { onProgress: applyProgress });
    const mk = new ChapterMarks(itemId, { onReply: applyChapters, onError: (m) => (actionError = m) });
    marks = mk;

    (async () => {
      try {
        const [d, c] = await Promise.all([api.item(itemId), api.course(courseId)]);
        if (cancelled) return;
        if (d.item.course_id !== courseId) throw new Error('This item is not part of that course.');
        const auto = takeAutoplay(itemId);
        // Autoplay cannot stop to ask, so it continues from the saved spot.
        const mode = auto && session.settings.resume_mode === 'ask' ? 'auto' : session.settings.resume_mode;
        const s = decideStart(d.item, mode, { t, p });
        detail = d;
        tree = c;
        remember(courseId, { tree: c });
        start = s;
        time = s.at;
        offer = s.offer;
        autoplay = auto;
        reporter = rep;
        setTitle(d.item.title, c.course.title);
      } catch (e) {
        if (!cancelled) error = messageOf(e);
      }
    })();

    return () => {
      cancelled = true;
      rep.destroy(); // one last position report
      mk.leave(); // and any chapter not yet confirmed
      mk.destroy();
      clearInterval(countdownTimer);
      clearTimeout(rateTimer);
    };
  });

  /** Takes the server's merged progress into the page and the outline. */
  function applyProgress(pr: Progress) {
    if (detail) detail.item.progress = pr;
    if (tree) {
      const it = findItem(tree.sections, itemId);
      if (it) it.progress = pr;
    }
  }

  /** Takes the server's answer about chapters into the page and the outline. */
  function applyChapters(r: ChaptersReply) {
    const pending = (k: number) => marks?.pending(k);
    if (detail) {
      paintChapters(detail.item.chapters, r.chapters, pending);
      detail.item.progress = mergeProgress(detail.item.progress, r.progress);
    }
    const it = tree && findItem(tree.sections, itemId);
    if (it) {
      paintChapters(it.chapters, r.chapters, pending);
      it.progress = mergeProgress(it.progress, r.progress);
    }
  }

  /** Shows a chapter as finished (or not) right away and tells the server a moment later. */
  function setChapter(key: number, done: boolean) {
    const lists = [detail?.item.chapters, tree ? findItem(tree.sections, itemId)?.chapters : undefined];
    for (const list of lists) {
      const c = list?.find((x) => chapterKey(x) === key);
      if (c) c.done = done;
    }
    marks?.mark(key, done);
  }

  /** The video ran through the end of a chapter. */
  function chapterPlayed(c: Chapter) {
    if (!c.done) setChapter(chapterKey(c), true);
  }

  /** A click on a chapter of the video being watched: jump there and play. */
  function openChapter(c: Chapter) {
    offer = false;
    time = c.start;
    player?.playAt(c.start);
    if (!wide.current && drawer) closeDrawer();
  }

  function dismissOffer() {
    offer = false;
  }

  function resume() {
    if (!start) return;
    offer = false;
    if (detail?.item.kind === 'pdf') pdf?.goTo(start.savedPage);
    else player?.seekTo(start.savedAt);
  }

  function startOver() {
    offer = false;
    if (detail?.item.kind === 'pdf') {
      pdf?.goTo(1);
    } else {
      player?.seekTo(0);
      reporter?.report('seek', { position: 0 }); // moving to 0 may not raise a seek event
    }
  }

  function persistRate(rate: number) {
    if (Math.abs(rate - session.settings.playback_rate) < 0.001) return;
    clearTimeout(rateTimer);
    rateTimer = setTimeout(() => {
      void saveSettings({ playback_rate: rate }).catch(() => undefined);
    }, 800);
  }

  function handleEnded() {
    if (!next || !session.settings.autoplay_next) return;
    countdown = 5;
    clearInterval(countdownTimer);
    countdownTimer = setInterval(() => {
      countdown -= 1;
      if (countdown <= 0) playNext();
    }, 1000);
  }

  function playNext() {
    clearInterval(countdownTimer);
    countdown = 0;
    if (!next) return;
    requestAutoplay(next.id);
    go(links.item(courseId, next.id));
  }

  function cancelCountdown() {
    clearInterval(countdownTimer);
    countdown = 0;
  }

  async function toggleDone() {
    const d = detail;
    if (!d || !reporter) return;
    const want = !isDone(d.item);
    busy = true;
    actionError = '';
    try {
      // A player sitting at the end must not quietly complete it again.
      if (!want) reporter.hold();
      applyProgress(await setDone(d.item.id, want));
      followItem(d.item, want);
      const t = tree && findItem(tree.sections, d.item.id);
      if (t) followItem(t, want);
    } catch (e) {
      actionError = messageOf(e);
    } finally {
      busy = false;
    }
  }

  async function toggleOutlineItem(item: Item) {
    if (item.id === itemId) return toggleDone();
    otherBusy = item.id;
    actionError = '';
    try {
      const want = !isDone(item);
      item.progress = await setDone(item.id, want);
      followItem(item, want);
    } catch (e) {
      actionError = messageOf(e);
    } finally {
      otherBusy = null;
    }
  }

  function flushNow() {
    reporter?.flush();
    marks?.leave();
  }
</script>

<!-- When the tab is closed or hidden, leave the position behind with a beacon. -->
<svelte:window
  onpagehide={flushNow}
  onkeydown={(e) => {
    if (e.key === 'Escape' && drawer && !wide.current) closeDrawer();
  }}
/>
<svelte:document
  onvisibilitychange={() => {
    if (document.visibilityState === 'hidden') flushNow();
  }}
/>

<div class="watch">
  {#if !error}
    <aside
      id="course-outline"
      class="side"
      class:open={sideOpen}
      aria-label="Course content"
      tabindex="-1"
      inert={!sideOpen}
      bind:this={sideEl}
    >
      <div class="side-inner">
        {#if tree}
          <Outline
            {courseId}
            title={tree.course.title}
            sections={tree.sections}
            currentId={itemId}
            busyId={busy ? itemId : otherBusy}
            onToggleDone={toggleOutlineItem}
            onClose={wide.current ? undefined : closeDrawer}
            shown={sideOpen}
            chapterNow={nowIndex}
            onChapterOpen={openChapter}
            onToggleChapter={(c) => setChapter(chapterKey(c), !c.done)}
          />
        {:else}
          <div class="side-ghost" aria-hidden="true">
            <div class="skeleton" style="height: 20px; width: 40%"></div>
            <div class="skeleton" style="height: 22px; width: 85%"></div>
            <div class="skeleton" style="height: 8px"></div>
            {#each Array(6) as _, i (i)}
              <div class="skeleton" style="height: 44px"></div>
            {/each}
          </div>
        {/if}
      </div>
    </aside>
    {#if drawer && !wide.current}
      <button type="button" class="scrim" tabindex="-1" aria-label="Close course content" onclick={closeDrawer}></button>
    {/if}
  {/if}

  <main id="main" tabindex="-1" class="stage" inert={!wide.current && drawer}>
    <div class="stage-in">
      {#if error}
        <p class="alert" role="alert"><Icon name="circle-alert" size={18} />{error}</p>
        <p><a class="btn btn-outline" href={links.course(courseId)}><Icon name="arrow-left" size={18} />Back to the course</a></p>
      {:else if !detail || !tree || !start || !reporter}
        <div class="bar">
          <div class="skeleton" style="height: 36px; width: 150px; border-radius: 999px"></div>
        </div>
        <div class="skeleton" style="aspect-ratio: 16 / 9; border-radius: var(--r-xl)"></div>
        <div class="skeleton" style="height: 34px; width: 55%"></div>
        <p class="sr-only" role="status">Loading…</p>
      {:else}
        {@const it = detail.item}
        {@const st = start}
        {@const rp = reporter}
        {@const tr = tree}

        <div class="bar">
          <button
            bind:this={toggleButton}
            type="button"
            class="btn btn-outline btn-sm"
            aria-expanded={sideOpen}
            aria-controls="course-outline"
            onclick={toggleSide}
          >
            <Icon name={sideOpen ? 'sidebar-close' : 'sidebar-open'} size={18} />
            <span class="bar-label">Course content</span>
          </button>
          <a class="crumb" href={links.course(courseId)} title="Course overview">{tr.course.title}</a>
          <nav class="lesson-nav" aria-label="Lesson navigation">
            {#if prev}
              <a class="btn btn-outline btn-sm" href={links.item(courseId, prev.id)} rel="prev" title={prev.title}>
                <Icon name="arrow-left" size={16} /><span class="nav-label">Previous</span>
                <span class="sr-only">: {prev.title}</span>
              </a>
            {/if}
            {#if next}
              <a class="btn btn-primary btn-sm" href={links.item(courseId, next.id)} rel="next" title={next.title}>
                <span class="nav-label">Next</span><Icon name="arrow-right" size={16} />
                <span class="sr-only">: {next.title}</span>
              </a>
            {/if}
          </nav>
        </div>

        {#if offer}
          <section class="offer" aria-label="Resume">
            <p>
              {#if it.kind === 'pdf'}
                Continue from page {st.savedPage}?
              {:else}
                {@const at = chapterIndexAt(chapters, st.savedAt)}
                Continue from {formatTime(st.savedAt)}{at >= 0 ? ` (${chapters[at].title})` : ''}?
              {/if}
            </p>
            <div class="offer-actions">
              <button type="button" class="btn btn-primary btn-sm" onclick={resume}>Resume</button>
              <button type="button" class="btn btn-ghost btn-sm" onclick={startOver}>Start over</button>
            </div>
          </section>
        {/if}

        {#if it.kind === 'video' || it.kind === 'audio'}
          <VideoPlayer
            bind:this={player}
            item={it}
            reporter={rp}
            startAt={st.at}
            {autoplay}
            rate={session.settings.playback_rate}
            onRate={persistRate}
            onEnded={handleEnded}
            onPlay={dismissOffer}
            onTakeover={cancelCountdown}
            shortcuts={wide.current || !drawer}
            onChapter={chapterPlayed}
            onTime={(s) => (time = s)}
          />
        {:else if it.kind === 'pdf'}
          <PdfViewer bind:this={pdf} item={it} reporter={rp} startPage={st.page} {offer} onPage={dismissOffer} />
        {:else if it.kind === 'image'}
          <ImageViewer item={it} reporter={rp} />
        {:else if it.kind === 'markdown' || it.kind === 'text' || it.kind === 'csv'}
          <TextViewer item={it} reporter={rp} />
        {:else if it.kind === 'link'}
          <LinkCard item={it} reporter={rp} />
        {:else}
          <NoPreview item={it} reporter={rp} />
        {/if}

        {#if countdown > 0 && next}
          <div class="countdown" role="status">
            <p>
              Playing "{next.title}" in {countdown}…
            </p>
            <div class="cd-actions">
              <button type="button" class="btn btn-primary btn-sm" onclick={playNext}>Play now</button>
              <button type="button" class="btn btn-ghost btn-sm" onclick={cancelCountdown}>Cancel</button>
            </div>
            <span class="cd-bar" aria-hidden="true"></span>
          </div>
        {/if}

        <header class="lesson-head">
          <h1>{it.title}</h1>
          {#if nowIndex >= 0}
            <p class="now">
              <span class="now-n">Chapter {nowIndex + 1} of {chapters.length}</span>
              <span class="now-t">{chapters[nowIndex].title}</span>
            </p>
          {/if}
          <div class="tags">
            {#if !isFlat(tr.sections)}
              <span class="chip">{detail.section_title}</span>
            {/if}
            <span class="chip"><Icon name={kindIcon(it.kind)} size={14} />{kindLabel(it)}</span>
            {#if chapters.length > 0 && !isDone(it)}
              <span class="chip"><Icon name="list" size={14} />{doneChapters(chapters)}/{chapters.length} chapters done</span>
            {/if}
            {#if isDone(it)}
              <span class="chip chip-ok"><Icon name="check" size={14} stroke={3} />Completed</span>
            {/if}
          </div>
        </header>

        <p class="actions">
          <button type="button" class="btn {isDone(it) ? 'btn-outline' : 'btn-primary'}" disabled={busy} onclick={toggleDone}>
            <Icon name="check" size={18} />{isDone(it) ? 'Mark not done' : chapters.length > 0 ? 'Mark all done' : 'Mark done'}
          </button>
          {#if it.kind !== 'link'}
            <a class="btn btn-outline" href={url.download(it.id)} download>
              <Icon name="download" size={18} />Download
            </a>
          {/if}
        </p>
        {#if actionError}
          <p class="alert" role="alert"><Icon name="circle-alert" size={18} />{actionError}</p>
        {/if}

        <!-- With the course content open beside the video, the chapters are listed there. -->
        {#if chapters.length > 0 && !sideOpen}
          <section class="chapters card" aria-labelledby="chapters-h">
            <header class="ch-head">
              <h2 id="chapters-h">Chapters</h2>
              <span class="ch-count">{doneChapters(chapters)}/{chapters.length} done</span>
            </header>
            <ChapterList
              {chapters}
              label="Chapters of {it.title}"
              href={(c) => links.chapter(courseId, it.id, c.start)}
              onOpen={openChapter}
              onToggle={(c) => setChapter(chapterKey(c), !c.done)}
              current={nowIndex}
              duration={it.duration ?? it.progress?.duration ?? null}
              follow
            />
          </section>
        {/if}

        {#if !next}
          <div class="last card">
            <span class="last-icon"><Icon name="trophy" size={22} /></span>
            <a href={links.course(courseId)}>That was the last item. Back to the course.</a>
          </div>
        {/if}
      {/if}
    </div>
  </main>
</div>

<style>
  .watch {
    display: flex;
    align-items: flex-start;
    min-height: calc(100dvh - var(--nav-h));
  }

  /* ----- the course-content sidebar ----- */
  .side {
    flex: none;
    position: sticky;
    top: var(--nav-h);
    width: 0;
    height: calc(100dvh - var(--nav-h));
    overflow: hidden;
    background: var(--surface);
    visibility: hidden;
    transition:
      width 0.32s var(--ease),
      visibility 0s 0.32s;
  }
  .side.open {
    width: var(--sidebar-w);
    visibility: visible;
    box-shadow: 1px 0 0 var(--border);
    transition:
      width 0.32s var(--ease),
      visibility 0s;
  }
  .side:focus {
    outline: none;
  }
  .side-inner {
    width: var(--sidebar-w);
    height: 100%;
  }
  .side-ghost {
    display: grid;
    gap: 12px;
    padding: 22px 18px;
  }
  .scrim {
    display: none;
  }

  @media (max-width: 1023.98px) {
    .side {
      position: fixed;
      top: var(--nav-h);
      bottom: 0;
      left: 0;
      z-index: 40;
      width: min(92vw, 410px);
      height: auto;
      transform: translateX(-104%);
      box-shadow: var(--shadow-lg);
      transition:
        transform 0.32s var(--ease),
        visibility 0s 0.32s;
    }
    .side.open {
      width: min(92vw, 410px);
      transform: none;
      transition:
        transform 0.32s var(--ease),
        visibility 0s;
    }
    .side-inner {
      width: 100%;
    }
    .scrim {
      display: block;
      position: fixed;
      inset: var(--nav-h) 0 0 0;
      z-index: 39;
      border: 0;
      padding: 0;
      background: var(--scrim);
      animation: fade 0.25s ease both;
    }
  }
  @keyframes fade {
    from {
      opacity: 0;
    }
  }

  /* ----- the lesson ----- */
  .stage {
    flex: 1;
    min-width: 0;
  }
  .stage:focus {
    outline: none;
  }
  .stage-in {
    display: grid;
    gap: 18px;
    max-width: 1240px;
    margin-inline: auto;
    padding: clamp(12px, 2vw, 24px) clamp(14px, 3vw, 40px) 96px;
  }
  .bar {
    display: flex;
    align-items: center;
    gap: 12px;
    min-width: 0;
  }
  .crumb {
    min-width: 0;
    overflow: hidden;
    color: var(--text-2);
    font-size: 0.875rem;
    font-weight: 600;
    text-decoration: none;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .crumb:hover {
    color: var(--accent-text);
    text-decoration: underline;
  }
  .lesson-nav {
    display: flex;
    gap: 8px;
    margin-left: auto;
  }

  .offer {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 8px 16px;
    padding: 10px 12px 10px 20px;
    border: 1px solid color-mix(in srgb, var(--accent) 40%, var(--border));
    border-radius: var(--r-pill);
    background: var(--accent-soft);
    font-weight: 600;
  }
  .offer-actions {
    display: flex;
    gap: 6px;
  }

  .countdown {
    position: relative;
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 8px 16px;
    overflow: hidden;
    padding: 10px 12px 12px 20px;
    border-radius: var(--r-lg);
    background: var(--surface);
    border: 1px solid var(--border);
    box-shadow: var(--shadow-md);
    font-weight: 600;
  }
  .cd-actions {
    display: flex;
    gap: 6px;
  }
  .cd-bar {
    position: absolute;
    left: 0;
    right: 0;
    bottom: 0;
    height: 3px;
    background: var(--accent);
    transform-origin: left;
    animation: drain 5s linear forwards;
  }
  @keyframes drain {
    to {
      transform: scaleX(0);
    }
  }

  .lesson-head {
    display: grid;
    gap: 12px;
  }
  .lesson-head h1 {
    font-size: clamp(1.4rem, 1rem + 1.4vw, 2rem);
    overflow-wrap: anywhere;
  }
  .now {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 0.25rem 0.6rem;
    margin: -4px 0 0;
    font-weight: 600;
  }
  .now-n {
    color: var(--accent-text);
    font-size: 0.8125rem;
    font-variant-numeric: tabular-nums;
  }
  .now-t {
    min-width: 0;
    color: var(--text-2);
    overflow-wrap: anywhere;
  }
  .tags {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
  }
  .chapters {
    display: grid;
    gap: 6px;
    padding: 14px 10px 10px;
  }
  .ch-head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 0.5rem 1rem;
    padding: 0 10px;
  }
  .ch-head h2 {
    font-size: 1rem;
  }
  .ch-count {
    color: var(--text-2);
    font-size: 0.8125rem;
    font-variant-numeric: tabular-nums;
  }
  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.6rem;
  }
  .last {
    display: flex;
    align-items: center;
    gap: 0.9rem;
    padding: 14px 18px;
    font-weight: 600;
  }
  .last-icon {
    display: grid;
    place-items: center;
    width: 44px;
    height: 44px;
    border-radius: 50%;
    background: var(--accent-soft);
    color: var(--accent-text);
  }

  @media (max-width: 640px) {
    .bar-label,
    .crumb,
    .nav-label {
      position: absolute;
      width: 1px;
      height: 1px;
      overflow: hidden;
      clip-path: inset(50%);
      white-space: nowrap;
    }
    .bar :global(.btn-sm) {
      padding-inline: 0;
      width: 40px;
      min-height: 40px;
    }
    .offer {
      border-radius: var(--r-lg);
    }
  }
</style>
