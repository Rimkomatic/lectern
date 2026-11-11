<script lang="ts">
  import { url } from '../lib/api';
  import type { ProgressReporter } from '../lib/progress';
  import type { Item } from '../lib/types';
  import Icon from './Icon.svelte';

  interface Props {
    item: Item;
    reporter: ProgressReporter;
  }

  let { item, reporter }: Props = $props();
  let failed = $state(false);
</script>

{#if failed}
  <p class="alert" role="alert"><Icon name="circle-alert" size={18} />The image could not be shown.</p>
  <p><a class="btn btn-outline" href={url.download(item.id)} download><Icon name="download" size={18} />Download</a></p>
{:else}
  <div class="frame">
    <img
      src={url.stream(item.id)}
      alt={item.title}
      onload={() => reporter.report('open')}
      onerror={() => (failed = true)}
    />
  </div>
{/if}

<style>
  .frame {
    display: grid;
    place-items: center;
    overflow: hidden;
    padding: clamp(10px, 2vw, 24px);
    border: 1px solid var(--border);
    border-radius: var(--r-xl);
    background: var(--surface-2);
  }
  img {
    max-height: calc(100dvh - var(--nav-h) - 190px);
    min-height: 0;
    border-radius: var(--r-md);
    object-fit: contain;
    box-shadow: var(--shadow-md);
  }
</style>
