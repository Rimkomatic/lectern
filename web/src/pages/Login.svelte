<script lang="ts">
  import { onMount } from 'svelte';
  import { api, messageOf } from '../lib/api';
  import { setTitle } from '../lib/brand';
  import { applyMe } from '../lib/session.svelte';
  import AuthLayout from '../components/AuthLayout.svelte';
  import Icon from '../components/Icon.svelte';
  import PasswordField from '../components/PasswordField.svelte';

  let username = $state('');
  let password = $state('');
  let error = $state('');
  let busy = $state(false);
  let first: HTMLInputElement | undefined = $state();

  onMount(() => {
    setTitle('Sign in');
    first?.focus();
  });

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    error = '';
    busy = true;
    try {
      applyMe(await api.login(username.trim(), password));
    } catch (err) {
      error = messageOf(err);
    } finally {
      busy = false;
    }
  }
</script>

<AuthLayout>
  <h2>Sign in</h2>
  <form class="stack" style="--gap: 1.1rem" onsubmit={submit}>
    <label class="field">
      <span>Username</span>
      <input
        class="input"
        name="username"
        autocomplete="username"
        autocapitalize="none"
        spellcheck="false"
        bind:this={first}
        bind:value={username}
        required
      />
    </label>
    <PasswordField label="Password" name="password" autocomplete="current-password" bind:value={password} required />
    {#if error}
      <p class="alert" role="alert"><Icon name="circle-alert" size={18} />{error}</p>
    {/if}
    <button type="submit" class="btn btn-primary btn-lg" disabled={busy}>
      {#if busy}<Icon name="loader" size={18} class="spin" />{/if}Sign in
    </button>
  </form>
</AuthLayout>
