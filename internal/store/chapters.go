package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"
)

// ChapterDef is a chapter as read from the timestamp file and stored with the item.
type ChapterDef struct {
	Start float64 `json:"start"` // seconds from the beginning
	Title string  `json:"title"`
}

// Chapter is a chapter as one person sees it.
type Chapter struct {
	Start float64 `json:"start"`
	Title string  `json:"title"`
	Done  bool    `json:"done"`
}

// ChapterKey identifies a chapter: its start in whole seconds. Marks follow a
// chapter through edits to its title and through chapters being added around it.
func ChapterKey(start float64) uint32 {
	if math.IsNaN(start) || start < 0 {
		return 0
	}
	return uint32(math.Min(math.Floor(start), math.MaxUint32))
}

// SameChapters reports whether two lists describe the same chapters.
func SameChapters(a, b []ChapterDef) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if math.Abs(a[i].Start-b[i].Start) > 1e-6 || a[i].Title != b[i].Title {
			return false
		}
	}
	return true
}

func parseDefs(raw sql.NullString) []ChapterDef {
	if !raw.Valid || strings.TrimSpace(raw.String) == "" {
		return nil
	}
	var defs []ChapterDef
	if err := json.Unmarshal([]byte(raw.String), &defs); err != nil {
		return nil
	}
	return defs
}

// chaptersOf turns stored chapters into what a person sees. A finished item has
// every chapter finished; otherwise the keys in marks are.
func chaptersOf(defs []ChapterDef, itemDone bool, marks map[uint32]bool) []Chapter {
	if len(defs) == 0 {
		return nil
	}
	out := make([]Chapter, len(defs))
	for i, d := range defs {
		out[i] = Chapter{Start: d.Start, Title: d.Title, Done: itemDone || marks[ChapterKey(d.Start)]}
	}
	return out
}

// userMarks returns, for the given items, the chapters this user has finished.
func (s *Store) userMarks(ctx context.Context, userID uint64, where string, args ...any) (map[uint64]map[uint32]bool, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT cp.item_id, cp.start_s FROM chapter_progress cp JOIN items i ON i.id = cp.item_id
		 WHERE cp.user_id = ? AND `+where, append([]any{userID}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uint64]map[uint32]bool{}
	for rows.Next() {
		var id uint64
		var k uint32
		if err := rows.Scan(&id, &k); err != nil {
			return nil, err
		}
		if out[id] == nil {
			out[id] = map[uint32]bool{}
		}
		out[id][k] = true
	}
	return out, rows.Err()
}

// writeChapters stores (or clears) an item's chapters inside the scanner's
// transaction. Marks on chapters that no longer exist are dropped.
func writeChapters(ctx context.Context, tx *sql.Tx, itemID uint64, old, want []ChapterDef) error {
	if SameChapters(old, want) {
		return nil
	}
	if len(want) == 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM item_chapters WHERE item_id = ?`, itemID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM chapter_progress WHERE item_id = ?`, itemID)
		return err
	}
	blob, err := json.Marshal(want)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO item_chapters (item_id, chapters, n) VALUES (?,?,?)
		 ON DUPLICATE KEY UPDATE chapters = VALUES(chapters), n = VALUES(n)`,
		itemID, string(blob), len(want)); err != nil {
		return err
	}
	keep := make([]uint64, len(want))
	for i, d := range want {
		keep[i] = uint64(ChapterKey(d.Start))
	}
	args := append([]any{itemID}, idArgs(keep)...)
	_, err = tx.ExecContext(ctx,
		`DELETE FROM chapter_progress WHERE item_id = ? AND start_s NOT IN (`+placeholders(len(keep))+`)`, args...)
	return err
}

// ItemChapters returns an item's chapters as stored, or nil.
func (s *Store) ItemChapters(ctx context.Context, itemID uint64) ([]ChapterDef, error) {
	var raw sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT chapters FROM item_chapters WHERE item_id = ?`, itemID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return parseDefs(raw), nil
}

// markAllChapters finishes (or clears) every chapter of an item for one user,
// for "mark done" / "mark not done" on the whole item.
func markAllChapters(ctx context.Context, tx *sql.Tx, userID, itemID uint64, done bool, now time.Time) error {
	if !done {
		_, err := tx.ExecContext(ctx, `DELETE FROM chapter_progress WHERE user_id = ? AND item_id = ?`, userID, itemID)
		return err
	}
	var raw sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT chapters FROM item_chapters WHERE item_id = ?`, itemID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return insertMarks(ctx, tx, userID, itemID, keysOf(parseDefs(raw)), now)
}

func keysOf(defs []ChapterDef) []uint32 {
	out := make([]uint32, len(defs))
	for i, d := range defs {
		out[i] = ChapterKey(d.Start)
	}
	return out
}

