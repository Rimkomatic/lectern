<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { doneCount, findItem, flatten, hasTracking, isDone, setDone } from '../lib/actions';
  import { api, messageOf } from '../lib/api';
  import { setTitle } from '../lib/brand';
  import { chapterIndexAt, chapterKey } from '../lib/chapters';
  import { formatDuration, formatTime, percent } from '../lib/format';
  import { COVER_ACCEPT, CoverError, prepareCover } from '../lib/image';
  import { continueSpot } from '../lib/resume';
  import { links } from '../lib/router.svelte';
  import { session } from '../lib/session.svelte';
  import type { Chapter, CourseTree, Item } from '../lib/types';
  import CourseCover from '../components/CourseCover.svelte';
  import Curriculum from '../components/Curriculum.svelte';
  import Icon from '../components/Icon.svelte';
  import ProgressBar from '../components/ProgressBar.svelte';

  let { courseId }: { courseId: number } = $props();

  let tree = $state<CourseTree | null>(null);
  let error = $state('');
  let busyId = $state<number | null>(null);
  let coverBusy = $state(false);
  let coverInput: HTMLInputElement | undefined = $state();
  let dropping = $state(false); // "Delete your progress in this course?" is on screen
  let dropBusy = $state(false);
  let dropped = $state(false); // the course was just dropped: say so
  let dropBtn: HTMLButtonElement | undefined = $state();
  let keepBtn: HTMLButtonElement | undefined = $state();
  let dropNote: HTMLParagraphElement | undefined = $state();

  const isAdmin = $derived(session.user?.role === 'admin');
  const first = $derived(tree ? flatten(tree.sections)[0] : undefined);
  const counts = $derived(tree ? doneCount(tree.sections) : { done: 0, total: 0 });
  const mode = $derived(session.settings.resume_mode);
  const trackable = $derived(tree ? hasTracking(tree) : false);

  onMount(() => {
    void load();
  });

  async function load() {
    try {
      tree = await api.course(courseId);
      setTitle(tree.course.title);
      error = '';
    } catch (e) {
      error = messageOf(e);
    }
  }

  /** Admins: shrink the chosen picture in the browser, upload it, show it. */
  async function pickCover(e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    input.value = ''; // choosing the same file again must still fire "change"
    const t = tree;
    if (!file || !t || coverBusy) return;
    coverBusy = true;
    error = '';
    try {
      const st = await api.admin.setCover(t.course.id, await prepareCover(file));
      t.course.has_cover = st.has_cover;
      t.course.cover_v = st.cover_v;
    } catch (err) {
      error = err instanceof CoverError ? err.message : messageOf(err);
    } finally {
      coverBusy = false;
    }
  }

  /** Admins: drop the uploaded picture; the folder's image or the artwork takes over. */
  async function removeCover() {
    const t = tree;
    if (!t || coverBusy) return;
    coverBusy = true;
    error = '';
    try {
      const st = await api.admin.removeCover(t.course.id);
      t.course.has_cover = st.has_cover;
      t.course.cover_v = st.cover_v;
    } catch (err) {
      error = messageOf(err);
    } finally {
      coverBusy = false;
    }
  }

  /** Asks first. The safe answer, "No", gets the focus. */
  async function askDrop() {
    dropping = true;
    dropped = false;
    await tick();
    keepBtn?.focus();
  }

  async function keepCourse() {
    dropping = false;
    await tick();
    dropBtn?.focus();
  }

  /** Deletes this person's progress in the course, then shows it as it is now: not started. */
  async function dropCourse() {
    const t = tree;
    if (!t || dropBusy) return;
    dropBusy = true;
    error = '';
    try {
      await api.dropCourse(t.course.id);
      dropping = false;
      await load();
      if (!error) {
        dropped = true;
        await tick();
        dropNote?.focus();
      }
    } catch (e) {
      error = messageOf(e);
    } finally {
      dropBusy = false;
    }
  }

  /** Ticks or un-ticks one chapter of a video: shown at once, saved, then the course is read again. */
  async function toggleChapter(item: Item, c: Chapter) {
    const done = !c.done;
    c.done = done;
    busyId = item.id;
    try {
      await api.chapters(item.id, done ? { done: [chapterKey(c)] } : { undone: [chapterKey(c)] });
      await load(); // progress, "continue" and the counts depend on it
    } catch (e) {
      c.done = !done;
      error = messageOf(e);
    } finally {
      busyId = null;
    }
  }

  async function toggleDone(item: Item) {
    busyId = item.id;
    try {
      await setDone(item.id, !isDone(item));
      await load(); // "where to continue" depends on what is done
    } catch (e) {
      error = messageOf(e);
    } finally {
      busyId = null;
    }
  }
