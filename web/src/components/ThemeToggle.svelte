<script lang="ts">
  import { theme, toggleTheme } from '../lib/theme.svelte';
  import Icon from './Icon.svelte';

  const dark = $derived(theme.effective === 'dark');
  const label = $derived(dark ? 'Switch to light mode' : 'Switch to dark mode');
</script>

<button type="button" class="btn btn-ghost btn-icon toggle" onclick={toggleTheme} title={label}>
  <span class="glyph" class:dark>
    <span class="sun"><Icon name="sun" /></span>
    <span class="moon"><Icon name="moon" /></span>
  </span>
  <span class="sr-only">{label}</span>
</button>

<style>
  .glyph {
    position: relative;
    display: block;
    width: 20px;
    height: 20px;
  }
  .sun,
  .moon {
    position: absolute;
    inset: 0;
    display: grid;
    place-items: center;
    transition:
      transform 0.45s var(--ease),
      opacity 0.3s ease;
  }
  /* showing light mode: offer the moon; showing dark mode: offer the sun */
  .sun {
    opacity: 0;
    transform: rotate(-90deg) scale(0.4);
  }
  .moon {
    opacity: 1;
    transform: none;
  }
  .glyph.dark .sun {
    opacity: 1;
    transform: none;
  }
  .glyph.dark .moon {
    opacity: 0;
    transform: rotate(90deg) scale(0.4);
  }
</style>
