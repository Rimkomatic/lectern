package scan

// KindInfo is what the scanner knows about a file type.
type KindInfo struct{ Kind, Mime string }

func k(kind, mime string) KindInfo { return KindInfo{kind, mime} }

// kindTable maps a lower-case extension to the item kind and the Content-Type
// the server will use for it. Kinds are the same ones the database enum allows.
var kindTable = map[string]KindInfo{
	// video
	"mp4": k("video", "video/mp4"), "m4v": k("video", "video/mp4"), "webm": k("video", "video/webm"),
	"mkv": k("video", "video/x-matroska"), "mov": k("video", "video/quicktime"),
	"avi": k("video", "video/x-msvideo"), "ogv": k("video", "video/ogg"),
	"mpg": k("video", "video/mpeg"), "mpeg": k("video", "video/mpeg"),
	"wmv": k("video", "video/x-ms-wmv"), "flv": k("video", "video/x-flv"),
	"3gp": k("video", "video/3gpp"), "m2ts": k("video", "video/mp2t"),

	// audio
	"mp3": k("audio", "audio/mpeg"), "m4a": k("audio", "audio/mp4"), "aac": k("audio", "audio/aac"),
	"ogg": k("audio", "audio/ogg"), "oga": k("audio", "audio/ogg"), "opus": k("audio", "audio/ogg"),
	"wav": k("audio", "audio/wav"), "flac": k("audio", "audio/flac"), "weba": k("audio", "audio/webm"),
	"mka": k("audio", "audio/x-matroska"), "wma": k("audio", "audio/x-ms-wma"),

	// documents
	"pdf": k("pdf", "application/pdf"),
	"md":  k("markdown", "text/markdown"), "markdown": k("markdown", "text/markdown"),
	"csv": k("csv", "text/csv"), "tsv": k("csv", "text/tab-separated-values"),
	"xlsx":    k("sheet", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"),
	"xls":     k("sheet", "application/vnd.ms-excel"),
	"ods":     k("sheet", "application/vnd.oasis.opendocument.spreadsheet"),
	"numbers": k("sheet", "application/vnd.apple.numbers"),

	// images
	"png": k("image", "image/png"), "jpg": k("image", "image/jpeg"), "jpeg": k("image", "image/jpeg"),
	"gif": k("image", "image/gif"), "webp": k("image", "image/webp"), "avif": k("image", "image/avif"),
	"bmp": k("image", "image/bmp"), "svg": k("image", "image/svg+xml"), "ico": k("image", "image/x-icon"),

	// links (turned into "other" by the walker if no usable URL is inside)
	"url": k("link", "text/plain"), "webloc": k("link", "text/plain"), "desktop": k("link", "text/plain"),

	// office files the browser cannot show: download only
	"docx": k("other", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"),
	"doc":  k("other", "application/msword"),
	"pptx": k("other", "application/vnd.openxmlformats-officedocument.presentationml.presentation"),
	"ppt":  k("other", "application/vnd.ms-powerpoint"),
	"odt":  k("other", "application/vnd.oasis.opendocument.text"),
	"odp":  k("other", "application/vnd.oasis.opendocument.presentation"),
	"epub": k("other", "application/epub+zip"),
	"zip":  k("other", "application/zip"), "7z": k("other", "application/x-7z-compressed"),
	"rar": k("other", "application/vnd.rar"), "gz": k("other", "application/gzip"),
	"tar": k("other", "application/x-tar"),
}

// plain-text style files, shown as text with a download button. ".ts" is
// TypeScript here, not MPEG transport streams; ".html" is shown as source.
var textExts = []string{
	"txt", "text", "log", "json", "jsonc", "yaml", "yml", "xml", "toml", "ini", "cfg", "conf", "env",
	"js", "mjs", "cjs", "ts", "tsx", "jsx", "py", "go", "rs", "java", "kt", "c", "h", "cpp", "hpp", "cs",
	"rb", "php", "sh", "bash", "zsh", "ps1", "sql", "css", "scss", "less", "html", "htm", "vue", "svelte",
	"r", "m", "swift", "lua", "pl", "dockerfile", "makefile", "gradle", "properties", "srt", "vtt", "tex", "rst",
	"ipynb",
}

func init() {
	for _, e := range textExts {
		if _, taken := kindTable[e]; !taken {
			kindTable[e] = k("text", "text/plain")
		}
	}
}

// Classify returns the kind and Content-Type for a lower-case extension
// (without the dot). Unknown types are "other" and downloaded as binary.
func Classify(ext string) KindInfo {
	if ki, ok := kindTable[ext]; ok {
		return ki
	}
	return KindInfo{"other", "application/octet-stream"}
}

// IsMedia is true for kinds the player handles.
func IsMedia(kind string) bool { return kind == "video" || kind == "audio" }

var coverStems = map[string]bool{"cover": true, "poster": true, "folder": true, "thumbnail": true, "thumb": true}
var coverExts = map[string]bool{"jpg": true, "jpeg": true, "png": true, "webp": true, "avif": true}

// IsCoverName tells whether a file in a course folder is its cover picture.
func IsCoverName(name string) bool {
	return coverExts[extOf(name)] && coverStems[toLowerASCII(stemOf(name))]
}

func toLowerASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}
