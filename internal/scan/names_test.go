package scan

import (
	"sort"
	"testing"
)

func TestCleanTitle(t *testing.T) {
	cases := map[string]string{
		"01 - Introduction":       "Introduction",
		"1. Welcome":              "Welcome",
		"001-Getting Started":     "Getting Started",
		"[03] Variables":          "Variables",
		"(4) Loops":               "Loops",
		"Section 2 - Functions":   "Functions",
		"Module 10: Testing":      "Testing",
		"Lecture 14. Recap":       "Recap",
		"1.2 - Sub topic":         "Sub topic",
		"05_data_structures":      "data structures",
		"07 Closing thoughts":     "Closing thoughts",
		"2024 Taxes":              "2024 Taxes", // four digits are a year, not an index
		"Plain name":              "Plain name",
		"01":                      "01", // never empty
		"Section 3":               "Section 3",
		"  spaced   out   name  ": "spaced out name",
	}
	for in, want := range cases {
		if got := CleanTitle(in); got != want {
			t.Errorf("CleanTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNaturalCompare(t *testing.T) {
	in := []string{"lesson 10", "Lesson 2", "lesson 1", "lesson 02a", "lesson 2b", "Lesson 100", "alpha", "Beta"}
	want := []string{"alpha", "Beta", "lesson 1", "Lesson 2", "lesson 02a", "lesson 2b", "lesson 10", "Lesson 100"}
	sort.Slice(in, func(i, j int) bool { return NaturalCompare(in[i], in[j]) < 0 })
	for i := range want {
		if in[i] != want[i] {
			t.Fatalf("got %v, want %v", in, want)
		}
	}
	if NaturalCompare("a", "a") != 0 || NaturalCompare("a", "A") == 0 {
		t.Error("equal strings must compare equal, case variants must still be ordered deterministically")
	}
}

func TestIgnored(t *testing.T) {
	for _, n := range []string{".DS_Store", "Thumbs.db", "~$report.docx", "movie.mp4.part", "x.crdownload", "@eaDir", "course.json", ".hidden"} {
		if !Ignored(n) {
			t.Errorf("%q should be ignored", n)
		}
	}
	for _, n := range []string{"lesson.mp4", "notes.txt", "README.md"} {
		if Ignored(n) {
			t.Errorf("%q should not be ignored", n)
		}
	}
}

func TestSubtitleLang(t *testing.T) {
	type c struct {
		media, sub string
		lang       string
		ok         bool
	}
	for _, tc := range []c{
		{"lesson", "lesson", "", true},
		{"lesson", "lesson.en", "en", true},
		{"lesson", "lesson.pt-BR", "pt-BR", true},
		{"lesson", "lesson.pt_BR.forced", "pt-BR", true},
		{"lesson", "lesson_english", "en", true},
		{"lesson", "Lesson - fr", "fr", true},
		{"lesson", "lesson 2", "", false},
		{"lesson", "lessons", "", false},
		{"lesson 1", "lesson 10.en", "", false},
		{"lesson", "other", "", false},

		// notes in the name, in any order, with brackets
		{"lesson", "lesson.captions", "", true},
		{"lesson", "lesson - Subtitles", "", true},
		{"lesson", "lesson.en.SDH", "en", true},
		{"lesson", "lesson.forced.en", "en", true},
		{"lesson", "lesson (English)", "en", true},
		{"lesson", "lesson [CC]", "", true},
		{"lesson", "lesson[fr]", "fr", true},
		{"lesson", "lesson.English.CC", "en", true},
		{"lesson", "lesson-en-forced", "en", true},
		{"lesson", "lesson_pt-BR_forced", "pt-BR", true},
		{"lesson", "lesson.pt-BR-forced", "pt-BR", true},
		{"lesson", "lesson.English-US", "en-US", true},
		{"lesson", "lesson.es-419", "es-419", true},
		{"lesson", "lesson.en-orig", "en-orig", true},
		{"lesson", "LESSON.EN", "en", true},
		// the video's own extension kept in the subtitle's name
		{"lesson.mp4", "lesson.mp4", "", true},
		{"lesson.mp4", "lesson.mp4.en", "en", true},
		// still not subtitles of it
		{"lesson", "lesson 2 en", "", false},
		{"lesson", "lesson-final", "", false},
		{"lesson", "lesson.en.fr", "", false}, // two languages
		{"Intro", "Intro to Go", "", false},   // a different title that starts the same way
		{"lesson", "lesson (2)", "", false},
		{"lesson", "lesson,en", "", false},
	} {
		lang, ok := SubtitleLang(tc.media, tc.sub)
		if lang != tc.lang || ok != tc.ok {
			t.Errorf("SubtitleLang(%q,%q) = %q,%v want %q,%v", tc.media, tc.sub, lang, ok, tc.lang, tc.ok)
		}
	}
	if LangLabel("pt-BR") != "Portuguese (BR)" || LangLabel("en") != "English" || LangLabel("") != "Subtitles" || LangLabel("xx") != "XX" {
		t.Error("LangLabel output unexpected")
	}
}

func TestGenericSubtitleLang(t *testing.T) {
	for _, tc := range []struct {
		stem string
		lang string
		ok   bool
	}{
		{"english", "en", true},
		{"en", "en", true},
		{"captions", "", true},
		{"subtitles.en", "en", true},
		{"Subtitles - French", "fr", true},
		{"captions_pt-BR", "pt-BR", true},
		{"", "", false},
		{"  ", "", false},
		{"02 - Next", "", false},
		{"notes about it", "", false},
		{"lesson 2", "", false},
		{"Intro", "", false},
	} {
		lang, ok := GenericSubtitleLang(tc.stem)
		if lang != tc.lang || ok != tc.ok {
			t.Errorf("GenericSubtitleLang(%q) = %q,%v want %q,%v", tc.stem, lang, ok, tc.lang, tc.ok)
		}
	}
}

func TestClassify(t *testing.T) {
	for ext, kind := range map[string]string{
		"mp4": "video", "mkv": "video", "mp3": "audio", "pdf": "pdf", "png": "image", "svg": "image",
		"md": "markdown", "txt": "text", "ts": "text", "csv": "csv", "xlsx": "sheet", "url": "link",
		"docx": "other", "zip": "other", "unknownext": "other", "": "other",
	} {
		if got := Classify(ext).Kind; got != kind {
			t.Errorf("Classify(%q) = %q, want %q", ext, got, kind)
		}
	}
	if !IsCoverName("Cover.JPG") || !IsCoverName("poster.png") || IsCoverName("cover.txt") || IsCoverName("covers.jpg") {
		t.Error("IsCoverName wrong")
	}
}

func TestParseLink(t *testing.T) {
	good := map[string]string{
		"url":     "[InternetShortcut]\r\nURL=https://example.com/a?b=1\r\n",
		"desktop": "[Desktop Entry]\nType=Link\nName=x\nURL=http://example.org/\n",
		"webloc":  `<?xml version="1.0"?><plist><dict><key>URL</key><string>https://example.com/?a=1&amp;b=2</string></dict></plist>`,
	}
	want := map[string]string{
		"url": "https://example.com/a?b=1", "desktop": "http://example.org/", "webloc": "https://example.com/?a=1&b=2",
	}
	for ext, body := range good {
		got, ok := ParseLink(ext, []byte(body))
		if !ok || got != want[ext] {
			t.Errorf("ParseLink(%s) = %q,%v want %q", ext, got, ok, want[ext])
		}
	}
	for _, body := range []string{
		"[InternetShortcut]\nURL=javascript:alert(1)\n",
		"[InternetShortcut]\nURL=file:///etc/passwd\n",
		"[InternetShortcut]\nURL=data:text/html,<script>\n",
		"[InternetShortcut]\n",
		"URL=https://\n",
	} {
		if got, ok := ParseLink("url", []byte(body)); ok {
			t.Errorf("ParseLink accepted %q -> %q", body, got)
		}
	}
	// a .desktop file that is an Application, not a Link, is not a course link
	if _, ok := ParseLink("desktop", []byte("[Desktop Entry]\nType=Application\nURL=https://x.org\n")); ok {
		t.Error("Application .desktop accepted as link")
	}
}
