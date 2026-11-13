package api

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"

	"lectern/internal/fsutil"
	"lectern/internal/scan"
	"lectern/internal/store"
)

func fileMissing() *apiError {
	return &apiError{Status: http.StatusNotFound, Code: "file_missing", Message: "the file is no longer on the server"}
}

// openFile opens the file a location points at. The path is resolved inside
// the library root; anything that would escape it is reported as not found.
func (s *Server) openFile(loc *store.FileLoc) (*os.File, os.FileInfo, error) {
	p, err := fsutil.ResolveUnder(loc.LibraryPath, path.Join(loc.CourseRel, loc.ItemRel), s.Cfg.FollowSymlinks)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil, fileMissing()
		}
		s.Log.Warn("refusing to open file", "path", loc.ItemRel, "err", err)
		return nil, nil, notFound()
	}
	f, err := os.Open(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil, fileMissing()
		}
		return nil, nil, &apiError{Status: http.StatusInternalServerError, Code: "file_unreadable", Message: "the server cannot read that file"}
	}
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		f.Close()
		return nil, nil, notFound()
	}
	return f, fi, nil
}

func safeFilename(name string) string {
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == '/' || r == '\\' || r == '"' {
			return '_'
		}
		return r
	}, name)
	if name == "" {
		return "file"
	}
	return name
}

func disposition(kind, name string) string {
	if v := mime.FormatMediaType(kind, map[string]string{"filename": safeFilename(name)}); v != "" {
		return v
	}
	return kind
}

const sandboxCSP = "default-src 'none'; style-src 'unsafe-inline'; sandbox"

// inlineType decides how a file is shown in the browser. Only types the web
// UI can display itself are inline; the rest are served as downloads. Text is
// always text/plain and SVG is sandboxed, so an uploaded file can never run
// script in this site's origin.
func inlineType(loc *store.FileLoc) (contentType, csp string, inline bool) {
	switch loc.Kind {
	case "video", "audio":
		return loc.Mime, "", true
	case "pdf":
		return "application/pdf", "", true
	case "image":
		if loc.Ext == "svg" {
			return "image/svg+xml", sandboxCSP, true
		}
		return loc.Mime, "", true
	case "markdown", "text", "csv":
		return "text/plain; charset=utf-8", sandboxCSP, true
	}
	return "application/octet-stream", "", false
}

func (s *Server) serveLoc(w http.ResponseWriter, r *http.Request, loc *store.FileLoc, wantInline bool) error {
	f, fi, err := s.openFile(loc)
	if err != nil {
		return err
	}
	defer f.Close()

	h := w.Header()
	ct, csp, canInline := inlineType(loc)
	if wantInline && canInline {
		h.Set("Content-Type", ct)
		if csp != "" {
			h.Set("Content-Security-Policy", csp)
		}
		h.Set("Content-Disposition", disposition("inline", loc.Name))
	} else {
		h.Set("Content-Type", loc.Mime)
		if loc.Mime == "" {
			h.Set("Content-Type", "application/octet-stream")
		}
		h.Set("Content-Disposition", disposition("attachment", loc.Name))
	}
	h.Set("Cache-Control", "private, no-cache")
	h.Set("ETag", fmt.Sprintf(`W/"%x-%x"`, fi.Size(), fi.ModTime().UnixNano()))
	http.ServeContent(w, r, loc.Name, fi.ModTime(), f)
	return nil
}

func (s *Server) streamItem(w http.ResponseWriter, r *http.Request, a *authCtx) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	loc, err := s.Store.ItemFile(r.Context(), id)
	if err != nil {
		return err
	}
	if loc.Kind == "link" {
		return badRequest("links have no file")
	}
	return s.serveLoc(w, r, loc, true)
}

// assetExts are the pictures a text page may show from its own folder. SVG is
// left out on purpose: opened on its own it could run script in this origin.
var assetExts = map[string]bool{"png": true, "jpg": true, "jpeg": true, "gif": true, "webp": true, "avif": true, "bmp": true}

