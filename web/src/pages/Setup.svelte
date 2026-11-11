<script lang="ts">
  import { onMount } from 'svelte';
  import { api, messageOf } from '../lib/api';
  import { setTitle } from '../lib/brand';
  import { applyMe } from '../lib/session.svelte';
  import AuthLayout from '../components/AuthLayout.svelte';
  import Icon from '../components/Icon.svelte';
  import PasswordField from '../components/PasswordField.svelte';

  let username = $state('');
  let displayName = $state('');
  let password = $state('');
  let confirm = $state('');
  let error = $state('');
  let busy = $state(false);

  onMount(() => setTitle('Set up'));

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    error = '';
    if (password.length < 8) {
      error = 'The password must be at least 8 characters.';
      return;
    }
    if (password !== confirm) {
      error = 'The two passwords do not match.';
      return;
    }
    busy = true;
    try {
      applyMe(await api.setup(username.trim(), password, displayName.trim() || username.trim()));
    } catch (err) {
      error = messageOf(err);
    } finally {
      busy = false;
    }
  }
</script>

<AuthLayout>
  <h2>Create the administrator account</h2>
  <form class="stack" style="--gap: 1.1rem" onsubmit={submit}>
    <label class="field">
      <span>Username</span>
      <input
        class="input"
        name="username"
        autocomplete="username"
        autocapitalize="none"
        spellcheck="false"
        bind:value={username}
        required
        minlength="3"
        maxlength="32"
      />
    </label>
    <label class="field">
      <span>Display name</span>
      <input class="input" name="display_name" autocomplete="name" bind:value={displayName} maxlength="64" />
    </label>
    <PasswordField label="Password" name="password" autocomplete="new-password" bind:value={password} required minlength={8} />
    <PasswordField label="Repeat password" name="confirm" autocomplete="new-password" bind:value={confirm} required minlength={8} />
    {#if error}
      <p class="alert" role="alert"><Icon name="circle-alert" size={18} />{error}</p>
    {/if}
    <button type="submit" class="btn btn-primary btn-lg" disabled={busy}>
      {#if busy}<Icon name="loader" size={18} class="spin" />{/if}Create account
    </button>
  </form>
</AuthLayout>
