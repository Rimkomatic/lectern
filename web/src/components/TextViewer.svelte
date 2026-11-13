<script lang="ts">
  import { onMount } from 'svelte';
  import { messageOf, url } from '../lib/api';
  import { parseCsv } from '../lib/csv';
  import { decodeBytes } from '../lib/decode';
  import { formatBytes } from '../lib/format';
  import { cleanHtml, markdownToHtml } from '../lib/markdown';
  import type { ProgressReporter } from '../lib/progress';
  import { assetRef, formatOf, isBlankHtml, type TextFormat } from '../lib/textformat';
  import type { Item } from '../lib/types';
  import Icon from './Icon.svelte';

  interface Props {
    item: Item;
    reporter: ProgressReporter;
  }

  let { item, reporter }: Props = $props();

  /** Bigger files are offered as downloads instead of being pulled into the page. */
  const LIMIT = 5 * 1024 * 1024;
  /** Cleaning a very large page would freeze a phone for seconds, so it is shown as plain text. */
  const FORMAT_LIMIT = 2_000_000;
  const MAX_ROWS = 1000;

  let phase = $state<'loading' | 'ready' | 'toolarge' | 'failed'>('loading');
  let error = $state('');
  let format = $state<TextFormat | 'csv'>('plain');
  /** The file as text. */
  let text = $state('');
  /** The page as it is shown (cleaned). Empty when there is nothing to show but the source. */
  let html = $state('');
  let view = $state<'formatted' | 'source'>('formatted');
  let note = $state('');
  /** Pictures that live on other web sites. They are never loaded. */
  let blocked = $state(0);
  let rows = $state<string[][]>([]);
  let truncated = $state(false);

  /** Cleans a page of Markdown or HTML so it can be shown, or decides to show its source. */
  function prepare(body: string) {
    if (body.length > FORMAT_LIMIT) {
      view = 'source';
      note = 'This file is large, so it is shown as plain text.';
      return;
    }
    // Pictures stored in the course beside the page are served by the app itself.
    const resolveImage = (ref: string) => url.asset(item.id, assetRef(ref));
    const out = format === 'markdown' ? markdownToHtml(body, { resolveImage }) : cleanHtml(body, { resolveImage });
    if (isBlankHtml(out.html)) {
      view = 'source';
      note = 'Nothing in this page can be shown here (scripts, styles and forms are not run), so its source is shown.';
      return;
    }
    html = out.html;
    blocked = out.blockedImages;
  }

  onMount(() => {
    const ctrl = new AbortController();

    (async () => {
      if (item.size > LIMIT) {
        phase = 'toolarge';
        return;
      }
      try {
        const res = await fetch(url.stream(item.id), { credentials: 'same-origin', signal: ctrl.signal });
        if (!res.ok) throw new Error(`The file could not be loaded (${res.status}).`);
        const body = decodeBytes(await res.arrayBuffer());
        if (item.kind === 'csv') {
          const parsed = parseCsv(body, MAX_ROWS);
          rows = parsed.rows;
          truncated = parsed.truncated;
          format = 'csv';
        } else {
          text = body;
          format = formatOf(item, body);
          if (format !== 'plain') prepare(body);
        }
        phase = 'ready';
        reporter.report('open');
      } catch (e) {
        if (ctrl.signal.aborted) return;
        error = messageOf(e);
        phase = 'failed';
      }
    })();

    return () => ctrl.abort();
  });
</script>

