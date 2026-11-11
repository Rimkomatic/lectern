<script lang="ts">
  import { url } from '../lib/api';
  import { ChapterTracker } from '../lib/chapters';
  import type { IconName } from '../lib/icons';
  import type { Patch, ProgressReporter } from '../lib/progress';
  import {
    actionFor,
    keyboardDriven,
    skipTarget,
    stepVolume,
    targetFacts,
    type PlayerAction,
  } from '../lib/shortcuts';
  import type { Chapter, Item } from '../lib/types';
  import { loadVolume, saveVolume } from '../lib/volume';
  import Icon from './Icon.svelte';

  interface Props {
    item: Item;
    reporter: ProgressReporter;
    /** Seconds to jump to once the file is ready (0 = from the start). */
    startAt: number;
    autoplay: boolean;
    /** The user's saved playback speed. */
    rate: number;
    /** Called (not debounced) when the user changes the speed. */
    onRate: (rate: number) => void;
    onEnded: () => void;
    /** Called when playback starts; lets the page drop its "resume?" prompt. */
    onPlay?: () => void;
    /** Called when the person replays or skips back after the end, so the page can cancel "next lesson in 5…". */
    onTakeover?: () => void;
    /** Space and the arrow keys control the player. Turn off while something covers it (the phone drawer). */
    shortcuts?: boolean;
    /** Called when playback runs through the end of one of the video's chapters. */
    onChapter?: (chapter: Chapter) => void;
    /** Called as the playback time moves (about four times a second). */
    onTime?: (seconds: number) => void;
  }

  let {
    item,
    reporter,
    startAt,
    autoplay,
    rate,
    onRate,
    onEnded,
    onPlay,
    onTakeover,
    shortcuts = true,
    onChapter,
    onTime,
  }: Props = $props();

  let media: HTMLMediaElement | undefined = $state();
  let failure = $state('');
  let lastPosition = 0;
  let seekTimer: ReturnType<typeof setTimeout> | undefined;
  let finished = false; // reached the end and nobody has acted since

  // The little pill that confirms a key press ("+10 s", "Volume 85%", ⏸).
  interface Hud {
    id: number;
    icon: IconName;
    text: string;
    level?: number; // 0–1, draws a volume bar
  }
  let hud = $state<Hud | null>(null);
  let announce = $state('');
  let hudId = 0;
  let hudTimer: ReturnType<typeof setTimeout> | undefined;
  let skipTotal = 0; // seconds skipped in a row, shown as one running total
  const tracker = new ChapterTracker(); // notices chapters being played to their end

  const tag = $derived(item.kind === 'audio' ? 'audio' : 'video');

  function patch(): Patch {
    const m = media;
    if (!m) return { position: lastPosition };
    const duration = Number.isFinite(m.duration) && m.duration > 0 ? m.duration : undefined;
    return { position: m.currentTime, duration };
  }

  /** Moves playback to the given second. */
  export function seekTo(seconds: number): void {
    if (media) media.currentTime = Math.max(0, seconds);
  }

  /** Jumps to the given second and plays from there (a click on a chapter). */
  export function playAt(seconds: number): void {
    const m = media;
    if (!m) return;
    m.currentTime = Math.max(0, seconds);
    void m.play().catch(() => undefined); // the click that got us here allows it
  }

  function handleMetadata() {
    const m = media;
    if (!m) return;
    m.defaultPlaybackRate = rate;
    m.playbackRate = rate;
    const duration = Number.isFinite(m.duration) && m.duration > 0 ? m.duration : undefined;
    if (startAt > 0 && (duration === undefined || startAt < duration)) m.currentTime = startAt;
    // Position is only sent when we actually jump, so an unanswered "resume?"
    // prompt leaves the saved spot alone.
    reporter.report('open', { duration, ...(startAt > 0 ? { position: startAt } : {}) });
    if (autoplay) void m.play().catch(() => undefined); // the browser may still refuse
  }

  /** The person acted after the end (replay, skip back): the page should stop waiting to move on. */
  function takeover() {
    if (!finished) return;
    finished = false;
    onTakeover?.();
  }

  function handlePlay() {
    reporter.startHeartbeat(patch);
    takeover();
    onPlay?.();
  }

  function handlePause() {
    reporter.stopHeartbeat();
    if (media && !media.ended) reporter.report('pause', patch());
  }

  function handleSeeked() {
    // Dragging the scrubber (or holding an arrow key) fires many of these;
    // report where it settles.
    clearTimeout(seekTimer);
    seekTimer = setTimeout(() => reporter.report('seek', patch()), 400);
    if (media && !media.ended) takeover();
  }

  function handleEnded() {
    finished = true;
    reporter.stopHeartbeat();
    const p = patch();
    reporter.report('end', p);
    if (item.chapters?.length && onChapter) {
      const last = tracker.ended(item.chapters, p.duration);
      if (last) onChapter(last);
    }
    onEnded();
  }

  function handleVolume() {
    if (media) saveVolume({ volume: media.volume, muted: media.muted });
  }

  // ---- keyboard ----

  function flash(icon: IconName, text: string, say: string, level?: number) {
    hud = { id: ++hudId, icon, text, level };
    announce = say;
    clearTimeout(hudTimer);
    hudTimer = setTimeout(() => {
      hud = null;
      skipTotal = 0;
    }, 900);
  }

  function run(a: PlayerAction, m: HTMLMediaElement) {
    switch (a.kind) {
      case 'toggle':
        if (m.paused || m.ended) {
          void m.play().catch(() => undefined);
          flash('play', '', 'Playing');
        } else {
          m.pause();
          flash('pause', '', 'Paused');
        }
        return;

      case 'skip': {
        if (m.readyState < 1) return; // the length is not known yet
        m.currentTime = skipTarget(m.currentTime, a.seconds, m.duration);
        // holding the key or tapping it repeatedly reads as one running total
        const same = skipTotal !== 0 && Math.sign(skipTotal) === Math.sign(a.seconds);
        skipTotal = same ? skipTotal + a.seconds : a.seconds;
        const n = Math.abs(skipTotal);
        const back = a.seconds < 0;
        flash(back ? 'chevrons-left' : 'chevrons-right', `${back ? '−' : '+'}${n} s`, `${back ? 'Back' : 'Forward'} ${n} seconds`);
        return;
      }

      case 'volume': {
        // Up also lifts a mute; Down keeps whatever mute there is.
        if (a.delta > 0 && m.muted) m.muted = false;
        m.volume = stepVolume(m.volume, a.delta);
        const level = m.muted ? 0 : m.volume;
        const pct = Math.round(level * 100);
        const icon: IconName = level === 0 ? 'volume-x' : level < 0.5 ? 'volume-1' : 'volume-2';
        flash(icon, level === 0 ? 'Muted' : `${pct}%`, level === 0 ? 'Muted' : `Volume ${pct} percent`, level);
        return;
      }
    }
  }

  // Listen on the window, before anything else sees the key, so the shortcuts
  // work wherever the focus is (and the browser's own handling of the same
  // keys inside the player does not run a second time).
  $effect(() => {
    if (!shortcuts) return;
    let lastRepeat = 0;

    const onKeyDown = (e: KeyboardEvent) => {
      const m = media;
      if (!m || e.defaultPrevented) return;
      const a = actionFor(e, targetFacts(e.target), keyboardDriven());
      if (!a) return;
      e.preventDefault(); // no page scrolling, no button press
      e.stopImmediatePropagation();
      if (e.repeat) {
        if (a.kind === 'toggle') return; // holding Space must not flicker
        const now = performance.now();
        if (now - lastRepeat < 120) return; // hold-to-scrub, at a sane pace
        lastRepeat = now;
      }
      run(a, m);
    };

    // A focused button is pressed when Space is released, in some browsers.
    const onKeyUp = (e: KeyboardEvent) => {
      if (e.key !== ' ' || e.defaultPrevented) return;
      if (!actionFor(e, targetFacts(e.target), keyboardDriven())) return;
      e.preventDefault();
      e.stopImmediatePropagation();
    };

    window.addEventListener('keydown', onKeyDown, true);
    window.addEventListener('keyup', onKeyUp, true);
    return () => {
      window.removeEventListener('keydown', onKeyDown, true);
      window.removeEventListener('keyup', onKeyUp, true);
    };
  });

  // Start every lesson at the volume the person last chose on this device.
  $effect(() => {
    const m = media;
    const saved = m && loadVolume();
    if (m && saved) {
      m.volume = saved.volume;
      m.muted = saved.muted;
    }
  });

  function handleTime() {
    const m = media;
    if (!m) return;
    lastPosition = m.currentTime;
    onTime?.(lastPosition);
    if (item.chapters?.length && onChapter) {
      for (const c of tracker.update(item.chapters, lastPosition)) onChapter(c);
    }
  }

  function handleRate() {
    if (media) onRate(media.playbackRate);
  }

  function handleError() {
    const code = media?.error?.code;
    failure =
      code === 4 || code === 3
        ? "Your browser can't play this file (unsupported format or codec). Download it to play it in another app."
        : 'The file could not be loaded.';
  }

  $effect(() => () => {
    clearTimeout(seekTimer);
    clearTimeout(hudTimer);
  });
