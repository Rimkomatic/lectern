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
	if strings.Join(advTitles, "|") != "Generics|Concurrency|Reflection" {
		t.Errorf("Advanced items (natural order; a subtitle that fits none of the videos is not a lesson) = %v", advTitles)
	}
	if strings.Join(g.Strays, "|") != "Advanced/lonely.srt" {
		t.Errorf("unmatched subtitle files = %v", g.Strays)
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

// subtitleFolder walks a library holding one course with the given files in a
// single section folder and returns the course.
func subtitleFolder(t *testing.T, files ...string) ScannedCourse {
	t.Helper()
	root := t.TempDir()
	for _, f := range files {
		write(t, root, "C/S/"+f, "x")
	}
	courses, err := Walk(context.Background(), root, WalkOptions{})
	if err != nil || len(courses) != 1 || courses[0].Err != nil {
		t.Fatalf("walk: %v %+v", err, courses)
	}
	return courses[0]
}

// subsOf lists "lesson title: lang,lang" for every item that has subtitles, and
// the titles of the items that are text files.
func subsOf(c ScannedCourse) (subs map[string]string, texts []string) {
	subs = map[string]string{}
	for _, sec := range c.Sections {
		for _, it := range sec.Items {
			if it.Kind == "text" {
				texts = append(texts, it.Title)
			}
			var langs []string
			for _, s := range it.Subtitles {
				langs = append(langs, s.Lang+"/"+s.Format)
			}
			if len(langs) > 0 {
				subs[it.Title] = strings.Join(langs, ",")
			}
		}
	}
	return subs, texts
}

func TestSubtitlePairing(t *testing.T) {
	t.Run("looser names", func(t *testing.T) {
		c := subtitleFolder(t,
			"01 - Intro.mp4", "01 - Intro.mp4.en.srt", // the video's extension kept in the name
			"02 - Loops.mp4", "02 - Loops (English) [CC].srt", "02 - Loops.fr.forced.vtt",
			"03 - Maps.mp4", "03 - Maps - Subtitles.srt", "03 - Maps.pt_BR.srt",
			"04 - Errors.mp4", "04 - Errors.mp4.srt",
		)
		subs, texts := subsOf(c)
		want := map[string]string{
			"Intro":  "en/srt",
			"Loops":  "en/srt,fr/vtt",
			"Maps":   "/srt,pt-BR/srt",
			"Errors": "/srt",
		}
		if len(subs) != len(want) {
			t.Fatalf("subtitles = %v", subs)
		}
		for k, v := range want {
			// attachment order follows the file names; compare as sets
			got := strings.Split(subs[k], ",")
			for _, w := range strings.Split(v, ",") {
				found := false
				for _, g := range got {
					found = found || g == w
				}
				if !found {
					t.Errorf("%s: want %q among %q", k, w, subs[k])
				}
			}
		}
		if len(texts) != 0 || len(c.Strays) != 0 {
			t.Errorf("nothing should be left over: texts=%v strays=%v", texts, c.Strays)
		}
	})

	t.Run("one video takes generic subtitle names", func(t *testing.T) {
		c := subtitleFolder(t, "lecture.mp4", "captions.srt", "subtitles.fr.vtt", "english.srt", "Notes on it.srt")
		subs, texts := subsOf(c)
		if got := subs["lecture"]; !strings.Contains(got, "/srt") || !strings.Contains(got, "fr/vtt") || !strings.Contains(got, "en/srt") {
			t.Errorf("lecture subtitles = %q", got)
		}
		// "Notes on it" names something else: left out, not guessed
		if strings.Join(c.Strays, "|") != "S/Notes on it.srt" || len(texts) != 0 {
			t.Errorf("strays=%v texts=%v", c.Strays, texts)
		}
	})

	t.Run("several videos: an unmatched subtitle is a stray, not a lesson", func(t *testing.T) {
		c := subtitleFolder(t, "01 - A.mp4", "02 - B.mp4", "01 - A.en.srt", "english.srt", "random.vtt")
		subs, texts := subsOf(c)
		if subs["A"] != "en/srt" || len(subs) != 1 {
			t.Errorf("subtitles = %v", subs)
		}
		if strings.Join(c.Strays, "|") != "S/english.srt|S/random.vtt" {
			t.Errorf("strays = %v", c.Strays)
		}
		if len(texts) != 0 || c.ItemCount() != 2 {
			t.Errorf("texts=%v items=%d: the strays must not be lessons", texts, c.ItemCount())
		}
	})

	t.Run("a folder with no video keeps its subtitle files as text", func(t *testing.T) {
		c := subtitleFolder(t, "talk-a.srt", "talk-b.vtt", "notes.pdf")
		_, texts := subsOf(c)
		if strings.Join(texts, "|") != "talk-a|talk-b" || len(c.Strays) != 0 {
			t.Errorf("texts=%v strays=%v", texts, c.Strays)
		}
	})

	t.Run("the longest matching video name wins", func(t *testing.T) {
		// Talk.en.srt fits Talk.mp4 (English) and Talk.en.mp4 (exactly): the exact one is meant
		c := subtitleFolder(t, "Talk.mp4", "Talk.en.mp4", "Talk.en.srt")
		subs, _ := subsOf(c)
		if _, onShort := subs["Talk"]; onShort || subs["Talk.en"] != "/srt" {
			t.Errorf("subtitles = %v", subs)
		}
	})
}
