package scan

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"lectern/internal/store"
	"lectern/internal/testdb"
)

// uniq gives every test file its own content (and so its own size/fingerprint).
func uniq(name string) string { return strings.Repeat(name+"|", 40+len(name)*7) }

func applyLibrary(t *testing.T) string {
	root := t.TempDir()
	c := "Go Basics"
	write(t, root, c+"/cover.png", uniq("cover"))
	write(t, root, c+"/README.md", uniq("readme"))
	write(t, root, c+"/01 - Intro/01 - Welcome.mp4", uniq("welcome"))
	write(t, root, c+"/01 - Intro/01 - Welcome.en.srt", "1\n00:00:00,000 --> 00:00:01,000\nhi\n")
	write(t, root, c+"/01 - Intro/02 - Docs.url", "[InternetShortcut]\nURL=https://go.dev/doc/\n")
	write(t, root, c+"/01 - Intro/notes.pdf", uniq("notes"))
	write(t, root, c+"/Advanced/1. Generics.mp4", uniq("generics"))
	write(t, root, c+"/Advanced/2. Concurrency.mp4", uniq("concurrency"))
	write(t, root, c+"/Advanced/3. Reflection.mp4", uniq("reflection"))
	write(t, root, "Podcasts/ep1.mp3", uniq("ep1"))
	write(t, root, "Podcasts/ep2.mp3", uniq("ep2"))
	mkdir(t, root, "Nothing here")
	return root
}

func itemByTitle(t *testing.T, tree *store.CourseTree, title string) store.Item {
	t.Helper()
	for _, s := range tree.Sections {
		for _, it := range s.Items {
			if it.Title == title {
				return it
			}
		}
	}
	t.Fatalf("item %q not in course tree", title)
	return store.Item{}
}

