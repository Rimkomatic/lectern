package api

import (
	"strings"
	"testing"
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
