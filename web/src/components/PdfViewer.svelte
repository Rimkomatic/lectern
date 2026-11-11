<script lang="ts">
  import type { PDFDocumentLoadingTask, PDFDocumentProxy, RenderTask } from 'pdfjs-dist';
  import { onMount } from 'svelte';
  import { messageOf, url } from '../lib/api';
  import type { ProgressReporter } from '../lib/progress';
  import type { Item } from '../lib/types';
  import Icon from './Icon.svelte';

  interface Props {
    item: Item;
    reporter: ProgressReporter;
    /** Page to open at (1 = the first). */
    startPage: number;
    /** A "resume?" prompt is showing: say nothing about the page until the user decides. */
    offer: boolean;
    /** The user turned to another page by hand. */
    onPage?: (page: number) => void;
  }

  let { item, reporter, startPage, offer, onPage }: Props = $props();

  const ZOOMS = [0.5, 0.75, 1, 1.25, 1.5, 2, 3];

  let phase = $state<'loading' | 'ready' | 'failed'>('loading');
  let error = $state('');
  let page = $state(1);
  let pages = $state(0);
  let zoom = $state(1);
  let canvas: HTMLCanvasElement | undefined = $state();
  let box: HTMLDivElement | undefined = $state();

  let doc: PDFDocumentProxy | null = null;
  /** A page asked for (by the resume prompt) before the document finished loading. */
  let wanted: number | null = null;
  let renderTask: RenderTask | null = null;
  let renderSeq = 0;
  let resizeTimer: ReturnType<typeof setTimeout> | undefined;

  /** Shows a page and records it as the reading position. */
  export function goTo(n: number): void {
    if (!Number.isFinite(n)) return;
    if (!doc) {
      wanted = Math.floor(n); // applied as soon as the document is ready
      return;
    }
    page = Math.min(Math.max(1, Math.floor(n)), pages);
    reporter.report('page', { page, pages });
    void render();
  }

  function userGoTo(n: number): void {
    if (!Number.isFinite(n)) return;
    goTo(n);
    onPage?.(page);
  }

  function setZoom(z: number): void {
    zoom = z;
    void render();
  }

  function stepZoom(dir: 1 | -1): void {
    const i = ZOOMS.findIndex((z) => z >= zoom - 1e-6);
    const next = ZOOMS[Math.min(Math.max((i < 0 ? ZOOMS.length - 1 : i) + dir, 0), ZOOMS.length - 1)];
    setZoom(next);
  }

  function onResize(): void {
    clearTimeout(resizeTimer);
    resizeTimer = setTimeout(() => void render(), 200);
  }

  async function render(): Promise<void> {
    const my = ++renderSeq;
    const prev = renderTask;
    if (prev) {
      prev.cancel();
      try {
        await prev.promise;
      } catch {
        // cancelled, as intended
      }
    }
    const d = doc;
    const c = canvas;
    if (!d || !c || my !== renderSeq) return;
    try {
      const pg = await d.getPage(page);
      if (my !== renderSeq) return;
      const base = pg.getViewport({ scale: 1 });
      const available = box?.clientWidth || base.width;
      const viewport = pg.getViewport({ scale: (available / base.width) * zoom });
      const dpr = window.devicePixelRatio || 1;
      c.width = Math.floor(viewport.width * dpr);
      c.height = Math.floor(viewport.height * dpr);
      c.style.width = `${Math.floor(viewport.width)}px`;
      c.style.height = `${Math.floor(viewport.height)}px`;
      const task = pg.render({
        canvas: c,
        viewport,
        transform: dpr === 1 ? undefined : [dpr, 0, 0, dpr, 0, 0],
      });
      renderTask = task;
      await task.promise;
    } catch (e) {
      if (my !== renderSeq || (e instanceof Error && e.name === 'RenderingCancelledException')) return;
      error = messageOf(e);
    }
  }

  onMount(() => {
    let cancelled = false;
    let loading: PDFDocumentLoadingTask | null = null;

    (async () => {
      try {
        // pdf.js is large, so it is only fetched when a PDF is opened.
        const pdfjs = await import('pdfjs-dist');
        const { default: workerSrc } = await import('pdfjs-dist/build/pdf.worker.min.mjs?url');
        pdfjs.GlobalWorkerOptions.workerSrc = workerSrc;
        loading = pdfjs.getDocument({ url: url.stream(item.id) });
        const d = await loading.promise;
        if (cancelled) {
          void loading.destroy(); // also stops the worker
          return;
        }
        doc = d;
        pages = d.numPages;
        page = Math.min(Math.max(1, Math.floor(wanted ?? startPage)), pages);
        phase = 'ready';
        // While a "resume?" prompt is open the saved page must stay as it is.
        reporter.report('open', offer && wanted === null ? { pages } : { page, pages });
        await render();
      } catch (e) {
        if (cancelled) return;
        error = messageOf(e);
        phase = 'failed';
      }
    })();

    return () => {
      cancelled = true;
      clearTimeout(resizeTimer);
      renderSeq++;
      renderTask?.cancel();
      void loading?.destroy(); // releases the document and its worker
      doc = null;
    };
  });
