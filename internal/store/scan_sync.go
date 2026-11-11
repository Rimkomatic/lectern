package store

import (
	"context"
	"database/sql"
	"time"
)

// ---- read side used by the scanner ----

type DBCourse struct {
	ID        uint64
	RelPath   string
	PathHash  []byte
	MissingAt *time.Time
}

func (s *Store) LibraryCourses(ctx context.Context, libraryID uint64) ([]DBCourse, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, rel_path, path_hash, missing_at FROM courses WHERE library_id = ?`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DBCourse
	for rows.Next() {
		var c DBCourse
		var miss sql.NullTime
		if err := rows.Scan(&c.ID, &c.RelPath, &c.PathHash, &miss); err != nil {
			return nil, err
		}
		c.MissingAt = timePtr(miss)
		out = append(out, c)
	}
	return out, rows.Err()
}

type DBItem struct {
	ID          uint64
	SectionID   uint64
	PathHash    []byte
	Size        int64
	Mtime       *time.Time
	Fingerprint []byte
	MissingAt   *time.Time
}

func (s *Store) CourseItems(ctx context.Context, courseID uint64) ([]DBItem, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, section_id, path_hash, size, mtime, fingerprint, missing_at FROM items WHERE course_id = ?`, courseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DBItem
	for rows.Next() {
		var it DBItem
		var mt, miss sql.NullTime
		if err := rows.Scan(&it.ID, &it.SectionID, &it.PathHash, &it.Size, &mt, &it.Fingerprint, &miss); err != nil {
			return nil, err
		}
		it.Mtime, it.MissingAt = timePtr(mt), timePtr(miss)
		out = append(out, it)
	}
	return out, rows.Err()
}

