package store

import (
	"context"
	"database/sql"
	"math"
	"time"
)

// ProgressInput is what a client reports about an item.
type ProgressInput struct {
	Event      string // open | heartbeat | pause | seek | end | close | page | done
	Position   *float64
	Duration   *float64
	Page       *int
	Pages      *int
	Done       *bool // explicit mark done / not done
	ClientTS   int64 // client clock in ms; used to ignore stale out-of-order reports
	DoneOnOpen bool  // resolved from the user's settings for non-media kinds
	// Chaptered is set for a video or audio file that has chapters: it is done
	// when all of its chapters are (see MarkChapters), never by how far it played.
	Chaptered bool
}

type ItemRef struct {
	ID       uint64
	CourseID uint64
	Kind     string
	Chapters int // number of chapters (0 = none)
}

func isMedia(kind string) bool { return kind == "video" || kind == "audio" }

func validSeconds(f float64) bool { return !math.IsNaN(f) && !math.IsInf(f, 0) && f >= 0 && f < 1e7 }

// MergeProgress folds a client report into the stored state. It is a pure
// function so the rules are easy to test:
//   - explicit Done wins and nothing else changes
//   - stale reports (older ClientTS) never move the position back
//   - "done" is sticky: re-watching never un-completes an item
//   - media completes at the threshold fraction of its duration or on "end",
//     unless it has chapters (then only finishing its chapters completes it)
//   - pdf completes on its last page, everything else on open (if enabled)
func MergeProgress(prev *Progress, in ProgressInput, kind string, threshold float64, now time.Time) Progress {
	p := Progress{State: "started"}
	if prev != nil {
		p = *prev
	}
	p.UpdatedAt = now

	if in.Done != nil {
		if *in.Done {
			if p.State != "done" {
				t := now
				p.State, p.DoneAt = "done", &t
			}
		} else {
			p.State, p.DoneAt = "started", nil
		}
		return p
	}

	if in.Duration != nil && validSeconds(*in.Duration) && *in.Duration > 0 {
		d := *in.Duration
		p.Duration = &d
	}
	if in.Pages != nil && *in.Pages > 0 {
		n := *in.Pages
		p.Pages = &n
	}

	fresh := in.ClientTS == 0 || in.ClientTS >= p.ClientTS
	if fresh {
		if in.Position != nil && isMedia(kind) && validSeconds(*in.Position) {
			pos := *in.Position
			if p.Duration != nil && pos > *p.Duration {
				pos = *p.Duration
			}
			p.Position = pos
			if pos > p.Furthest {
				p.Furthest = pos
			}
		}
		if in.Page != nil && kind == "pdf" && *in.Page > 0 {
			n := *in.Page
			p.Page = &n
		}
		if in.ClientTS > p.ClientTS {
			p.ClientTS = in.ClientTS
		}
	}

	if p.State != "done" {
		done := false
		switch {
		case isMedia(kind) && in.Chaptered:
			done = false // finished chapter by chapter
		case isMedia(kind):
			done = in.Event == "end" ||
				(p.Duration != nil && *p.Duration > 0 && p.Position >= threshold**p.Duration)
		case kind == "pdf":
			done = p.Pages != nil && p.Page != nil && *p.Page >= *p.Pages
		default:
			done = in.Event == "open" && in.DoneOnOpen
		}
		if done {
			t := now
			p.State, p.DoneAt = "done", &t
		}
	}
	return p
}

func (s *Store) GetProgress(ctx context.Context, userID, itemID uint64) (*Progress, error) {
	p, err := scanProgress(s.db.QueryRowContext(ctx, progressSelect+` WHERE user_id = ? AND item_id = ?`, userID, itemID))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return p, err
}

const progressSelect = `SELECT state, position, furthest, duration, page, pages, done_at, client_ts, updated_at FROM progress`

func scanProgress(sc scanner) (*Progress, error) {
	var p Progress
	var dur sql.NullFloat64
	var page, pages sql.NullInt64
	var done sql.NullTime
	if err := sc.Scan(&p.State, &p.Position, &p.Furthest, &dur, &page, &pages, &done, &p.ClientTS, &p.UpdatedAt); err != nil {
		return nil, err
	}
	p.Duration, p.Page, p.Pages, p.DoneAt = floatPtr(dur), intPtr(page), intPtr(pages), timePtr(done)
	return &p, nil
}

// ApplyProgress merges a report into the user's progress row inside one
// transaction (row locked) and records the item as the course's last-seen item.
//
// Concurrent reports for the same row (two tabs, or a beacon racing a normal
// request) and first plays of the same item by several users can make InnoDB
// pick a deadlock victim; those are safe to repeat, so they are retried.
func (s *Store) ApplyProgress(ctx context.Context, userID uint64, ref ItemRef, in ProgressInput, threshold float64) (*Progress, error) {
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * 25 * time.Millisecond):
			}
		}
		p, err := s.applyProgressOnce(ctx, userID, ref, in, threshold)
		if err == nil {
			s.backfillItemMeta(ctx, ref, p)
			return p, nil
		}
		if !IsRetryable(err) {
			return nil, err
		}
		lastErr = err
	}
	return nil, lastErr
}

