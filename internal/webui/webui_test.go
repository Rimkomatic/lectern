package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func get(h http.Handler, method, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

func TestSPAHandler(t *testing.T) {
	h := handler(fstest.MapFS{
		"index.html":        {Data: []byte("<!doctype html><title>app</title>")},
		"assets/app-abc.js": {Data: []byte("console.log(1)")},
		"favicon.svg":       {Data: []byte("<svg/>")},
	})

	rec := get(h, "GET", "/")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "<title>app</title>") {
		t.Fatalf("/: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Content-Security-Policy"), "default-src 'self'") || rec.Header().Get("Cache-Control") != "no-cache" {
		t.Errorf("index.html headers: %v", rec.Header())
	}

	rec = get(h, "GET", "/assets/app-abc.js")
	if rec.Code != 200 || !strings.Contains(rec.Header().Get("Cache-Control"), "immutable") {
		t.Errorf("asset: %d %v", rec.Code, rec.Header())
	}
	if rec = get(h, "GET", "/some/client/route"); rec.Code != 200 || !strings.Contains(rec.Body.String(), "<title>app</title>") {
		t.Errorf("client route should fall back to index.html: %d", rec.Code)
	}
	if rec = get(h, "GET", "/assets/missing.js"); rec.Code != 404 {
		t.Errorf("missing asset must be 404, not HTML: %d", rec.Code)
	}
	if rec = get(h, "GET", "/assets"); rec.Code != 200 { // a directory name is a client route
		t.Errorf("directory: %d", rec.Code)
	}
	if rec = get(h, "GET", "/../../etc/passwd"); rec.Code == 200 && strings.Contains(rec.Body.String(), "root:") {
		t.Error("path traversal")
	}
	if rec = get(h, "POST", "/"); rec.Code != 405 {
		t.Errorf("POST: %d", rec.Code)
	}
	if rec = get(h, "HEAD", "/favicon.svg"); rec.Code != 200 {
		t.Errorf("HEAD: %d", rec.Code)
	}
}

func TestPlaceholderWithoutUI(t *testing.T) {
	h := handler(fstest.MapFS{".gitkeep": {}})
	rec := get(h, "GET", "/")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Lectern is running") {
		t.Errorf("placeholder: %d %s", rec.Code, rec.Body.String())
	}
}

// The small container image has no /etc/mime.types, so the manifest's type has
// to come from the program itself.
func TestManifestType(t *testing.T) {
	h := handler(fstest.MapFS{
		"index.html":           {Data: []byte("<!doctype html>")},
		"manifest.webmanifest": {Data: []byte(`{"name":"Lectern"}`)},
		"robots.txt":           {Data: []byte("User-agent: *\nDisallow: /\n")},
		"icons/icon-192.png":   {Data: []byte("\x89PNG\r\n\x1a\n")},
	})
	rec := get(h, "GET", "/manifest.webmanifest")
	if ct := rec.Header().Get("Content-Type"); rec.Code != 200 || !strings.HasPrefix(ct, "application/manifest+json") {
		t.Errorf("manifest: %d %q", rec.Code, ct)
	}
	if rec = get(h, "GET", "/robots.txt"); rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain") {
		t.Errorf("robots.txt: %d %v", rec.Code, rec.Header())
	}
	if rec = get(h, "GET", "/icons/icon-192.png"); rec.Code != 200 || rec.Header().Get("Content-Type") != "image/png" {
		t.Errorf("icon: %d %v", rec.Code, rec.Header())
	}
}
