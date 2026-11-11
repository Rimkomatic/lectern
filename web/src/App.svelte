<script lang="ts">
  import { onMount } from 'svelte';
  import { links, router, startRouter } from './lib/router.svelte';
  import { bootstrap, session } from './lib/session.svelte';
  import Icon from './components/Icon.svelte';
  import Logo from './components/Logo.svelte';
  import Nav from './components/Nav.svelte';
  import Admin from './pages/Admin.svelte';
  import Course from './pages/Course.svelte';
  import Home from './pages/Home.svelte';
  import Login from './pages/Login.svelte';
  import Settings from './pages/Settings.svelte';
  import Setup from './pages/Setup.svelte';
  import Watch from './pages/Watch.svelte';

  const route = $derived(router.route);

  onMount(() => {
    startRouter();
    void bootstrap();
  });

  // The address bar holds the route (#/...), so the usual "#main" skip link would break routing.
  function skipToContent(e: MouseEvent) {
    e.preventDefault();
    document.getElementById('main')?.focus();
  }
</script>

{#if !session.ready}
  <div class="splash" role="status">
    <Logo size={48} />
    <span class="sr-only">Loading…</span>
  </div>
{:else if session.error}
  <main id="main" class="splash">
    <div class="card card-pad fail">
      <p class="alert" role="alert"><Icon name="circle-alert" size={18} />{session.error}</p>
      <button type="button" class="btn btn-primary" onclick={() => location.reload()}>Try again</button>
    </div>
  </main>
{:else if session.needsSetup}
  <Setup />
{:else if !session.user}
  <Login />
{:else}
  <a class="skip" href={links.home} onclick={skipToContent}>Skip to content</a>
  <Nav />
  {#if route.name === 'item'}
    <!-- A new item, or the same item at another time or page, starts a fresh page. -->
    {#key `${route.courseId}/${route.itemId}/${route.t ?? ''}/${route.p ?? ''}`}
      <Watch courseId={route.courseId} itemId={route.itemId} t={route.t} p={route.p} />
    {/key}
  {:else}
    <main id="main" tabindex="-1" class="page" class:page-narrow={route.name === 'settings' || route.name === 'notfound'}>
      {#if route.name === 'home'}
        <Home />
      {:else if route.name === 'course'}
        {#key route.courseId}
          <Course courseId={route.courseId} />
        {/key}
      {:else if route.name === 'settings'}
        <Settings />
      {:else if route.name === 'admin'}
        {#if session.user.role === 'admin'}
          <Admin />
        {:else}
          <p class="alert" role="alert"><Icon name="circle-alert" size={18} />Only administrators can open this page.</p>
        {/if}
      {:else}
        <div class="lost rise">
          <h1>Page not found</h1>
          <p class="muted">That address doesn't lead anywhere.</p>
          <a class="btn btn-primary" href={links.home}><Icon name="arrow-left" size={18} />Back to courses</a>
        </div>
      {/if}
    </main>
  {/if}
{/if}

<style>
  .splash {
    min-height: 100dvh;
    display: grid;
    place-items: center;
    padding: 24px;
  }
  .fail {
    display: grid;
    gap: 1.25rem;
    justify-items: start;
    max-width: 440px;
  }
  .lost {
    display: grid;
    gap: 1rem;
    justify-items: start;
    padding-top: 6vh;
  }
  main:focus {
    outline: none;
  }
</style>