func (s *Store) applyProgressOnce(ctx context.Context, userID uint64, ref ItemRef, in ProgressInput, threshold float64) (*Progress, error) {
	now := nowUTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// A no-op upsert rather than INSERT IGNORE: when the row exists it takes
	// the exclusive lock straight away, instead of a shared one that the
	// following SELECT ... FOR UPDATE would have to upgrade (a deadlock
	// waiting to happen with two concurrent reports).
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO progress (user_id, item_id, state, position, furthest, client_ts, updated_at)
		 VALUES (?, ?, 'started', 0, 0, 0, ?)
		 ON DUPLICATE KEY UPDATE user_id = user_id`, userID, ref.ID, now); err != nil {
		return nil, err
	}
	prev, err := scanProgress(tx.QueryRowContext(ctx, progressSelect+` WHERE user_id = ? AND item_id = ? FOR UPDATE`, userID, ref.ID))
	if err != nil {
		return nil, err
	}
	in.Chaptered = in.Chaptered || (ref.Chapters > 0 && isMedia(ref.Kind))
	next := MergeProgress(prev, in, ref.Kind, threshold, now)

	// Marking a video with chapters done (or not done) does the same to every chapter.
	if in.Done != nil && ref.Chapters > 0 {
		if err := markAllChapters(ctx, tx, userID, ref.ID, *in.Done, now); err != nil {
			return nil, err
		}
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE progress SET state = ?, position = ?, furthest = ?, duration = ?, page = ?, pages = ?,
		        done_at = ?, client_ts = ?, updated_at = ?
		 WHERE user_id = ? AND item_id = ?`,
		next.State, next.Position, next.Furthest, next.Duration, next.Page, next.Pages,
		next.DoneAt, next.ClientTS, next.UpdatedAt, userID, ref.ID); err != nil {
		return nil, err
	}

	if in.Done == nil {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO course_state (user_id, course_id, last_item_id, last_seen_at) VALUES (?,?,?,?)
			 ON DUPLICATE KEY UPDATE last_item_id = ?, last_seen_at = ?`,
			userID, ref.CourseID, ref.ID, now, ref.ID, now); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &next, nil
}

// backfillItemMeta fills in item metadata the scanner could not read (the
// first player to report it wins). It runs after the progress transaction so
// that many users starting the same new item do not queue on its row; failing
// is harmless, the next report tries again.
func (s *Store) backfillItemMeta(ctx context.Context, ref ItemRef, p *Progress) {
	if p.Duration != nil && isMedia(ref.Kind) {
		_, _ = s.db.ExecContext(ctx, `UPDATE items SET duration = ? WHERE id = ? AND duration IS NULL`, *p.Duration, ref.ID)
	}
	if p.Pages != nil && ref.Kind == "pdf" {
		_, _ = s.db.ExecContext(ctx, `UPDATE items SET pages = ? WHERE id = ? AND pages IS NULL`, *p.Pages, ref.ID)
	}
}

// ComputeResume picks what to offer when a user re-opens a course.
//   - last item unfinished  -> continue it ("continue")
//   - last item finished    -> next unfinished item in course order ("next")
//   - everything finished   -> nil, completed = true
//   - never opened          -> nil, completed = false (or "next" if some items
//     were marked done by hand, or the remembered item no longer exists)
func ComputeResume(sections []Section, lastItemID uint64) (*Resume, bool) {
	type ref struct {
		it  *Item
		sec *Section
	}
	var flat []ref
	for si := range sections {
		for ii := range sections[si].Items {
			flat = append(flat, ref{&sections[si].Items[ii], &sections[si]})
		}
	}
	if len(flat) == 0 {
		return nil, false
	}
	isDone := func(it *Item) bool { return it.Progress != nil && it.Progress.State == "done" }

	all := true
	for _, r := range flat {
		if !isDone(r.it) {
			all = false
			break
		}
	}
	if all {
		return nil, true
	}

	mk := func(r ref, reason string) *Resume {
		out := &Resume{ItemID: r.it.ID, SectionID: r.sec.ID, Title: r.it.Title, Kind: r.it.Kind, Reason: reason}
		if reason == "continue" && r.it.Progress != nil {
			out.Position = r.it.Progress.Position
			out.Page = r.it.Progress.Page
		}
		return out
	}

	last := -1
	for i, r := range flat {
		if r.it.ID == lastItemID {
			last = i
			break
		}
	}
	if last < 0 {
		// The remembered item is gone (removed from disk) or nothing was ever
		// opened. If the learner has finished things anyway, point at the first
		// item they have not; otherwise let the UI offer "start course".
		for _, r := range flat {
			if isDone(r.it) {
				for _, r2 := range flat {
					if !isDone(r2.it) {
						return mk(r2, "next"), false
					}
				}
			}
		}
		return nil, false
	}
	if !isDone(flat[last].it) {
		return mk(flat[last], "continue"), false
	}
	for i := last + 1; i < len(flat); i++ {
		if !isDone(flat[i].it) {
			return mk(flat[i], "next"), false
		}
	}
	for i := 0; i < last; i++ {
		if !isDone(flat[i].it) {
			return mk(flat[i], "next"), false
		}
	}
	return nil, false
}
