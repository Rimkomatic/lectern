// Package webui embeds the built single-page app (web/ -> internal/webui/dist)
// into the Go binary and serves it.
package webui

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// csp is applied to the HTML shell. Scripts and styles come only from this
// origin; 'wasm-unsafe-eval' is for pdf.js, blob: workers/media for the player.
const csp = "default-src 'self'; script-src 'self' 'wasm-unsafe-eval'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: blob:; media-src 'self' blob:; font-src 'self' data:; connect-src 'self'; " +
	"worker-src 'self' blob:; frame-src 'self'; object-src 'none'; base-uri 'none'; form-action 'self'; " +
	"frame-ancestors 'self'"

// Handler serves the embedded app.
func Handler() http.Handler {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return handler(emptyFS{})
	}
	return handler(sub)
}

type emptyFS struct{}

func (emptyFS) Open(string) (fs.File, error) { return nil, fs.ErrNotExist }

const placeholder = `<!doctype html><meta charset="utf-8"><title>Lectern</title>
<body style="font-family:sans-serif;max-width:40em;margin:3em auto">
<h1>Lectern is running</h1>
<p>The API is up, but this binary was built without the web interface.</p>
<p>Build it with <code>make web</code> (or <code>cd web &amp;&amp; npm ci &amp;&amp; npm run build</code>) and rebuild the server.</p>
</body>`

func handler(fsys fs.FS) http.Handler {
	_, err := fs.Stat(fsys, "index.html")
	hasUI := err == nil
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !hasUI {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Content-Security-Policy", csp)
			_, _ = w.Write([]byte(placeholder))
			return
		}
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name == "" {
			name = "index.html"
		}
		if fi, err := fs.Stat(fsys, name); err != nil || fi.IsDir() {
			if path.Ext(name) != "" {
				http.NotFound(w, r) // a missing asset must not turn into the HTML page
				return
			}
			name = "index.html" // client-side route
		}
		h := w.Header()
		if strings.HasPrefix(name, "assets/") {
			h.Set("Cache-Control", "public, max-age=31536000, immutable") // content-hashed file names
		} else {
			h.Set("Cache-Control", "no-cache")
		}
		if name == "index.html" {
			h.Set("Content-Security-Policy", csp)
		}
		http.ServeFileFS(w, r, fsys, name)
	})
}
