<script lang="ts">
  import { onMount } from 'svelte';
  import { api, messageOf } from '../lib/api';
  import { setTitle } from '../lib/brand';
  import { saveSettings, session } from '../lib/session.svelte';
  import { setTheme, theme, type ThemePref } from '../lib/theme.svelte';
  import type { ResumeMode } from '../lib/types';
  import Icon from '../components/Icon.svelte';

  let displayName = $state(session.user?.display_name ?? '');
  let profileMsg = $state('');

  let resumeMode = $state<ResumeMode>(session.settings.resume_mode);
  let autoplayNext = $state(session.settings.autoplay_next);
  let rate = $state(session.settings.playback_rate);
  let doneOnOpen = $state(session.settings.done_on_open);
  let prefsMsg = $state('');

  let currentPassword = $state('');
  let newPassword = $state('');
  let confirmPassword = $state('');
  let passwordMsg = $state('');

  onMount(() => setTitle('Settings'));

  const themes: Array<{ id: ThemePref; label: string; icon: 'system' | 'sun' | 'moon' }> = [
    { id: 'system', label: 'System', icon: 'system' },
    { id: 'light', label: 'Light', icon: 'sun' },
    { id: 'dark', label: 'Dark', icon: 'moon' },
  ];

  async function saveProfile(e: SubmitEvent) {
    e.preventDefault();
    profileMsg = '';
    try {
      session.user = await api.updateProfile(displayName.trim());
      profileMsg = 'Saved.';
    } catch (err) {
      profileMsg = messageOf(err);
    }
  }

  async function savePrefs(e: SubmitEvent) {
    e.preventDefault();
    prefsMsg = '';
    const speed = Number(rate);
    if (!Number.isFinite(speed) || speed < 0.25 || speed > 4) {
      prefsMsg = 'Playback speed must be between 0.25 and 4.';
      return;
    }
    try {
      await saveSettings({
        resume_mode: resumeMode,
        autoplay_next: autoplayNext,
        playback_rate: speed,
        done_on_open: doneOnOpen,
      });
      prefsMsg = 'Saved.';
    } catch (err) {
      prefsMsg = messageOf(err);
    }
  }

  async function changePassword(e: SubmitEvent) {
    e.preventDefault();
    passwordMsg = '';
    if (newPassword.length < 8) {
      passwordMsg = 'The new password must be at least 8 characters.';
      return;
    }
    if (newPassword !== confirmPassword) {
      passwordMsg = 'The two new passwords do not match.';
      return;
    }
    try {
      await api.changePassword(currentPassword, newPassword);
      currentPassword = newPassword = confirmPassword = '';
      passwordMsg = 'Password changed. Other devices have been signed out.';
    } catch (err) {
      passwordMsg = messageOf(err);
    }
  }
</script>

