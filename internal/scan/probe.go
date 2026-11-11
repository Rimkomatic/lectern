package scan

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
	"math/bits"
	"os"
)

// ProbeDuration reads a media duration (seconds) straight from the container
// header, without ffmpeg. It understands MP4/MOV (moov/mvhd) and
// Matroska/WebM (Segment/Info/Duration). Anything else returns false and the
// duration is filled in the first time a browser plays the file.
func ProbeDuration(path, ext string) (float64, bool) {
	switch ext {
	case "mp4", "m4v", "m4a", "mov", "3gp", "3g2":
		return probeFile(path, probeMP4)
	case "mkv", "webm", "mka", "weba":
		return probeFile(path, probeMKV)
	}
	return 0, false
}

func probeFile(path string, fn func(io.ReadSeeker, int64) (float64, bool)) (d float64, ok bool) {
	defer func() {
		if recover() != nil {
			d, ok = 0, false
		}
	}()
	f, err := os.Open(path)
	if err != nil {
		return 0, false
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return 0, false
	}
	d, ok = fn(f, fi.Size())
	if !ok || math.IsNaN(d) || math.IsInf(d, 0) || d <= 0 || d > 60*60*24*30 {
		return 0, false
	}
	return d, true
}

var errBad = errors.New("bad container")

// ---- MP4 / MOV ----

// readBox parses the box header at pos and returns its type and body range.
func readBox(r io.ReadSeeker, pos, limit int64) (typ string, bodyStart, end int64, err error) {
	if _, err = r.Seek(pos, io.SeekStart); err != nil {
		return
	}
	var hdr [16]byte
	if _, err = io.ReadFull(r, hdr[:8]); err != nil {
		return
	}
	size := int64(binary.BigEndian.Uint32(hdr[0:4]))
	typ = string(hdr[4:8])
	hdrLen := int64(8)
	switch size {
	case 1: // 64-bit size follows
		if _, err = io.ReadFull(r, hdr[8:16]); err != nil {
			return
		}
		u := binary.BigEndian.Uint64(hdr[8:16])
		if u > math.MaxInt64 {
			err = errBad
			return
		}
		size, hdrLen = int64(u), 16
	case 0: // runs to the end of the enclosing box
		size = limit - pos
	}
	if size < hdrLen || pos+size > limit {
		err = errBad
		return
	}
	return typ, pos + hdrLen, pos + size, nil
}

func findBox(r io.ReadSeeker, start, limit int64, want string, maxBoxes int) (bodyStart, end int64, ok bool) {
	pos := start
	for i := 0; i < maxBoxes && pos+8 <= limit; i++ {
		typ, bs, e, err := readBox(r, pos, limit)
		if err != nil {
			return 0, 0, false
		}
		if typ == want {
			return bs, e, true
		}
		pos = e
	}
	return 0, 0, false
}

func probeMP4(r io.ReadSeeker, size int64) (float64, bool) {
	bs, be, ok := findBox(r, 0, size, "moov", 64)
	if !ok {
		return 0, false
	}
	bs, be, ok = findBox(r, bs, be, "mvhd", 32)
	if !ok || be-bs < 20 {
		return 0, false
	}
	if _, err := r.Seek(bs, io.SeekStart); err != nil {
		return 0, false
	}
	var b [32]byte
	n := int(min(be-bs, int64(len(b))))
	if _, err := io.ReadFull(r, b[:n]); err != nil {
		return 0, false
	}
	var timescale uint32
	var dur uint64
	if b[0] == 1 {
		if n < 32 {
			return 0, false
		}
		timescale = binary.BigEndian.Uint32(b[20:24])
		dur = binary.BigEndian.Uint64(b[24:32])
		if dur == math.MaxUint64 {
			return 0, false
		}
	} else {
		timescale = binary.BigEndian.Uint32(b[12:16])
		dur = uint64(binary.BigEndian.Uint32(b[16:20]))
		if dur == math.MaxUint32 {
			return 0, false
		}
	}
	if timescale == 0 {
		return 0, false
	}
	return float64(dur) / float64(timescale), true
}

// ---- Matroska / WebM (EBML) ----

const (
	idEBML          = 0x1A45DFA3
	idSegment       = 0x18538067
	idInfo          = 0x1549A966
	idCluster       = 0x1F43B675
	idTimecodeScale = 0x2AD7B1
	idDuration      = 0x4489
)