</script>

<svelte:window onresize={onResize} />

{#if phase === 'failed'}
  <p class="alert" role="alert">
    <Icon name="circle-alert" size={18} />
    <span>{error || 'This PDF could not be shown here.'} Showing your browser's own viewer instead.</span>
  </p>
  <iframe class="fallback" title={item.title} src={url.stream(item.id)} width="100%" height="700"></iframe>
  <p><a class="btn btn-outline" href={url.download(item.id)} download><Icon name="download" size={18} />Download</a></p>
{:else}
  <div class="pdf">
    {#if phase === 'loading'}
      <div class="loading" role="status"><Icon name="loader" size={20} class="spin" />Loading…</div>
    {/if}
    <div class="toolbar" hidden={phase !== 'ready'}>
      <button type="button" class="btn btn-ghost btn-icon btn-sm" disabled={page <= 1} onclick={() => userGoTo(page - 1)}>
        <Icon name="chevron-left" size={18} /><span class="sr-only">Previous page</span>
      </button>
      <label class="pager">
        Page
        <input
          class="input input-sm"
          type="number"
          min="1"
          max={pages}
          value={page}
          onchange={(e) => userGoTo(Number(e.currentTarget.value))}
        />
        of {pages}
      </label>
      <button type="button" class="btn btn-ghost btn-icon btn-sm" disabled={page >= pages} onclick={() => userGoTo(page + 1)}>
        <Icon name="chevron-right" size={18} /><span class="sr-only">Next page</span>
      </button>
      <span class="sep" aria-hidden="true"></span>
      <button type="button" class="btn btn-ghost btn-icon btn-sm" aria-label="Zoom out" disabled={zoom <= ZOOMS[0]} onclick={() => stepZoom(-1)}>
        <Icon name="zoom-out" size={18} />
      </button>
      <span class="zoom">{Math.round(zoom * 100)}%</span>
      <button type="button" class="btn btn-ghost btn-icon btn-sm" aria-label="Zoom in" disabled={zoom >= ZOOMS[ZOOMS.length - 1]} onclick={() => stepZoom(1)}>
        <Icon name="zoom-in" size={18} />
      </button>
    </div>
    {#if error && phase === 'ready'}
      <p class="alert" role="alert"><Icon name="circle-alert" size={18} />{error}</p>
    {/if}
    <div class="well" hidden={phase !== 'ready'}>
      <!-- The page is drawn to fit this box's width, so the box must have no padding of its own. -->
      <div class="scroll viewport" bind:this={box}>
        <canvas bind:this={canvas} hidden={phase !== 'ready'}></canvas>
      </div>
    </div>
  </div>
{/if}

<style>
  .pdf {
    display: grid;
    gap: 14px;
  }
  .loading {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.6rem;
    min-height: 320px;
    border: 1px solid var(--border);
    border-radius: var(--r-xl);
    background: var(--surface);
    color: var(--text-2);
    font-weight: 600;
  }
  .toolbar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: center;
    justify-self: center;
    gap: 4px;
    padding: 6px 10px;
    border: 1px solid var(--border);
    border-radius: var(--r-pill);
    background: var(--surface);
    box-shadow: var(--shadow-sm);
  }
  .pager {
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0 0.4rem;
    color: var(--text-2);
    font-size: 0.875rem;
    font-weight: 600;
    font-variant-numeric: tabular-nums;
  }
  .pager .input {
    width: 4.2rem;
    border-radius: var(--r-pill);
    text-align: center;
  }
  .sep {
    width: 1px;
    height: 22px;
    margin: 0 6px;
    background: var(--border);
  }
  .zoom {
    min-width: 3.4rem;
    color: var(--text-2);
    font-size: 0.875rem;
    font-weight: 600;
    text-align: center;
    font-variant-numeric: tabular-nums;
  }
  .well {
    padding: clamp(8px, 1.6vw, 18px);
    border: 1px solid var(--border);
    border-radius: var(--r-xl);
    background: var(--surface-2);
  }
  .viewport {
    max-height: calc(100dvh - var(--nav-h) - 170px);
    min-height: 240px;
    border-radius: var(--r-md);
    box-shadow: var(--shadow-md);
    background: #fff;
  }
  .fallback {
    border: 1px solid var(--border);
    border-radius: var(--r-xl);
    background: var(--surface);
  }
</style>
