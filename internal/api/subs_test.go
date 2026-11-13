package api

import (
	"strings"
	"testing"
	"unicode/utf16"
)

func TestSRTToVTT(t *testing.T) {
	srt := "\xef\xbb\xbf1\r\n00:00:01,500 --> 00:00:03,25\r\nHello, world\r\n\r\n2\r\n00:01:00,000 --> 00:01:02,000 X1:10\r\n<i>Second</i> cue, with commas\r\n"
	got := string(SRTToVTT([]byte(srt)))
	want := "WEBVTT\n\n1\n00:00:01.500 --> 00:00:03.250\nHello, world\n\n2\n00:01:00.000 --> 00:01:02.000 X1:10\n<i>Second</i> cue, with commas\n"
	if !strings.HasPrefix(got, want) {
		t.Fatalf("got:\n%q\nwant prefix:\n%q", got, want)
	}
}

func TestSRTWindows1252(t *testing.T) {
	// "Café – “quoted”" in Windows-1252
	raw := []byte("1\n00:00:00,000 --> 00:00:01,000\nCaf\xe9 \x96 \x93quoted\x94\n")
	got := string(SRTToVTT(raw))
	if !strings.Contains(got, "Café – “quoted”") {
		t.Fatalf("legacy encoding not converted: %q", got)
	}
}

func TestNormalizeVTT(t *testing.T) {
	if got := string(NormalizeVTT([]byte("WEBVTT\r\n\r\n00:00.000 --> 00:01.000\r\nhi\r\n"))); !strings.HasPrefix(got, "WEBVTT\n\n00:00.000") || strings.Contains(got, "\r") {
		t.Errorf("good vtt changed wrongly: %q", got)
	}
	if got := string(NormalizeVTT([]byte("00:00.000 --> 00:01.000\nhi\n"))); !strings.HasPrefix(got, "WEBVTT\n\n") {
		t.Errorf("header not added: %q", got)
	}
}

// utf16Bytes encodes s as UTF-16, with or without a byte-order mark.
func utf16Bytes(s string, bigEndian, bom bool) []byte {
	var out []byte
	put := func(u uint16) {
		if bigEndian {
			out = append(out, byte(u>>8), byte(u))
		} else {
			out = append(out, byte(u), byte(u>>8))
		}
	}
	if bom {
		put(0xFEFF)
	}
	for _, u := range utf16.Encode([]rune(s)) {
		put(u)
	}
	return out
}

func TestSRTUTF16(t *testing.T) {
	const cue = "1\r\n00:00:01,500 --> 00:00:03,250\r\nCafé 你好 一 😀\r\n\r\n2\r\n00:01:00,000 --> 00:01:02,000\r\nSecond\r\n"
	want := "WEBVTT\n\n1\n00:00:01.500 --> 00:00:03.250\nCafé 你好 一 😀\n\n2\n00:01:00.000 --> 00:01:02.000\nSecond\n"
	for _, tc := range []struct {
		name string
		raw  []byte
	}{
		{"little endian with a mark", utf16Bytes(cue, false, true)},
		{"big endian with a mark", utf16Bytes(cue, true, true)},
		{"little endian, no mark", utf16Bytes(cue, false, false)},
		{"big endian, no mark", utf16Bytes(cue, true, false)},
		{"a stray last byte", append(utf16Bytes(cue, false, true), 0x00)},
	} {
		got := string(SRTToVTT(tc.raw))
		if !strings.HasPrefix(got, want) {
			t.Errorf("%s:\n got %q\nwant prefix %q", tc.name, got, want)
		}
		if strings.ContainsRune(got, 0) || strings.ContainsRune(got, 0xFFFD) || strings.ContainsRune(got, 0xFEFF) {
			t.Errorf("%s: junk characters in %q", tc.name, got)
		}
	}
	// WebVTT in UTF-16 gets its header checked the same way
	vtt := string(NormalizeVTT(utf16Bytes("WEBVTT\n\n00:00.000 --> 00:01.000\nhi\n", false, true)))
	if !strings.HasPrefix(vtt, "WEBVTT\n\n00:00.000") {
		t.Errorf("vtt in UTF-16: %q", vtt)
	}
}

func TestDecodeTextLeavesOtherEncodingsAlone(t *testing.T) {
	// UTF-8 with a mark, plain UTF-8, Windows-1252, and text too short to judge
	for in, want := range map[string]string{
		"\xef\xbb\xbfhello é": "hello é",
		"naïve – ok":          "naïve – ok",
		"Caf\xe9 \x96 ok":     "Café – ok",
		"hi":                  "hi",
		"":                    "",
		"1\n00:00:01,000 --> 00:00:02,000\nplain ascii, no zero bytes\n": "1\n00:00:01,000 --> 00:00:02,000\nplain ascii, no zero bytes\n",
	} {
		if got := decodeText([]byte(in)); got != want {
			t.Errorf("decodeText(%q) = %q, want %q", in, got, want)
		}
	}
}
