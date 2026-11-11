<script lang="ts">
  import { onMount } from 'svelte';
  import { messageOf, url } from '../lib/api';
  import { parseCsv } from '../lib/csv';
  import { formatBytes } from '../lib/format';
  import { renderMarkdown } from '../lib/markdown';
  import type { ProgressReporter } from '../lib/progress';
  import type { Item } from '../lib/types';
  import Icon from './Icon.svelte';

  interface Props {
    item: Item;
    reporter: ProgressReporter;
  }

  let { item, reporter }: Props = $props();

  /** Bigger files are offered as downloads instead of being pulled into the page. */
  const LIMIT = 5 * 1024 * 1024;
  const MAX_ROWS = 1000;

  let phase = $state<'loading' | 'ready' | 'toolarge' | 'failed'>('loading');
  let error = $state('');
  let text = $state('');
  let html = $state('');
  let rows = $state<string[][]>([]);
  let truncated = $state(false);

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
        const body = await res.text();
        if (item.kind === 'markdown') {
          html = renderMarkdown(body);
        } else if (item.kind === 'csv') {
          const parsed = parseCsv(body, MAX_ROWS);
          rows = parsed.rows;
          truncated = parsed.truncated;
        } else {
          text = body;
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
{:else if item.kind === 'markdown'}
  <div class="paper card"><article class="prose">{@html html}</article></div>
{:else if item.kind === 'csv'}
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
  <div class="paper card"><pre>{text}</pre></div>
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

  /* rendered markdown (the HTML is injected, so these rules reach it with :global) */
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
    border-radius: var(--r-md);
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
