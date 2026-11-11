<script lang="ts">
  import { onMount } from 'svelte';
  import { api, messageOf, url } from '../lib/api';
  import { setTitle } from '../lib/brand';
  import { formatWhen } from '../lib/format';
  import { session } from '../lib/session.svelte';
  import type { Library, ScanStatus, User } from '../lib/types';
  import Icon from '../components/Icon.svelte';
  import ProgressBar from '../components/ProgressBar.svelte';

  let status = $state<ScanStatus | null>(null);
  let libraries = $state<Library[]>([]);
  let users = $state<User[]>([]);
  let error = $state('');
  let notice = $state('');

  let newUsername = $state('');
  let newDisplayName = $state('');
  let newPassword = $state('');
  let newRole = $state('user');
  let creating = $state(false);

  /** The user whose "delete" is waiting for a second click. */
  let deleting = $state<number | null>(null);

  const isSelf = (u: User) => u.id === session.user?.id;

  async function loadLibraries() {
    try {
      libraries = await api.admin.libraries();
    } catch (e) {
      error = messageOf(e);
    }
  }

  async function loadUsers() {
    try {
      users = await api.admin.users();
    } catch (e) {
      error = messageOf(e);
    }
  }

  onMount(() => {
    setTitle('Admin');
    void loadLibraries();
    void loadUsers();
    api.admin
      .scanStatus()
      .then((s) => (status = s))
      .catch((e) => (error = messageOf(e)));

    // The server pushes scan progress; the browser reconnects by itself if the stream drops.
    const events = new EventSource(url.events);
    events.addEventListener('scan', (ev) => {
      try {
        const s = JSON.parse((ev as MessageEvent<string>).data) as ScanStatus;
        const wasRunning = status?.running ?? false;
        status = s;
        if (wasRunning && !s.running) void loadLibraries();
      } catch {
        // ignore a malformed event
      }
    });
    return () => events.close();
  });

  async function scanNow() {
    error = '';
    try {
      await api.admin.startScan();
    } catch (e) {
      error = messageOf(e);
    }
  }

  async function createUser(e: SubmitEvent) {
    e.preventDefault();
    error = notice = '';
    creating = true;
    try {
      const u = await api.admin.createUser({
        username: newUsername.trim(),
        display_name: newDisplayName.trim() || newUsername.trim(),
        password: newPassword,
        role: newRole,
      });
      notice = `Created ${u.username}.`;
      newUsername = newDisplayName = newPassword = '';
      newRole = 'user';
      await loadUsers();
    } catch (err) {
      error = messageOf(err);
    } finally {
      creating = false;
    }
  }

  async function patchUser(u: User, patch: { role?: string; disabled?: boolean; password?: string }) {
    error = notice = '';
    try {
      await api.admin.patchUser(u.id, patch);
      if (patch.password) notice = `Password changed for ${u.username}.`;
    } catch (err) {
      error = messageOf(err);
    }
    await loadUsers(); // also resets a role selector the server refused
  }

  async function setPassword(e: SubmitEvent, u: User) {
    e.preventDefault();
    const form = e.currentTarget as HTMLFormElement;
    const password = String(new FormData(form).get('password') ?? '');
    await patchUser(u, { password });
    form.reset();
  }

  async function removeUser(u: User) {
    error = notice = '';
    deleting = null;
    try {
      await api.admin.deleteUser(u.id);
      notice = `Deleted ${u.username}.`;
    } catch (err) {
      error = messageOf(err);
    }
    await loadUsers();
  }
</script>

