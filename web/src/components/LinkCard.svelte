<script lang="ts">
  import type { ProgressReporter } from '../lib/progress';
  import type { Item } from '../lib/types';
  import Icon from './Icon.svelte';

  interface Props {
    item: Item;
    reporter: ProgressReporter;
  }

  let { item, reporter }: Props = $props();

  /** Only web links are ever turned into clickable anchors. */
  const target = $derived.by(() => {
    try {
      const u = new URL(item.link_url ?? '');
      return u.protocol === 'http:' || u.protocol === 'https:' ? u : null;
    } catch {
      return null;
    }
  });
</script>

{#if target}
  <div class="link card">
    <span class="badge"><Icon name="link" size={30} /></span>
    <div class="text">
      <p class="host">{target.host}</p>
      <p class="href"><small>{target.href}</small></p>
    </div>
    <a
      class="btn btn-primary btn-lg"
      href={target.href}
      target="_blank"
      rel="noopener noreferrer"
      onclick={() => reporter.report('open')}
      onauxclick={() => reporter.report('open')}
    >
      <Icon name="external" size={18} />Open {target.host}
    </a>
  </div>
{:else}
  <p class="alert" role="alert"><Icon name="circle-alert" size={18} />This link file does not contain a valid web address.</p>
{/if}

<style>
  .link {
    display: grid;
    justify-items: center;
    gap: 18px;
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
    background: var(--accent-soft);
    color: var(--accent-text);
  }
  .host {
    font-size: 1.25rem;
    font-weight: 700;
    letter-spacing: -0.02em;
  }
  .href {
    max-width: min(60ch, 80vw);
    color: var(--text-2);
    overflow-wrap: anywhere;
  }
</style>
