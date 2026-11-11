package scan

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestChapterStem(t *testing.T) {
	cases := map[string]string{
		"Talk.chapters.txt":        "Talk",
		"Talk.TIMESTAMPS.TXT":      "Talk",
		"My Talk.mp4.chapters.txt": "My Talk.mp4",
		"a.b.chapters.txt":         "a.b",
	}
	for name, want := range cases {
		got, ok := ChapterStem(name)
		if !ok || got != want {
			t.Errorf("ChapterStem(%q) = %q, %v; want %q", name, got, ok, want)
		}
	}
	for _, name := range []string{"chapters.txt", ".chapters.txt", "Talk.txt", "Talk.chapters", "Talk.chapters.md", "timestamps.txt"} {
		if got, ok := ChapterStem(name); ok {
			t.Errorf("ChapterStem(%q) = %q, want no match", name, got)
		}
	}
}

func TestParseChapters(t *testing.T) {
	type c = Chapter
	cases := []struct {
		name string
		in   string
		want []Chapter
	}{
		{"plain", "0:00 Intro\n5:30 Setup\n1:02:15 Functions\n", []c{{0, "Intro"}, {330, "Setup"}, {3735, "Functions"}}},
		{"separators", "00:00 - Intro\n05:30 – Setup\n10:00 — Loops\n15:00: Maps\n20:00 | Wrap up\n25:00) Bye",
			[]c{{0, "Intro"}, {330, "Setup"}, {600, "Loops"}, {900, "Maps"}, {1200, "Wrap up"}, {1500, "Bye"}}},
		{"brackets and bullets", "- [0:00] Intro\n* (4:10) Part one\n• 9:59 Part two\n", []c{{0, "Intro"}, {250, "Part one"}, {599, "Part two"}}},
		{"numbered", "1. 0:00 Intro\n2) 3:00 Next\n", []c{{0, "Intro"}, {180, "Next"}}},
		{"fractions", "0:00 A\n0:12.5 B\n0:30,250 C\n", []c{{0, "A"}, {12.5, "B"}, {30.25, "C"}}},
		{"long videos", "0:00 Start\n10:00:00 Ten hours in\n", []c{{0, "Start"}, {36000, "Ten hours in"}}},
		{"minutes above 59 without hours", "0:00 A\n90:00 B\n", []c{{0, "A"}, {5400, "B"}}},
		{"unsorted", "5:00 B\n0:00 A\n2:00 Between\n", []c{{0, "A"}, {120, "Between"}, {300, "B"}}},
		{"same second keeps the first", "0:00 A\n1:00 B\n1:00 B again\n1:00.9 and again\n", []c{{0, "A"}, {60, "B"}}},
		{"untitled", "0:00\n1:00 Named\n2:00 -\n", []c{{0, "Chapter 1"}, {60, "Named"}, {120, "Chapter 3"}}},
		{"notes and headings are skipped", "Chapters\n\n# Part 1\n0:00 Intro\nsome words 3:00\nTotal: 2 parts\n3:00 Real one\n", []c{{0, "Intro"}, {180, "Real one"}}},
		{"windows line ends and a BOM", "\ufeff0:00 Intro\r\n2:00 Next\r\n", []c{{0, "Intro"}, {120, "Next"}}},
		{"impossible times are skipped", "0:00 A\n0:75 bad seconds\n1:75:00 bad minutes\n2:00 B\n", []c{{0, "A"}, {120, "B"}}},
		{"extra spaces are tidied", "0:00    A   lot    of space  \n", []c{{0, "A lot of space"}}},
		{"nothing usable", "hello\nworld\n12\n", nil},
		{"empty", "", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseChapters(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %#v\nwant %#v", got, tc.want)
			}
		})
	}

	t.Run("limits", func(t *testing.T) {
		var sb strings.Builder
		for i := 0; i < 1500; i++ {
			sb.WriteString(formatStamp(i*2) + " Chapter\n")
		}
		if got := ParseChapters(sb.String()); len(got) != maxChapters {
			t.Errorf("kept %d chapters, want %d", len(got), maxChapters)
		}
		long := "0:00 " + strings.Repeat("é", 500)
		got := ParseChapters(long)
		if len(got) != 1 || len([]rune(got[0].Title)) != maxChapterTitle {
			t.Errorf("title not cut to %d characters: %d", maxChapterTitle, len([]rune(got[0].Title)))
		}
	})
}

