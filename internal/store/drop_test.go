package store

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

// recorded counts what is stored for one person in one course: item state,
// finished chapters and the continue-learning entry.
func (f *fixture) recorded(t *testing.T, userID, courseID uint64) (items, chapters, state int) {
	t.Helper()
	count := func(q string) int {
		var n int
		if err := f.s.db.QueryRowContext(context.Background(), q, userID, courseID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	items = count(`SELECT COUNT(*) FROM progress p JOIN items i ON i.id = p.item_id WHERE p.user_id = ? AND i.course_id = ?`)
	chapters = count(`SELECT COUNT(*) FROM chapter_progress cp JOIN items i ON i.id = cp.item_id WHERE cp.user_id = ? AND i.course_id = ?`)
	state = count(`SELECT COUNT(*) FROM course_state WHERE user_id = ? AND course_id = ?`)
	return
}

func TestDropCourse(t *testing.T) {
	f := newFixture(t)
	s, ctx := f.s, context.Background()
	ann, bob, cy := mustUser(t, s, "ann", "user"), mustUser(t, s, "bob", "user"), mustUser(t, s, "cy", "user")

	// A second course, so that "this course" can be told from "everything".
	libs, err := s.ListLibraries(ctx, true)
	if err != nil || len(libs) != 1 {
		t.Fatalf("libraries: %v %+v", err, libs)
	}
	mtime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	other, _, err := s.SyncCourse(ctx, SyncCourse{
		LibraryID: libs[0].ID, RelPath: "Other", Title: "Other", DirMtime: mtime,
		Sections: []SyncSection{{RelPath: "S", Title: "S", Ord: 1, Items: []SyncItem{{
			RelPath: "S/x.mp4", Name: "x.mp4", Title: "x", Kind: "video", Mtime: mtime, Size: 10, Ord: 1,
			Fingerprint: bytes.Repeat([]byte{9}, 32),
		}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	otherTree, err := s.CourseTree(ctx, other, 0)
	if err != nil || len(otherTree.Sections) != 1 || len(otherTree.Sections[0].Items) != 1 {
		t.Fatalf("second course: %v %+v", err, otherTree)
	}
	otherItem := otherTree.Sections[0].Items[0].ID
	otherRef := ItemRef{ID: otherItem, CourseID: other, Kind: "video"}

	report := func(u *User, ref ItemRef, in ProgressInput) {
		t.Helper()
		if _, err := s.ApplyProgress(ctx, u.ID, ref, in, 0.95); err != nil {
			t.Fatal(err)
		}
	}
	tick := func(u *User, item uint64) { // a finished chapter
		t.Helper()
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO chapter_progress (user_id, item_id, start_s, done_at) VALUES (?, ?, 0, ?)`, u.ID, item, mtime); err != nil {
			t.Fatal(err)
		}
	}

	// Ann and Bob have watched a bit of both courses and ticked a chapter in each.
	for _, u := range []*User{ann, bob} {
		report(u, f.ref(f.video, "video"), ProgressInput{Event: "heartbeat", Position: f64(30), Duration: f64(100), ClientTS: 1})
		report(u, f.ref(f.pdf, "pdf"), ProgressInput{Event: "page", Page: intp(2), Pages: intp(9), ClientTS: 1})
		report(u, otherRef, ProgressInput{Event: "heartbeat", Position: f64(5), Duration: f64(50), ClientTS: 1})
		tick(u, f.video)
		tick(u, otherItem)
	}
	// Cy only marked one lesson done by hand: that leaves no continue-learning entry.
	report(cy, f.ref(f.link, "link"), ProgressInput{Event: "done", Done: boolp(true), ClientTS: 1})

	// A file that vanished from the folder keeps its progress for a while; that goes too.
	if _, err := s.db.ExecContext(ctx, `UPDATE items SET missing_at = ? WHERE id = ?`, mtime, f.pdf); err != nil {
		t.Fatal(err)
	}

	type counts struct{ items, chapters, state int }
	got := func(u *User, course uint64) counts {
		i, c, st := f.recorded(t, u.ID, course)
		return counts{i, c, st}
	}
	for _, u := range []*User{ann, bob} {
		if g := got(u, f.course); g != (counts{2, 1, 1}) {
			t.Fatalf("%s before: first course %+v", u.Username, g)
		}
		if g := got(u, other); g != (counts{1, 1, 1}) {
			t.Fatalf("%s before: second course %+v", u.Username, g)
		}
	}
	if g := got(cy, f.course); g != (counts{1, 0, 0}) {
		t.Fatalf("cy before: %+v", g)
	}
	before, err := s.CourseTree(ctx, f.course, ann.ID)
	if err != nil || before.Course.LastSeenAt == nil {
		t.Fatalf("ann's course before: %v %+v", err, before)
	}

	t.Run("drops one person's progress in one course", func(t *testing.T) {
		if err := s.DropCourse(ctx, ann.ID, f.course); err != nil {
			t.Fatal(err)
		}
		if g := got(ann, f.course); g != (counts{}) {
			t.Errorf("ann's first course still has %+v", g)
		}
		if g := got(ann, other); g != (counts{1, 1, 1}) {
			t.Errorf("ann's second course changed: %+v", g)
		}
		if g := got(bob, f.course); g != (counts{2, 1, 1}) {
			t.Errorf("bob's first course changed: %+v", g)
		}
		if g := got(bob, other); g != (counts{1, 1, 1}) {
			t.Errorf("bob's second course changed: %+v", g)
		}
		if g := got(cy, f.course); g != (counts{1, 0, 0}) {
			t.Errorf("cy changed: %+v", g)
		}
	})

	t.Run("the course looks untouched to her, the files stay", func(t *testing.T) {
		after, err := s.CourseTree(ctx, f.course, ann.ID)
		if err != nil {
			t.Fatal(err)
		}
		if after.Resume != nil || after.Completed || after.Course.DoneCount != 0 || after.Course.LastSeenAt != nil {
			t.Errorf("tree after: resume=%+v completed=%v done=%d last=%v", after.Resume, after.Completed, after.Course.DoneCount, after.Course.LastSeenAt)
		}
		if after.Course.ItemCount != before.Course.ItemCount {
			t.Errorf("item count %d, was %d", after.Course.ItemCount, before.Course.ItemCount)
		}
		for _, sec := range after.Sections {
			for _, it := range sec.Items {
				if it.Progress != nil {
					t.Errorf("%s still has progress %+v", it.Title, it.Progress)
				}
			}
		}
		cont, err := s.Continue(ctx, ann.ID, 12)
		if err != nil || len(cont) != 1 || cont[0].Course.ID != other {
			t.Errorf("continue-learning = %+v (%v), want only the second course", cont, err)
		}
		bobs, err := s.Continue(ctx, bob.ID, 12)
		if err != nil || len(bobs) != 2 {
			t.Errorf("bob's continue-learning = %d entries (%v), want 2", len(bobs), err)
		}
	})

	t.Run("can be repeated", func(t *testing.T) {
		if err := s.DropCourse(ctx, ann.ID, f.course); err != nil {
			t.Fatalf("second drop: %v", err)
		}
		if g := got(ann, other); g != (counts{1, 1, 1}) {
			t.Errorf("ann's second course changed: %+v", g)
		}
	})

	t.Run("a lesson marked done by hand goes too", func(t *testing.T) {
		if err := s.DropCourse(ctx, cy.ID, f.course); err != nil {
			t.Fatal(err)
		}
		if g := got(cy, f.course); g != (counts{}) {
			t.Errorf("cy still has %+v", g)
		}
	})

	t.Run("starting again starts from nothing", func(t *testing.T) {
		report(ann, f.ref(f.video, "video"), ProgressInput{Event: "heartbeat", Position: f64(5), Duration: f64(100), ClientTS: 1})
		p, err := s.GetProgress(ctx, ann.ID, f.video)
		if err != nil || p.Position != 5 || p.Furthest != 5 {
			t.Errorf("progress after dropping and watching again: %+v (%v), want 5 and 5, not the old 30", p, err)
		}
	})

	t.Run("a course she cannot see is not found, and nothing is deleted", func(t *testing.T) {
		if err := s.DropCourse(ctx, ann.ID, 987654); !errors.Is(err, ErrNotFound) {
			t.Errorf("unknown course: %v", err)
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE courses SET missing_at = ? WHERE id = ?`, mtime, other); err != nil {
			t.Fatal(err)
		}
		if err := s.DropCourse(ctx, ann.ID, other); !errors.Is(err, ErrNotFound) {
			t.Errorf("vanished course: %v", err)
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE courses SET missing_at = NULL WHERE id = ?`, other); err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE libraries SET enabled = 0`); err != nil {
			t.Fatal(err)
		}
		if err := s.DropCourse(ctx, ann.ID, other); !errors.Is(err, ErrNotFound) {
			t.Errorf("course of a switched-off library: %v", err)
		}
		if g := got(ann, other); g != (counts{1, 1, 1}) {
			t.Errorf("ann's second course lost data: %+v", g)
		}
	})
}
