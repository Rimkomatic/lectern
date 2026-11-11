<script lang="ts">
  import type { Snippet } from 'svelte';
  import { APP_NAME } from '../lib/brand';
  import Logo from './Logo.svelte';
  import ThemeToggle from './ThemeToggle.svelte';

  let { children }: { children: Snippet } = $props();
</script>

<div class="auth">
  <div class="corner"><ThemeToggle /></div>
  <main id="main" class="center">
    <h1 class="brand">
      <Logo size={44} />
      <span>{APP_NAME}</span>
    </h1>
    <div class="card panel rise">
      {@render children()}
    </div>
  </main>
</div>

<style>
  .auth {
    position: relative;
    min-height: 100dvh;
    display: grid;
    background:
      radial-gradient(60rem 36rem at 15% -10%, color-mix(in srgb, var(--accent) 16%, transparent), transparent 70%),
      radial-gradient(48rem 30rem at 100% 110%, color-mix(in srgb, var(--accent) 10%, transparent), transparent 70%),
      var(--bg);
  }
  .corner {
    position: absolute;
    top: 16px;
    right: 16px;
  }
  .center {
    width: min(100% - 32px, 440px);
    margin: auto;
    padding: 48px 0;
    display: grid;
    gap: 28px;
    justify-items: center;
  }
  .brand {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    font-size: 1.75rem;
    font-weight: 750;
    letter-spacing: -0.04em;
  }
  .panel {
    width: 100%;
    padding: clamp(24px, 5vw, 36px);
    border-radius: var(--r-xl);
    box-shadow: var(--shadow-lg);
  }
  .panel :global(h2) {
    font-size: 1.5rem;
    letter-spacing: -0.03em;
    margin-bottom: 1.5rem;
  }
</style>