func formatStamp(sec int) string {
	return fmt.Sprintf("%d:%02d:%02d", sec/3600, sec%3600/60, sec%60)
}

func TestWalkChapterFiles(t *testing.T) {
	root := t.TempDir()
	c := "Course"
	write(t, root, c+"/Talk.mp4", "video")
	write(t, root, c+"/talk.CHAPTERS.txt", "0:00 Intro\n10:00 Middle\n20:00 End\n")
	write(t, root, c+"/Full Name.mp4", "video")
	write(t, root, c+"/Full Name.mp4.timestamps.txt", "0:00 Only one\n")
	write(t, root, c+"/Broken.mp4", "video")
	write(t, root, c+"/Broken.chapters.txt", "no times in here\n")
	write(t, root, c+"/Stranger.chapters.txt", "0:00 For a video that is not here\n")
	write(t, root, c+"/Twice.mp4", "video")
	write(t, root, c+"/Twice.chapters.txt", "0:00 First file wins\n")
	write(t, root, c+"/Twice.timestamps.txt", "0:00 Second file\n")
	write(t, root, c+"/Notes.md", "# notes")

	courses, err := Walk(context.Background(), root, WalkOptions{})
	if err != nil || len(courses) != 1 {
		t.Fatalf("walk: %v %d", err, len(courses))
	}
	items := map[string]ScannedItem{}
	for _, s := range courses[0].Sections {
		for _, it := range s.Items {
			items[it.Name] = it
		}
	}
	talk := items["Talk.mp4"]
	if len(talk.Chapters) != 3 || talk.Chapters[1].Start != 600 || talk.Chapters[2].Title != "End" {
		t.Errorf("Talk chapters = %+v", talk.Chapters)
	}
	if _, listed := items["talk.CHAPTERS.txt"]; listed {
		t.Error("a timestamp file that belongs to a video must not be listed as a lesson")
	}
	if got := items["Full Name.mp4"].Chapters; len(got) != 1 || got[0].Title != "Only one" {
		t.Errorf("a file named after the whole video name is not paired: %+v", got)
	}
	if got := items["Broken.mp4"].Chapters; got != nil {
		t.Errorf("a file without times must not give chapters: %+v", got)
	}
	if it, ok := items["Broken.chapters.txt"]; !ok || it.Kind != "text" {
		t.Errorf("a timestamp file without times is shown as a text file so the mistake is visible: %+v", it)
	}
	if it, ok := items["Stranger.chapters.txt"]; !ok || it.Kind != "text" {
		t.Errorf("a timestamp file without a video is a plain text file: %+v", it)
	}
	if got := items["Twice.mp4"].Chapters; len(got) != 1 || got[0].Title != "First file wins" {
		t.Errorf("two files for one video: %+v", got)
	}
	if _, listed := items["Twice.timestamps.txt"]; listed {
		t.Error("the second timestamp file for a video is not listed either")
	}
}

