package scan

import (
	"html"
	"io"
	"net/url"
	"os"
	"regexp"
	"strings"
)

const maxLinkFile = 64 << 10

var plistURL = regexp.MustCompile(`(?s)<key>\s*URL\s*</key>\s*<string>(.*?)</string>`)

// ReadLink reads a shortcut file (.url, .webloc, .desktop) and returns the
// http(s) address inside, if there is one.
func ReadLink(path, ext string) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxLinkFile))
	if err != nil {
		return "", false
	}
	return ParseLink(ext, data)
}

// ParseLink extracts the target address from the content of a shortcut file.
// Only http and https addresses are accepted: anything else (javascript:,
// file:, data:, ...) must never become a clickable link.
func ParseLink(ext string, data []byte) (string, bool) {
	text := strings.TrimPrefix(string(data), "\xef\xbb\xbf")
	var raw string
	switch ext {
	case "webloc":
		if m := plistURL.FindStringSubmatch(text); m != nil {
			raw = html.UnescapeString(strings.TrimSpace(m[1]))
		}
	case "url", "desktop":
		isLink := ext == "url"
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(line)
			key, val, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			switch strings.ToLower(strings.TrimSpace(key)) {
			case "url":
				if raw == "" {
					raw = strings.TrimSpace(val)
				}
			case "type":
				if strings.EqualFold(strings.TrimSpace(val), "link") {
					isLink = true
				}
			}
		}
		if !isLink {
			raw = ""
		}
	}
	return SafeURL(raw)
}

// SafeURL validates and normalises an http(s) address.
func SafeURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 2048 {
		return "", false
	}
	for _, r := range raw {
		if r < 0x20 || r == 0x7f {
			return "", false
		}
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return u.String(), true
	}
	return "", false
}
