<script lang="ts">
  import { onMount } from 'svelte';
  import { url } from '../lib/api';
  import { formatBytes } from '../lib/format';
  import type { ProgressReporter } from '../lib/progress';
  import type { Item } from '../lib/types';
  import Icon from './Icon.svelte';

  interface Props {
    item: Item;
    reporter: ProgressReporter;
  }

  let { item, reporter }: Props = $props();

  // Seeing the page counts as opening the file.
  onMount(() => reporter.report('open'));
</script>

<div class="nop card">
  <span class="badge"><Icon name="file" size={30} /></span>
  <p class="why">This file type can't be previewed in the browser.</p>
  <p class="file">
    {item.name}
    <small>({formatBytes(item.size)})</small>
  </p>
  <p><a class="btn btn-primary btn-lg" href={url.download(item.id)} download><Icon name="download" size={18} />Download</a></p>
</div>

<style>
  .nop {
    display: grid;
    justify-items: center;
    gap: 14px;
    padding: clamp(28px, 6vw, 64px) 24px;
    border-radius: var(--r-xl);
    text-align: center;
  }
  .badge {
    display: grid;
    place-items: center;
    width: 72px;
    height: 72px;
    border-radius: 50%;
    background: var(--surface-2);
    color: var(--text-2);
  }
  .why {
    font-size: 1.125rem;
    font-weight: 650;
  }
  .file {
    color: var(--text-2);
    overflow-wrap: anywhere;
  }
</style>
