import { ApiError, api, setUnauthorizedHandler } from './api';
import type { Me, Settings, User } from './types';

export const defaultSettings: Settings = {
  resume_mode: 'ask',
  autoplay_next: true,
  playback_rate: 1,
  done_on_open: true,
};

/** Who is signed in. Pages read this; only the functions below change it. */
export const session = $state<{
  ready: boolean;
  needsSetup: boolean;
  user: User | null;
  settings: Settings;
  version: string;
  error: string;
}>({
  ready: false,
  needsSetup: false,
  user: null,
  settings: { ...defaultSettings },
  version: '',
  error: '',
});

export function applyMe(me: Me): void {
  session.user = me.user;
  session.settings = me.settings;
  session.needsSetup = false;
}

/** Finds out whether the server needs first-time setup or we are signed in. */
export async function bootstrap(): Promise<void> {
  try {
    const s = await api.setupStatus();
    session.version = s.version;
    if (s.needs_setup) {
      session.needsSetup = true;
    } else {
      try {
        applyMe(await api.me());
      } catch (e) {
        if (!(e instanceof ApiError && e.status === 401)) throw e;
      }
    }
  } catch (e) {
    session.error = e instanceof Error ? e.message : String(e);
  } finally {
    session.ready = true;
  }
}

export async function signOut(): Promise<void> {
  try {
    await api.logout();
  } finally {
    session.user = null;
    session.settings = { ...defaultSettings };
  }
}

/** Saves settings on the server and keeps the local copy in step. */
export async function saveSettings(patch: Partial<Settings>): Promise<Settings> {
  const saved = await api.putSettings(patch);
  session.settings = saved;
  return saved;
}

// A 401 from any API call means the session ended: show the login page.
setUnauthorizedHandler(() => {
  session.user = null;
});
