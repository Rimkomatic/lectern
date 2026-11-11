package scan

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Chapter is one entry of a video's timestamp file: where a topic starts.
type Chapter struct {
	Start float64 // seconds from the beginning
	Title string
}

const (
	maxChapters     = 1000
	maxChapterTitle = 200
)

// chapterSuffixes end the name of a timestamp file: "Talk.chapters.txt" (or
// "Talk.timestamps.txt") belongs to "Talk.mp4".
var chapterSuffixes = []string{".chapters.txt", ".timestamps.txt"}

// ChapterStem returns the name of the video a timestamp file belongs to
// ("Talk" for "Talk.chapters.txt"), or false if the name is not one.
func ChapterStem(name string) (string, bool) {
	lower := strings.ToLower(name)
	for _, suf := range chapterSuffixes {
		if len(name) > len(suf) && strings.HasSuffix(lower, suf) {
			return name[:len(name)-len(suf)], true
		}
	}
	return "", false
}

// chapterFits tells whether a timestamp file with this stem is for the media
// file with this name: "Talk" matches "Talk.mp4", and so does "Talk.mp4".
func chapterFits(stem, mediaName string) bool {
	return strings.EqualFold(stem, stemOf(mediaName)) || strings.EqualFold(stem, mediaName)
}

// A line is: optional list number or bullet, a time (H:MM:SS, MM:SS or M:SS,
// optionally with .fraction), optionally in brackets, a separator, the title.
//
//	0:00 Introduction
//	05:30 - Setting up
//	[1:02:15] Functions
//	3. 1:30:00: Wrap-up
var chapterLine = regexp.MustCompile(
	`^\s*(?:\d{1,3}[.)]\s+)?[-*•·▪‣>]*\s*[\[(]?\s*((?:\d{1,3}:)?\d{1,2}:\d{2}(?:[.,]\d{1,3})?)\s*[\])]?\s*(?:[-–—:|•·.)]+\s*)?(.*)$`)

// parseStamp reads "H:MM:SS", "MM:SS" or "M:SS", each with an optional
// ".fraction". Minutes and seconds must be below 60 when there is an hour.
func parseStamp(s string) (float64, bool) {
	s = strings.Replace(s, ",", ".", 1)
	frac := 0.0
	if i := strings.IndexByte(s, '.'); i >= 0 {
		f, err := strconv.ParseFloat("0"+s[i:], 64)
		if err != nil {
			return 0, false
		}
		frac, s = f, s[:i]
	}
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, false
	}
	n := make([]int, len(parts))
	for i, p := range parts {
		v, err := strconv.Atoi(p)
		if err != nil || v < 0 {
			return 0, false
		}
		n[i] = v
	}
	var h, m, sec int
	if len(n) == 3 {
		h, m, sec = n[0], n[1], n[2]
		if m >= 60 {
			return 0, false
		}
	} else {
		m, sec = n[0], n[1]
	}
	if sec >= 60 {
		return 0, false
	}
	return float64(h*3600+m*60+sec) + frac, true
}

// ParseChapters reads a timestamp file. Lines that do not start with a time are
// ignored (headings, notes). The result is in time order with one chapter per
// whole second; chapters without a title are called "Chapter N". It returns nil
// when no line holds a time.
func ParseChapters(text string) []Chapter {
	text = strings.TrimPrefix(strings.ToValidUTF8(text, ""), "\ufeff")
	var out []Chapter
	for _, line := range strings.Split(text, "\n") {
		m := chapterLine.FindStringSubmatch(strings.TrimRight(line, "\r"))
		if m == nil {
			continue
		}
		start, ok := parseStamp(m[1])
		if !ok {
			continue
		}
		title := strings.Join(strings.Fields(m[2]), " ")
		if r := []rune(title); len(r) > maxChapterTitle {
			title = string(r[:maxChapterTitle])
		}
		out = append(out, Chapter{Start: start, Title: title})
		if len(out) >= 4*maxChapters { // far more than any real list
			break
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start < out[j].Start })

	kept := out[:0]
	for _, c := range out {
		if n := len(kept); n > 0 && int64(kept[n-1].Start) == int64(c.Start) {
			continue // two chapters in the same second cannot be told apart
		}
		kept = append(kept, c)
	}
	if len(kept) > maxChapters {
		kept = kept[:maxChapters]
	}
	if len(kept) == 0 {
		return nil
	}
	for i := range kept {
		if kept[i].Title == "" {
			kept[i].Title = fmt.Sprintf("Chapter %d", i+1)
		}
	}
	return kept
}

// readChapters loads a timestamp file. It reports false when the file is too
// large, unreadable or holds no times (it is then listed as a plain text file,
// so the mistake can be seen).
func readChapters(f ent) ([]Chapter, bool) {
	if f.info == nil || f.info.Size() > maxMetaFile {
		return nil, false
	}
	fh, err := os.Open(f.abs)
	if err != nil {
		return nil, false
	}
	defer fh.Close()
	raw, err := io.ReadAll(io.LimitReader(fh, maxMetaFile))
	if err != nil {
		return nil, false
	}
	chs := ParseChapters(string(raw))
	return chs, len(chs) > 0
}
