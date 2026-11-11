package scan

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"lectern/internal/fsutil"
	"lectern/internal/store"
	"lectern/internal/testdb"
)

// scanSetup opens an empty test database with one library and returns what the
// tests below need to scan it and look at the result.
type scanSetup struct {
	t    *testing.T
	ctx  context.Context
	st   *store.Store
	lib  store.Library
	app  *Applier
	user *store.User
	root string
}

func newScanSetup(t *testing.T) *scanSetup {
	t.Helper()
	conn, _ := testdb.Open(t)
	s := &scanSetup{t: t, ctx: context.Background(), st: store.New(conn), root: t.TempDir()}
	if err := s.st.SyncLibraries(s.ctx, []store.LibrarySpec{{Label: "Main", Path: s.root}}); err != nil {
		t.Fatal(err)
	}
	libs, err := s.st.ListLibraries(s.ctx, true)
	if err != nil || len(libs) != 1 {
		t.Fatalf("libraries: %v %v", libs, err)
	}
	s.lib = libs[0]
	s.app = &Applier{Store: s.st}
	if s.user, err = s.st.CreateUser(s.ctx, "ann", "Ann", "x", "user"); err != nil {
		t.Fatal(err)
	}
	return s
}

func (s *scanSetup) scan() Counts {
	s.t.Helper()
	c, err := s.app.ScanLibrary(s.ctx, s.lib, nil)
	if err != nil {
		s.t.Fatalf("scan: %v", err)
	}
	return c
}

func (s *scanSetup) course(title string) store.CourseSummary {
	s.t.Helper()
	cs, err := s.st.ListCourses(s.ctx, s.user.ID, "")
	if err != nil {
		s.t.Fatal(err)
	}
	for _, c := range cs {
		if c.Title == title {
			return c
		}
	}
	s.t.Fatalf("course %q not listed", title)
	return store.CourseSummary{}
}

func (s *scanSetup) tree(id uint64) *store.CourseTree {
	s.t.Helper()
	tr, err := s.st.CourseTree(s.ctx, id, s.user.ID)
	if err != nil {
		s.t.Fatal(err)
	}
	return tr
}

// diskPath is where the media layer would look for a stored location.
func diskPath(t *testing.T, loc *store.FileLoc) string {
	t.Helper()
	p, err := fsutil.ResolveUnder(loc.LibraryPath, path.Join(loc.CourseRel, loc.ItemRel), false)
	if err != nil {
		t.Fatalf("location does not resolve: %v", err)
	}
	return p
}

