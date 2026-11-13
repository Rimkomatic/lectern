package api

import (
	"encoding/binary"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// cp1252High maps bytes 0x80-0x9F of Windows-1252 to Unicode (0 = undefined).
var cp1252High = [32]rune{
	0x20AC, 0, 0x201A, 0x0192, 0x201E, 0x2026, 0x2020, 0x2021, 0x02C6, 0x2030, 0x0160, 0x2039, 0x0152, 0, 0x017D, 0,
	0, 0x2018, 0x2019, 0x201C, 0x201D, 0x2022, 0x2013, 0x2014, 0x02DC, 0x2122, 0x0161, 0x203A, 0x0153, 0, 0x017E, 0x0178,
}

// decodeText returns UTF-8 text from subtitle bytes. It understands UTF-8 (with
// or without a byte-order mark) and UTF-16 (little or big endian; with a mark,
// or without one when the text is clearly 16-bit). Anything else is read as
// Windows-1252, the usual encoding of old subtitle files.
func decodeText(b []byte) string {
	switch {
	case len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE:
		return decodeUTF16(b[2:], binary.LittleEndian)
	case len(b) >= 2 && b[0] == 0xFE && b[1] == 0xFF:
		return decodeUTF16(b[2:], binary.BigEndian)
	}
	if order := sniffUTF16(b); order != nil {
		return decodeUTF16(b, order)
	}
	b = []byte(strings.TrimPrefix(string(b), "\xef\xbb\xbf"))
	if utf8.Valid(b) {
		return string(b)
	}
	var sb strings.Builder
	sb.Grow(len(b) + len(b)/4)
	for _, c := range b {
		switch {
		case c >= 0x80 && c < 0xA0 && cp1252High[c-0x80] != 0:
			sb.WriteRune(cp1252High[c-0x80])
		default:
			sb.WriteRune(rune(c))
		}
	}
	return sb.String()
}

func decodeUTF16(b []byte, order binary.ByteOrder) string {
	u := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 { // a stray last byte is dropped
		u = append(u, order.Uint16(b[i:]))
	}
	return strings.TrimPrefix(string(utf16.Decode(u)), "\uFEFF")
}

// sniffUTF16 recognises UTF-16 text that has no byte-order mark. Subtitle files
// are mostly digits and punctuation, which in 16-bit text leave a zero byte next
// to every one of them, in the same position each time; text in any 8-bit
// encoding has no zero bytes at all.
func sniffUTF16(b []byte) binary.ByteOrder {
	if len(b) > 512 {
		b = b[:512]
	}
	pairs := len(b) / 2
	if pairs < 4 {
		return nil
	}
	var even, odd int // zero bytes at even and at odd positions
	for i, c := range b[:pairs*2] {
		if c == 0 {
			if i%2 == 0 {
				even++
			} else {
				odd++
			}
		}
	}
	switch {
	case odd*4 >= pairs && odd >= 4*even: // 'A' is 41 00: the high byte comes second
		return binary.LittleEndian
	case even*4 >= pairs && even >= 4*odd:
		return binary.BigEndian
	}
	return nil
}

var srtTiming = regexp.MustCompile(`^(\s*)(\d{1,3}):(\d{2}):(\d{2})[,.](\d{1,3})(\s*-->\s*)(\d{1,3}):(\d{2}):(\d{2})[,.](\d{1,3})(.*)$`)

func vttStamp(h, m, s, ms string) string {
	hh, _ := strconv.Atoi(h)
	mm, _ := strconv.Atoi(m)
	ss, _ := strconv.Atoi(s)
	for len(ms) < 3 {
		ms += "0"
	}
	return fmt.Sprintf("%02d:%02d:%02d.%s", hh, mm, ss, ms)
}

// SRTToVTT converts SubRip subtitles to WebVTT, which is what browsers play.
func SRTToVTT(in []byte) []byte {
	text := strings.ReplaceAll(decodeText(in), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	var sb strings.Builder
	sb.WriteString("WEBVTT\n\n")
	for _, ln := range strings.Split(text, "\n") {
		if m := srtTiming.FindStringSubmatch(ln); m != nil {
			ln = vttStamp(m[2], m[3], m[4], m[5]) + " --> " + vttStamp(m[7], m[8], m[9], m[10]) + m[11]
		}
		sb.WriteString(ln)
		sb.WriteByte('\n')
	}
	return []byte(sb.String())
}

// NormalizeVTT fixes encoding and makes sure the WEBVTT header is present.
func NormalizeVTT(in []byte) []byte {
	text := strings.ReplaceAll(decodeText(in), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	if !strings.HasPrefix(text, "WEBVTT") {
		text = "WEBVTT\n\n" + text
	}
	return []byte(text)
}
