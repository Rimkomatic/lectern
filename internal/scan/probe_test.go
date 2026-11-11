package scan

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func box(typ string, body []byte) []byte {
	out := make([]byte, 8, 8+len(body))
	binary.BigEndian.PutUint32(out[0:4], uint32(8+len(body)))
	copy(out[4:8], typ)
	return append(out, body...)
}

func mvhdV0(timescale, duration uint32) []byte {
	b := make([]byte, 100)
	binary.BigEndian.PutUint32(b[12:16], timescale)
	binary.BigEndian.PutUint32(b[16:20], duration)
	return box("mvhd", b)
}

func mvhdV1(timescale uint32, duration uint64) []byte {
	b := make([]byte, 112)
	b[0] = 1
	binary.BigEndian.PutUint32(b[20:24], timescale)
	binary.BigEndian.PutUint64(b[24:32], duration)
	return box("mvhd", b)
}

func TestProbeMP4Synthetic(t *testing.T) {
	ftyp := box("ftyp", []byte("isom\x00\x00\x00\x00isom"))
	mdat := box("mdat", make([]byte, 5000))
	cases := []struct {
		name string
		data []byte
		want float64
		ok   bool
	}{
		{"moov after mdat (v0)", bytes.Join([][]byte{ftyp, mdat, box("moov", mvhdV0(1000, 90500))}, nil), 90.5, true},
		{"moov first (v0)", bytes.Join([][]byte{ftyp, box("moov", mvhdV0(600, 3600)), mdat}, nil), 6, true},
		{"version 1", bytes.Join([][]byte{ftyp, box("moov", mvhdV1(90000, 90000*3725)), mdat}, nil), 3725, true},
		{"unknown duration", bytes.Join([][]byte{ftyp, box("moov", mvhdV0(1000, 0xFFFFFFFF))}, nil), 0, false},
		{"zero timescale", bytes.Join([][]byte{ftyp, box("moov", mvhdV0(0, 10))}, nil), 0, false},
		{"no moov", bytes.Join([][]byte{ftyp, mdat}, nil), 0, false},
		{"truncated", bytes.Join([][]byte{ftyp, box("moov", mvhdV0(1000, 5000))}, nil)[:40], 0, false},
		{"garbage", []byte("this is not an mp4 file at all, just text"), 0, false},
	}
	for _, c := range cases {
		got, ok := probeMP4(bytes.NewReader(c.data), int64(len(c.data)))
		if ok != c.ok || (ok && math.Abs(got-c.want) > 1e-6) {
			t.Errorf("%s: got %v,%v want %v,%v", c.name, got, ok, c.want, c.ok)
		}
	}
}

// ebml builds an element with a 1- or 2-byte size.
func ebml(id []byte, body []byte) []byte {
	var size []byte
	if len(body) < 127 {
		size = []byte{0x80 | byte(len(body))}
	} else {
		size = []byte{0x40 | byte(len(body)>>8), byte(len(body))}
	}
	return bytes.Join([][]byte{id, size, body}, nil)
}

func f64(v float64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, math.Float64bits(v))
	return b
}

func f32(v float32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, math.Float32bits(v))
	return b
}