</script>

{#if error && !tree}
  <div class="stack" style="--gap: 1rem; justify-items: start">
    <p class="alert" role="alert"><Icon name="circle-alert" size={18} />{error}</p>
    <a class="btn btn-outline" href={links.home}><Icon name="arrow-left" size={18} />Back to courses</a>
  </div>
{:else if !tree}
  <div class="hero card" aria-hidden="true">
    <div class="skeleton cover-ghost"></div>
    <div class="info">
      <div class="skeleton" style="height: 40px; width: 70%"></div>
      <div class="skeleton" style="height: 18px; width: 90%"></div>
      <div class="skeleton" style="height: 18px; width: 60%"></div>
    </div>
  </div>
  <p class="sr-only" role="status">Loading…</p>
{:else}
  {@const c = tree.course}
  <div class="rise">
    <a class="btn btn-ghost btn-sm back" href={links.home}><Icon name="arrow-left" size={16} />All courses</a>

    <div class="hero card">
      <div class="cover-wrap">
        <div class="cover"><CourseCover course={c} eager /></div>
        {#if isAdmin}
          <div class="cover-tools">
            <input
              bind:this={coverInput}
              type="file"
              accept={COVER_ACCEPT}
              hidden
              tabindex="-1"
              onchange={pickCover}
            />
            <button type="button" class="tool" disabled={coverBusy} onclick={() => coverInput?.click()}>
              <Icon name={coverBusy ? 'loader' : 'image'} size={16} class={coverBusy ? 'spin' : ''} />
              {coverBusy ? 'Saving…' : 'Change cover'}
            </button>
            {#if c.cover_v > 0}
              <button
                type="button"
                class="tool round"
                title="Remove the uploaded cover"
                aria-label="Remove the uploaded cover"
                disabled={coverBusy}
                onclick={removeCover}
              >
                <Icon name="trash" size={16} />
              </button>
            {/if}
          </div>
        {/if}
      </div>
      <div class="info">
        <h1>{c.title}</h1>
        {#if c.description}
          <p class="desc muted">{c.description}</p>
        {/if}

        <div class="chips">
          <span class="chip"><Icon name="layers" size={14} />{counts.total} {counts.total === 1 ? 'item' : 'items'}</span>
          {#if c.duration_s > 0}
            <span class="chip"><Icon name="clock" size={14} />{formatDuration(c.duration_s)}</span>
          {/if}
          {#if tree.completed}
            <span class="chip chip-ok"><Icon name="check" size={14} stroke={3} />Completed</span>
          {/if}
        </div>

        <div class="prog">
          <ProgressBar value={percent(counts.done, counts.total)} label="Course progress" />
          <div class="prog-row">
            <span class="prog-text">
              {percent(counts.done, counts.total)}% complete · {counts.done}/{counts.total} items
            </span>
            {#if trackable}
              <div class="drop">
                {#if dropping}
                  <span class="drop-q" id="drop-q">Delete your progress in this course?</span>
                  <button
                    type="button"
                    class="btn btn-danger btn-sm"
                    aria-describedby="drop-q"
                    disabled={dropBusy}
                    onclick={dropCourse}
                  >
                    {dropBusy ? 'Dropping…' : 'Yes, drop it'}
                  </button>
                  <button
                    bind:this={keepBtn}
                    type="button"
                    class="btn btn-ghost btn-sm"
                    aria-describedby="drop-q"
                    disabled={dropBusy}
                    onclick={keepCourse}
                  >
                    No
                  </button>
                {:else}
                  <button
                    bind:this={dropBtn}
                    type="button"
                    class="btn btn-ghost btn-sm"
                    title="Stop tracking this course and delete your progress in it"
                    onclick={askDrop}
                  >
                    <Icon name="x" size={14} />Drop course
                  </button>
                {/if}
              </div>
            {/if}
          </div>
          {#if dropped && !trackable}
            <p class="drop-note" role="status" tabindex="-1" bind:this={dropNote}>
              Course dropped. Your progress in it is deleted; the course stays in the library.
            </p>
          {/if}
        </div>

        {#if error}
          <p class="alert" role="alert"><Icon name="circle-alert" size={18} />{error}</p>
        {/if}

        {#if tree.completed}
          <section class="resume" aria-label="Resume">
            <span class="rs-icon"><Icon name="trophy" size={22} /></span>
            <div class="rs-text">
              <p class="rs-title">You have completed this course.</p>
            </div>
            {#if first}
              <a class="btn btn-outline" href={links.item(c.id, first.id)}>
                <Icon name="restart" size={16} />Start again from the beginning
              </a>
            {/if}
          </section>
        {:else if tree.resume}
          {@const rs = tree.resume}
          {@const spot = rs.reason === 'continue' ? continueSpot(rs.kind, 'started', rs.position, rs.page, mode) : {}}
          <section class="resume" aria-label="Resume">
            <div class="rs-text">
              {#if rs.reason === 'continue'}
                <p class="rs-lead">{mode === 'ask' ? 'Continue where you left off?' : 'Where you left off:'}</p>
                <p class="rs-title">
                  <strong>{rs.title}</strong>
                  {#if spot.t !== undefined}
                    {@const rit = findItem(tree.sections, rs.item_id)}
                    {@const ci = rit?.chapters ? chapterIndexAt(rit.chapters, spot.t) : -1}
                    <span class="at">at {formatTime(spot.t)}{rit?.chapters && ci >= 0 ? ` · ${rit.chapters[ci].title}` : ''}</span>
                  {:else if spot.p !== undefined}
                    <span class="at">on page {spot.p}</span>
                  {/if}
                </p>
              {:else}
                <p class="rs-lead">Up next:</p>
                <p class="rs-title"><strong>{rs.title}</strong></p>
              {/if}
            </div>
            <div class="rs-actions">
              <a class="btn btn-primary btn-lg" href={links.item(c.id, rs.item_id, spot)}>
                <Icon name="play" size={18} />Continue
              </a>
              {#if mode === 'ask' && first}
                <a class="btn btn-outline btn-lg" href={links.item(c.id, first.id)}>Start from the beginning</a>
              {/if}
            </div>
          </section>
        {:else if first}
          <section class="resume start" aria-label="Resume">
            <a class="btn btn-primary btn-lg" href={links.item(c.id, first.id)}>
              <Icon name="play" size={18} />Start course
            </a>
          </section>
        {/if}
      </div>
    </div>

    <h2 class="content-h">Course content</h2>
    {#if tree.sections.length === 0}
      <div class="card card-pad muted">This course has no files yet.</div>
    {:else}
      <Curriculum
        courseId={c.id}
        sections={tree.sections}
        {busyId}
        onToggleDone={toggleDone}
        onToggleChapter={toggleChapter}
      />
    {/if}
  </div>
{/if}

<style>
  .back {
    margin: 0 0 14px -0.5rem;
    color: var(--text-2);
  }
  .hero {
    display: grid;
    grid-template-columns: minmax(240px, 380px) 1fr;
    gap: clamp(20px, 3vw, 40px);
    padding: clamp(14px, 2vw, 22px);
    margin-bottom: clamp(28px, 4vw, 44px);
    border-radius: var(--r-xl);
    box-shadow: var(--shadow-md);
  }
  .cover-wrap {
    position: relative;
    align-self: start;
  }
  .cover,
  .cover-ghost {
    aspect-ratio: 16 / 10;
    overflow: hidden;
    border-radius: var(--r-lg);
    box-shadow: var(--shadow-sm);
  }
  .cover-ghost {
    align-self: start;
  }
  /* admin only: sits on the picture, always visible (touch screens have no hover) */
  .cover-tools {
    position: absolute;
    right: 10px;
    bottom: 10px;
    display: flex;
    gap: 8px;
  }
  .tool {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 0.45rem;
    min-height: 40px;
    padding: 0 0.95rem;
    border: 1px solid rgb(255 255 255 / 0.3);
    border-radius: var(--r-pill);
    background: rgb(12 14 18 / 0.74);
    color: #fff;
    font: 600 0.8125rem/1 var(--font);
    white-space: nowrap;
    cursor: pointer;
    -webkit-backdrop-filter: blur(8px);
    backdrop-filter: blur(8px);
    transition: background-color var(--dur);
  }
  .tool:hover:not(:disabled) {
    background: rgb(12 14 18 / 0.9);
  }
  .tool:disabled {
    opacity: 0.7;
    cursor: progress;
  }
  .tool.round {
    width: 40px;
    padding: 0;
  }
  .tool:focus-visible {
    outline-offset: 2px;
  }
  .info {
    display: grid;
    align-content: start;
    gap: 16px;
    min-width: 0;
    padding: 6px clamp(0px, 1vw, 12px) 6px 0;
  }
  .desc {
    max-width: 68ch;
    font-size: 1.0625rem;
    white-space: pre-line;
  }
  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
  }
  .prog {
    display: grid;
    gap: 8px;
    max-width: 520px;
  }
  .prog-text {
    color: var(--text-2);
    font-size: 0.875rem;
    font-variant-numeric: tabular-nums;
  }
  .prog-row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 4px 12px;
  }
  .drop {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px 8px;
  }
  .drop-q {
    color: var(--danger);
    font-size: 0.875rem;
    font-weight: 600;
  }
  .drop-note {
    margin: 0;
    color: var(--text-2);
    font-size: 0.875rem;
  }

  .resume {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 14px 20px;
    margin-top: 4px;
    padding: 16px 18px;
    border: 1px solid color-mix(in srgb, var(--accent) 40%, var(--border));
    border-radius: var(--r-lg);
    background: var(--accent-soft);
  }
  .resume.start {
    padding: 0;
    border: 0;
    background: none;
    justify-content: flex-start;
  }
  .rs-text {
    flex: 1 1 240px;
    min-width: 0;
  }
  .rs-lead {
    color: var(--text-2);
    font-size: 0.875rem;
    font-weight: 600;
  }
  .rs-title {
    font-size: 1.0625rem;
    overflow-wrap: anywhere;
  }
  .at {
    margin-left: 0.4rem;
    color: var(--text-2);
    font-size: 0.9375rem;
    font-variant-numeric: tabular-nums;
  }
  .rs-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.6rem;
  }
  .rs-icon {
    display: grid;
    place-items: center;
    width: 44px;
    height: 44px;
    border-radius: 50%;
    background: var(--ok-soft);
    color: var(--ok);
  }

  .content-h {
    margin-bottom: 16px;
    font-size: 1.5rem;
  }

  @media (max-width: 820px) {
    .hero {
      grid-template-columns: 1fr;
    }
    .cover-wrap,
    .cover-ghost {
      max-width: 560px;
    }
  }
</style>
