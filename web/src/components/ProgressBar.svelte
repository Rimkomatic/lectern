<script lang="ts">
  let {
    value,
    label = 'Progress',
    size = 'md',
  }: { value: number; label?: string; size?: 'sm' | 'md' } = $props();

  const v = $derived(Math.max(0, Math.min(100, Math.round(Number.isFinite(value) ? value : 0))));
</script>

<div class="track {size}" role="progressbar" aria-label={label} aria-valuemin="0" aria-valuemax="100" aria-valuenow={v}>
  <div class="fill" class:done={v >= 100} style:width="{v}%"></div>
</div>

<style>
  .track {
    width: 100%;
    overflow: hidden;
    border-radius: var(--r-pill);
    background: var(--surface-3);
  }
  .md {
    height: 8px;
  }
  .sm {
    height: 5px;
  }
  .fill {
    height: 100%;
    min-width: 0;
    border-radius: inherit;
    background: var(--accent);
    transition: width 0.5s var(--ease);
  }
  .fill.done {
    background: var(--ok);
  }
</style>
