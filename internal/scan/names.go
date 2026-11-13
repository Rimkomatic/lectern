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

// subtitleWords are the notes people add to a subtitle file's name besides the
// language: "lesson.en.forced.srt", "lesson - Subtitles.srt", "lesson (CC).srt".
var subtitleWords = map[string]bool{
	"subtitles": true, "subtitle": true, "subs": true, "sub": true,
	"captions": true, "caption": true, "cc": true, "sdh": true,
	"forced": true, "default": true, "full": true, "auto": true,
	"generated": true, "autogenerated": true,
}

// regionPart is what may follow a language inside one word: pt-BR, zh-Hans, es-419.
var regionPart = regexp.MustCompile(`^(?:[a-zA-Z]{2}|[0-9]{3}|[a-zA-Z]{4})$`)

// subtitleToken reads one word of a subtitle file name. kind is "lang" (code
// holds the language), "note" (a word such as "forced") or "" (neither).
func subtitleToken(tok string) (code, kind string) {
	low := strings.ToLower(tok)
	if subtitleWords[low] {
		return "", "note"
	}
	if c, ok := langWords[low]; ok {
		return c, "lang"
	}
	if langToken.MatchString(tok) {
		return strings.ReplaceAll(tok, "_", "-"), "lang"
	}
	return "", ""
}

func isDash(r rune) bool { return r == '-' || r == '_' }

// subtitleSuffix reads what follows a media file's name in a subtitle file's
// name: a language ("en", "pt-BR", "English") and notes ("forced", "SDH",
// "Subtitles"), in any order, with dots, spaces, dashes or brackets between.
// ok is false if anything else is in there ("lesson 2" is not "lesson").
func subtitleSuffix(rest string) (lang string, ok bool) {
	fields := strings.FieldsFunc(rest, func(r rune) bool {
		switch r {
		case '.', ',', '(', ')', '[', ']', '{', '}':
			return true
		}
		return unicode.IsSpace(r)
	})
	haveLang := false
	for _, f := range fields {
		f = strings.Trim(f, "-_")
		if f == "" {
			continue
		}
		if code, kind := subtitleToken(f); kind != "" {
			if kind == "lang" {
				if haveLang {
					return "", false
				}
				lang, haveLang = code, true
			}
			continue
		}
		// Several words joined by dashes: "en-forced", "pt-BR-forced", "English-US".
		parts := strings.FieldsFunc(f, isDash)
		if len(parts) < 2 {
			return "", false
		}
		for i := 0; i < len(parts); i++ {
			code, kind := subtitleToken(parts[i])
			switch kind {
			case "":
				return "", false
			case "note":
				continue
			}
			if haveLang {
				return "", false
			}
			if i+1 < len(parts) && regionPart.MatchString(parts[i+1]) && !subtitleWords[strings.ToLower(parts[i+1])] {
				code += "-" + parts[i+1]
				i++
			}
			lang, haveLang = code, true
		}
	}
	return normLang(lang), true
}

// normLang writes a language code the usual way: "en", "pt-BR", "es-419".
func normLang(code string) string {
	base, region, found := strings.Cut(strings.ReplaceAll(code, "_", "-"), "-")
	base = strings.ToLower(base)
	if !found {
		return base
	}
	if len(region) == 2 {
		return base + "-" + strings.ToUpper(region)
	}
	return base + "-" + strings.ToLower(region)
}

// SubtitleLang decides whether a subtitle file stem belongs to a media file
// stem and extracts its language code. "lesson" matches "lesson", "lesson.en",
// "lesson.pt-BR.forced", "lesson_english", "lesson - fr", "lesson.en.SDH",
// "lesson (English)" and "lesson - Subtitles"; not "lesson 2" or "lessons".
// Callers also try it with the media file's full name, which finds the
// "lesson.mp4.en.srt" that some tools write.
func SubtitleLang(mediaStem, subStem string) (lang string, ok bool) {
	if strings.EqualFold(mediaStem, subStem) {
		return "", true
	}
	if len(subStem) <= len(mediaStem) || !strings.EqualFold(subStem[:len(mediaStem)], mediaStem) {
		return "", false
	}
	rest := subStem[len(mediaStem):]
	switch rest[0] {
	case '.', '_', '-', ' ', '(', '[':
	default:
		return "", false
	}
	return subtitleSuffix(rest)
}

// GenericSubtitleLang reads a subtitle file stem that names no particular
// video, only a language or the kind of file ("english", "en", "subtitles.en",
// "captions"). ok is false for anything with a title in it.
func GenericSubtitleLang(subStem string) (lang string, ok bool) {
	if strings.TrimSpace(subStem) == "" {
		return "", false
	}
	return subtitleSuffix(subStem)
}