<div class="rise">
  <h1 class="page-title">Administration</h1>

  {#if error}
    <p class="alert top" role="alert"><Icon name="circle-alert" size={18} />{error}</p>
  {/if}
  {#if notice}
    <p class="notice top" role="status"><Icon name="check" size={18} stroke={3} />{notice}</p>
  {/if}

  <div class="cols">
    <section class="card card-pad">
      <div class="head">
        <h2>Library scan</h2>
        <button type="button" class="btn btn-primary btn-sm" disabled={status?.running} onclick={scanNow}>
          <Icon name="refresh" size={16} class={status?.running ? 'spin' : ''} />Scan now
        </button>
      </div>
      {#if status?.running}
        <p>
          Scanning{status.library ? ` ${status.library}` : ''}… {status.done}/{status.total}
        </p>
        {#if status.current}<p class="muted current"><small>{status.current}</small></p>{/if}
        <div class="bar"><ProgressBar value={status.total ? (status.done / status.total) * 100 : 0} label="Scan progress" /></div>
      {:else if status?.finished_at}
        <p>
          Last scan finished {formatWhen(status.finished_at)}.
          {#if status.last}
            {status.last.courses} courses, {status.last.items} items
            ({status.last.courses_new} new, {status.last.courses_renamed} renamed, {status.last.courses_missing} missing).
          {/if}
        </p>
      {:else}
        <p class="muted">No scan has run since the server started.</p>
      {/if}
      {#if status?.errors && status.errors.length > 0}
        <ul class="problems" aria-label="Scan problems">
          {#each status.errors as msg, i (i)}
            <li>{msg}</li>
          {/each}
        </ul>
      {/if}
    </section>

    <section class="card card-pad">
      <h2>Libraries</h2>
      {#if libraries.length === 0}
        <p class="muted">No libraries are configured. Set <code>LECTERN_LIBRARIES</code> on the server.</p>
      {:else}
        <div class="scroll scroll-x libs">
          <table class="table">
            <thead>
              <tr><th scope="col">Name</th><th scope="col">Path</th><th scope="col">Courses</th><th scope="col">Last scan</th></tr>
            </thead>
            <tbody>
              {#each libraries as lib (lib.id)}
                <tr>
                  <td>{lib.label}{lib.enabled ? '' : ' (disabled)'}</td>
                  <td class="path"><code>{#each lib.path.split('/') as seg, i}{#if i > 0}/<wbr />{/if}{seg}{/each}</code></td>
                  <td>{lib.course_count}</td>
                  <td>{formatWhen(lib.last_scan_at)}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </section>
  </div>

  <section class="card card-pad block">
    <h2>Users</h2>
    <div class="scroll scroll-x people">
      <table class="table">
        <thead>
          <tr>
            <th scope="col">User</th><th scope="col">Role</th><th scope="col">Status</th>
            <th scope="col">Last sign-in</th><th scope="col">Actions</th>
          </tr>
        </thead>
        <tbody>
          {#each users as u (u.id)}
            <tr>
              <td>
                <div class="who">
                  <span class="avatar" aria-hidden="true">{[...(u.display_name || u.username)][0]?.toLocaleUpperCase()}</span>
                  <span>{u.display_name} <small class="muted">({u.username})</small></span>
                </div>
              </td>
              <td>
                <span class="select">
                  <select
                    class="input input-sm"
                    aria-label={`Role of ${u.username}`}
                    value={u.role}
                    disabled={isSelf(u)}
                    onchange={(e) => patchUser(u, { role: e.currentTarget.value })}
                  >
                    <option value="user">User</option>
                    <option value="admin">Administrator</option>
                  </select>
                </span>
              </td>
              <td><span class="chip" class:chip-ok={!u.disabled}>{u.disabled ? 'Disabled' : 'Active'}</span></td>
              <td class="nowrap">{formatWhen(u.last_login_at)}</td>
              <td>
                <div class="actions">
                  <button type="button" class="btn btn-outline btn-sm" disabled={isSelf(u)} onclick={() => patchUser(u, { disabled: !u.disabled })}>
                    {u.disabled ? 'Enable' : 'Disable'}
                  </button>
                  <details>
                    <summary class="btn btn-outline btn-sm"><Icon name="key" size={14} />Set password</summary>
                    <form class="pw" onsubmit={(e) => setPassword(e, u)}>
                      <input
                        class="input input-sm"
                        type="password"
                        name="password"
                        aria-label={`New password for ${u.username}`}
                        autocomplete="new-password"
                        minlength="8"
                        required
                      />
                      <button type="submit" class="btn btn-primary btn-sm">Save</button>
                    </form>
                  </details>
                  {#if deleting === u.id}
                    <span class="confirm">Delete {u.username} and their progress?</span>
                    <button type="button" class="btn btn-danger btn-sm" onclick={() => removeUser(u)}>Yes, delete</button>
                    <button type="button" class="btn btn-ghost btn-sm" onclick={() => (deleting = null)}>No</button>
                  {:else}
                    <button type="button" class="btn btn-danger btn-sm" disabled={isSelf(u)} onclick={() => (deleting = u.id)}>
                      <Icon name="trash" size={14} />Delete
                    </button>
                  {/if}
                </div>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  </section>

  <section class="card card-pad block">
    <h2>Add a user</h2>
    <form class="adduser" onsubmit={createUser}>
      <label class="field">
        <span>Username</span>
        <input class="input" bind:value={newUsername} required minlength="3" maxlength="32" autocomplete="off" />
      </label>
      <label class="field">
        <span>Display name</span>
        <input class="input" bind:value={newDisplayName} maxlength="64" autocomplete="off" />
      </label>
      <label class="field">
        <span>Password</span>
        <input class="input" type="password" bind:value={newPassword} required minlength="8" autocomplete="new-password" />
      </label>
      <label class="field">
        <span>Role</span>
        <span class="select wide">
          <select class="input" bind:value={newRole}>
            <option value="user">User</option>
            <option value="admin">Administrator</option>
          </select>
        </span>
      </label>
      <div class="submit">
        <button type="submit" class="btn btn-primary" disabled={creating}>
          <Icon name="user-plus" size={18} />Create user
        </button>
      </div>
    </form>
  </section>
</div>

<style>
  .page-title {
    margin-bottom: clamp(20px, 3vw, 32px);
  }
  .top {
    margin-bottom: 16px;
  }
  .cols {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    gap: 20px;
    align-items: start;
  }
  /* the libraries table needs more room than the scan card (long paths, timestamps) */
  @media (min-width: 980px) {
    .cols {
      grid-template-columns: minmax(0, 1fr) minmax(0, 1.7fr);
    }
  }
  .path {
    min-width: 12ch;
    overflow-wrap: anywhere;
  }
  /* on a phone the tables scroll sideways inside their card instead of squeezing every column */
  .libs :global(.table) {
    min-width: 560px;
  }
  .people :global(.table) {
    min-width: 800px;
  }
  .people :global(.select) {
    min-width: 11rem;
  }
  .block {
    margin-top: 20px;
  }
  section h2 {
    margin-bottom: 1rem;
  }
  .head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    margin-bottom: 1rem;
  }
  .head h2 {
    margin: 0;
  }
  .current {
    margin-top: 2px;
    overflow-wrap: anywhere;
  }
  .bar {
    margin-top: 12px;
  }
  .problems {
    margin: 14px 0 0;
    padding: 12px 16px 12px 34px;
    border-radius: var(--r-md);
    background: var(--danger-soft);
    color: var(--danger);
    font-size: 0.875rem;
  }
  code {
    padding: 0.1em 0.4em;
    border-radius: 6px;
    background: var(--surface-2);
    overflow-wrap: anywhere;
  }
  .nowrap {
    white-space: nowrap;
  }
  .who {
    display: flex;
    align-items: center;
    gap: 0.7rem;
    font-weight: 600;
  }
  .avatar {
    flex: none;
    display: grid;
    place-items: center;
    width: 34px;
    height: 34px;
    border-radius: 50%;
    background: var(--accent-soft);
    color: var(--accent-text);
    font-size: 0.875rem;
    font-weight: 750;
  }
  .actions {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.4rem;
  }
  .confirm {
    color: var(--danger);
    font-size: 0.875rem;
    font-weight: 600;
  }
  details {
    position: relative;
  }
  summary {
    list-style: none;
    cursor: pointer;
  }
  summary::-webkit-details-marker {
    display: none;
  }
  details[open] > summary {
    background: var(--surface-2);
  }
  .pw {
    display: flex;
    gap: 0.4rem;
    margin-top: 0.5rem;
  }
  .pw .input {
    width: 12rem;
    border-radius: var(--r-pill);
  }
  .adduser {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(min(100%, 220px), 1fr));
    gap: 1rem;
    align-items: end;
  }
  .select.wide {
    display: block;
  }
  .submit {
    display: flex;
  }
</style>