func TestProbeMKVSynthetic(t *testing.T) {
	header := ebml([]byte{0x1A, 0x45, 0xDF, 0xA3}, ebml([]byte{0x42, 0x82}, []byte("webm")))
	scale := ebml([]byte{0x2A, 0xD7, 0xB1}, []byte{0x0F, 0x42, 0x40}) // 1,000,000 ns
	cluster := ebml([]byte{0x1F, 0x43, 0xB6, 0x75}, nil)
	unknownSize := []byte{0x01, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
	segment := func(children ...[]byte) []byte {
		return bytes.Join(append([][]byte{{0x18, 0x53, 0x80, 0x67}, unknownSize}, children...), nil)
	}
	info := func(parts ...[]byte) []byte { return ebml([]byte{0x15, 0x49, 0xA9, 0x66}, bytes.Join(parts, nil)) }
	dur64 := ebml([]byte{0x44, 0x89}, f64(12345)) // 12345 ms
	dur32 := ebml([]byte{0x44, 0x89}, f32(2500))
	seekHead := ebml([]byte{0x11, 0x4D, 0x9B, 0x74}, make([]byte, 20))

	cases := []struct {
		name string
		data []byte
		want float64
		ok   bool
	}{
		{"float64 duration", bytes.Join([][]byte{header, segment(info(scale, dur64), cluster)}, nil), 12.345, true},
		{"float32 duration", bytes.Join([][]byte{header, segment(seekHead, info(scale, dur32), cluster)}, nil), 2.5, true},
		{"default timescale", bytes.Join([][]byte{header, segment(info(dur64), cluster)}, nil), 12.345, true},
		{"custom timescale (1us)", bytes.Join([][]byte{header, segment(info(ebml([]byte{0x2A, 0xD7, 0xB1}, []byte{0x03, 0xE8}), dur64))}, nil), 12345e-6, true},
		{"no duration element", bytes.Join([][]byte{header, segment(info(scale), cluster)}, nil), 0, false},
		{"cluster before info", bytes.Join([][]byte{header, segment(cluster, info(scale, dur64))}, nil), 0, false},
		{"not matroska", []byte("RIFF....WAVEfmt "), 0, false},
		{"truncated header", header[:6], 0, false},
	}
	for _, c := range cases {
		got, ok := probeMKV(bytes.NewReader(c.data), int64(len(c.data)))
		if ok != c.ok || (ok && math.Abs(got-c.want) > 1e-6) {
			t.Errorf("%s: got %v,%v want %v,%v", c.name, got, ok, c.want, c.ok)
		}
	}
}

func FuzzProbe(f *testing.F) {
	f.Add([]byte("\x00\x00\x00\x18ftypisom\x00\x00\x00\x00isom\x00\x00\x00\x08free"))
	f.Add([]byte{0x1A, 0x45, 0xDF, 0xA3, 0x80, 0x18, 0x53, 0x80, 0x67, 0x01, 0xFF})
	f.Fuzz(func(t *testing.T, data []byte) {
		probeMP4(bytes.NewReader(data), int64(len(data)))
		probeMKV(bytes.NewReader(data), int64(len(data)))
	})
}

// TestProbeRealFiles checks the parsers against files made by ffmpeg.
func TestProbeRealFiles(t *testing.T) {
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	type tc struct {
		file string
		args []string
	}
	src := []string{"-f", "lavfi", "-i", "testsrc=duration=3:size=64x48:rate=10", "-f", "lavfi", "-i", "sine=frequency=440:duration=3"}
	cases := []tc{
		{"a.mp4", []string{"-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest"}},
		{"b.mp4", []string{"-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest", "-movflags", "+faststart"}},
		{"c.webm", []string{"-c:v", "libvpx", "-c:a", "libvorbis", "-shortest"}},
		{"d.mkv", []string{"-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest"}},
		{"e.m4a", []string{"-vn", "-c:a", "aac", "-shortest"}},
	}
	for _, c := range cases {
		out := filepath.Join(dir, c.file)
		args := append(append([]string{"-hide_banner", "-loglevel", "error", "-y"}, src...), c.args...)
		args = append(args, out)
		if b, err := exec.Command(ff, args...).CombinedOutput(); err != nil {
			t.Logf("ffmpeg could not make %s (%v: %s); skipping it", c.file, err, b)
			continue
		}
		ext := filepath.Ext(c.file)[1:]
		got, ok := ProbeDuration(out, ext)
		if !ok || math.Abs(got-3) > 0.35 {
			t.Errorf("%s: probe = %v,%v want about 3s", c.file, got, ok)
		}
	}
	// a non-media file with a media extension must not panic or report a duration
	junk := filepath.Join(dir, "junk.mp4")
	os.WriteFile(junk, bytes.Repeat([]byte("x"), 4096), 0o644)
	if d, ok := ProbeDuration(junk, "mp4"); ok {
		t.Errorf("junk probed as %v", d)
	}
	if _, ok := ProbeDuration(filepath.Join(dir, "missing.mp4"), "mp4"); ok {
		t.Error("missing file probed ok")
	}
}

func TestFingerprint(t *testing.T) {
	dir := t.TempDir()
	mk := func(name string, data []byte) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	big := bytes.Repeat([]byte("0123456789abcdef"), 40000) // 640 KB
	a, b := mk("a", big), mk("b", big)
	changedMiddle := append([]byte(nil), big...)
	changedMiddle[len(big)/2] ^= 0xFF
	c := mk("c", changedMiddle)
	d := mk("d", append(append([]byte(nil), big...), 'x'))
	small := mk("small", []byte("hello"))

	fp := func(p string) []byte {
		fi, _ := os.Stat(p)
		out, err := Fingerprint(p, fi.Size())
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	if !bytes.Equal(fp(a), fp(b)) {
		t.Error("identical files must match")
	}
	if !bytes.Equal(fp(a), fp(c)) {
		t.Error("a change in the unsampled middle is expected to be invisible")
	}
	if bytes.Equal(fp(a), fp(d)) {
		t.Error("different size must differ")
	}
	if !HasFingerprint(fp(small)) || HasFingerprint(nil) || HasFingerprint(UnreadableFingerprint()) {
		t.Error("HasFingerprint wrong")
	}
	if _, err := Fingerprint(filepath.Join(dir, "nope"), 10); err == nil {
		t.Error("missing file should error")
	}
}