// readVint reads an EBML variable-length integer. Element ids keep their
// length-marker bits; sizes drop them (and report the all-ones "unknown").
func readVint(r io.Reader, keepMarker bool) (val uint64, length int, unknown bool, err error) {
	var first [1]byte
	if _, err = io.ReadFull(r, first[:]); err != nil {
		return
	}
	b := first[0]
	if b == 0 {
		err = errBad
		return
	}
	length = bits.LeadingZeros8(b) + 1
	mask := uint64(1)<<(8-uint(length)) - 1
	if keepMarker {
		val = uint64(b)
	} else {
		val = uint64(b) & mask
	}
	allOnes := uint64(b)&mask == mask
	if length > 1 {
		var rest [7]byte
		if _, err = io.ReadFull(r, rest[:length-1]); err != nil {
			return
		}
		for _, c := range rest[:length-1] {
			val = val<<8 | uint64(c)
			if c != 0xFF {
				allOnes = false
			}
		}
	}
	unknown = !keepMarker && allOnes
	return
}

type element struct {
	id                 uint64
	bodyStart, bodyEnd int64
	unknown            bool
}

func readElement(r io.ReadSeeker, pos, limit int64) (e element, err error) {
	if pos >= limit {
		return e, errBad
	}
	if _, err = r.Seek(pos, io.SeekStart); err != nil {
		return
	}
	id, idLen, _, err := readVint(r, true)
	if err != nil || idLen > 4 {
		return e, errBad
	}
	size, sizeLen, unknown, err := readVint(r, false)
	if err != nil {
		return e, errBad
	}
	e.id = id
	e.bodyStart = pos + int64(idLen+sizeLen)
	if unknown || size > uint64(limit) {
		e.bodyEnd, e.unknown = limit, unknown
	} else {
		e.bodyEnd = e.bodyStart + int64(size)
	}
	if e.bodyEnd > limit {
		e.bodyEnd = limit // truncated file: read what is there
	}
	if e.bodyStart > e.bodyEnd {
		return e, errBad
	}
	return e, nil
}

func probeMKV(r io.ReadSeeker, size int64) (float64, bool) {
	head, err := readElement(r, 0, size)
	if err != nil || head.id != idEBML {
		return 0, false
	}
	seg, err := readElement(r, head.bodyEnd, size)
	if err != nil || seg.id != idSegment {
		return 0, false
	}
	pos := seg.bodyStart
	for i := 0; i < 64 && pos < seg.bodyEnd; i++ {
		el, err := readElement(r, pos, seg.bodyEnd)
		if err != nil {
			return 0, false
		}
		switch el.id {
		case idInfo:
			return parseMKVInfo(r, el)
		case idCluster:
			return 0, false // media data starts: no Info up front, give up
		}
		if el.unknown {
			return 0, false
		}
		pos = el.bodyEnd
	}
	return 0, false
}

func parseMKVInfo(r io.ReadSeeker, info element) (float64, bool) {
	if info.bodyEnd-info.bodyStart > 1<<20 {
		return 0, false
	}
	scale := uint64(1_000_000) // default: milliseconds
	var dur float64
	var haveDur bool
	pos := info.bodyStart
	for i := 0; i < 64 && pos < info.bodyEnd; i++ {
		el, err := readElement(r, pos, info.bodyEnd)
		if err != nil {
			break
		}
		n := el.bodyEnd - el.bodyStart
		switch {
		case el.id == idTimecodeScale && n >= 1 && n <= 8:
			if v, ok := readUint(r, el.bodyStart, n); ok && v > 0 {
				scale = v
			}
		case el.id == idDuration && (n == 4 || n == 8):
			var b [8]byte
			if _, err := r.Seek(el.bodyStart, io.SeekStart); err != nil {
				return 0, false
			}
			if _, err := io.ReadFull(r, b[:n]); err != nil {
				return 0, false
			}
			if n == 4 {
				dur = float64(math.Float32frombits(binary.BigEndian.Uint32(b[:4])))
			} else {
				dur = math.Float64frombits(binary.BigEndian.Uint64(b[:8]))
			}
			haveDur = true
		}
		pos = el.bodyEnd
	}
	if !haveDur {
		return 0, false
	}
	return dur * float64(scale) / 1e9, true
}

func readUint(r io.ReadSeeker, pos, n int64) (uint64, bool) {
	var b [8]byte
	if _, err := r.Seek(pos, io.SeekStart); err != nil {
		return 0, false
	}
	if _, err := io.ReadFull(r, b[8-n:]); err != nil {
		return 0, false
	}
	return binary.BigEndian.Uint64(b[:]), true
}
