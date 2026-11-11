package scan

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"io"
	"os"
)

const fpChunk = 64 << 10

// Fingerprint is a cheap identity for a file's content: SHA-256 over the size
// and its first and last 64 KiB. It is only used to recognise a file that was
// renamed or moved, so that people keep their progress.
func Fingerprint(path string, size int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	h := sha256.New()
	var sz [8]byte
	binary.BigEndian.PutUint64(sz[:], uint64(size))
	h.Write(sz[:])
	if size <= 2*fpChunk {
		if _, err := io.Copy(h, io.LimitReader(f, size)); err != nil {
			return nil, err
		}
		return h.Sum(nil), nil
	}
	buf := make([]byte, fpChunk)
	if _, err := io.ReadFull(f, buf); err != nil {
		return nil, err
	}
	h.Write(buf)
	if n, err := f.ReadAt(buf, size-fpChunk); n != len(buf) {
		if err == nil {
			err = io.ErrUnexpectedEOF
		}
		return nil, err
	}
	h.Write(buf)
	return h.Sum(nil), nil
}

// HasFingerprint is true for a stored fingerprint that can be compared. An
// all-zero value marks files that could not be read.
func HasFingerprint(fp []byte) bool {
	return len(fp) == sha256.Size && !bytes.Equal(fp, make([]byte, sha256.Size))
}

// UnreadableFingerprint is stored for files that could not be fingerprinted so
// they are not retried forever.
func UnreadableFingerprint() []byte { return make([]byte, sha256.Size) }