</script>

<div class="frame" class:audio={item.kind === 'audio'}>
  {#if item.kind === 'audio'}
    <div class="art" aria-hidden="true">
      <span class="disc"><Icon name="audio" size={34} /></span>
    </div>
  {/if}
  <svelte:element
    this={tag}
    bind:this={media}
    class="media"
    controls
    preload="metadata"
    src={url.stream(item.id)}
    onloadedmetadata={handleMetadata}
    onplay={handlePlay}
    onpause={handlePause}
    onseeking={() => tracker.seeking()}
    onseeked={handleSeeked}
    onended={handleEnded}
    ontimeupdate={handleTime}
    onratechange={handleRate}
    onvolumechange={handleVolume}
    onerror={handleError}
  >
    {#each item.subtitles as sub (sub.id)}
      <track kind="subtitles" src={url.subtitle(sub.id)} srclang={sub.lang} label={sub.label} />
    {/each}
  </svelte:element>

  {#if hud}
    {#key hud.id}
      <div class="hud" aria-hidden="true">
        <span class="pill" class:solo={!hud.text}>
          <Icon name={hud.icon} size={22} />
          {#if hud.text}<span>{hud.text}</span>{/if}
          {#if hud.level !== undefined}
            <span class="meter"><i style:width="{Math.round(hud.level * 100)}%"></i></span>
          {/if}
        </span>
      </div>
    {/key}
  {/if}
  <p class="sr-only" role="status">{announce}</p>
</div>

{#if failure}
  <p class="alert" role="alert">
    <Icon name="circle-alert" size={18} />
    <span>
      {failure}
      <a href={url.download(item.id)} download>Download</a>
    </span>
  </p>
{/if}

<style>
  .frame {
    position: relative;
    overflow: hidden;
    border-radius: var(--r-xl);
    background: #000;
    box-shadow: var(--shadow-lg);
  }
  .media {
    display: block;
    width: 100%;
    aspect-ratio: 16 / 9;
    max-height: calc(100dvh - var(--nav-h) - 150px);
    background: #000;
    object-fit: contain;
  }
  /* sound only: artwork above the native player */
  .frame.audio {
    border: 1px solid var(--border);
    background: var(--surface);
    box-shadow: var(--shadow-md);
  }
  .frame.audio .media {
    aspect-ratio: auto;
    height: 56px;
    max-height: none;
    padding: 6px 12px 12px;
    background: transparent;
  }
  .art {
    display: grid;
    place-items: center;
    padding: clamp(28px, 6vw, 56px) 16px 14px;
    background:
      radial-gradient(40rem 14rem at 50% 0%, color-mix(in srgb, var(--accent) 22%, transparent), transparent 70%),
      var(--surface);
  }
  .disc {
    display: grid;
    place-items: center;
    width: 84px;
    height: 84px;
    border-radius: 50%;
    background: var(--accent);
    color: var(--on-accent);
    box-shadow: 0 14px 34px -10px var(--accent-glow);
  }

  /* key-press feedback: a pill at the top, over the picture */
  .hud {
    position: absolute;
    inset: 14px 0 auto;
    display: grid;
    justify-items: center;
    pointer-events: none;
  }
  .pill {
    display: inline-flex;
    align-items: center;
    gap: 0.55rem;
    min-height: 44px;
    padding: 0 1.1rem;
    border-radius: var(--r-pill);
    background: rgb(0 0 0 / 0.68);
    color: #fff;
    font-size: 0.9375rem;
    font-weight: 650;
    font-variant-numeric: tabular-nums;
    -webkit-backdrop-filter: blur(10px);
    backdrop-filter: blur(10px);
    box-shadow: 0 10px 30px -12px rgb(0 0 0 / 0.6);
    animation: pop 0.14s var(--ease) both;
  }
  .pill.solo {
    width: 44px;
    padding: 0;
    justify-content: center;
  }
  /* sound only: the pill sits in the middle of the artwork, above the player bar */
  .frame.audio .hud {
    inset: 0 0 56px;
    align-content: center;
  }
  .frame.audio .pill {
    background: rgb(14 16 20 / 0.88);
  }
  .meter {
    width: 72px;
    height: 4px;
    overflow: hidden;
    border-radius: var(--r-pill);
    background: rgb(255 255 255 / 0.28);
  }
  .meter i {
    display: block;
    height: 100%;
    border-radius: inherit;
    background: #fff;
  }
  @keyframes pop {
    from {
      opacity: 0;
      transform: scale(0.92);
    }
  }
</style>
