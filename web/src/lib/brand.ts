/** The product name. "Lectern" is a placeholder: change it here and in the Go server's UI strings. */
export const APP_NAME = 'Lectern';

/** "Lesson · Course · Lectern". Empty parts are skipped. */
export function setTitle(...parts: Array<string | null | undefined>): void {
  document.title = [...parts.filter((p): p is string => !!p), APP_NAME].join(' · ');
}