<div class="rise">
  <h1 class="page-title">Settings</h1>

  <div class="stack" style="--gap: 20px">
    <section class="card card-pad">
      <h2>Appearance</h2>
      <fieldset>
        <legend class="sr-only">Theme</legend>
        <div class="themes">
          {#each themes as t (t.id)}
            <label class="theme-opt">
              <input
                class="sr-only"
                type="radio"
                name="theme"
                value={t.id}
                checked={theme.pref === t.id}
                onchange={() => setTheme(t.id)}
              />
              <span class="swatch swatch-{t.id}" aria-hidden="true">
                <span class="sw-bar"></span>
                <span class="sw-card"></span>
                <span class="sw-dot"></span>
              </span>
              <span class="theme-name"><Icon name={t.icon} size={16} />{t.label}</span>
            </label>
          {/each}
        </div>
      </fieldset>
      <p class="hint muted">Saved on this device only. "System" follows your device's light or dark setting.</p>
    </section>

    <section class="card card-pad">
      <h2>Profile</h2>
      <form class="stack" style="--gap: 1rem" onsubmit={saveProfile}>
        <p class="muted">Signed in as <strong>{session.user?.username}</strong></p>
        <label class="field narrow">
          <span>Display name</span>
          <input class="input" bind:value={displayName} required maxlength="64" />
        </label>
        <div class="row">
          <button type="submit" class="btn btn-primary">Save</button>
          {#if profileMsg}<span class="msg" role="status">{profileMsg}</span>{/if}
        </div>
      </form>
    </section>

    <section class="card card-pad">
      <h2>Playback</h2>
      <form class="stack" style="--gap: 1.25rem" onsubmit={savePrefs}>
        <fieldset>
          <legend>When you open something you did not finish</legend>
          <div class="choices">
            <label class="choice">
              <input class="radio" type="radio" name="resume" value="ask" bind:group={resumeMode} />
              <span>Ask whether to continue</span>
            </label>
            <label class="choice">
              <input class="radio" type="radio" name="resume" value="auto" bind:group={resumeMode} />
              <span>Continue automatically</span>
            </label>
            <label class="choice">
              <input class="radio" type="radio" name="resume" value="never" bind:group={resumeMode} />
              <span>Always start from the beginning</span>
            </label>
          </div>
        </fieldset>

        <div>
          <label class="toggle-row">
            <span>Play the next video automatically</span>
            <input class="switch" type="checkbox" bind:checked={autoplayNext} />
          </label>
          <label class="toggle-row">
            <span>Mark images, notes, links and files as done when I open them</span>
            <input class="switch" type="checkbox" bind:checked={doneOnOpen} />
          </label>
        </div>

        <label class="field narrow">
          <span>Playback speed</span>
          <input class="input" type="number" min="0.25" max="4" step="0.25" bind:value={rate} />
        </label>

        <div class="row">
          <button type="submit" class="btn btn-primary">Save</button>
          {#if prefsMsg}<span class="msg" role="status">{prefsMsg}</span>{/if}
        </div>
      </form>
    </section>

    <section class="card card-pad">
      <h2>Password</h2>
      <form class="stack" style="--gap: 1rem" onsubmit={changePassword}>
        <label class="field narrow">
          <span>Current password</span>
          <input class="input" type="password" autocomplete="current-password" bind:value={currentPassword} required />
        </label>
        <label class="field narrow">
          <span>New password</span>
          <input class="input" type="password" autocomplete="new-password" bind:value={newPassword} required minlength="8" />
        </label>
        <label class="field narrow">
          <span>Repeat new password</span>
          <input class="input" type="password" autocomplete="new-password" bind:value={confirmPassword} required minlength="8" />
        </label>
        <div class="row">
          <button type="submit" class="btn btn-primary">Change password</button>
          {#if passwordMsg}<span class="msg" role="status">{passwordMsg}</span>{/if}
        </div>
      </form>
    </section>
  </div>
</div>

<style>
  .page-title {
    margin-bottom: clamp(20px, 3vw, 32px);
  }
  section h2 {
    margin-bottom: 1.1rem;
  }
  .narrow {
    max-width: 360px;
  }
  .msg {
    color: var(--text-2);
    font-weight: 600;
  }
  .hint {
    margin-top: 0.9rem;
    font-size: 0.875rem;
  }

  .choices {
    display: grid;
    gap: 0.6rem;
  }
  .choice {
    align-items: center;
    font-weight: 550;
  }

  /* theme picker */
  .themes {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 0.75rem;
    max-width: 560px;
  }
  .theme-opt {
    display: grid;
    gap: 0.6rem;
    padding: 0.6rem 0.6rem 0.75rem;
    border: 1px solid var(--border);
    border-radius: var(--r-lg);
    background: var(--surface);
    cursor: pointer;
    transition:
      border-color var(--dur),
      background-color var(--dur);
  }
  .theme-opt:hover {
    border-color: var(--border-input);
  }
  .theme-opt:has(input:checked) {
    border-color: var(--accent);
    background: var(--accent-soft);
  }
  .theme-opt:has(input:focus-visible) {
    outline: 2px solid var(--focus);
    outline-offset: 2px;
  }
  .theme-name {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 0.4rem;
    font-size: 0.875rem;
    font-weight: 650;
  }
  /* tiny pictures of each theme; their colours are fixed on purpose */
  .swatch {
    position: relative;
    display: block;
    height: 64px;
    overflow: hidden;
    border: 1px solid rgb(128 128 128 / 0.35);
    border-radius: var(--r-md);
    --sw-bg: #f6f4ef;
    --sw-card: #ffffff;
    --sw-line: #e2ddd1;
  }
  .swatch-dark {
    --sw-bg: #0d0f12;
    --sw-card: #1c2027;
    --sw-line: #363c47;
  }
  .swatch-system {
    background: linear-gradient(115deg, #f6f4ef 0 50%, #0d0f12 50% 100%);
    --sw-card: transparent;
  }
  .swatch:not(.swatch-system) {
    background: var(--sw-bg);
  }
  .sw-card {
    position: absolute;
    left: 12px;
    top: 14px;
    width: 52%;
    height: 12px;
    border-radius: 6px;
    background: var(--sw-card);
    box-shadow: 0 0 0 1px var(--sw-line);
  }
  .sw-bar {
    position: absolute;
    left: 12px;
    top: 36px;
    width: 34%;
    height: 8px;
    border-radius: 4px;
    background: var(--sw-line);
  }
  .sw-dot {
    position: absolute;
    right: 12px;
    bottom: 10px;
    width: 18px;
    height: 18px;
    border-radius: 50%;
    background: #f4a720;
  }
  .swatch-system .sw-card {
    background: linear-gradient(115deg, #fff 0 50%, #1c2027 50% 100%);
    box-shadow: none;
  }
  .swatch-system .sw-bar {
    background: linear-gradient(115deg, #e2ddd1 0 50%, #363c47 50% 100%);
  }
</style>
