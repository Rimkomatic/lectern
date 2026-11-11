package store

import (
	"context"
	"database/sql"
	"time"
)

type Library struct {
	ID          uint64     `json:"id"`
	Label       string     `json:"label"`
	Path        string     `json:"path"`
	Enabled     bool       `json:"enabled"`
	LastScanAt  *time.Time `json:"last_scan_at"`
	CourseCount int        `json:"course_count"`
}

type LibrarySpec struct{ Label, Path string }

// SyncLibraries makes the libraries table mirror the configured roots:
// configured roots are upserted and enabled, any other root is disabled (its
// courses and all progress are kept).
func (s *Store) SyncLibraries(ctx context.Context, specs []LibrarySpec) error {
	hashes := make([]any, 0, len(specs))
	for _, sp := range specs {
		h := PathHash(sp.Path)
		hashes = append(hashes, h)
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO libraries (label, path, path_hash, enabled) VALUES (?,?,?,1)
			 ON DUPLICATE KEY UPDATE label = ?, path = ?, enabled = 1`,
			sp.Label, sp.Path, h, sp.Label, sp.Path); err != nil {
			return err
		}
	}
	if len(hashes) == 0 {
		_, err := s.db.ExecContext(ctx, `UPDATE libraries SET enabled = 0`)
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE libraries SET enabled = 0 WHERE path_hash NOT IN (`+placeholders(len(hashes))+`)`, hashes...)
	return err
}

func (s *Store) ListLibraries(ctx context.Context, enabledOnly bool) ([]Library, error) {
	q := `SELECT l.id, l.label, l.path, l.enabled, l.last_scan_at,
	             (SELECT COUNT(*) FROM courses c WHERE c.library_id = l.id AND c.missing_at IS NULL)
	      FROM libraries l`
	if enabledOnly {
		q += ` WHERE l.enabled = 1`
	}
	q += ` ORDER BY l.id`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Library{}
	for rows.Next() {
		var l Library
		var last sql.NullTime
		if err := rows.Scan(&l.ID, &l.Label, &l.Path, &l.Enabled, &last, &l.CourseCount); err != nil {
			return nil, err
		}
		l.LastScanAt = timePtr(last)
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Store) SetLibraryScanned(ctx context.Context, id uint64, t time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE libraries SET last_scan_at = ? WHERE id = ?`, t.UTC(), id)
	return err
}
