// State of the course-content sidebar on the lesson page that has to outlive
// a single lesson: the page is rebuilt for every item, but the sidebar should
// not blink, forget which sections were open, or jump back to the top.

import type { CourseTree } from './types';

const SIDEBAR_KEY = 'lectern-sidebar';

function readOpen(): boolean {
  try {
    return localStorage.getItem(SIDEBAR_KEY) !== '0';
  } catch {
    return true;
  }
}

/** Whether the sidebar is open on a wide screen. Remembered per device. */
export const sidebar = $state({ open: readOpen() });

export function setSidebarOpen(open: boolean): void {
  sidebar.open = open;
  try {
    localStorage.setItem(SIDEBAR_KEY, open ? '1' : '0');
  } catch {
    // not saved, still applied for this visit
  }
}

interface Memory {
  courseId: number;
  tree: CourseTree | null;
  expanded: number[];
  scrollTop: number;
}

// One course is enough: people watch one course at a time.
let memory: Memory | null = null;

/** What was left behind for this course, if the last lesson page belonged to it. */
export function recall(courseId: number): Memory | null {
  return memory && memory.courseId === courseId ? memory : null;
}

export function remember(courseId: number, patch: Partial<Omit<Memory, 'courseId'>>): void {
  const base: Memory =
    memory && memory.courseId === courseId ? memory : { courseId, tree: null, expanded: [], scrollTop: 0 };
  memory = { ...base, ...patch };
}

/** For tests. */
export function forgetAll(): void {
  memory = null;
}
