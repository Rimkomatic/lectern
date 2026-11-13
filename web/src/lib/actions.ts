import { api } from './api';
import { chapterFraction, chapterIndexAt, doneChapters, hasChapters } from './chapters';
import { formatBytes, formatDuration } from './format';
import type { IconName } from './icons';
import type { CourseTree, Item, Kind, Progress, Section } from './types';

/** Marks an item done or not done. Returns the server's merged progress. */
export async function setDone(itemId: number, done: boolean): Promise<Progress> {
  const r = await api.progress(itemId, { event: 'done', done, client_ts: Date.now() });
  return r.progress;
}

export function isDone(item: Item): boolean {
  return item.progress?.state === 'done';
}

/** After an item is marked done or not done, a video's chapters follow it. */
export function followItem(item: Item, done: boolean): void {
  for (const c of item.chapters ?? []) c.done = done;
}

/** How many lessons an item stands for: one, or one per chapter of a video that has them. */
export function lessons(item: Item): number {
  return item.chapters?.length ? item.chapters.length : 1;
}

/** How many of its lessons are finished. A finished item has all of them finished. */
export function lessonsDone(item: Item): number {
  if (isDone(item)) return lessons(item);
  return hasChapters(item) ? doneChapters(item.chapters ?? []) : 0;
}

/** True for a course that is just a list of files: no folders, only the implicit section. */
export function isFlat(sections: Section[]): boolean {
  return sections.length === 1 && sections[0].loose === true;
}

/**
 * Whether this person has anything recorded in the course: a lesson opened or
 * finished, a chapter ticked, or a place to continue from. "Drop course" is
 * only offered then, since there is nothing to delete otherwise.
 */
export function hasTracking(tree: CourseTree): boolean {
  if (tree.course.last_seen_at) return true;
  return flatten(tree.sections).some((it) => it.progress !== null || doneChapters(it.chapters ?? []) > 0);
}

/** All items of a course in learning order. */
export function flatten(sections: Section[]): Item[] {
  return sections.flatMap((s) => s.items);
}

export function findItem(sections: Section[], itemId: number): Item | undefined {
  for (const s of sections) {
    const it = s.items.find((i) => i.id === itemId);
    if (it) return it;
  }
  return undefined;
}

/** Counts finished lessons; each chapter of a video is a lesson of its own. */
export function doneCount(sections: Section[]): { done: number; total: number } {
  let done = 0;
  let total = 0;
  for (const s of sections) {
    for (const it of s.items) {
      total += lessons(it);
      done += lessonsDone(it);
    }
  }
  return { done, total };
}

export type StatusKey = 'done' | 'started' | 'new';

/** done / started (opened but not finished) / new (never opened). */
export function statusKey(item: Item): StatusKey {
  if (isDone(item)) return 'done';
  return item.progress ? 'started' : 'new';
}

export function statusLabel(item: Item): string {
  const names: Record<StatusKey, string> = { done: 'done', started: 'in progress', new: 'not started' };
  return names[statusKey(item)];
}

/**
 * How far through an item someone is, 0..1 (1 once it is done). Items that
 * were opened but have no measurable length show a small stub so "started" is visible.
 */
export function fraction(item: Item): number {
  const p = item.progress;
  if (!p) return 0;
  if (p.state === 'done') return 1;
  const clamp = (v: number) => Math.min(0.97, Math.max(0.05, v));
  if ((item.kind === 'video' || item.kind === 'audio') && item.chapters?.length) {
    // finished chapters, plus how far into the one being played
    const ch = item.chapters;
    const i = chapterIndexAt(ch, p.position);
    const partial = i >= 0 && !ch[i].done ? chapterFraction(ch, i, p.position, p.duration ?? item.duration) : 0;
    return clamp((doneChapters(ch) + partial) / ch.length);
  }
  if (item.kind === 'video' || item.kind === 'audio') {
    const dur = p.duration ?? item.duration ?? 0;
    if (dur > 0) return clamp(p.position / dur);
  }
  if (item.kind === 'pdf') {
    const pages = p.pages ?? item.pages ?? 0;
    if (pages > 0 && p.page) return clamp(p.page / pages);
  }
  return 0.1;
}

/** Finished and total lessons, and the media time, of one section. */
export function sectionStats(section: Section): { done: number; total: number; seconds: number } {
  let done = 0;
  let total = 0;
  let seconds = 0;
  for (const it of section.items) {
    total += lessons(it);
    done += lessonsDone(it);
    if ((it.kind === 'video' || it.kind === 'audio') && it.duration) seconds += it.duration;
  }
  return { done, total, seconds };
}

const KIND_ICONS: Record<Kind, IconName> = {
  video: 'video',
  audio: 'audio',
  pdf: 'pdf',
  image: 'image',
  markdown: 'notes',
  text: 'text',
  csv: 'table',
  sheet: 'sheet',
  link: 'link',
  other: 'file',
};

export function kindIcon(kind: Kind): IconName {
  return KIND_ICONS[kind] ?? 'file';
}

export function kindLabel(item: Item): string {
  const names: Record<string, string> = {
    video: 'Video',
    audio: 'Audio',
    pdf: 'PDF',
    image: 'Image',
    markdown: 'Notes',
    text: 'Text',
    csv: 'Table',
    sheet: 'Spreadsheet',
    link: 'Link',
    other: 'File',
  };
  return names[item.kind] ?? 'File';
}

/** The short detail shown next to an item: its length, page count or size. Empty for links. */
export function itemMeta(item: Item): string {
  switch (item.kind) {
    case 'video':
    case 'audio': {
      const n = item.chapters?.length ?? 0;
      return [formatDuration(item.duration), n > 0 ? `${n} ${n === 1 ? 'chapter' : 'chapters'}` : '']
        .filter(Boolean)
        .join(' · ');
    }
    case 'pdf':
      return item.pages ? `${item.pages} ${item.pages === 1 ? 'page' : 'pages'}` : formatBytes(item.size);
    case 'link':
      return '';
    default:
      return formatBytes(item.size);
  }
}