// assetPath resolves ref, a reference written in a page ("images/a.png"),
// against the folder of the page itself. The result is relative to the course
// folder; ok is false for anything that is not a plain file reference or that
// would leave the course.
func assetPath(itemRel, ref string) (rel string, ok bool) {
	ref = strings.ReplaceAll(ref, "\\", "/") // pages written on Windows
	if i := strings.IndexAny(ref, "?#"); i >= 0 {
		ref = ref[:i]
	}
	if ref == "" || len(ref) > 1024 || strings.ContainsRune(ref, 0) || strings.Contains(ref, "://") || strings.HasPrefix(ref, "//") {
		return "", false
	}
	if strings.HasPrefix(ref, "/") {
		rel = path.Clean(strings.TrimLeft(ref, "/")) // from the course folder
	} else {
		rel = path.Join(path.Dir(itemRel), ref)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", false
	}
	return rel, true
}

// getItemAsset serves a picture that a text page refers to, looked up beside
// that page and never outside its course folder. It lets the images of an HTML
// or Markdown lesson show up, with the same sign-in as the page itself.
func (s *Server) getItemAsset(w http.ResponseWriter, r *http.Request, a *authCtx) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	loc, err := s.Store.ItemFile(r.Context(), id)
	if err != nil {
		return err
	}
	if loc.Kind == "link" || loc.ItemRel == "" {
		return notFound()
	}
	rel, ok := assetPath(loc.ItemRel, r.URL.Query().Get("path"))
	if !ok {
		return notFound()
	}
	ext := strings.ToLower(strings.TrimPrefix(path.Ext(rel), "."))
	if !assetExts[ext] {
		return notFound()
	}
	asset := *loc
	asset.ItemRel, asset.Name, asset.Ext, asset.Kind, asset.Mime = rel, path.Base(rel), ext, "image", scan.Classify(ext).Mime
	return s.serveLoc(w, r, &asset, true)
}

func (s *Server) downloadItem(w http.ResponseWriter, r *http.Request, a *authCtx) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	loc, err := s.Store.ItemFile(r.Context(), id)
	if err != nil {
		return err
	}
	if loc.Kind == "link" {
		return badRequest("links have no file")
	}
	return s.serveLoc(w, r, loc, false)
}

// getCover serves a course's cover: the picture an admin uploaded if there is
// one, otherwise the cover image found in the course folder.
func (s *Server) getCover(w http.ResponseWriter, r *http.Request, a *authCtx) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	if served, err := s.serveCustomCover(w, r, id); served || err != nil {
		return err
	}
	loc, err := s.Store.CoverFile(r.Context(), id)
	if err != nil {
		return err
	}
	ext := strings.ToLower(strings.TrimPrefix(path.Ext(loc.ItemRel), "."))
	ki := scan.Classify(ext)
	if ki.Kind != "image" {
		return notFound()
	}
	loc.Name, loc.Ext, loc.Mime, loc.Kind = path.Base(loc.ItemRel), ext, ki.Mime, "image"
	w.Header().Set("Cache-Control", "private, max-age=300")
	f, fi, err := s.openFile(loc)
	if err != nil {
		return err
	}
	defer f.Close()
	w.Header().Set("Content-Type", ki.Mime)
	w.Header().Set("ETag", fmt.Sprintf(`W/"%x-%x"`, fi.Size(), fi.ModTime().UnixNano()))
	http.ServeContent(w, r, loc.Name, fi.ModTime(), f)
	return nil
}

const maxSubtitleBytes = 4 << 20

func (s *Server) getSubtitle(w http.ResponseWriter, r *http.Request, a *authCtx) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	loc, err := s.Store.SubtitleFile(r.Context(), id)
	if err != nil {
		return err
	}
	f, _, err := s.openFile(loc)
	if err != nil {
		return err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxSubtitleBytes))
	if err != nil {
		return err
	}
	var out []byte
	if loc.Format == "srt" {
		out = SRTToVTT(raw)
	} else {
		out = NormalizeVTT(raw)
	}
	h := w.Header()
	h.Set("Content-Type", "text/vtt; charset=utf-8")
	h.Set("Cache-Control", "private, max-age=300")
	h.Set("Content-Length", strconv.Itoa(len(out)))
	if r.Method != http.MethodHead {
		_, _ = w.Write(out)
	}
	return nil
}