func TestWalkFlatCourse(t *testing.T) {
	root := t.TempDir()
	write(t, root, "Just Videos/01 - One.mp4", "v")
	write(t, root, "Just Videos/02 - Two.mp4", "v")
	write(t, root, "Just Videos/10 - Ten.mp4", "v")
	courses, err := Walk(context.Background(), root, WalkOptions{})
	if err != nil || len(courses) != 1 {
		t.Fatalf("walk: %v %d", err, len(courses))
	}
	c := courses[0]
	if c.IsFile || len(c.Sections) != 1 || c.Sections[0].RelPath != "" || c.Sections[0].Title != RootSectionTitle {
		t.Fatalf("a folder of loose videos is one implicit section: %+v", c.Sections)
	}
	var got []string
	for _, it := range c.Sections[0].Items {
		got = append(got, it.Title+"="+it.RelPath)
	}
	if strings.Join(got, "|") != "One=01 - One.mp4|Two=02 - Two.mp4|Ten=10 - Ten.mp4" {
		t.Errorf("items = %v", got)
	}
}

func TestWalkRootFiles(t *testing.T) {
	root := t.TempDir()
	write(t, root, "Solo Talk.mp4", "video")
	write(t, root, "Solo Talk.en.srt", "1\n00:00:00,000 --> 00:00:01,000\nhi\n")
	write(t, root, "Solo Talk.chapters.txt", "0:00 Intro\n5:00 Main part\n")
	write(t, root, "Solo Talk.jpg", "pic")
	write(t, root, "Podcast.mp3", "audio")
	write(t, root, "Podcast-poster.png", "pic")
	write(t, root, "Other Show.mkv", "video")
	write(t, root, "Other Show Poster.jpg", "not the same name: not its picture")
	write(t, root, "book.pdf", "%PDF-1.4")
	write(t, root, "readme.txt", "words")
	write(t, root, "orphan.srt", "text")
	write(t, root, "loose.url", "[InternetShortcut]\nURL=https://example.com\n")
	write(t, root, "Real Course/lesson.mp4", "video")

	courses, err := Walk(context.Background(), root, WalkOptions{})
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]ScannedCourse{}
	for _, c := range courses {
		by[c.RelPath] = c
	}
	if len(courses) != 4 {
		var names []string
		for _, c := range courses {
			names = append(names, c.RelPath)
		}
		t.Fatalf("want the folder and three video/audio files as courses, got %v", names)
	}
	for _, ignored := range []string{"book.pdf", "readme.txt", "orphan.srt", "loose.url", "Solo Talk.jpg", "Solo Talk.en.srt"} {
		if _, ok := by[ignored]; ok {
			t.Errorf("%s must not become a course", ignored)
		}
	}
	if by["Real Course"].IsFile {
		t.Error("a folder is not a file course")
	}

	solo := by["Solo Talk.mp4"]
	if !solo.IsFile || solo.Title != "Solo Talk" {
		t.Fatalf("solo course = %+v", solo)
	}
	if len(solo.Sections) != 1 || solo.Sections[0].RelPath != "" || len(solo.Sections[0].Items) != 1 {
		t.Fatalf("a file course has one implicit section with one item: %+v", solo.Sections)
	}
	it := solo.Sections[0].Items[0]
	if it.RelPath != "" || it.Name != "Solo Talk.mp4" || it.Title != "Solo Talk" || it.Kind != "video" || it.Size != 5 {
		t.Errorf("item = %+v", it)
	}
	if len(it.Subtitles) != 1 || it.Subtitles[0].RelPath != "../Solo Talk.en.srt" || it.Subtitles[0].Lang != "en" {
		t.Errorf("subtitles are reached from the file as ../name: %+v", it.Subtitles)
	}
	if solo.CoverRel != "../Solo Talk.jpg" {
		t.Errorf("cover = %q", solo.CoverRel)
	}
	if len(it.Chapters) != 2 || it.Chapters[1].Title != "Main part" {
		t.Errorf("chapters = %+v", it.Chapters)
	}

	pod := by["Podcast.mp3"]
	if pod.Sections[0].Items[0].Kind != "audio" || pod.CoverRel != "../Podcast-poster.png" {
		t.Errorf("audio file course = %+v", pod)
	}
	if other := by["Other Show.mkv"]; other.CoverRel != "" {
		t.Errorf("a picture with a different name must not be taken: %q", other.CoverRel)
	}
}
