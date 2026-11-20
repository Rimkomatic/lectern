<script lang="ts">
  import { onMount } from 'svelte';
  import { setTitle } from '../lib/brand';
  import AuthLayout from '../components/AuthLayout.svelte';
  import Icon from '../components/Icon.svelte';

  onMount(() => setTitle('Setup is closed'));
</script>

<AuthLayout>
  <h2>Setup is closed</h2>
  <div class="stack" style="--gap: 1rem">
    <p>
      Nobody has created an account yet. For safety, this page is only open for a short time after Lectern starts, and
      that time is over.
    </p>
    <p>
      <strong>Restart Lectern</strong>, then check again. With the usual Docker setup that is
      <code class="cmd">docker compose restart app</code>
    </p>
    <p class="muted">
      You can also skip this page: set <code>LECTERN_ADMIN_USER</code> and <code>LECTERN_ADMIN_PASSWORD</code>, then
      restart.
    </p>
    <button type="button" class="btn btn-primary btn-lg" onclick={() => location.reload()}>
      <Icon name="refresh" size={18} />Check again
    </button>
  </div>
</AuthLayout>

<style>
  p {
    line-height: 1.55;
  }
  .cmd {
    display: block;
    margin-top: 0.5rem;
    padding: 0.6rem 0.75rem;
    border-radius: var(--r-md);
    background: var(--surface-2);
    border: 1px solid var(--border);
    overflow-x: auto;
    white-space: nowrap;
    user-select: all;
  }
</style>
