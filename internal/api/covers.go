package api

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"lectern/internal/store"
)

// maxCoverBytes caps an uploaded cover picture. The web app shrinks pictures
// well below this before sending them; the limit protects the server from
// everything else.
const maxCoverBytes = 5 << 20

// sniffImage names the picture format of data by its first bytes, or returns
// "" for anything Lectern does not accept as a cover. SVG is left out on
// purpose: it can carry script.
func sniffImage(b []byte) string {
	switch {
	case len(b) >= 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF:
		return "image/jpeg"
	case bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")):
		return "image/png"
	case len(b) >= 12 && string(b[0:4]) == "RIFF" && string(b[8:12]) == "WEBP":
		return "image/webp"
	case len(b) >= 12 && string(b[4:8]) == "ftyp" && (string(b[8:12]) == "avif" || string(b[8:12]) == "avis"):
		return "image/avif"
	}
	return ""
}

// etagMatches implements the weak comparison of If-None-Match.
func etagMatches(header, etag string) bool {
	for _, part := range strings.Split(header, ",") {
		p := strings.TrimSpace(part)
		if p == "*" || strings.TrimPrefix(p, "W/") == etag {
			return true
		}
	}
	return false
}

// serveCustomCover sends the cover an admin uploaded. It reports false (and
// writes nothing) when the course has none, so the caller can fall back to the
// image in the course folder.
func (s *Server) serveCustomCover(w http.ResponseWriter, r *http.Request, courseID uint64) (bool, error) {
	meta, err := s.Store.CustomCover(r.Context(), courseID, false)
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	h := w.Header()
	etag := fmt.Sprintf(`"cover-%d"`, meta.Version)
	h.Set("ETag", etag)
	// A URL that carries the current version always returns the same picture,
	// so browsers may keep it for good; any other URL is checked every time.
	if r.URL.Query().Get("v") == strconv.FormatInt(meta.Version, 10) {
		h.Set("Cache-Control", "private, max-age=31536000, immutable")
	} else {
		h.Set("Cache-Control", "private, no-cache")
	}
	if etagMatches(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return true, nil
	}

	full, err := s.Store.CustomCover(r.Context(), courseID, true)
	if err != nil {
		return false, err
	}
	h.Set("ETag", fmt.Sprintf(`"cover-%d"`, full.Version)) // in case it was replaced a moment ago
	h.Set("Content-Type", full.Mime)
	h.Set("Content-Security-Policy", sandboxCSP)
	h.Set("Content-Length", strconv.Itoa(len(full.Data)))
	if r.Method != http.MethodHead {
		_, _ = w.Write(full.Data)
	}
	return true, nil
}

// adminSetCover stores the request body (a JPEG, PNG, WebP or AVIF picture) as
// the course's cover.
func (s *Server) adminSetCover(w http.ResponseWriter, r *http.Request, a *authCtx) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	tooLarge := &apiError{Status: http.StatusRequestEntityTooLarge, Code: "too_large", Message: "the picture is larger than 5 MB"}
	if r.ContentLength > maxCoverBytes {
		return tooLarge
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxCoverBytes)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		var big *http.MaxBytesError
		if errors.As(err, &big) {
			return tooLarge
		}
		return badRequest("the upload was interrupted")
	}
	if len(data) == 0 {
		return badRequest("the request body is empty")
	}
	mime := sniffImage(data)
	if mime == "" {
		return &apiError{Status: http.StatusUnsupportedMediaType, Code: "unsupported_image", Message: "use a JPEG, PNG, WebP or AVIF picture"}
	}
	if _, err := s.Store.SetCustomCover(r.Context(), id, mime, data); err != nil {
		return err
	}
	return s.writeCoverState(w, r, id)
}

// adminDeleteCover removes the uploaded cover; the course goes back to the
// image in its folder, or to the drawn artwork.
func (s *Server) adminDeleteCover(w http.ResponseWriter, r *http.Request, a *authCtx) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	if err := s.Store.DeleteCustomCover(r.Context(), id); err != nil {
		return err
	}
	return s.writeCoverState(w, r, id)
}

func (s *Server) writeCoverState(w http.ResponseWriter, r *http.Request, id uint64) error {
	st, err := s.Store.CoverState(r.Context(), id)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, st)
	return nil
}
