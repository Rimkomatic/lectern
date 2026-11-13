// Shapes of the JSON the Go API returns (internal/store/catalog.go and friends).

export type Role = 'admin' | 'user';

export interface User {
  id: number;
  username: string;
  display_name: string;
  role: Role;
  disabled: boolean;
  created_at: string;
  last_login_at: string | null;
}

export type ResumeMode = 'ask' | 'auto' | 'never';

export interface Settings {
  resume_mode: ResumeMode;
  autoplay_next: boolean;
  playback_rate: number;
  done_on_open: boolean;
}

export interface Me {
  user: User;
  settings: Settings;
}

export type Kind =
  | 'video'
  | 'audio'
  | 'pdf'
  | 'image'
  | 'markdown'
  | 'text'
  | 'csv'
  | 'sheet'
  | 'link'
  | 'other';

export interface Progress {
  state: 'started' | 'done';
  position: number;
  furthest: number;
  duration: number | null;
  page: number | null;
  pages: number | null;
  done_at: string | null;
  updated_at: string;
}

export interface Subtitle {
  id: number;
  lang: string;
  label: string;
  format: string;
}

/** A named stretch of a long video, from its "<name>.chapters.txt" file. */
export interface Chapter {
  /** Seconds from the start of the video. */
  start: number;
  title: string;
  /** Whether the signed-in person has finished it. */
  done: boolean;
}

export interface Item {
  id: number;
  course_id: number;
  section_id: number;
  name: string;
  title: string;
  kind: Kind;
  ext: string;
  mime: string;
  size: number;
  duration: number | null;
  pages: number | null;
  link_url?: string;
  ord: number;
  subtitles: Subtitle[];
  progress: Progress | null;
  /** Only on videos that have a chapters file. Each one counts as a lesson of its own. */
  chapters?: Chapter[];
}

export interface Section {
  id: number;
  title: string;
  ord: number;
  /** The implicit section that holds files lying directly in the course folder. */
  loose?: boolean;
  items: Item[];
}

export interface CourseSummary {
  id: number;
  title: string;
  description: string;
  library: string;
  /** An uploaded cover or an image found in the course folder. */
  has_cover: boolean;
  /** Version of the uploaded cover; 0 when there is none. Goes into the picture's URL. */
  cover_v: number;
  item_count: number;
  done_count: number;
  duration_s: number;
  last_seen_at: string | null;
}

/** What marking chapters returns: this person's chapters and the item's progress. */
export interface ChaptersReply {
  chapters: Chapter[];
  progress: Progress;
}

/** What changing a cover returns. */
export interface CoverState {
  has_cover: boolean;
  cover_v: number;
}

export interface Resume {
  item_id: number;
  section_id: number;
  title: string;
  kind: Kind;
  position: number;
  page: number | null;
  reason: 'continue' | 'next';
}

export interface CourseTree {
  course: CourseSummary;
  sections: Section[];
  resume: Resume | null;
  completed: boolean;
}

export interface ItemDetail {
  item: Item;
  course_title: string;
  section_title: string;
}

export interface ContinueItem {
  id: number;
  title: string;
  kind: Kind;
  duration: number | null;
  state: 'started' | 'done';
  position: number;
  page: number | null;
  pages: number | null;
}

export interface ContinueEntry {
  course: CourseSummary;
  item: ContinueItem | null;
}

export interface Library {
  id: number;
  label: string;
  path: string;
  enabled: boolean;
  last_scan_at: string | null;
  course_count: number;
}

export interface ScanCounts {
  courses: number;
  items: number;
  courses_new: number;
  courses_renamed: number;
  courses_missing: number;
  courses_failed: number;
  items_added: number;
  items_updated: number;
  items_moved: number;
  items_missing: number;
  /** Subtitle files that sit among videos but match none of them by name. */
  subs_unpaired?: number;
  /** The first of them, as "course/section/file.srt". */
  unpaired_subs?: string[];
}

/** When the server scans by itself (apart from start-up and "Scan now"). */
export interface ScanSchedule {
  mode: 'daily' | 'interval' | 'off';
  at?: string; // daily: "03:00"
  every_seconds?: number; // interval
  zone?: string; // daily: the time zone the time is in
}

export interface ScanStatus {
  running: boolean;
  trigger?: string;
  started_at?: string;
  finished_at?: string;
  library?: string;
  done: number;
  total: number;
  current?: string;
  errors?: string[];
  last?: ScanCounts;
  schedule?: ScanSchedule;
  /** When the next scan by the clock is due. */
  next_at?: string;
}

export type ProgressEventName =
  | 'heartbeat'
  | 'pause'
  | 'seek'
  | 'end'
  | 'open'
  | 'close'
  | 'page'
  | 'done';

export interface ProgressBody {
  event: ProgressEventName;
  position?: number;
  duration?: number;
  page?: number;
  pages?: number;
  done?: boolean;
  client_ts: number;
}
