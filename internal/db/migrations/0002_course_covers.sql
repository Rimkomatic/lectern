-- Course covers chosen by an admin. The picture is kept here rather than in the
-- library folder, which Lectern never writes to. A row outranks any cover image
-- found in the folder and goes away together with its course.
-- v is the upload time in milliseconds: it changes with every upload, so it can
-- go into the picture's URL and ETag.

CREATE TABLE IF NOT EXISTS course_covers (
  course_id  BIGINT UNSIGNED NOT NULL,
  mime       VARCHAR(32)     NOT NULL,
  v          BIGINT          NOT NULL,
  data       MEDIUMBLOB      NOT NULL,
  updated_at DATETIME(3)     NOT NULL,
  PRIMARY KEY (course_id),
  CONSTRAINT fk_covers_course FOREIGN KEY (course_id) REFERENCES courses (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
