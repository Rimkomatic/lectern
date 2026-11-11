package scan

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mkdir(t *testing.T, root, rel string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(rel)), 0o755); err != nil {
		t.Fatal(err)
	}
}

func sampleLibrary(t *testing.T) string {
	root := t.TempDir()
	c := "Go Basics"
	write(t, root, c+"/cover.jpg", "jpg")
	write(t, root, c+"/course.json", `{"title":"Go Basics 101","description":"Learn Go from zero","order":["Advanced","Intro"]}`)
	write(t, root, c+"/README.md", "# read me")
	write(t, root, c+"/01 - Intro/01 - Welcome.mp4", "not really a video")
	write(t, root, c+"/01 - Intro/01 - Welcome.en.srt", "1\n00:00:00,000 --> 00:00:01,000\nhi\n")
	write(t, root, c+"/01 - Intro/01 - Welcome.es.vtt", "WEBVTT\n")
	write(t, root, c+"/01 - Intro/02 - Docs.url", "[InternetShortcut]\nURL=https://go.dev/doc/\n")
	write(t, root, c+"/01 - Intro/03 - Broken.url", "[InternetShortcut]\nURL=javascript:alert(1)\n")
	write(t, root, c+"/01 - Intro/notes.pdf", "%PDF-1.4")
	write(t, root, c+"/01 - Intro/extras/a.txt", "a")
	write(t, root, c+"/01 - Intro/extras/deeper/b.txt", "b")
	write(t, root, c+"/Advanced/1. Generics.mp4", "video")
	write(t, root, c+"/Advanced/10. Reflection.mp4", "video")
	write(t, root, c+"/Advanced/2. Concurrency.mp4", "video")
	write(t, root, c+"/Advanced/.hidden", "x")
	write(t, root, c+"/Advanced/Thumbs.db", "x")
	write(t, root, c+"/Advanced/lonely.srt", "orphan subtitle")
	mkdir(t, root, c+"/Empty section")
	mkdir(t, root, "Empty course")
	write(t, root, "loose-file.txt", "belongs to nobody")
	write(t, root, "._junk", "x")
	write(t, root, "Second/one.mp3", "audio")
	return root
}

func TestWalkLayout(t *testing.T) {
	root := sampleLibrary(t)
	courses, err := Walk(context.Background(), root, WalkOptions{})
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]ScannedCourse{}
	for _, c := range courses {
		by[c.RelPath] = c
	}
	if len(courses) != 3 {
		t.Fatalf("want 3 course folders (Empty course, Go Basics, Second), got %d", len(courses))
	}
	if len(by["Empty course"].Sections) != 0 {
		t.Error("empty course should have no sections")
	}

	g := by["Go Basics"]
	if g.Err != nil {
		t.Fatal(g.Err)
	}
	if g.Title != "Go Basics 101" || g.Description != "Learn Go from zero" || g.CoverRel != "cover.jpg" {
		t.Errorf("metadata wrong: %+v", g)
	}
	var titles []string
	for _, s := range g.Sections {
		titles = append(titles, s.Title)
	}
	if strings.Join(titles, "|") != "Course files|Advanced|Intro" {
		t.Fatalf("section order = %v", titles)
	}
	root0 := g.Sections[0]
	if root0.RelPath != "" || len(root0.Items) != 1 || root0.Items[0].Kind != "markdown" {
		t.Errorf("implicit section wrong: %+v", root0)
	}

	adv := g.Sections[1]
	var advTitles []string
	for _, it := range adv.Items {
		advTitles = append(advTitles, it.Title)
	}
	if strings.Join(advTitles, "|") != "Generics|Concurrency|Reflection|lonely" {
		t.Errorf("Advanced items (natural order, orphan subtitle kept as text) = %v", advTitles)
	}

	intro := g.Sections[2]
	if intro.RelPath != "01 - Intro" {
		t.Errorf("intro rel path %q", intro.RelPath)
	}
	var got []string
	for _, it := range intro.Items {
		got = append(got, it.Title+":"+it.Kind)
	}
	want := "Welcome:video|Docs:link|Broken:other|notes:pdf|extras / a:text|extras / deeper / b:text"
	if strings.Join(got, "|") != want {
		t.Errorf("Intro items\n got %v\nwant %v", got, want)
	}
	w := intro.Items[0]
	if len(w.Subtitles) != 2 || w.Subtitles[0].Lang != "en" || w.Subtitles[0].Format != "srt" || w.Subtitles[1].Lang != "es" || w.Subtitles[1].Label != "Spanish" {
		t.Errorf("subtitles not paired: %+v", w.Subtitles)
	}
	if intro.Items[1].LinkURL != "https://go.dev/doc/" {
		t.Errorf("link url = %q", intro.Items[1].LinkURL)
	}
	if intro.Items[5].RelPath != "01 - Intro/extras/deeper/b.txt" {
		t.Errorf("deep item rel path = %q", intro.Items[5].RelPath)
	}
	if by["Second"].Sections[0].Title != RootSectionTitle || by["Second"].Sections[0].Items[0].Kind != "audio" {
		t.Errorf("course with only loose files: %+v", by["Second"].Sections)
	}
}

