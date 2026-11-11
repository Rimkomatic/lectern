-- Lectern initial schema (MySQL 8 / InnoDB / utf8mb4).
-- Identity of courses, sections and items is path_hash = SHA-256(relative path),
-- so long paths never hit InnoDB's index key limit and trailing-space/collation
-- quirks cannot merge two different files.

CREATE TABLE IF NOT EXISTS users (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  username      VARCHAR(64)  NOT NULL,
  display_name  VARCHAR(128) NOT NULL DEFAULT '',
  pw_hash       VARCHAR(255) NOT NULL,
  role          ENUM('admin','user') NOT NULL DEFAULT 'user',
  disabled      TINYINT(1)   NOT NULL DEFAULT 0,
  created_at    DATETIME(3)  NOT NULL,
  last_login_at DATETIME(3)  NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uq_users_username (username)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS sessions (
  id           BINARY(32)      NOT NULL,
  user_id      BIGINT UNSIGNED NOT NULL,
  created_at   DATETIME(3)     NOT NULL,
  last_seen_at DATETIME(3)     NOT NULL,
  expires_at   DATETIME(3)     NOT NULL,
  user_agent   VARCHAR(255)    NOT NULL DEFAULT '',
  ip           VARCHAR(64)     NOT NULL DEFAULT '',
  PRIMARY KEY (id),
  KEY idx_sessions_user (user_id),
  KEY idx_sessions_expires (expires_at),
  CONSTRAINT fk_sessions_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS libraries (
  id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  label        VARCHAR(128)    NOT NULL,
  path         VARCHAR(1024)   NOT NULL,
  path_hash    BINARY(32)      NOT NULL,
  enabled      TINYINT(1)      NOT NULL DEFAULT 1,
  last_scan_at DATETIME(3)     NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uq_libraries_path (path_hash)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS courses (
  id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  library_id  BIGINT UNSIGNED NOT NULL,
  rel_path    VARCHAR(512)    NOT NULL,
  path_hash   BINARY(32)      NOT NULL,
  title       VARCHAR(512)    NOT NULL,
  description TEXT            NULL,
  cover_rel   VARCHAR(1024)   NULL,
  dir_mtime   DATETIME(3)     NULL,
  missing_at  DATETIME(3)     NULL,
  created_at  DATETIME(3)     NOT NULL,
  updated_at  DATETIME(3)     NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uq_courses_path (library_id, path_hash),
  KEY idx_courses_missing (missing_at),
  CONSTRAINT fk_courses_library FOREIGN KEY (library_id) REFERENCES libraries (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS sections (
  id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  course_id  BIGINT UNSIGNED NOT NULL,
  rel_path   VARCHAR(512)    NOT NULL,
  path_hash  BINARY(32)      NOT NULL,
  title      VARCHAR(512)    NOT NULL,
  ord        INT             NOT NULL DEFAULT 0,
  missing_at DATETIME(3)     NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uq_sections_path (course_id, path_hash),
  KEY idx_sections_missing (missing_at),
  CONSTRAINT fk_sections_course FOREIGN KEY (course_id) REFERENCES courses (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS items (
  id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  course_id   BIGINT UNSIGNED NOT NULL,
  section_id  BIGINT UNSIGNED NOT NULL,
  rel_path    TEXT            NOT NULL,
  path_hash   BINARY(32)      NOT NULL,
  name        VARCHAR(512)    NOT NULL,
  title       VARCHAR(512)    NOT NULL,
  kind        ENUM('video','audio','pdf','image','markdown','text','csv','sheet','link','other') NOT NULL,
  ext         VARCHAR(16)     NOT NULL DEFAULT '',
  mime        VARCHAR(127)    NOT NULL DEFAULT '',
  size        BIGINT UNSIGNED NOT NULL DEFAULT 0,
  mtime       DATETIME(3)     NULL,
  duration    DOUBLE          NULL,
  pages       INT UNSIGNED    NULL,
  link_url    VARCHAR(2048)   NULL,
  fingerprint BINARY(32)      NULL,
  ord         INT             NOT NULL DEFAULT 0,
  missing_at  DATETIME(3)     NULL,
  created_at  DATETIME(3)     NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uq_items_path (course_id, path_hash),
  KEY idx_items_section (section_id, ord),
  KEY idx_items_fingerprint (size, fingerprint),
  KEY idx_items_missing (missing_at),
  CONSTRAINT fk_items_course  FOREIGN KEY (course_id)  REFERENCES courses (id)  ON DELETE CASCADE,
  CONSTRAINT fk_items_section FOREIGN KEY (section_id) REFERENCES sections (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS subtitles (
  id        BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  item_id   BIGINT UNSIGNED NOT NULL,
  rel_path  TEXT            NOT NULL,
  path_hash BINARY(32)      NOT NULL,
  lang      VARCHAR(32)     NOT NULL DEFAULT 'und',
  label     VARCHAR(128)    NOT NULL DEFAULT '',
  format    ENUM('vtt','srt') NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uq_subtitles_path (item_id, path_hash),
  CONSTRAINT fk_subtitles_item FOREIGN KEY (item_id) REFERENCES items (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS progress (
  user_id    BIGINT UNSIGNED NOT NULL,
  item_id    BIGINT UNSIGNED NOT NULL,
  state      ENUM('started','done') NOT NULL DEFAULT 'started',
  position   DOUBLE          NOT NULL DEFAULT 0,
  furthest   DOUBLE          NOT NULL DEFAULT 0,
  duration   DOUBLE          NULL,
  page       INT UNSIGNED    NULL,
  pages      INT UNSIGNED    NULL,
  done_at    DATETIME(3)     NULL,
  client_ts  BIGINT          NOT NULL DEFAULT 0,
  updated_at DATETIME(3)     NOT NULL,
  PRIMARY KEY (user_id, item_id),
  KEY idx_progress_item (item_id),
  CONSTRAINT fk_progress_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
  CONSTRAINT fk_progress_item FOREIGN KEY (item_id) REFERENCES items (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS course_state (
  user_id      BIGINT UNSIGNED NOT NULL,
  course_id    BIGINT UNSIGNED NOT NULL,
  last_item_id BIGINT UNSIGNED NULL,
  last_seen_at DATETIME(3)     NOT NULL,
  PRIMARY KEY (user_id, course_id),
  KEY idx_course_state_recent (user_id, last_seen_at),
  CONSTRAINT fk_cstate_user   FOREIGN KEY (user_id)      REFERENCES users (id)   ON DELETE CASCADE,
  CONSTRAINT fk_cstate_course FOREIGN KEY (course_id)    REFERENCES courses (id) ON DELETE CASCADE,
  CONSTRAINT fk_cstate_item   FOREIGN KEY (last_item_id) REFERENCES items (id)   ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS user_settings (
  user_id    BIGINT UNSIGNED NOT NULL,
  data       JSON            NOT NULL,
  updated_at DATETIME(3)     NOT NULL,
  PRIMARY KEY (user_id),
  CONSTRAINT fk_settings_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
