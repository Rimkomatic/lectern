-- Chapters of a long video: the list of timestamps read from "<name>.chapters.txt",
-- and each person's finished chapters. A chapter is identified by its start in
-- whole seconds, so editing titles or adding chapters keeps the others' marks.

CREATE TABLE IF NOT EXISTS item_chapters (
  item_id  BIGINT UNSIGNED NOT NULL,
  chapters JSON            NOT NULL,
  n        INT UNSIGNED    NOT NULL,
  PRIMARY KEY (item_id),
  CONSTRAINT fk_item_chapters_item FOREIGN KEY (item_id) REFERENCES items (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS chapter_progress (
  user_id BIGINT UNSIGNED NOT NULL,
  item_id BIGINT UNSIGNED NOT NULL,
  start_s INT UNSIGNED    NOT NULL,
  done_at DATETIME(3)     NOT NULL,
  PRIMARY KEY (user_id, item_id, start_s),
  KEY idx_chapter_progress_item (item_id),
  CONSTRAINT fk_chprog_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
  CONSTRAINT fk_chprog_item FOREIGN KEY (item_id) REFERENCES items (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
