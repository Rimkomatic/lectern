package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// CustomCover is a cover picture an admin uploaded for a course.
type CustomCover struct {
	Mime    string
	Version int64  // changes with every upload; goes into URLs and ETags
	Data    []byte // only filled in when asked for
}

// CoverState says whether a course shows a cover picture (an uploaded one or
// one found in its folder) and which version of the uploaded one it is.
type CoverState struct {
	HasCover bool  `json:"has_cover"`
	Version  int64 `json:"cover_v"` // 0 when there is no uploaded cover
}

// CustomCover returns the uploaded cover of a course that can be seen, or
// ErrNotFound. The picture itself is only read when withData is set, so a
// revalidation costs one tiny query.
func (s *Store) CustomCover(ctx context.Context, courseID uint64, withData bool) (*CustomCover, error) {
	cols := "cv.mime, cv.v"
	if withData {
		cols += ", cv.data"
	}
	var cc CustomCover
	dest := []any{&cc.Mime, &cc.Version}
	if withData {
		dest = append(dest, &cc.Data)
	}
	err := s.db.QueryRowContext(ctx, `
		SELECT `+cols+`
		FROM course_covers cv
		JOIN courses c   ON c.id = cv.course_id
		JOIN libraries l ON l.id = c.library_id
		WHERE cv.course_id = ? AND c.missing_at IS NULL AND l.enabled = 1`, courseID).Scan(dest...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &cc, nil
}

// CoverState reports the cover of a course that can be seen, or ErrNotFound.
func (s *Store) CoverState(ctx context.Context, courseID uint64) (CoverState, error) {
	var st CoverState
	err := s.db.QueryRowContext(ctx, `
		SELECT (c.cover_rel IS NOT NULL OR cv.course_id IS NOT NULL), COALESCE(cv.v, 0)
		FROM courses c
		JOIN libraries l ON l.id = c.library_id
		LEFT JOIN course_covers cv ON cv.course_id = c.id
		WHERE c.id = ? AND c.missing_at IS NULL AND l.enabled = 1`, courseID).Scan(&st.HasCover, &st.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return st, ErrNotFound
	}
	return st, err
}

// SetCustomCover stores the picture as the course's cover, replacing any
// earlier one, and returns its version. ErrNotFound if the course cannot be seen.
func (s *Store) SetCustomCover(ctx context.Context, courseID uint64, mime string, data []byte) (int64, error) {
	if _, err := s.CoverState(ctx, courseID); err != nil {
		return 0, err
	}
	v := time.Now().UnixMilli()
	// REPLACE writes the row whether or not one exists; nothing refers to this
	// table, so its delete half is harmless and the picture crosses the wire once.
	if _, err := s.db.ExecContext(ctx,
		`REPLACE INTO course_covers (course_id, mime, v, data, updated_at) VALUES (?,?,?,?,?)`,
		courseID, mime, v, data, nowUTC()); err != nil {
		return 0, err
	}
	return v, nil
}

// DeleteCustomCover removes the uploaded cover, so the course goes back to the
// image in its folder (or the drawn artwork). Having none is fine.
// ErrNotFound if the course cannot be seen.
func (s *Store) DeleteCustomCover(ctx context.Context, courseID uint64) error {
	if _, err := s.CoverState(ctx, courseID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM course_covers WHERE course_id = ?`, courseID)
	return err
}
