<script lang="ts">
  import { isFlat, itemMeta, kindIcon, kindLabel, sectionStats, statusKey } from '../lib/actions';
  import { url } from '../lib/api';
  import { formatDuration } from '../lib/format';
  import { links } from '../lib/router.svelte';
  import type { Chapter, Item, Section } from '../lib/types';
  import ChapterList from './ChapterList.svelte';
  import Icon from './Icon.svelte';
  import StatusToggle from './StatusToggle.svelte';

  interface Props {
    courseId: number;
    sections: Section[];
    /** An item whose done-toggle is being saved. */
    busyId?: number | null;
    onToggleDone: (item: Item) => void;
    /** The tick on a chapter of a video. */
    onToggleChapter: (item: Item, chapter: Chapter) => void;
  }

  let { courseId, sections, busyId = null, onToggleDone, onToggleChapter }: Props = $props();

  // A course that is only a list of files shows just the list.
  const flat = $derived(isFlat(sections));
</script>

<div class="curriculum">
  {#if flat}
    <section class="sec">
      <ol class="items" aria-label="Lessons">
        {@render rows(sections[0].items)}
      </ol>
    </section>
  {:else}
    {#each sections as section (section.id)}
      {@const st = sectionStats(section)}
      {@const meta = st.seconds > 0 ? `${st.done}/${st.total} done · ${formatDuration(st.seconds)}` : `${st.done}/${st.total} done`}
      <section class="sec">
        <header class="sec-head">
          <h3>{section.title}</h3>
          <span class="sec-meta">{meta}</span>
        </header>
        <ol class="items">
          {@render rows(section.items)}
        </ol>
      </section>
    {/each}
  {/if}
</div>

{#snippet rows(items: Item[])}
  {#each items as item (item.id)}
    {@const info = itemMeta(item)}
    <li class="entry" data-status={statusKey(item)}>
      <div class="row">
        <StatusToggle {item} busy={busyId === item.id} onToggle={onToggleDone} />
        <a class="open" href={links.item(courseId, item.id)}>
          <span class="kind"><Icon name={kindIcon(item.kind)} size={18} /></span>
          <span class="text">
            <span class="t">{item.title}</span>
            <small>{kindLabel(item)}{info ? ` · ${info}` : ''}</small>
          </span>
        </a>
        {#if item.kind !== 'link'}
          <a class="btn btn-ghost btn-sm dl" href={url.download(item.id)} download title="Download">
            <Icon name="download" size={16} /><span class="dl-label">Download</span>
          </a>
        {/if}
      </div>
      {#if item.chapters?.length}
        <div class="nested">
          <ChapterList
            chapters={item.chapters}
            label="Chapters of {item.title}"
            href={(c) => links.chapter(courseId, item.id, c.start)}
            onToggle={(c) => onToggleChapter(item, c)}
            duration={item.duration ?? item.progress?.duration ?? null}
          />
        </div>
      {/if}
    </li>
  {/each}
{/snippet}

<style>
  .curriculum {
    display: grid;
    gap: 16px;
  }
  .sec {
    overflow: hidden;
    border: 1px solid var(--border);
    border-radius: var(--r-lg);
    background: var(--surface);
    box-shadow: var(--shadow-sm);
  }
  .sec-head {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    justify-content: space-between;
    gap: 0.25rem 1rem;
    padding: 16px 20px;
    border-bottom: 1px solid var(--border);
    background: color-mix(in srgb, var(--surface-2) 60%, var(--surface));
  }
  .sec-meta {
    color: var(--text-2);
    font-size: 0.8125rem;
    font-variant-numeric: tabular-nums;
  }
  .items {
    margin: 0;
    padding: 6px;
    list-style: none;
  }
  .row {
    display: flex;
    align-items: center;
    gap: 6px;
    padding-right: 6px;
    border-radius: var(--r-md);
    transition: background-color var(--dur);
  }
  .row:hover {
    background: var(--surface-2);
  }
  .open {
    flex: 1;
    min-width: 0;
    display: flex;
    align-items: center;
    gap: 0.85rem;
    padding: 9px 6px;
    border-radius: var(--r-md);
    color: var(--text);
    text-decoration: none;
  }
  .open:hover {
    text-decoration: none;
  }
  .kind {
    flex: none;
    display: grid;
    place-items: center;
    width: 38px;
    height: 38px;
    border-radius: 12px;
    background: var(--surface-2);
    color: var(--text-2);
    transition: background-color var(--dur);
  }
  .row:hover .kind {
    background: var(--surface);
  }
  .text {
    display: grid;
    min-width: 0;
  }
  .t {
    font-weight: 600;
    overflow-wrap: anywhere;
  }
  small {
    color: var(--text-2);
  }
  .entry[data-status='done'] .t {
    color: var(--text-2);
  }
  /* the chapters of a long video, listed under it */
  .nested {
    margin: 0 6px 10px 62px;
    padding-left: 8px;
    border-left: 2px solid var(--border);
  }
  .dl {
    flex: none;
    color: var(--text-2);
  }
  @media (max-width: 640px) {
    .nested {
      margin-left: 20px;
    }
    .dl {
      width: 36px;
      padding: 0;
    }
    .dl-label {
      position: absolute;
      width: 1px;
      height: 1px;
      overflow: hidden;
      clip-path: inset(50%);
      white-space: nowrap;
    }
  }
</style>