func TestScanLifecycle(t *testing.T) {
	conn, _ := testdb.Open(t)
	ctx := context.Background()
	st := store.New(conn)
	root := applyLibrary(t)
	if err := st.SyncLibraries(ctx, []store.LibrarySpec{{Label: "Main", Path: root}}); err != nil {
		t.Fatal(err)
	}
	libs, err := st.ListLibraries(ctx, true)
	if err != nil || len(libs) != 1 {
		t.Fatalf("libraries: %v %v", libs, err)
	}
	lib := libs[0]
	app := &Applier{Store: st}
	scan := func() Counts {
		t.Helper()
		c, err := app.ScanLibrary(ctx, lib, nil)
		if err != nil {
			t.Fatalf("scan: %v", err)
		}
		return c
	}
	user, err := st.CreateUser(ctx, "ann", "Ann", "x", "user")
	if err != nil {
		t.Fatal(err)
	}
	courseID := func(title string) uint64 {
		t.Helper()
		cs, err := st.ListCourses(ctx, user.ID, "")
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range cs {
			if c.Title == title {
				return c.ID
			}
		}
		t.Fatalf("course %q not listed (have %d)", title, len(cs))
		return 0
	}
	tree := func(id uint64) *store.CourseTree {
		t.Helper()
		tr, err := st.CourseTree(ctx, id, user.ID)
		if err != nil {
			t.Fatal(err)
		}
		return tr
	}

	// ---- first scan
	c := scan()
	if c.Courses != 2 || c.CoursesNew != 2 || c.ItemsAdded != 9 || c.CoursesMissing != 0 || c.CoursesFailed != 0 {
		t.Fatalf("first scan counts: %+v", c)
	}
	gid := courseID("Go Basics")
	tr := tree(gid)
	var order []string
	for _, s := range tr.Sections {
		order = append(order, s.Title)
	}
	if strings.Join(order, "|") != "Course files|Intro|Advanced" {
		t.Fatalf("sections = %v", order)
	}
	welcome := itemByTitle(t, tr, "Welcome")
	if len(welcome.Subtitles) != 1 || welcome.Subtitles[0].Lang != "en" {
		t.Fatalf("subtitles: %+v", welcome.Subtitles)
	}
	if itemByTitle(t, tr, "Docs").LinkURL != "https://go.dev/doc/" {
		t.Error("link url not stored")
	}
	if n := tr.Course.ItemCount; n != 7 {
		t.Errorf("item_count = %d, want 7 (the cover and the subtitle are not items)", n)
	}

	// progress to keep across renames
	gen := itemByTitle(t, tr, "Generics")
	if _, err := st.ApplyProgress(ctx, user.ID, store.ItemRef{ID: gen.ID, CourseID: gid, Kind: "video"},
		store.ProgressInput{Event: "heartbeat", Position: fp(42), Duration: fp(100), ClientTS: 1}, 0.95); err != nil {
		t.Fatal(err)
	}

	// ---- nothing changed
	if c := scan(); c.ItemsAdded+c.ItemsUpdated+c.ItemsMoved+c.ItemsMissing+c.CoursesNew+c.CoursesMissing != 0 {
		t.Errorf("idempotent rescan changed things: %+v", c)
	}

	// ---- file renamed inside a section: same row, progress kept
	os.Rename(filepath.Join(root, "Go Basics/Advanced/1. Generics.mp4"), filepath.Join(root, "Go Basics/Advanced/1. Generics (v2).mp4"))
	c = scan()
	if c.ItemsMoved != 1 || c.ItemsAdded != 0 || c.ItemsMissing != 0 {
		t.Fatalf("file rename counts: %+v", c)
	}
	tr = tree(gid)
	moved := itemByTitle(t, tr, "Generics (v2)")
	if moved.ID != gen.ID {
		t.Errorf("renamed file got a new id %d, want %d", moved.ID, gen.ID)
	}
	if moved.Progress == nil || moved.Progress.Position != 42 {
		t.Errorf("progress lost on rename: %+v", moved.Progress)
	}

	// ---- file moved to another section
	os.Rename(filepath.Join(root, "Go Basics/Advanced/3. Reflection.mp4"), filepath.Join(root, "Go Basics/01 - Intro/9. Reflection.mp4"))
	if c = scan(); c.ItemsMoved != 1 || c.ItemsAdded != 0 {
		t.Fatalf("file move counts: %+v", c)
	}

	// ---- course folder renamed: same course, progress kept
	os.Rename(filepath.Join(root, "Go Basics"), filepath.Join(root, "Go Basics v2"))
	c = scan()
	if c.CoursesRenamed != 1 || c.CoursesNew != 0 || c.CoursesMissing != 0 || c.ItemsAdded != 0 {
		t.Fatalf("course rename counts: %+v", c)
	}
	if got := courseID("Go Basics v2"); got != gid {
		t.Errorf("renamed course has id %d, want %d", got, gid)
	}
	if p := itemByTitle(t, tree(gid), "Generics (v2)").Progress; p == nil || p.Position != 42 {
		t.Errorf("progress lost on course rename: %+v", p)
	}

	// ---- file deleted: hidden but remembered
	os.Remove(filepath.Join(root, "Go Basics v2/README.md"))
	if c = scan(); c.ItemsMissing != 1 {
		t.Fatalf("deleted file counts: %+v", c)
	}
	if tr = tree(gid); tr.Course.ItemCount != 6 {
		t.Errorf("item_count after delete = %d", tr.Course.ItemCount)
	}
	// ...and it comes back with its old row
	write(t, root, "Go Basics v2/README.md", uniq("readme"))
	if c = scan(); c.ItemsMissing != 0 || c.ItemsAdded != 0 {
		t.Fatalf("file back counts: %+v", c)
	}
	if tr = tree(gid); tr.Course.ItemCount != 7 {
		t.Errorf("item_count after restore = %d", tr.Course.ItemCount)
	}

	// ---- changed file: updated in place
	write(t, root, "Go Basics v2/README.md", uniq("readme")+"more")
	if c = scan(); c.ItemsUpdated != 1 {
		t.Fatalf("changed file counts: %+v", c)
	}

	// ---- unmounted library: refuse to touch anything
	moved2 := root + ".offline"
	if err := os.Rename(root, moved2); err != nil {
		t.Fatal(err)
	}
	if _, err := app.ScanLibrary(ctx, lib, nil); err == nil {
		t.Fatal("scan of a missing library path must fail")
	}
	os.Mkdir(root, 0o755) // mount point with nothing mounted
	if _, err := app.ScanLibrary(ctx, lib, nil); err == nil || !strings.Contains(err.Error(), "looks empty") {
		t.Fatalf("scan of an empty library with known courses must refuse, got %v", err)
	}
	if n, _ := st.CountLiveCourses(ctx, lib.ID); n != 2 {
		t.Fatalf("courses were flagged missing during an outage: %d live", n)
	}
	os.Remove(root)
	os.Rename(moved2, root)
	if c = scan(); c.CoursesMissing != 0 || c.ItemsMissing != 0 || c.ItemsAdded != 0 {
		t.Fatalf("after remount: %+v", c)
	}

	// ---- course folder removed: flagged missing, then purged with its progress
	os.RemoveAll(filepath.Join(root, "Podcasts"))
	if c = scan(); c.CoursesMissing != 1 {
		t.Fatalf("removed course counts: %+v", c)
	}
	if n, _ := st.CountLiveCourses(ctx, lib.ID); n != 1 {
		t.Fatalf("live courses = %d", n)
	}
	os.RemoveAll(filepath.Join(root, "Go Basics v2/Advanced"))
	scan()
	n, err := st.PurgeMissing(ctx, time.Now().Add(time.Hour))
	if err != nil || n == 0 {
		t.Fatalf("purge: %d %v", n, err)
	}
	if p, _ := st.GetProgress(ctx, user.ID, gen.ID); p != nil {
		t.Errorf("progress of a purged item should be gone: %+v", p)
	}
}

func fp(v float64) *float64 { return &v }

func TestFingerprintFillAndMoveWithoutDBFingerprint(t *testing.T) {
	conn, _ := testdb.Open(t)
	ctx := context.Background()
	st := store.New(conn)
	root := applyLibrary(t)
	st.SyncLibraries(ctx, []store.LibrarySpec{{Label: "Main", Path: root}})
	libs, _ := st.ListLibraries(ctx, true)
	app := &Applier{Store: st}
	if _, err := app.ScanLibrary(ctx, libs[0], nil); err != nil {
		t.Fatal(err)
	}
	// new rows have no fingerprint yet; the runner's filler computes them
	targets, err := st.FingerprintTargets(ctx, 100)
	if err != nil || len(targets) == 0 {
		t.Fatalf("expected items awaiting a fingerprint: %v %v", targets, err)
	}
	r := NewRunner(app, 0, 0, nil)
	r.fillFingerprints(ctx)
	if left, _ := st.FingerprintTargets(ctx, 100); len(left) != 0 {
		t.Fatalf("%d items still without fingerprint", len(left))
	}

	// rename again, this time with stored fingerprints on both sides
	os.Rename(filepath.Join(root, "Podcasts/ep1.mp3"), filepath.Join(root, "Podcasts/episode-one.mp3"))
	c, err := app.ScanLibrary(ctx, libs[0], nil)
	if err != nil || c.ItemsMoved != 1 || c.ItemsAdded != 0 {
		t.Fatalf("fingerprint move: %+v %v", c, err)
	}
}
