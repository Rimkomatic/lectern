<script lang="ts">
  import { APP_NAME } from '../lib/brand';
  import { links, router } from '../lib/router.svelte';
  import { session, signOut } from '../lib/session.svelte';
  import Icon from './Icon.svelte';
  import Logo from './Logo.svelte';
  import ThemeToggle from './ThemeToggle.svelte';

  const name = $derived(session.user?.display_name || session.user?.username || '');
  const initial = $derived([...name][0]?.toLocaleUpperCase() ?? '?');
  const onCourses = $derived(
    router.route.name === 'home' || router.route.name === 'course' || router.route.name === 'item',
  );
</script>

<header class="bar">
  <div class="inner">
    <a class="brand" href={links.home}>
      <Logo size={32} />
      <span class="word">{APP_NAME}</span>
    </a>

    <nav aria-label="Main">
      <a class="link" href={links.home} aria-current={onCourses ? 'page' : undefined}>
        <Icon name="courses" size={18} /><span>Courses</span>
      </a>
      {#if session.user?.role === 'admin'}
        <a class="link" href={links.admin} aria-current={router.route.name === 'admin' ? 'page' : undefined}>
          <Icon name="shield" size={18} /><span>Admin</span>
        </a>
      {/if}
    </nav>

    <div class="tools">
      <ThemeToggle />
      <a class="me" href={links.settings} aria-current={router.route.name === 'settings' ? 'page' : undefined}>
        <span class="avatar" aria-hidden="true">{initial}</span>
        <span class="who">{name}</span>
      </a>
      <button type="button" class="btn btn-ghost btn-icon" title="Sign out" onclick={() => void signOut()}>
        <Icon name="log-out" /><span class="sr-only">Sign out</span>
      </button>
    </div>
  </div>
</header>

<style>
  .bar {
    position: sticky;
    top: 0;
    z-index: 30;
    height: var(--nav-h);
    border-bottom: 1px solid var(--border);
    background: var(--glass);
    -webkit-backdrop-filter: saturate(1.5) blur(16px);
    backdrop-filter: saturate(1.5) blur(16px);
  }
  .inner {
    display: flex;
    align-items: center;
    gap: 0.5rem 1.25rem;
    height: 100%;
    padding-inline: clamp(12px, 2.2vw, 28px);
  }
  .brand {
    display: inline-flex;
    align-items: center;
    gap: 0.65rem;
    color: var(--text);
    text-decoration: none;
    border-radius: var(--r-pill);
  }
  .word {
    font-size: 1.1875rem;
    font-weight: 750;
    letter-spacing: -0.035em;
  }
  nav {
    display: flex;
    align-items: center;
    gap: 0.25rem;
    margin-right: auto;
  }
  .link {
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
    height: 40px;
    padding: 0 1rem;
    border-radius: var(--r-pill);
    color: var(--text-2);
    font-size: 0.9375rem;
    font-weight: 600;
    text-decoration: none;
    transition:
      background-color var(--dur),
      color var(--dur);
  }
  .link:hover {
    background: var(--surface-2);
    color: var(--text);
    text-decoration: none;
  }
  .link[aria-current='page'] {
    background: var(--accent-soft);
    color: var(--accent-text);
  }
  .tools {
    display: flex;
    align-items: center;
    gap: 0.25rem;
  }
  .me {
    display: inline-flex;
    align-items: center;
    gap: 0.6rem;
    height: 44px;
    padding: 0 0.95rem 0 0.4rem;
    border-radius: var(--r-pill);
    color: var(--text);
    font-size: 0.875rem;
    font-weight: 600;
    text-decoration: none;
    transition: background-color var(--dur);
  }
  .me:hover {
    background: var(--surface-2);
    text-decoration: none;
  }
  .me[aria-current='page'] {
    background: var(--surface-2);
  }
  .avatar {
    display: grid;
    place-items: center;
    width: 32px;
    height: 32px;
    border-radius: 50%;
    background: linear-gradient(135deg, var(--accent), color-mix(in srgb, var(--accent) 55%, #e5603f));
    color: var(--on-accent);
    font-size: 0.875rem;
    font-weight: 750;
  }
  .who {
    max-width: 14ch;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  @media (max-width: 720px) {
    .link {
      padding: 0;
      width: 40px;
      justify-content: center;
    }
    .link span {
      position: absolute;
      width: 1px;
      height: 1px;
      overflow: hidden;
      clip-path: inset(50%);
      white-space: nowrap;
    }
    .who {
      display: none;
    }
    .me {
      padding: 0 0.4rem;
    }
  }
  @media (max-width: 420px) {
    .word {
      display: none;
    }
  }
</style>
