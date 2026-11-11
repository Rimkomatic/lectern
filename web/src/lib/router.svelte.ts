// A tiny hash router: the whole app lives at "/" and the page is chosen by
// the part after "#", so the Go server needs no route table for it.

export type Route =
  | { name: 'home' }
  | { name: 'course'; courseId: number }
  | { name: 'item'; courseId: number; itemId: number; t?: number; p?: number }
  | { name: 'settings' }
  | { name: 'admin' }
  | { name: 'notfound' };

function id(s: string | undefined): number | null {
  if (!s || !/^[1-9]\d{0,15}$/.test(s)) return null;
  return Number(s);
}

function num(v: string | null): number | undefined {
  if (v === null || v === '') return undefined;
  const n = Number(v);
  return Number.isFinite(n) && n >= 0 ? n : undefined;
}

export function parseHash(hash: string): Route {
  let h = hash.startsWith('#') ? hash.slice(1) : hash;
  const q = h.indexOf('?');
  const query = new URLSearchParams(q >= 0 ? h.slice(q + 1) : '');
  if (q >= 0) h = h.slice(0, q);
  const parts = h.split('/').filter(Boolean);

  if (parts.length === 0) return { name: 'home' };
  switch (parts[0]) {
    case 'settings':
      return parts.length === 1 ? { name: 'settings' } : { name: 'notfound' };
    case 'admin':
      return parts.length === 1 ? { name: 'admin' } : { name: 'notfound' };
    case 'course': {
      const courseId = id(parts[1]);
      if (courseId === null) return { name: 'notfound' };
      if (parts.length === 2) return { name: 'course', courseId };
      if (parts.length === 4 && parts[2] === 'item') {
        const itemId = id(parts[3]);
        if (itemId === null) return { name: 'notfound' };
        return { name: 'item', courseId, itemId, t: num(query.get('t')), p: num(query.get('p')) };
      }
      return { name: 'notfound' };
    }
  }
  return { name: 'notfound' };
}

/** Builders for href values. */
export const links = {
  home: '#/',
  settings: '#/settings',
  admin: '#/admin',
  course: (courseId: number) => `#/course/${courseId}`,
  item: (courseId: number, itemId: number, opts: { t?: number; p?: number } = {}) => {
    const q = new URLSearchParams();
    if (opts.t !== undefined && opts.t > 0) q.set('t', String(Math.floor(opts.t)));
    if (opts.p !== undefined && opts.p > 1) q.set('p', String(Math.floor(opts.p)));
    const qs = q.toString();
    return `#/course/${courseId}/item/${itemId}${qs ? '?' + qs : ''}`;
  },
  /** A video opened at a chapter. Unlike item(), a start of 0 is kept: it means "from the top", not "resume". */
  chapter: (courseId: number, itemId: number, start: number) =>
    `#/course/${courseId}/item/${itemId}?t=${Math.max(0, Math.floor(start))}`,
};

export const router = $state<{ route: Route }>({ route: parseHash(location.hash) });

let started = false;
export function startRouter(): void {
  if (started) return;
  started = true;
  window.addEventListener('hashchange', () => {
    router.route = parseHash(location.hash);
    window.scrollTo(0, 0);
  });
}

export function go(hash: string): void {
  location.hash = hash;
}