func TestWalkSymlinks(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "lib")
	outside := filepath.Join(base, "outside")
	write(t, root, "C/S/real.txt", "r")
	write(t, root, "C/S/inner/target.txt", "inside")
	write(t, outside, "secret.txt", "secret")
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(root, "C/S/leak.txt")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	os.Symlink(outside, filepath.Join(root, "C/S/outdir"))
	os.Symlink(filepath.Join(root, "C/S/inner/target.txt"), filepath.Join(root, "C/S/alias.txt"))
	os.Symlink(filepath.Join(root, "C"), filepath.Join(root, "C/S/loop")) // cycle
	os.Symlink(filepath.Join(base, "does-not-exist"), filepath.Join(root, "C/S/dangling.txt"))

	names := func(follow bool) string {
		cs, err := Walk(context.Background(), root, WalkOptions{FollowSymlinks: follow})
		if err != nil || len(cs) != 1 || cs[0].Err != nil {
			t.Fatalf("walk: %v %+v", err, cs)
		}
		var out []string
		for _, s := range cs[0].Sections {
			for _, it := range s.Items {
				out = append(out, it.RelPath)
			}
		}
		return strings.Join(out, ",")
	}
	strict := names(false)
	if strings.Contains(strict, "leak.txt") || strings.Contains(strict, "outdir") || strings.Contains(strict, "secret") {
		t.Errorf("links leaving the library were followed: %s", strict)
	}
	if !strings.Contains(strict, "C/S/alias.txt") && !strings.Contains(strict, "S/alias.txt") {
		t.Errorf("link inside the library should work: %s", strict)
	}
	if strings.Contains(strict, "dangling") {
		t.Errorf("dangling link listed: %s", strict)
	}
	loose := names(true)
	if !strings.Contains(loose, "leak.txt") || !strings.Contains(loose, "outdir/secret.txt") {
		t.Errorf("FollowSymlinks should allow outside targets: %s", loose)
	}
	if strings.Count(loose, "real.txt") > 2 {
		t.Errorf("symlink loop was not cut: %s", loose)
	}
}

func TestWalkMissingRoot(t *testing.T) {
	if _, err := Walk(context.Background(), filepath.Join(t.TempDir(), "nope"), WalkOptions{}); err == nil {
		t.Error("missing library path must be an error, not an empty library")
	}
	f := filepath.Join(t.TempDir(), "file")
	os.WriteFile(f, []byte("x"), 0o644)
	if _, err := Walk(context.Background(), f, WalkOptions{}); err == nil {
		t.Error("a file is not a library")
	}
}