func TestSingleFileCourses(t *testing.T) {
	s := newScanSetup(t)
	write(t, s.root, "Solo Talk.mp4", uniq("solo"))
	write(t, s.root, "Solo Talk.en.srt", "1\n00:00:00,000 --> 00:00:01,000\nhi\n")
	write(t, s.root, "Solo Talk.jpg", uniq("pic"))
	write(t, s.root, "Folder Course/a.mp4", uniq("a"))
	write(t, s.root, "Folder Course/b.mp4", uniq("b"))
	write(t, s.root, "notes.pdf", uniq("notes"))

	c := s.scan()
	if c.Courses != 2 || c.CoursesNew != 2 || c.Items != 3 || c.CoursesFailed != 0 {
		t.Fatalf("first scan: %+v", c)
	}
	solo := s.course("Solo Talk")
	if solo.ItemCount != 1 || !solo.HasCover {
		t.Fatalf("summary = %+v", solo)
	}
	tr := s.tree(solo.ID)
	if len(tr.Sections) != 1 || !tr.Sections[0].Loose || len(tr.Sections[0].Items) != 1 {
		t.Fatalf("sections = %+v", tr.Sections)
	}
	it := tr.Sections[0].Items[0]
	if it.Title != "Solo Talk" || it.Kind != "video" || len(it.Subtitles) != 1 {
		t.Fatalf("item = %+v", it)
	}

	// The stored locations lead to the real files.
	loc, err := s.st.ItemFile(s.ctx, it.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := diskPath(t, loc), filepath.Join(s.root, "Solo Talk.mp4"); got != want {
		t.Errorf("video path %s, want %s", got, want)
	}
	sub, err := s.st.SubtitleFile(s.ctx, it.Subtitles[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := diskPath(t, sub), filepath.Join(s.root, "Solo Talk.en.srt"); got != want {
		t.Errorf("subtitle path %s, want %s", got, want)
	}
	cover, err := s.st.CoverFile(s.ctx, solo.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := diskPath(t, cover), filepath.Join(s.root, "Solo Talk.jpg"); got != want {
		t.Errorf("cover path %s, want %s", got, want)
	}

	if c := s.scan(); c.ItemsAdded+c.ItemsUpdated+c.ItemsMoved+c.ItemsMissing+c.CoursesNew+c.CoursesMissing+c.CoursesRenamed != 0 {
		t.Errorf("a second scan changed things: %+v", c)
	}

	// Watch part of it.
	if _, err := s.st.ApplyProgress(s.ctx, s.user.ID, store.ItemRef{ID: it.ID, CourseID: solo.ID, Kind: "video"},
		store.ProgressInput{Event: "heartbeat", Position: fp(42), Duration: fp(100), ClientTS: 1}, 0.95); err != nil {
		t.Fatal(err)
	}

	// Renaming the file (and what goes with it) keeps the course and its progress.
	mv := func(from, to string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(s.root, to)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(filepath.Join(s.root, from), filepath.Join(s.root, to)); err != nil {
			t.Fatal(err)
		}
	}
	mv("Solo Talk.mp4", "Renamed Solo.mp4")
	mv("Solo Talk.en.srt", "Renamed Solo.en.srt")
	mv("Solo Talk.jpg", "Renamed Solo.jpg")
	c = s.scan()
	if c.CoursesRenamed != 1 || c.CoursesNew != 0 || c.CoursesMissing != 0 {
		t.Fatalf("rename scan: %+v", c)
	}
	renamed := s.course("Renamed Solo")
	if renamed.ID != solo.ID || !renamed.HasCover {
		t.Fatalf("the course must be the same row: %+v vs %+v", renamed, solo)
	}
	tr = s.tree(renamed.ID)
	it2 := tr.Sections[0].Items[0]
	if it2.ID != it.ID || it2.Name != "Renamed Solo.mp4" || it2.Progress == nil || it2.Progress.Position != 42 || len(it2.Subtitles) != 1 {
		t.Fatalf("item after the rename = %+v progress=%+v", it2, it2.Progress)
	}
	loc, _ = s.st.ItemFile(s.ctx, it2.ID)
	if got, want := diskPath(t, loc), filepath.Join(s.root, "Renamed Solo.mp4"); got != want {
		t.Errorf("video path after rename %s, want %s", got, want)
	}

	// Moving it into a folder is recognised by the content, even when the
	// modification time changes: the fingerprint is what matches.
	fpv, err := Fingerprint(filepath.Join(s.root, "Renamed Solo.mp4"), loc.Size)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.st.SetFingerprint(s.ctx, it2.ID, fpv); err != nil {
		t.Fatal(err)
	}
	mv("Renamed Solo.mp4", "Moved/Renamed Solo.mp4")
	later := time.Now().Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(s.root, "Moved/Renamed Solo.mp4"), later, later); err != nil {
		t.Fatal(err)
	}
	mv("Renamed Solo.en.srt", "Moved/Renamed Solo.en.srt")
	c = s.scan()
	if c.CoursesRenamed != 1 || c.CoursesNew != 0 {
		t.Fatalf("move scan: %+v", c)
	}
	moved := s.course("Moved")
	if moved.ID != solo.ID {
		t.Fatalf("the folder course must be the same row as the file course: %d vs %d", moved.ID, solo.ID)
	}
	tr = s.tree(moved.ID)
	it3 := tr.Sections[0].Items[0]
	if it3.ID != it.ID || it3.Progress == nil || it3.Progress.Position != 42 {
		t.Fatalf("progress lost on the move: %+v", it3)
	}
	loc, _ = s.st.ItemFile(s.ctx, it3.ID)
	if got, want := diskPath(t, loc), filepath.Join(s.root, "Moved", "Renamed Solo.mp4"); got != want {
		t.Errorf("video path after the move %s, want %s", got, want)
	}

	// Another, different file never takes over a vanished course.
	if err := os.RemoveAll(filepath.Join(s.root, "Moved")); err != nil {
		t.Fatal(err)
	}
	write(t, s.root, "Unrelated.mp4", uniq("unrelated"))
	c = s.scan()
	if c.CoursesNew != 1 || c.CoursesRenamed != 0 || c.CoursesMissing != 1 {
		t.Fatalf("an unrelated video must be a new course: %+v", c)
	}
	if s.course("Unrelated").ID == solo.ID {
		t.Error("unrelated video reused the vanished course")
	}
}

func TestChapterLifecycle(t *testing.T) {
	s := newScanSetup(t)
	other, err := s.st.CreateUser(s.ctx, "bob", "Bob", "x", "user")
	if err != nil {
		t.Fatal(err)
	}
	write(t, s.root, "Talk.mp4", uniq("talk"))
	write(t, s.root, "Talk.chapters.txt", "0:00 Intro\n10:00 Middle\n20:00 End\n")
	s.scan()

	talk := s.course("Talk")
	if talk.ItemCount != 3 || talk.DoneCount != 0 {
		t.Fatalf("a video with three chapters counts three lessons: %+v", talk)
	}
	item := s.tree(talk.ID).Sections[0].Items[0]
	if len(item.Chapters) != 3 || item.Chapters[1].Start != 600 || item.Chapters[1].Title != "Middle" {
		t.Fatalf("chapters = %+v", item.Chapters)
	}
	loc, _ := s.st.ItemFile(s.ctx, item.ID)
	if loc.Chapters != 3 {
		t.Fatalf("ItemFile.Chapters = %d", loc.Chapters)
	}
	ref := store.ItemRef{ID: item.ID, CourseID: talk.ID, Kind: "video", Chapters: 3}

	state := func() (string, int, int) {
		t.Helper()
		c := s.course("Talk")
		it := s.tree(talk.ID).Sections[0].Items[0]
		st := "none"
		if it.Progress != nil {
			st = it.Progress.State
		}
		n := 0
		for _, ch := range it.Chapters {
			if ch.Done {
				n++
			}
		}
		if n != c.DoneCount {
			t.Errorf("the tree shows %d finished chapters but the summary says %d", n, c.DoneCount)
		}
		return st, c.DoneCount, c.ItemCount
	}
	total := 3
	want := func(wantState string, wantDone int) {
		t.Helper()
		if st, done, n := state(); st != wantState || done != wantDone || n != total {
			t.Fatalf("state=%s done=%d/%d, want state=%s done=%d/%d", st, done, n, wantState, wantDone, total)
		}
	}

	// Watching past 95 % does not finish a video that has chapters.
	if _, err := s.st.ApplyProgress(s.ctx, s.user.ID, ref,
		store.ProgressInput{Event: "end", Position: fp(1000), Duration: fp(1000), ClientTS: 1}, 0.95); err != nil {
		t.Fatal(err)
	}
	want("started", 0)

	// Chapters one by one.
	chs, p, err := s.st.MarkChapters(s.ctx, s.user.ID, item.ID, []uint32{0}, nil)
	if err != nil || p.State != "started" || !chs[0].Done || chs[1].Done {
		t.Fatalf("mark first: %v %+v %+v", err, p, chs)
	}
	want("started", 1)
	if _, p, err = s.st.MarkChapters(s.ctx, s.user.ID, item.ID, []uint32{0, 5, 99999}, nil); err != nil || p.State != "started" {
		t.Fatalf("starts that are not chapters are ignored: %v %+v", err, p)
	}
	want("started", 1)
	if _, p, err = s.st.MarkChapters(s.ctx, s.user.ID, item.ID, []uint32{600, 1200}, nil); err != nil || p.State != "done" || p.DoneAt == nil {
		t.Fatalf("all chapters finished: %v %+v", err, p)
	}
	want("done", 3)
	// Someone else has finished nothing.
	otherTree, err := s.st.CourseTree(s.ctx, talk.ID, other.ID)
	if err != nil {
		t.Fatal(err)
	}
	if oc := otherTree.Sections[0].Items[0].Chapters; len(oc) != 3 || oc[0].Done || oc[1].Done || otherTree.Course.DoneCount != 0 {
		t.Errorf("another user sees %+v / %+v", oc, otherTree.Course)
	}

	// Taking one back un-finishes the item and keeps the rest.
	if _, p, err = s.st.MarkChapters(s.ctx, s.user.ID, item.ID, nil, []uint32{600}); err != nil || p.State != "started" || p.DoneAt != nil {
		t.Fatalf("unmark: %v %+v", err, p)
	}
	want("started", 2)

	// Marking the whole item does the same to every chapter.
	done := true
	if _, err := s.st.ApplyProgress(s.ctx, s.user.ID, ref, store.ProgressInput{Event: "done", Done: &done, ClientTS: 2}, 0.95); err != nil {
		t.Fatal(err)
	}
	want("done", 3)
	done = false
	if _, err := s.st.ApplyProgress(s.ctx, s.user.ID, ref, store.ProgressInput{Event: "done", Done: &done, ClientTS: 3}, 0.95); err != nil {
		t.Fatal(err)
	}
	want("started", 0)

	// Edit the file: retitle one chapter, drop one, add one. Marks follow the start times.
	if _, _, err := s.st.MarkChapters(s.ctx, s.user.ID, item.ID, []uint32{0, 600, 1200}, nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.st.MarkChapters(s.ctx, s.user.ID, item.ID, nil, []uint32{1200}); err != nil {
		t.Fatal(err)
	}
	write(t, s.root, "Talk.chapters.txt", "0:00 Intro\n10:00 Middle part\n30:00 New ending\n")
	s.scan()
	it := s.tree(talk.ID).Sections[0].Items[0]
	if len(it.Chapters) != 3 || it.Chapters[1].Title != "Middle part" || !it.Chapters[0].Done || !it.Chapters[1].Done || it.Chapters[2].Done {
		t.Fatalf("after the edit: %+v", it.Chapters)
	}
	want("started", 2)

	// A finished video stays finished when chapters are added to it.
	if _, p, err = s.st.MarkChapters(s.ctx, s.user.ID, item.ID, []uint32{1800}, nil); err != nil || p.State != "done" {
		t.Fatalf("finish: %v %+v", err, p)
	}
	write(t, s.root, "Talk.chapters.txt", "0:00 Intro\n10:00 Middle part\n30:00 New ending\n40:00 Bonus\n")
	s.scan()
	total = 4
	want("done", 4)
	if _, p, err = s.st.MarkChapters(s.ctx, s.user.ID, item.ID, nil, []uint32{0}); err != nil || p.State != "started" {
		t.Fatalf("unmark on a finished video: %v %+v", err, p)
	}
	want("started", 3) // the other chapters stay finished

	// Without the file it is an ordinary video again.
	if err := os.Remove(filepath.Join(s.root, "Talk.chapters.txt")); err != nil {
		t.Fatal(err)
	}
	s.scan()
	back := s.course("Talk")
	if back.ItemCount != 1 {
		t.Fatalf("item_count = %d, want 1", back.ItemCount)
	}
	if it := s.tree(talk.ID).Sections[0].Items[0]; len(it.Chapters) != 0 {
		t.Errorf("chapters linger: %+v", it.Chapters)
	}
	var marks int
	if err := s.st.DB().QueryRow(`SELECT COUNT(*) FROM chapter_progress WHERE item_id = ?`, item.ID).Scan(&marks); err != nil || marks != 0 {
		t.Errorf("chapter marks linger: %d %v", marks, err)
	}
	if _, _, err := s.st.MarkChapters(s.ctx, s.user.ID, item.ID, []uint32{0}, nil); err == nil {
		t.Error("marking chapters of an item without chapters must fail")
	}
}

// A video renamed to another extension that means the same thing (mp4 -> m4v)
// keeps its title and type, but the stored file name must follow.
func TestFileCourseExtensionChange(t *testing.T) {
	s := newScanSetup(t)
	write(t, s.root, "Same Title.mp4", uniq("same"))
	s.scan()
	c := s.course("Same Title")
	if err := os.Rename(filepath.Join(s.root, "Same Title.mp4"), filepath.Join(s.root, "Same Title.m4v")); err != nil {
		t.Fatal(err)
	}
	s.scan()
	again := s.course("Same Title")
	if again.ID != c.ID {
		t.Fatalf("a different course: %d vs %d", again.ID, c.ID)
	}
	it := s.tree(c.ID).Sections[0].Items[0]
	if it.Name != "Same Title.m4v" || it.Ext != "m4v" {
		t.Errorf("stored name/ext = %q/%q", it.Name, it.Ext)
	}
	loc, _ := s.st.ItemFile(s.ctx, it.ID)
	if got, want := diskPath(t, loc), filepath.Join(s.root, "Same Title.m4v"); got != want {
		t.Errorf("path %s, want %s", got, want)
	}
}
