// Light/dark theme. The choice is per device (localStorage), not per account:
// "system" follows the operating system, "light" and "dark" are explicit.
// public/theme-init.js applies the saved choice before first paint; this
// module keeps it in step afterwards.

export type ThemePref = 'system' | 'light' | 'dark';
export type Theme = 'light' | 'dark';

export const THEME_KEY = 'lectern-theme';
const META_COLOR: Record<Theme, string> = { light: '#f6f4ef', dark: '#0d0f12' };

/** What the page is showing now, and what the person asked for. */
export const theme = $state<{ pref: ThemePref; effective: Theme }>({ pref: 'system', effective: 'light' });

function darkQuery(): MediaQueryList | null {
  return typeof window !== 'undefined' && typeof window.matchMedia === 'function'
    ? window.matchMedia('(prefers-color-scheme: dark)')
    : null;
}

export function readPref(): ThemePref {
  try {
    const v = localStorage.getItem(THEME_KEY);
    if (v === 'light' || v === 'dark') return v;
  } catch {
    // storage blocked: behave like "system"
  }
  return 'system';
}

export function resolve(pref: ThemePref): Theme {
  if (pref === 'light' || pref === 'dark') return pref;
  return darkQuery()?.matches ? 'dark' : 'light';
}

function paint(t: Theme, animate: boolean): void {
  const root = document.documentElement;
  if (animate) {
    root.classList.add('theme-fade');
    setTimeout(() => root.classList.remove('theme-fade'), 320);
  }
  root.setAttribute('data-theme', t);
  document.querySelector('meta[name="theme-color"]')?.setAttribute('content', META_COLOR[t]);
}

function sync(animate: boolean): void {
  theme.effective = resolve(theme.pref);
  paint(theme.effective, animate);
}

let started = false;
/** Call once at startup, before the app is mounted. */
export function initTheme(): void {
  theme.pref = readPref();
  sync(false);
  if (started) return;
  started = true;
  darkQuery()?.addEventListener('change', () => {
    if (theme.pref === 'system') sync(true);
  });
  // another tab changed it
  window.addEventListener('storage', (e) => {
    if (e.key !== THEME_KEY && e.key !== null) return;
    theme.pref = readPref();
    sync(true);
  });
}

export function setTheme(pref: ThemePref): void {
  theme.pref = pref;
  try {
    if (pref === 'system') localStorage.removeItem(THEME_KEY);
    else localStorage.setItem(THEME_KEY, pref);
  } catch {
    // not saved, still applied for this visit
  }
  sync(true);
}

/** The quick switch in the top bar: flips whatever is showing now. */
export function toggleTheme(): void {
  setTheme(theme.effective === 'dark' ? 'light' : 'dark');
}