// CourseItemHashes returns the path hashes of every known item of a course
// (used to recognise a renamed course folder).
func (s *Store) CourseItemHashes(ctx context.Context, courseID uint64) (map[string]struct{}, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT path_hash FROM items WHERE course_id = ?`, courseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]struct{}{}
	for rows.Next() {
		var h []byte
		if err := rows.Scan(&h); err != nil {
			return nil, err
		}
		out[string(h)] = struct{}{}
	}
	return out, rows.Err()
}

// ---- write side ----

type SyncSubtitle struct{ RelPath, Lang, Label, Format string }

type SyncItem struct {
	ExistingID  uint64 // row matched by path
	MovedFromID uint64 // row matched by fingerprint (renamed/moved file): reuse it, keeping progress
	RelPath     string
	Name        string
	Title       string
	Kind        string
	Ext         string
	Mime        string
	Size        int64
	Mtime       time.Time
	Duration    *float64
	LinkURL     string
	Ord         int
	Fingerprint []byte
	Changed     bool // size or mtime differ from what is stored
	Subtitles   []SyncSubtitle
	Chapters    []ChapterDef // from the item's timestamp file; none if it has no such file
}

type SyncSection struct {
	RelPath string // "" = files directly in the course folder
	Title   string
	Ord     int
	Items   []SyncItem
}

type SyncCourse struct {
	LibraryID   uint64
	ExistingID  uint64
	RelPath     string
	Title       string
	Description string
	CoverRel    string
	DirMtime    time.Time
	Sections    []SyncSection
}

type SyncStats struct {
	CourseNew    bool
	ItemsAdded   int
	ItemsUpdated int
	ItemsMoved   int
	ItemsMissing int
}

type exItem struct {
	id, sectionID uint64
	title, kind   string
	name, ext     string
	mime, link    string
	size          int64
	mtime         *time.Time
	ord           int
	missing       bool
	chapters      []ChapterDef
}

// SyncCourse writes one scanned course (sections, items, subtitles) in a single
// transaction. Rows that were not seen are flagged missing, never deleted, so
// users keep their progress if files come back (or were only renamed).
func (s *Store) SyncCourse(ctx context.Context, sc SyncCourse) (uint64, SyncStats, error) {
	var st SyncStats
	now := nowUTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, st, err
	}
	defer tx.Rollback()

	// course row
	courseID := sc.ExistingID
	if courseID == 0 {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO courses (library_id, rel_path, path_hash, title, description, cover_rel, dir_mtime, created_at, updated_at)
			 VALUES (?,?,?,?,?,?,?,?,?)`,
			sc.LibraryID, sc.RelPath, PathHash(sc.RelPath), sc.Title, nullStr(sc.Description), nullStr(sc.CoverRel),
			sc.DirMtime.UTC(), now, now)
		if err != nil {
			return 0, st, err
		}
		id, _ := res.LastInsertId()
		courseID, st.CourseNew = uint64(id), true
	} else if _, err := tx.ExecContext(ctx,
		`UPDATE courses SET rel_path = ?, path_hash = ?, title = ?, description = ?, cover_rel = ?, dir_mtime = ?,
		        missing_at = NULL, updated_at = ? WHERE id = ?`,
		sc.RelPath, PathHash(sc.RelPath), sc.Title, nullStr(sc.Description), nullStr(sc.CoverRel),
		sc.DirMtime.UTC(), now, courseID); err != nil {
		return 0, st, err
	}

	// sections
	type exSec struct {
		id      uint64
		rel     string
		title   string
		ord     int
		missing bool
	}
	existingSec := map[string]exSec{}
	{
		rows, err := tx.QueryContext(ctx, `SELECT id, path_hash, rel_path, title, ord, missing_at FROM sections WHERE course_id = ?`, courseID)
		if err != nil {
			return 0, st, err
		}
		for rows.Next() {
			var e exSec
			var h []byte
			var miss sql.NullTime
			if err := rows.Scan(&e.id, &h, &e.rel, &e.title, &e.ord, &miss); err != nil {
				rows.Close()
				return 0, st, err
			}
			e.missing = miss.Valid
			existingSec[string(h)] = e
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return 0, st, err
		}
		rows.Close()
	}
	secID := make([]uint64, len(sc.Sections))
	touchedSec := map[uint64]bool{}
	for i, sec := range sc.Sections {
		h := PathHash(sec.RelPath)
		if e, ok := existingSec[string(h)]; ok {
			secID[i] = e.id
			if e.title != sec.Title || e.ord != sec.Ord || e.missing || e.rel != sec.RelPath {
				if _, err := tx.ExecContext(ctx,
					`UPDATE sections SET rel_path = ?, title = ?, ord = ?, missing_at = NULL WHERE id = ?`,
					sec.RelPath, sec.Title, sec.Ord, e.id); err != nil {
					return 0, st, err
				}
			}
		} else {
			res, err := tx.ExecContext(ctx,
				`INSERT INTO sections (course_id, rel_path, path_hash, title, ord) VALUES (?,?,?,?,?)`,
				courseID, sec.RelPath, h, sec.Title, sec.Ord)
			if err != nil {
				return 0, st, err
			}
			id, _ := res.LastInsertId()
			secID[i] = uint64(id)
		}
		touchedSec[secID[i]] = true
	}

	// existing items
	existingItems := map[uint64]exItem{}
	{
		rows, err := tx.QueryContext(ctx,
			`SELECT i.id, i.section_id, i.title, i.kind, i.name, i.ext, i.mime, i.size, i.mtime, i.link_url, i.ord, i.missing_at,
			        ic.chapters
			 FROM items i LEFT JOIN item_chapters ic ON ic.item_id = i.id WHERE i.course_id = ?`, courseID)
		if err != nil {
			return 0, st, err
		}
		for rows.Next() {
			var e exItem
			var mt, miss sql.NullTime
			var link, chapters sql.NullString
			if err := rows.Scan(&e.id, &e.sectionID, &e.title, &e.kind, &e.name, &e.ext, &e.mime, &e.size, &mt, &link, &e.ord, &miss,
				&chapters); err != nil {
				rows.Close()
				return 0, st, err
			}
			e.mtime, e.link, e.missing = timePtr(mt), link.String, miss.Valid
			e.chapters = parseDefs(chapters)
			existingItems[e.id] = e
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return 0, st, err
		}
		rows.Close()
	}

	touchedItem := map[uint64]bool{}
	type pendingSubs struct {
		itemID uint64
		subs   []SyncSubtitle
	}
	var subsToSync []pendingSubs

	for si, sec := range sc.Sections {
		for _, it := range sec.Items {
			mt := it.Mtime.UTC().Truncate(time.Millisecond)
			setDur := it.Duration != nil || it.Changed
			setFP := it.Fingerprint != nil || it.Changed
			var fp any
			if it.Fingerprint != nil {
				fp = it.Fingerprint
			}
			var itemID uint64

			switch {
			case it.MovedFromID != 0:
				itemID = it.MovedFromID
				if _, err := tx.ExecContext(ctx,
					`UPDATE items SET section_id = ?, rel_path = ?, path_hash = ?, name = ?, title = ?, kind = ?, ext = ?, mime = ?,
					        size = ?, mtime = ?, duration = IF(?, ?, duration), link_url = ?, ord = ?,
					        fingerprint = IF(?, ?, fingerprint), missing_at = NULL
					 WHERE id = ?`,
					secID[si], it.RelPath, PathHash(it.RelPath), it.Name, it.Title, it.Kind, it.Ext, it.Mime,
					it.Size, mt, setDur, it.Duration, nullStr(it.LinkURL), it.Ord, setFP, fp, itemID); err != nil {
					return 0, st, err
				}
				st.ItemsMoved++

			case it.ExistingID != 0:
				itemID = it.ExistingID
				e := existingItems[itemID]
				same := !e.missing && e.sectionID == secID[si] && e.title == it.Title && e.kind == it.Kind &&
					e.name == it.Name && e.ext == it.Ext && e.mime == it.Mime && e.size == it.Size &&
					e.mtime != nil && e.mtime.Equal(mt) &&
					e.link == it.LinkURL && e.ord == it.Ord && !setDur && !setFP
				if !same {
					if _, err := tx.ExecContext(ctx,
						`UPDATE items SET section_id = ?, name = ?, title = ?, kind = ?, ext = ?, mime = ?, size = ?, mtime = ?,
						        duration = IF(?, ?, duration), link_url = ?, ord = ?,
						        fingerprint = IF(?, ?, fingerprint), missing_at = NULL
						 WHERE id = ?`,
						secID[si], it.Name, it.Title, it.Kind, it.Ext, it.Mime, it.Size, mt,
						setDur, it.Duration, nullStr(it.LinkURL), it.Ord, setFP, fp, itemID); err != nil {
						return 0, st, err
					}
					st.ItemsUpdated++
				}

			default:
				res, err := tx.ExecContext(ctx,
					`INSERT INTO items (course_id, section_id, rel_path, path_hash, name, title, kind, ext, mime, size, mtime,
					                    duration, link_url, fingerprint, ord, created_at)
					 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
					courseID, secID[si], it.RelPath, PathHash(it.RelPath), it.Name, it.Title, it.Kind, it.Ext, it.Mime,
					it.Size, mt, it.Duration, nullStr(it.LinkURL), fp, it.Ord, now)
				if err != nil {
					return 0, st, err
				}
				id, _ := res.LastInsertId()
				itemID = uint64(id)
				st.ItemsAdded++
			}
			touchedItem[itemID] = true
			subsToSync = append(subsToSync, pendingSubs{itemID: itemID, subs: it.Subtitles})
			if err := writeChapters(ctx, tx, itemID, existingItems[itemID].chapters, it.Chapters); err != nil {
				return 0, st, err
			}
		}
	}

	// vanished items and sections
	var gone []uint64
	for id, e := range existingItems {
		if !touchedItem[id] && !e.missing {
			gone = append(gone, id)
		}
	}
	for start := 0; start < len(gone); start += 500 {
		end := min(start+500, len(gone))
		args := append([]any{now}, idArgs(gone[start:end])...)
		if _, err := tx.ExecContext(ctx,
			`UPDATE items SET missing_at = ? WHERE id IN (`+placeholders(end-start)+`)`, args...); err != nil {
			return 0, st, err
		}
	}
	st.ItemsMissing = len(gone)
	for _, e := range existingSec {
		if !touchedSec[e.id] && !e.missing {
			if _, err := tx.ExecContext(ctx, `UPDATE sections SET missing_at = ? WHERE id = ?`, now, e.id); err != nil {
				return 0, st, err
			}
		}
	}

	// subtitles: diff against what is stored for the touched items
	type exSub struct {
		id          uint64
		lang, label string
		format      string
	}
	existingSubs := map[uint64]map[string]exSub{}
	{
		rows, err := tx.QueryContext(ctx, `
			SELECT st.id, st.item_id, st.path_hash, st.lang, st.label, st.format
			FROM subtitles st JOIN items i ON i.id = st.item_id WHERE i.course_id = ?`, courseID)
		if err != nil {
			return 0, st, err
		}
		for rows.Next() {
			var e exSub
			var itemID uint64
			var h []byte
			if err := rows.Scan(&e.id, &itemID, &h, &e.lang, &e.label, &e.format); err != nil {
				rows.Close()
				return 0, st, err
			}
			if existingSubs[itemID] == nil {
				existingSubs[itemID] = map[string]exSub{}
			}
			existingSubs[itemID][string(h)] = e
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return 0, st, err
		}
		rows.Close()
	}
	for _, ps := range subsToSync {
		have := existingSubs[ps.itemID]
		want := map[string]bool{}
		for _, sub := range ps.subs {
			h := PathHash(sub.RelPath)
			want[string(h)] = true
			if e, ok := have[string(h)]; ok {
				if e.lang != sub.Lang || e.label != sub.Label || e.format != sub.Format {
					if _, err := tx.ExecContext(ctx, `UPDATE subtitles SET lang = ?, label = ?, format = ? WHERE id = ?`,
						sub.Lang, sub.Label, sub.Format, e.id); err != nil {
						return 0, st, err
					}
				}
				continue
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO subtitles (item_id, rel_path, path_hash, lang, label, format) VALUES (?,?,?,?,?,?)`,
				ps.itemID, sub.RelPath, h, sub.Lang, sub.Label, sub.Format); err != nil {
				return 0, st, err
			}
		}
		for h, e := range have {
			if !want[h] {
				if _, err := tx.ExecContext(ctx, `DELETE FROM subtitles WHERE id = ?`, e.id); err != nil {
					return 0, st, err
				}
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, st, err
	}
	return courseID, st, nil
}

// MarkCoursesMissing flags every non-missing course of the library that was not
// seen in the latest scan. It returns how many were flagged.
func (s *Store) MarkCoursesMissing(ctx context.Context, libraryID uint64, seen map[uint64]bool) (int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM courses WHERE library_id = ? AND missing_at IS NULL`, libraryID)
	if err != nil {
		return 0, err
	}
	var gone []uint64
	for rows.Next() {
		var id uint64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		if !seen[id] {
			gone = append(gone, id)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	now := nowUTC()
	for start := 0; start < len(gone); start += 500 {
		end := min(start+500, len(gone))
		args := append([]any{now}, idArgs(gone[start:end])...)
		if _, err := s.db.ExecContext(ctx,
			`UPDATE courses SET missing_at = ? WHERE id IN (`+placeholders(end-start)+`)`, args...); err != nil {
			return 0, err
		}
	}
	return len(gone), nil
}

// CountLiveCourses is the number of non-missing courses in a library.
func (s *Store) CountLiveCourses(ctx context.Context, libraryID uint64) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM courses WHERE library_id = ? AND missing_at IS NULL`, libraryID).Scan(&n)
	return n, err
}

// PurgeMissing deletes rows that have been missing since before the cutoff
// (cascading to their progress).
func (s *Store) PurgeMissing(ctx context.Context, before time.Time) (int64, error) {
	var total int64
	for _, q := range []string{
		`DELETE FROM items WHERE missing_at IS NOT NULL AND missing_at < ?`,
		`DELETE FROM sections WHERE missing_at IS NOT NULL AND missing_at < ?`,
		`DELETE FROM courses WHERE missing_at IS NOT NULL AND missing_at < ?`,
	} {
		res, err := s.db.ExecContext(ctx, q, before.UTC())
		if err != nil {
			return total, err
		}
		n, _ := res.RowsAffected()
		total += n
	}
	return total, nil
}

// ---- fingerprints (used to recognise renamed files) ----

type FPTarget struct {
	ItemID      uint64
	LibraryPath string
	CourseRel   string
	ItemRel     string
}

func (s *Store) FingerprintTargets(ctx context.Context, limit int) ([]FPTarget, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT i.id, l.path, c.rel_path, i.rel_path
		FROM items i
		JOIN courses c   ON c.id = i.course_id
		JOIN libraries l ON l.id = c.library_id
		WHERE i.fingerprint IS NULL AND i.size > 0 AND i.missing_at IS NULL AND c.missing_at IS NULL AND l.enabled = 1
		ORDER BY i.id LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FPTarget
	for rows.Next() {
		var t FPTarget
		if err := rows.Scan(&t.ItemID, &t.LibraryPath, &t.CourseRel, &t.ItemRel); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) SetFingerprint(ctx context.Context, itemID uint64, fp []byte) error {
	_, err := s.db.ExecContext(ctx, `UPDATE items SET fingerprint = ? WHERE id = ?`, fp, itemID)
	return err
}

// IsRetryable reports a MySQL deadlock or lock-wait timeout: the transaction
// was rolled back and is safe to run again.
func IsRetryable(err error) bool {
	n := mysqlErrNo(err)
	return n == 1213 || n == 1205
}
