import type {
  ChaptersReply,
  ContinueEntry,
  CourseSummary,
  CourseTree,
  CoverState,
  ItemDetail,
  Library,
  Me,
  Progress,
  ProgressBody,
  ScanStatus,
  Settings,
  User,
} from './types';

/** An error response from the server: {"error":{"code","message"}}. */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;

  constructor(status: number, code: string, message: string) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
  }
}

let onUnauthorized: (() => void) | null = null;

/** Called when the server says the session is gone (any 401 except a failed login). */
export function setUnauthorizedHandler(fn: (() => void) | null): void {
  onUnauthorized = fn;
}

/** JSON in, JSON out; a Blob body is sent as it is (picture uploads). */
async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  let res: Response;
  try {
    const raw = body instanceof Blob;
    res = await fetch('/api/v1' + path, {
      method,
      credentials: 'same-origin',
      headers:
        body !== undefined
          ? { 'Content-Type': raw ? body.type || 'application/octet-stream' : 'application/json' }
          : undefined,
      body: body !== undefined ? (raw ? body : JSON.stringify(body)) : undefined,
    });
  } catch {
    throw new ApiError(0, 'network', 'Cannot reach the server. Check your connection.');
  }
  if (res.status === 204) return undefined as T;

  const text = await res.text();
  let data: unknown = null;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      // not JSON (e.g. a proxy error page)
    }
  }
  if (!res.ok) {
    const err = (data as { error?: { code?: string; message?: string } } | null)?.error;
    if (res.status === 401 && path !== '/auth/login') onUnauthorized?.();
    throw new ApiError(res.status, err?.code ?? 'error', err?.message ?? `Request failed (${res.status})`);
  }
  return data as T;
}

export const api = {
  setupStatus: () => request<{ needs_setup: boolean; version: string }>('GET', '/setup'),
  setup: (username: string, password: string, display_name: string) =>
    request<Me>('POST', '/setup', { username, password, display_name }),
  login: (username: string, password: string) => request<Me>('POST', '/auth/login', { username, password }),
  logout: () => request<void>('POST', '/auth/logout'),

  me: () => request<Me>('GET', '/me'),
  updateProfile: (display_name: string) => request<User>('PATCH', '/me', { display_name }),
  changePassword: (current_password: string, new_password: string) =>
    request<void>('PUT', '/me/password', { current_password, new_password }),
  putSettings: (s: Partial<Settings>) => request<Settings>('PUT', '/me/settings', s),

  courses: (q = '') => request<CourseSummary[]>('GET', '/courses' + (q ? `?q=${encodeURIComponent(q)}` : '')),
  course: (id: number) => request<CourseTree>('GET', `/courses/${id}`),
  continue: () => request<ContinueEntry[]>('GET', '/continue'),
  item: (id: number) => request<ItemDetail>('GET', `/items/${id}`),
  progress: (id: number, body: ProgressBody) =>
    request<{ progress: Progress }>('POST', `/items/${id}/progress`, body),
  /** Finishes and un-finishes chapters of a video; each is named by its start in whole seconds. */
  chapters: (id: number, body: { done?: number[]; undone?: number[] }) =>
    request<ChaptersReply>('POST', `/items/${id}/chapters`, body),

  admin: {
    libraries: () => request<Library[]>('GET', '/admin/libraries'),
    scanStatus: () => request<ScanStatus>('GET', '/admin/scan'),
    startScan: () => request<{ started: boolean }>('POST', '/admin/scan'),
    users: () => request<User[]>('GET', '/admin/users'),
    createUser: (u: { username: string; display_name: string; password: string; role: string }) =>
      request<User>('POST', '/admin/users', u),
    patchUser: (
      id: number,
      p: { display_name?: string; role?: string; disabled?: boolean; password?: string },
    ) => request<User>('PATCH', `/admin/users/${id}`, p),
    deleteUser: (id: number) => request<void>('DELETE', `/admin/users/${id}`),
    setCover: (courseId: number, picture: Blob) =>
      request<CoverState>('PUT', `/admin/courses/${courseId}/cover`, picture),
    removeCover: (courseId: number) => request<CoverState>('DELETE', `/admin/courses/${courseId}/cover`),
  },
};

/** URLs the browser loads directly (media elements, downloads, images). */
export const url = {
  stream: (itemId: number) => `/api/v1/items/${itemId}/stream`,
  download: (itemId: number) => `/api/v1/items/${itemId}/download`,
  chapters: (itemId: number) => `/api/v1/items/${itemId}/chapters`,
  /** v is the course's cover_v: a new upload gets a new URL, so no stale picture is shown. */
  cover: (courseId: number, v = 0) => `/api/v1/courses/${courseId}/cover${v > 0 ? `?v=${v}` : ''}`,
  subtitle: (id: number) => `/api/v1/subtitles/${id}`,
  events: '/api/v1/admin/events',
};

/** A readable message for anything thrown by the API layer. */
export function messageOf(e: unknown): string {
  return e instanceof Error ? e.message : String(e);
}
