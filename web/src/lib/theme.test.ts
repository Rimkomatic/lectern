import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

function stubSystemDark(dark: boolean) {
  const listeners: Array<() => void> = [];
  const mql = {
    matches: dark,
    addEventListener: (_: string, fn: () => void) => listeners.push(fn),
    removeEventListener: () => {},
  };
  vi.stubGlobal('matchMedia', () => mql);
  return {
    flip(next: boolean) {
      mql.matches = next;
      listeners.forEach((fn) => fn());
    },
  };
}

async function load() {
  vi.resetModules();
  return await import('./theme.svelte');
}

describe('theme', () => {
  beforeEach(() => {
    localStorage.clear();
    document.documentElement.removeAttribute('data-theme');
    document.head.innerHTML = '<meta name="theme-color" content="#000">';
  });
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  it('follows the system until a choice is made', async () => {
    stubSystemDark(true);
    const t = await load();
    t.initTheme();
    expect(t.theme.pref).toBe('system');
    expect(t.theme.effective).toBe('dark');
    expect(document.documentElement.getAttribute('data-theme')).toBe('dark');
    expect(document.querySelector('meta[name="theme-color"]')?.getAttribute('content')).toBe('#0d0f12');
  });

  it('uses the saved choice over the system setting', async () => {
    stubSystemDark(true);
    localStorage.setItem('lectern-theme', 'light');
    const t = await load();
    t.initTheme();
    expect(t.theme.pref).toBe('light');
    expect(document.documentElement.getAttribute('data-theme')).toBe('light');
  });

  it('toggle flips what is showing and remembers it', async () => {
    stubSystemDark(false);
    const t = await load();
    t.initTheme();
    t.toggleTheme();
    expect(t.theme.effective).toBe('dark');
    expect(localStorage.getItem('lectern-theme')).toBe('dark');
    t.toggleTheme();
    expect(t.theme.effective).toBe('light');
    expect(localStorage.getItem('lectern-theme')).toBe('light');
  });

  it('going back to "system" forgets the saved choice and tracks the OS again', async () => {
    const sys = stubSystemDark(false);
    const t = await load();
    t.initTheme();
    t.setTheme('dark');
    t.setTheme('system');
    expect(localStorage.getItem('lectern-theme')).toBeNull();
    expect(t.theme.effective).toBe('light');
    sys.flip(true);
    expect(t.theme.effective).toBe('dark');
  });

  it('ignores OS changes while a choice is pinned', async () => {
    const sys = stubSystemDark(false);
    const t = await load();
    t.initTheme();
    t.setTheme('light');
    sys.flip(true);
    expect(t.theme.effective).toBe('light');
  });

  it('ignores garbage in storage', async () => {
    stubSystemDark(false);
    localStorage.setItem('lectern-theme', 'purple');
    const t = await load();
    t.initTheme();
    expect(t.theme.pref).toBe('system');
  });
});