func insertMarks(ctx context.Context, tx *sql.Tx, userID, itemID uint64, keys []uint32, now time.Time) error {
	for start := 0; start < len(keys); start += 200 {
		end := min(start+200, len(keys))
		var sb strings.Builder
		sb.WriteString(`INSERT INTO chapter_progress (user_id, item_id, start_s, done_at) VALUES `)
		args := make([]any, 0, (end-start)*4)
		for i := start; i < end; i++ {
			if i > start {
				sb.WriteByte(',')
			}
			sb.WriteString("(?,?,?,?)")
			args = append(args, userID, itemID, keys[i], now)
		}
		sb.WriteString(` ON DUPLICATE KEY UPDATE done_at = done_at`)
		if _, err := tx.ExecContext(ctx, sb.String(), args...); err != nil {
			return err
		}
	}
	return nil
}

// MarkChapters finishes and un-finishes chapters of one item for one user and
// keeps the item's own state in step: the item is done exactly when all of its
// chapters are. Starts that are not chapters of the item are ignored. It
// returns the user's chapters and the item's progress after the change.
func (s *Store) MarkChapters(ctx context.Context, userID uint64, itemID uint64, done, undone []uint32) ([]Chapter, *Progress, error) {
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * 25 * time.Millisecond):
			}
		}
		chs, p, err := s.markChaptersOnce(ctx, userID, itemID, done, undone)
		if err == nil || !IsRetryable(err) {
			return chs, p, err
		}
		lastErr = err
	}
	return nil, nil, lastErr
}

func (s *Store) markChaptersOnce(ctx context.Context, userID, itemID uint64, done, undone []uint32) ([]Chapter, *Progress, error) {
	now := nowUTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()

	var raw sql.NullString
	err = tx.QueryRowContext(ctx,
		`SELECT ic.chapters FROM item_chapters ic JOIN items i ON i.id = ic.item_id
		 JOIN courses c ON c.id = i.course_id
		 WHERE ic.item_id = ? AND i.missing_at IS NULL AND c.missing_at IS NULL`, itemID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	defs := parseDefs(raw)
	valid := map[uint32]bool{}
	for _, d := range defs {
		valid[ChapterKey(d.Start)] = true
	}

	// Same lock order as ApplyProgress: the progress row first.
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO progress (user_id, item_id, state, position, furthest, client_ts, updated_at)
		 VALUES (?, ?, 'started', 0, 0, 0, ?)
		 ON DUPLICATE KEY UPDATE user_id = user_id`, userID, itemID, now); err != nil {
		return nil, nil, err
	}
	prev, err := scanProgress(tx.QueryRowContext(ctx, progressSelect+` WHERE user_id = ? AND item_id = ? FOR UPDATE`, userID, itemID))
	if err != nil {
		return nil, nil, err
	}

	keep := func(in []uint32) []uint32 {
		var out []uint32
		seen := map[uint32]bool{}
		for _, k := range in {
			if valid[k] && !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
		return out
	}
	done, undone = keep(done), keep(undone)

	// A finished item has every chapter finished; spell that out first, so that
	// taking one away leaves the others (and a no-op leaves the item done).
	if prev.State == "done" {
		if err := insertMarks(ctx, tx, userID, itemID, keysOf(defs), now); err != nil {
			return nil, nil, err
		}
	}
	if err := insertMarks(ctx, tx, userID, itemID, done, now); err != nil {
		return nil, nil, err
	}
	for _, k := range undone {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM chapter_progress WHERE user_id = ? AND item_id = ? AND start_s = ?`, userID, itemID, k); err != nil {
			return nil, nil, err
		}
	}

	marks := map[uint32]bool{}
	rows, err := tx.QueryContext(ctx, `SELECT start_s FROM chapter_progress WHERE user_id = ? AND item_id = ?`, userID, itemID)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var k uint32
		if err := rows.Scan(&k); err != nil {
			rows.Close()
			return nil, nil, err
		}
		marks[k] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, nil, err
	}
	rows.Close()

	all := true
	for _, d := range defs {
		if !marks[ChapterKey(d.Start)] {
			all = false
			break
		}
	}
	next := *prev
	switch {
	case all && prev.State != "done":
		t := now
		next.State, next.DoneAt, next.UpdatedAt = "done", &t, now
	case !all && prev.State == "done":
		next.State, next.DoneAt, next.UpdatedAt = "started", nil, now
	}
	if next.State != prev.State {
		if _, err := tx.ExecContext(ctx,
			`UPDATE progress SET state = ?, done_at = ?, updated_at = ? WHERE user_id = ? AND item_id = ?`,
			next.State, next.DoneAt, next.UpdatedAt, userID, itemID); err != nil {
			return nil, nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	return chaptersOf(defs, next.State == "done", marks), &next, nil
}
