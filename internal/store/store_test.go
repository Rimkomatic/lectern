package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"lectern/internal/testdb"
)

// These tests need MySQL (see internal/testdb); they are skipped without it.

func newStore(t *testing.T) *Store {
	t.Helper()
	conn, _ := testdb.Open(t)
	return New(conn)
}

func mustUser(t *testing.T, s *Store, name, role string) *User {
	t.Helper()
	u, err := s.CreateUser(context.Background(), name, strings.ToUpper(name[:1])+name[1:], "hash-"+name, role)
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	return u
}

func TestUsers(t *testing.T) {
	s, ctx := newStore(t), context.Background()
	a := mustUser(t, s, "alice", "admin")
	if a.Role != "admin" || a.Disabled || a.PwHash != "hash-alice" {
		t.Fatalf("unexpected user %+v", a)
	}
	if _, err := s.CreateUser(ctx, "ALICE", "Other", "h", "user"); !errors.Is(err, ErrConflict) {
		t.Fatalf("usernames are case-insensitive, got %v", err)
	}
	got, err := s.UserByUsername(ctx, "ALICE")
	if err != nil || got.ID != a.ID {
		t.Fatalf("case-insensitive lookup: %v %+v", err, got)
	}
	if _, err := s.UserByUsername(ctx, "nobody"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v", err)
	}
	b := mustUser(t, s, "bob", "user")
	if n, _ := s.CountUsers(ctx); n != 2 {
		t.Fatalf("count %d", n)
	}
	if list, _ := s.ListUsers(ctx); len(list) != 2 {
		t.Fatalf("list %+v", list)
	}

	// setting the same hash again is fine; an unknown user is not
	if err := s.SetPassword(ctx, b.ID, "hash-bob"); err != nil {
		t.Fatalf("unchanged hash: %v", err)
	}
	if err := s.SetPassword(ctx, b.ID, "new-hash"); err != nil {
		t.Fatal(err)
	}
	if u, _ := s.UserByID(ctx, b.ID); u.PwHash != "new-hash" {
		t.Fatalf("hash not stored: %+v", u)
	}
	if err := s.SetPassword(ctx, 9999, "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v", err)
	}

	name := "Bobby"
	u, err := s.UpdateUser(ctx, b.ID, UserPatch{DisplayName: &name})
	if err != nil || u.DisplayName != "Bobby" {
		t.Fatalf("update: %v %+v", err, u)
	}
	if _, err := s.UpdateUser(ctx, 9999, UserPatch{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v", err)
	}

	if u, _ := s.UserByID(ctx, b.ID); u.LastLoginAt != nil {
		t.Fatalf("last login before any login: %+v", u)
	}
	if err := s.TouchLogin(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	if u, _ := s.UserByID(ctx, b.ID); u.LastLoginAt == nil || time.Since(*u.LastLoginAt) > time.Minute {
		t.Fatalf("last login not recorded: %+v", u)
	}
}

func TestLastAdminProtection(t *testing.T) {
	s, ctx := newStore(t), context.Background()
	a := mustUser(t, s, "a", "admin")
	user, disabled := "user", true

	if _, err := s.UpdateUser(ctx, a.ID, UserPatch{Role: &user}); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("demoting the only admin: %v", err)
	}
	if _, err := s.UpdateUser(ctx, a.ID, UserPatch{Disabled: &disabled}); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("disabling the only admin: %v", err)
	}
	if err := s.DeleteUser(ctx, a.ID); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("deleting the only admin: %v", err)
	}

	b := mustUser(t, s, "b", "admin")
	if _, err := s.UpdateUser(ctx, b.ID, UserPatch{Role: &user}); err != nil {
		t.Fatalf("with two admins one may be demoted: %v", err)
	}
	if err := s.DeleteUser(ctx, a.ID); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("a is the only admin again: %v", err)
	}

	// a disabled admin does not count as an admin
	c := mustUser(t, s, "c", "admin")
	if _, err := s.UpdateUser(ctx, c.ID, UserPatch{Disabled: &disabled}); err != nil {
		t.Fatalf("disabling a spare admin: %v", err)
	}
	if _, err := s.UpdateUser(ctx, a.ID, UserPatch{Role: &user}); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("a disabled admin must not count as a replacement: %v", err)
	}
	if err := s.DeleteUser(ctx, c.ID); err != nil {
		t.Fatalf("deleting a disabled admin: %v", err)
	}
	if err := s.DeleteUser(ctx, 9999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestCreateFirstAdminOnlyOnce(t *testing.T) {
	s, ctx := newStore(t), context.Background()
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := s.CreateFirstAdmin(ctx, fmt.Sprintf("admin%d", i), "Admin", "hash")
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)
	ok, conflict := 0, 0
	for err := range results {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrConflict):
			conflict++
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if ok != 1 || conflict != 7 {
		t.Fatalf("expected exactly one winner, got %d ok / %d conflict", ok, conflict)
	}
	if n, _ := s.CountUsers(ctx); n != 1 {
		t.Fatalf("%d users", n)
	}
	if _, err := s.CreateFirstAdmin(ctx, "late", "Late", "h"); !errors.Is(err, ErrConflict) {
		t.Fatalf("setup must stay closed once there is a user: %v", err)
	}
}