{#if phase === 'loading'}
  <div class="loading" role="status"><Icon name="loader" size={20} class="spin" />Loading…</div>
{:else if phase === 'toolarge'}
  <div class="note card">
    <p>This file is {formatBytes(item.size)}, too large to show here.</p>
    <p><a class="btn btn-primary" href={url.download(item.id)} download><Icon name="download" size={18} />Download</a></p>
  </div>
{:else if phase === 'failed'}
  <p class="alert" role="alert"><Icon name="circle-alert" size={18} />{error}</p>
  <p><a class="btn btn-outline" href={url.download(item.id)} download><Icon name="download" size={18} />Download</a></p>
{:else if format === 'csv'}
  <div class="sheet card">
    <div class="scroll">
      <table>
        {#if rows.length > 0}
          <thead>
            <tr>
              {#each rows[0] as cell, c (c)}
                <th scope="col">{cell}</th>
              {/each}
            </tr>
          </thead>
          <tbody>
            {#each rows.slice(1) as row, r (r)}
              <tr>
                {#each row as cell, c (c)}
                  <td>{cell}</td>
                {/each}
              </tr>
            {/each}
          </tbody>
        {/if}
      </table>
    </div>
  </div>
  {#if truncated}
    <p class="muted">Showing the first {MAX_ROWS} rows. <a href={url.download(item.id)} download>Download</a> the full file.</p>
  {/if}
{:else}
  {#if html !== '' || note || (blocked > 0 && view === 'formatted')}
    <div class="bar">
      {#if html !== ''}
        <div class="seg" role="group" aria-label="Show as">
          <button type="button" aria-pressed={view === 'formatted'} onclick={() => (view = 'formatted')}>Formatted</button>
          <button type="button" aria-pressed={view === 'source'} onclick={() => (view = 'source')}>Source</button>
        </div>
      {/if}
      {#if note}
        <p class="muted">{note}</p>
      {/if}
      {#if blocked > 0 && view === 'formatted'}
        <p class="muted">{blocked} {blocked === 1 ? 'image' : 'images'} from other websites {blocked === 1 ? 'is' : 'are'} not loaded.</p>
      {/if}
    </div>
  {/if}
  {#if view === 'formatted' && html !== ''}
    <div class="paper card"><article class="prose">{@html html}</article></div>
  {:else}
    <div class="paper card"><pre>{text}</pre></div>
  {/if}
{/if}

<style>
  .loading {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.6rem;
    min-height: 240px;
    border: 1px solid var(--border);
    border-radius: var(--r-xl);
    background: var(--surface);
    color: var(--text-2);
    font-weight: 600;
  }
  .note {
    display: grid;
    justify-items: start;
    gap: 14px;
    padding: 24px;
    border-radius: var(--r-xl);
  }
  .bar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.5rem 1rem;
    margin: 0 0 12px;
  }
  .bar p {
    margin: 0;
    font-size: 0.875rem;
  }
  .seg {
    display: inline-flex;
    gap: 2px;
    padding: 3px;
    border: 1px solid var(--border);
    border-radius: var(--r-pill);
    background: var(--surface);
  }
  .seg button {
    min-height: 34px;
    padding: 0 1rem;
    border: 0;
    border-radius: var(--r-pill);
    background: none;
    color: var(--text-2);
    font: 600 0.875rem/1 var(--font);
    cursor: pointer;
    transition:
      background-color var(--dur),
      color var(--dur);
  }
  .seg button:hover {
    color: var(--text);
  }
  .seg button[aria-pressed='true'] {
    background: var(--accent);
    color: var(--on-accent);
  }
  .paper {
    padding: clamp(18px, 4vw, 48px);
    border-radius: var(--r-xl);
  }
  .paper pre {
    margin: 0;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    font-size: 0.9rem;
    line-height: 1.6;
  }

  /* a rendered Markdown or HTML lesson (the HTML is injected, so these rules reach it with :global) */
  .prose {
    max-width: 76ch;
    margin-inline: auto;
    font-size: 1.0625rem;
    line-height: 1.7;
    overflow-wrap: anywhere;
  }
  .prose :global(> :first-child) {
    margin-top: 0;
  }
  .prose :global(> :last-child) {
    margin-bottom: 0;
  }
  .prose :global(h1),
  .prose :global(h2),
  .prose :global(h3),
  .prose :global(h4) {
    margin: 1.8em 0 0.6em;
    line-height: 1.25;
  }
  .prose :global(h1) {
    font-size: 2rem;
  }
  .prose :global(h2) {
    padding-bottom: 0.3em;
    border-bottom: 1px solid var(--border);
    font-size: 1.5rem;
  }
  .prose :global(h3) {
    font-size: 1.25rem;
  }
  .prose :global(p),
  .prose :global(ul),
  .prose :global(ol),
  .prose :global(blockquote),
  .prose :global(pre),
  .prose :global(table) {
    margin: 0 0 1.1em;
  }
  .prose :global(ul),
  .prose :global(ol) {
    padding-left: 1.5em;
  }
  .prose :global(li + li) {
    margin-top: 0.3em;
  }
  .prose :global(blockquote) {
    padding: 0.2em 1.1em;
    border-left: 4px solid var(--accent);
    border-radius: 0 var(--r-sm) var(--r-sm) 0;
    background: var(--accent-soft);
    color: var(--text);
  }
  .prose :global(code) {
    padding: 0.15em 0.4em;
    border-radius: 6px;
    background: var(--surface-2);
    font-size: 0.88em;
  }
  .prose :global(pre) {
    padding: 1em 1.2em;
    border: 1px solid var(--border);
    border-radius: var(--r-md);
    background: var(--surface-2);
    line-height: 1.55;
  }
  .prose :global(pre code) {
    padding: 0;
    background: none;
    font-size: 0.875rem;
  }
  .prose :global(img) {
    max-width: 100%;
    height: auto;
    border-radius: var(--r-md);
  }
  .prose :global(figure) {
    margin: 0 0 1.1em;
  }
  .prose :global(figcaption) {
    color: var(--text-2);
    font-size: 0.9em;
  }
  .prose :global(dt) {
    font-weight: 650;
  }
  .prose :global(dd) {
    margin: 0 0 0.6em 1.5em;
  }
  .prose :global(kbd) {
    padding: 0.1em 0.4em;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--surface-2);
    font-size: 0.85em;
  }
  .prose :global(hr) {
    margin: 2em 0;
  }
  .prose :global(table) {
    display: block;
    overflow-x: auto;
    border-collapse: collapse;
  }
  .prose :global(th),
  .prose :global(td) {
    padding: 0.5em 0.9em;
    border: 1px solid var(--border);
  }
  .prose :global(th) {
    background: var(--surface-2);
    text-align: left;
  }

  /* csv */
  .sheet {
    overflow: hidden;
    border-radius: var(--r-xl);
  }
  .sheet .scroll {
    max-height: calc(100dvh - var(--nav-h) - 170px);
  }
  table {
    width: 100%;
    border-collapse: separate;
    border-spacing: 0;
    font-size: 0.9rem;
    font-variant-numeric: tabular-nums;
  }
  th,
  td {
    padding: 0.65rem 1rem;
    border-bottom: 1px solid var(--border);
    text-align: left;
    white-space: nowrap;
  }
  th {
    position: sticky;
    top: 0;
    z-index: 1;
    background: var(--surface-2);
    font-weight: 650;
  }
  tbody tr:nth-child(even) td {
    background: color-mix(in srgb, var(--surface-2) 45%, var(--surface));
  }
  tbody tr:last-child td {
    border-bottom: 0;
  }
</style>
