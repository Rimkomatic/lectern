package scan

import (
	"regexp"
	"strings"
	"unicode"
)

// Ignored reports whether a file or folder name must be skipped by the scanner.
func Ignored(name string) bool {
	if name == "" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "~$") {
		return true
	}
	switch strings.ToLower(name) {
	case "thumbs.db", "desktop.ini", "@eadir", "#recycle", "lost+found", "$recycle.bin",
		"system volume information", "__macosx", "node_modules", "course.json":
		return true
	}
	switch strings.ToLower(extOf(name)) {
	case "part", "crdownload", "tmp", "temp", "aria2", "!qb", "download", "partial":
		return true
	}
	return false
}

// extOf returns the lower-case extension without the dot.
func extOf(name string) string {
	i := strings.LastIndexByte(name, '.')
	if i <= 0 || i == len(name)-1 {
		return ""
	}
	return strings.ToLower(name[i+1:])
}

// stemOf strips the extension.
func stemOf(name string) string {
	i := strings.LastIndexByte(name, '.')
	if i <= 0 {
		return name
	}
	return name[:i]
}

// NaturalCompare orders names the way people expect: case-insensitive, with
// digit runs compared by value ("lesson 2" < "lesson 10"). It returns -1, 0 or 1.
func NaturalCompare(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	i, j := 0, 0
	for i < len(ra) && j < len(rb) {
		ca, cb := ra[i], rb[j]
		if unicode.IsDigit(ca) && unicode.IsDigit(cb) {
			si := i
			for i < len(ra) && unicode.IsDigit(ra[i]) {
				i++
			}
			sj := j
			for j < len(rb) && unicode.IsDigit(rb[j]) {
				j++
			}
			na := strings.TrimLeft(string(ra[si:i]), "0")
			nb := strings.TrimLeft(string(rb[sj:j]), "0")
			if len(na) != len(nb) {
				if len(na) < len(nb) {
					return -1
				}
				return 1
			}
			if na != nb {
				if na < nb {
					return -1
				}
				return 1
			}
			continue
		}
		la, lb := unicode.ToLower(ca), unicode.ToLower(cb)
		if la != lb {
			if la < lb {
				return -1
			}
			return 1
		}
		i++
		j++
	}
	switch {
	case len(ra)-i < len(rb)-j:
		return -1
	case len(ra)-i > len(rb)-j:
		return 1
	}
	// Same under case folding and digit values: fall back to raw bytes so the
	// order is always deterministic.
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

var numberPrefixes = []*regexp.Regexp{
	// "Section 1 - ", "Module 2: ", "Lecture 14. "
	regexp.MustCompile(`(?i)^(?:section|module|chapter|part|lesson|lecture|week|unit|day)\s*\d+\s*[-–—:._)]+\s*`),
	// "[01] ", "[01]- "
	regexp.MustCompile(`^\[\d+\]\s*[-–—:._)]*\s*`),
	// "(01) "
	regexp.MustCompile(`^\(\d+\)\s*[-–—:._]*\s*`),
	// "01 - ", "1. ", "1.2 - ", "01_", "01) ", "001-"
	regexp.MustCompile(`^\d+(?:\.\d+)*\s*[-–—:._)]+\s*`),
	// "01 Introduction" (up to 3 digits so years and "2024 Taxes" survive)
	regexp.MustCompile(`^\d{1,3}\s+`),
}

// CleanTitle turns a folder or file stem into a display title by removing
// ordering prefixes ("01 - ", "Section 2: ", "[03]") and tidying underscores.
// It never returns an empty string: if cleaning removes everything the
// original is kept.
func CleanTitle(s string) string {
	orig := strings.TrimSpace(s)
	t := orig
	for _, re := range numberPrefixes {
		if loc := re.FindStringIndex(t); loc != nil && loc[1] < len(t) {
			t = t[loc[1]:]
			break
		}
	}
	if strings.Contains(t, "_") && !strings.Contains(t, " ") {
		t = strings.ReplaceAll(t, "_", " ")
	}
	t = strings.Join(strings.Fields(t), " ")
	if t == "" {
		return orig
	}
	return t
}

var langNames = map[string]string{
	"en": "English", "es": "Spanish", "fr": "French", "de": "German", "pt": "Portuguese",
	"it": "Italian", "ru": "Russian", "ja": "Japanese", "ko": "Korean", "zh": "Chinese",
	"ar": "Arabic", "hi": "Hindi", "bn": "Bengali", "tr": "Turkish", "nl": "Dutch",
	"pl": "Polish", "sv": "Swedish", "da": "Danish", "fi": "Finnish", "no": "Norwegian",
	"nb": "Norwegian", "cs": "Czech", "el": "Greek", "he": "Hebrew", "id": "Indonesian",
	"th": "Thai", "vi": "Vietnamese", "uk": "Ukrainian", "ro": "Romanian", "hu": "Hungarian",
	"fa": "Persian", "ur": "Urdu", "ta": "Tamil", "te": "Telugu", "ms": "Malay",
	"eng": "English", "spa": "Spanish", "fre": "French", "fra": "French", "ger": "German",
	"deu": "German", "por": "Portuguese", "ita": "Italian", "rus": "Russian", "jpn": "Japanese",
	"kor": "Korean", "chi": "Chinese", "zho": "Chinese", "ara": "Arabic", "hin": "Hindi",
}

var langWords = map[string]string{ // full names used as tokens -> iso-ish code
	"english": "en", "spanish": "es", "french": "fr", "german": "de", "portuguese": "pt",
	"italian": "it", "russian": "ru", "japanese": "ja", "korean": "ko", "chinese": "zh",
	"arabic": "ar", "hindi": "hi", "bengali": "bn", "turkish": "tr", "dutch": "nl",
	"polish": "pl", "swedish": "sv", "indonesian": "id", "vietnamese": "vi", "ukrainian": "uk",
}

var langToken = regexp.MustCompile(`^[a-zA-Z]{2,3}(?:[-_][a-zA-Z0-9]{2,4})?$`)

// LangLabel gives a human label for a language code ("pt-BR" -> "Portuguese (BR)").
func LangLabel(code string) string {
	if code == "" {
		return "Subtitles"
	}
	base, region, _ := strings.Cut(strings.ReplaceAll(code, "_", "-"), "-")
	name, ok := langNames[strings.ToLower(base)]
	if !ok {
		return strings.ToUpper(code)
	}
	if region != "" {
		return name + " (" + strings.ToUpper(region) + ")"
	}
	return name
}

// SubtitleLang decides whether a subtitle file stem belongs to a media file
// stem and extracts its language code. "lesson" matches "lesson", "lesson.en",
// "lesson.pt-BR.forced", "lesson_english" and "lesson - fr".
func SubtitleLang(mediaStem, subStem string) (lang string, ok bool) {
	if strings.EqualFold(mediaStem, subStem) {
		return "", true
	}
	if len(subStem) <= len(mediaStem) || !strings.EqualFold(subStem[:len(mediaStem)], mediaStem) {
		return "", false
	}
	rest := subStem[len(mediaStem):]
	sep := rest[0]
	if sep != '.' && sep != '_' && sep != '-' && sep != ' ' {
		return "", false
	}
	rest = strings.Trim(rest, "._- ")
	if rest == "" {
		return "", true
	}
	first := strings.FieldsFunc(rest, func(r rune) bool { return r == '.' || r == ' ' })
	if len(first) == 0 {
		return "", true
	}
	tok := first[0]
	if code, isWord := langWords[strings.ToLower(tok)]; isWord {
		return code, true
	}
	if langToken.MatchString(tok) {
		return strings.ReplaceAll(tok, "_", "-"), true
	}
	// Something else after the media name ("lesson 2.srt" for "lesson") is not
	// a subtitle of that media file.
	return "", false
}