func TestSessions(t *testing.T) {
	s, ctx := newStore(t), context.Background()
	u := mustUser(t, s, "sam", "user")
	h1, h2, h3 := bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32), bytes.Repeat([]byte{3}, 32)
	now := time.Now()

	if err := s.CreateSession(ctx, h1, u.ID, now.Add(time.Hour), "UA", "1.2.3.4"); err != nil {
		t.Fatal(err)
	}
	// very long agent strings are cut, not rejected
	if err := s.CreateSession(ctx, h2, u.ID, now.Add(time.Hour), strings.Repeat("é", 300), strings.Repeat("9", 100)); err != nil {
		t.Fatalf("long user agent: %v", err)
	}
	if err := s.CreateSession(ctx, h3, u.ID, now.Add(-time.Minute), "UA", "ip"); err != nil {
		t.Fatal(err)
	}

	sess, user, err := s.SessionUser(ctx, h1)
	if err != nil || user.ID != u.ID || sess.UserID != u.ID || !sess.ExpiresAt.After(now) {
		t.Fatalf("lookup: %v %+v %+v", err, sess, user)
	}
	if _, _, err := s.SessionUser(ctx, bytes.Repeat([]byte{9}, 32)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown session: %v", err)
	}
	if _, _, err := s.SessionUser(ctx, h3); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired session must not be returned: %v", err)
	}

	later := now.Add(48 * time.Hour)
	if err := s.TouchSession(ctx, h1, later); err != nil {
		t.Fatal(err)
	}
	if sess, _, _ := s.SessionUser(ctx, h1); sess.ExpiresAt.Sub(later).Abs() > time.Second {
		t.Fatalf("touch did not extend the session: %v vs %v", sess.ExpiresAt, later)
	}

	if n, err := s.PurgeExpiredSessions(ctx); err != nil || n != 1 {
		t.Fatalf("purge: %d %v", n, err)
	}
	if err := s.DeleteOtherUserSessions(ctx, u.ID, h1); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.SessionUser(ctx, h2); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other session should be gone: %v", err)
	}
	if _, _, err := s.SessionUser(ctx, h1); err != nil {
		t.Fatalf("kept session should remain: %v", err)
	}

	// a disabled account's sessions stop working at once
	disabled := true
	if _, err := s.UpdateUser(ctx, u.ID, UserPatch{Disabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.SessionUser(ctx, h1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("disabled user kept a working session: %v", err)
	}
	enabled := false
	_, _ = s.UpdateUser(ctx, u.ID, UserPatch{Disabled: &enabled})
	if err := s.DeleteUserSessions(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.SessionUser(ctx, h1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("sessions not deleted: %v", err)
	}
}

func TestSettings(t *testing.T) {
	s, ctx := newStore(t), context.Background()
	u := mustUser(t, s, "kim", "user")

	st, err := s.GetSettings(ctx, u.ID)
	if err != nil || st != DefaultSettings() {
		t.Fatalf("defaults: %v %+v", err, st)
	}
	want := Settings{ResumeMode: "auto", AutoplayNext: false, PlaybackRate: 1.5, DoneOnOpen: false}
	if err := s.PutSettings(ctx, u.ID, want); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetSettings(ctx, u.ID); got != want {
		t.Fatalf("round trip: %+v", got)
	}

	bad := want
	bad.ResumeMode = "sometimes"
	if err := s.PutSettings(ctx, u.ID, bad); err == nil {
		t.Fatal("invalid resume mode accepted")
	}
	bad = want
	bad.PlaybackRate = 10
	if err := s.PutSettings(ctx, u.ID, bad); err == nil {
		t.Fatal("invalid playback rate accepted")
	}
	if got, _ := s.GetSettings(ctx, u.ID); got != want {
		t.Fatalf("a rejected update changed the stored settings: %+v", got)
	}

	// keys added in a later version fall back to their defaults
	if _, err := s.db.ExecContext(ctx, `UPDATE user_settings SET data = ? WHERE user_id = ?`, `{"resume_mode":"never"}`, u.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetSettings(ctx, u.ID)
	if got.ResumeMode != "never" || got.PlaybackRate != 1 || !got.AutoplayNext || !got.DoneOnOpen {
		t.Fatalf("partial document: %+v", got)
	}
	// a stored document that no longer validates is ignored
	if _, err := s.db.ExecContext(ctx, `UPDATE user_settings SET data = ? WHERE user_id = ?`, `{"resume_mode":"bogus"}`, u.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetSettings(ctx, u.ID); got != DefaultSettings() {
		t.Fatalf("invalid stored document: %+v", got)
	}
}

// ---- progress against a real course ----

type fixture struct {
	s                *Store
	course           uint64
	video, pdf, link uint64
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	s, ctx := newStore(t), context.Background()
	if err := s.SyncLibraries(ctx, []LibrarySpec{{Label: "Lib", Path: "/lib"}}); err != nil {
		t.Fatal(err)
	}
	libs, err := s.ListLibraries(ctx, true)
	if err != nil || len(libs) != 1 {
		t.Fatalf("libraries: %v %+v", err, libs)
	}
	mtime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	item := func(rel, kind string, ord int) SyncItem {
		return SyncItem{
			RelPath: rel, Name: rel[strings.LastIndex(rel, "/")+1:], Title: rel, Kind: kind, Mtime: mtime, Size: 10,
			Ord: ord, Fingerprint: bytes.Repeat([]byte{byte(ord)}, 32),
		}
	}
	cid, _, err := s.SyncCourse(ctx, SyncCourse{
		LibraryID: libs[0].ID, RelPath: "Course", Title: "Course", DirMtime: mtime,
		Sections: []SyncSection{{RelPath: "S1", Title: "S1", Ord: 1, Items: []SyncItem{
			item("S1/a.mp4", "video", 1), item("S1/b.pdf", "pdf", 2), item("S1/c.url", "link", 3),
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	tree, err := s.CourseTree(ctx, cid, 0)
	if err != nil || len(tree.Sections) != 1 || len(tree.Sections[0].Items) != 3 {
		t.Fatalf("fixture tree: %v %+v", err, tree)
	}
	it := tree.Sections[0].Items
	return &fixture{s: s, course: cid, video: it[0].ID, pdf: it[1].ID, link: it[2].ID}
}

func (f *fixture) ref(id uint64, kind string) ItemRef {
	return ItemRef{ID: id, CourseID: f.course, Kind: kind}
}

func TestApplyProgressEndToEnd(t *testing.T) {
	f := newFixture(t)
	s, ctx := f.s, context.Background()
	ann, bob := mustUser(t, s, "ann", "user"), mustUser(t, s, "bob", "user")
	video, pdf, link := f.ref(f.video, "video"), f.ref(f.pdf, "pdf"), f.ref(f.link, "link")

	// opening a video creates a started row and teaches the item its duration
	p, err := s.ApplyProgress(ctx, ann.ID, video, ProgressInput{Event: "open", Duration: f64(100), ClientTS: 1}, 0.95)
	if err != nil || p.State != "started" || p.Position != 0 {
		t.Fatalf("open: %v %+v", err, p)
	}
	d, err := s.ItemDetail(ctx, f.video, ann.ID)
	if err != nil || d.Item.Duration == nil || *d.Item.Duration != 100 {
		t.Fatalf("duration not back-filled: %v %+v", err, d)
	}

	p, _ = s.ApplyProgress(ctx, ann.ID, video, ProgressInput{Event: "heartbeat", Position: f64(40), ClientTS: 2}, 0.95)
	if p.Position != 40 {
		t.Fatalf("heartbeat: %+v", p)
	}
	// a late, older report is ignored
	p, _ = s.ApplyProgress(ctx, ann.ID, video, ProgressInput{Event: "heartbeat", Position: f64(5), ClientTS: 1}, 0.95)
	if p.Position != 40 {
		t.Fatalf("stale report applied: %+v", p)
	}
	if got, _ := s.GetProgress(ctx, ann.ID, f.video); got.Position != 40 {
		t.Fatalf("not persisted: %+v", got)
	}

	// "continue learning" and the course page both point at the video
	cont, err := s.Continue(ctx, ann.ID, 10)
	if err != nil || len(cont) != 1 || cont[0].Item == nil || cont[0].Item.ID != f.video || cont[0].Item.Position != 40 {
		t.Fatalf("continue: %v %+v", err, cont)
	}
	tree, _ := s.CourseTree(ctx, f.course, ann.ID)
	if tree.Resume == nil || tree.Resume.ItemID != f.video || tree.Resume.Reason != "continue" || tree.Resume.Position != 40 {
		t.Fatalf("resume: %+v", tree.Resume)
	}

	// bob sees none of it
	if _, err := s.GetProgress(ctx, bob.ID, f.video); !errors.Is(err, ErrNotFound) {
		t.Fatalf("progress leaked between users: %v", err)
	}
	if bt, _ := s.CourseTree(ctx, f.course, bob.ID); bt.Resume != nil || bt.Course.DoneCount != 0 {
		t.Fatalf("bob's tree: %+v", bt)
	}
	if c, _ := s.Continue(ctx, bob.ID, 10); len(c) != 0 {
		t.Fatalf("bob's continue row: %+v", c)
	}

	// finishing the video moves "resume" on to the next unfinished item
	p, _ = s.ApplyProgress(ctx, ann.ID, video, ProgressInput{Event: "end", Position: f64(100), ClientTS: 3}, 0.95)
	if p.State != "done" {
		t.Fatalf("end: %+v", p)
	}
	tree, _ = s.CourseTree(ctx, f.course, ann.ID)
	if tree.Resume == nil || tree.Resume.ItemID != f.pdf || tree.Resume.Reason != "next" || tree.Course.DoneCount != 1 {
		t.Fatalf("after finishing: resume=%+v done=%d", tree.Resume, tree.Course.DoneCount)
	}

	// marking things done by hand does not change which item was "last seen"
	if _, err := s.ApplyProgress(ctx, ann.ID, pdf, ProgressInput{Event: "done", Done: boolp(true), ClientTS: 4}, 0.95); err != nil {
		t.Fatal(err)
	}
	if cont, _ := s.Continue(ctx, ann.ID, 10); len(cont) != 1 || cont[0].Item.ID != f.video {
		t.Fatalf("explicit done changed the last item: %+v", cont)
	}
	// opening a link with done_on_open completes it, and with it the course
	if _, err := s.ApplyProgress(ctx, ann.ID, link, ProgressInput{Event: "open", DoneOnOpen: true, ClientTS: 5}, 0.95); err != nil {
		t.Fatal(err)
	}
	tree, _ = s.CourseTree(ctx, f.course, ann.ID)
	if !tree.Completed || tree.Resume != nil || tree.Course.DoneCount != 3 {
		t.Fatalf("completed course: %+v", tree)
	}
	if cont, _ := s.Continue(ctx, ann.ID, 10); len(cont) != 0 {
		t.Fatalf("a finished course should leave the continue row: %+v", cont)
	}
	list, _ := s.ListCourses(ctx, ann.ID, "")
	if len(list) != 1 || list[0].DoneCount != 3 || list[0].ItemCount != 3 {
		t.Fatalf("course list: %+v", list)
	}
}

func TestApplyProgressConcurrent(t *testing.T) {
	f := newFixture(t)
	s, ctx := f.s, context.Background()
	video := f.ref(f.video, "video")

	var users []*User
	for i := 0; i < 6; i++ {
		users = append(users, mustUser(t, s, fmt.Sprintf("user%d", i), "user"))
	}
	// Six people start the same new video at once, each sending eight reports
	// that may arrive in any order. Nothing may fail (deadlocks are retried)
	// and the newest report by client timestamp must win for everyone.
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for _, u := range users {
		for i := 1; i <= 8; i++ {
			wg.Add(1)
			go func(uid uint64, i int) {
				defer wg.Done()
				_, err := s.ApplyProgress(ctx, uid, video,
					ProgressInput{Event: "heartbeat", Position: f64(float64(i)), Duration: f64(100), ClientTS: int64(i)}, 0.95)
				if err != nil {
					errs <- err
				}
			}(u.ID, i)
		}
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent report failed: %v", err)
	}
	for _, u := range users {
		p, err := s.GetProgress(ctx, u.ID, f.video)
		if err != nil || p.Position != 8 || p.ClientTS != 8 {
			t.Errorf("user %d: %v %+v", u.ID, err, p)
		}
	}
	d, _ := s.ItemDetail(ctx, f.video, users[0].ID)
	if d.Item.Duration == nil || *d.Item.Duration != 100 {
		t.Errorf("duration: %+v", d.Item.Duration)
	}
}

func TestDeletingAUserRemovesTheirData(t *testing.T) {
	f := newFixture(t)
	s, ctx := f.s, context.Background()
	u := mustUser(t, s, "gone", "user")
	keep := mustUser(t, s, "keep", "user")
	for _, id := range []uint64{u.ID, keep.ID} {
		if _, err := s.ApplyProgress(ctx, id, f.ref(f.video, "video"), ProgressInput{Event: "heartbeat", Position: f64(10), ClientTS: 1}, 0.95); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.PutSettings(ctx, u.ID, DefaultSettings()); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteUser(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"progress", "course_state", "user_settings", "sessions"} {
		var n int
		if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE user_id = ?", u.ID).Scan(&n); err != nil || n != 0 {
			t.Errorf("%s still has %d rows for the deleted user (%v)", table, n, err)
		}
	}
	if _, err := s.GetProgress(ctx, keep.ID, f.video); err != nil {
		t.Errorf("the other user's progress was lost: %v", err)
	}
	// the course and its items stay
	if _, err := s.CourseTree(ctx, f.course, keep.ID); err != nil {
		t.Errorf("course gone: %v", err)
	}
}
