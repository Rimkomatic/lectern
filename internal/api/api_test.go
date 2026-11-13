package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"lectern/internal/auth"
	"lectern/internal/config"
	"lectern/internal/scan"
	"lectern/internal/store"
	"lectern/internal/testdb"
)

// ---- test environment ----

type env struct {
	t      *testing.T
	ts     *httptest.Server
	st     *store.Store
	srv    *Server
	runner *scan.Runner
	root   string
	app    *scan.Applier
	lib    store.Library
}

func put(t *testing.T, root, rel string, data []byte) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func fakeVideo(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte('a' + i%26)
	}
	return b
}

func newEnv(t *testing.T) *env {
	t.Helper()
	old := auth.Iterations
	auth.Iterations = 1000
	t.Cleanup(func() { auth.Iterations = old })

	conn, _ := testdb.Open(t)
	st := store.New(conn)
	root := t.TempDir()

	put(t, root, "Go Basics/README.md", []byte("# Hello\n"))
	put(t, root, "Go Basics/01 - Intro/01 - Welcome.mp4", fakeVideo(200_000))
	put(t, root, "Go Basics/01 - Intro/01 - Welcome.en.srt", []byte("1\n00:00:01,500 --> 00:00:03,000\nHi there\n"))
	put(t, root, "Go Basics/01 - Intro/02 - Docs.url", []byte("[InternetShortcut]\nURL=https://go.dev/doc/\n"))
	put(t, root, "Go Basics/01 - Intro/03 - Slides.pdf", []byte("%PDF-1.4 fake"))
	put(t, root, "Go Basics/01 - Intro/04 - Diagram.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"><script>alert(2)</script></svg>`))
	put(t, root, "Go Basics/01 - Intro/05 - Page.html", []byte("<script>alert(3)</script>"))
	put(t, root, "Go Basics/01 - Intro/06 - Données.csv", []byte("a,b\n1,2\n"))
	put(t, root, "Go Basics/01 - Intro/07 - Handout.docx", []byte("PK\x03\x04 not really"))
	put(t, root, "Go Basics/02 - Next/01 - Lesson.mp4", fakeVideo(50_000))
	put(t, root, "Go Basics/02 - Next/02 - Lesson 2.mp3", fakeVideo(30_000))
	put(t, root, "Podcasts/ep1.mp3", fakeVideo(10_000))

	if err := st.SyncLibraries(context.Background(), []store.LibrarySpec{{Label: "Main", Path: root}}); err != nil {
		t.Fatal(err)
	}
	libs, _ := st.ListLibraries(context.Background(), true)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	app := &scan.Applier{Store: st, Log: log}
	if _, err := app.ScanLibrary(context.Background(), libs[0], nil); err != nil {
		t.Fatal(err)
	}
	runner := scan.NewRunner(app, scan.Schedule{}, 0, log)

	cfg := &config.Config{
		SessionTTL: 24 * time.Hour, CookieSecure: "auto", DoneThreshold: 0.95,
		Libraries: []config.Library{{Label: "Main", Path: root}},
	}
	srv := New(cfg, st, runner, nil, "test", log)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(func() { srv.Close(); ts.Close() })
	return &env{t: t, ts: ts, st: st, srv: srv, runner: runner, root: root, app: app, lib: libs[0]}
}

type client struct {
	e  *env
	hc *http.Client
}

func (e *env) client() *client {
	jar, _ := cookiejar.New(nil)
	return &client{e: e, hc: &http.Client{Jar: jar, Timeout: 20 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

type resp struct {
	*http.Response
	body []byte
}

// req sends a request. body may be nil, a string (sent as-is) or any value (JSON).
func (c *client) req(method, path string, body any, hdr ...string) resp {
	c.e.t.Helper()
	var rd io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		rd = strings.NewReader(b)
	default:
		j, err := json.Marshal(b)
		if err != nil {
			c.e.t.Fatal(err)
		}
		rd = bytes.NewReader(j)
	}
	r, err := http.NewRequest(method, c.e.ts.URL+path, rd)
	if err != nil {
		c.e.t.Fatal(err)
	}
	if body != nil {
		if _, isStr := body.(string); !isStr {
			r.Header.Set("Content-Type", "application/json")
		}
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		r.Header.Set(hdr[i], hdr[i+1])
	}
	res, err := c.hc.Do(r)
	if err != nil {
		c.e.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return resp{res, b}
}

// js sends a request, decodes a JSON response into out (if non-nil) and returns the status.
func (c *client) js(method, path string, body any, out any, hdr ...string) int {
	c.e.t.Helper()
	r := c.req(method, path, body, hdr...)
	if out != nil && len(r.body) > 0 && r.StatusCode < 400 {
		if err := json.Unmarshal(r.body, out); err != nil {
			c.e.t.Fatalf("%s %s: bad JSON %q: %v", method, path, r.body, err)
		}
	}
	return r.StatusCode
}

func errCode(t *testing.T, r resp) string {
	t.Helper()
	var b errorBody
	if err := json.Unmarshal(r.body, &b); err != nil {
		t.Fatalf("not an error body (%d): %q", r.StatusCode, r.body)
	}
	return b.Error.Code
}

func expectStatus(t *testing.T, r resp, want int, wantCode string) {
	t.Helper()
	if r.StatusCode != want {
		t.Fatalf("status %d, want %d (body %q)", r.StatusCode, want, r.body)
	}
	if wantCode != "" {
		if got := errCode(t, r); got != wantCode {
			t.Fatalf("error code %q, want %q", got, wantCode)
		}
	}
}

func (c *client) login(user, pass string) int {
	return c.js("POST", "/api/v1/auth/login", map[string]string{"username": user, "password": pass}, nil)
}

func findCourse(t *testing.T, c *client, title string) store.CourseSummary {
	t.Helper()
	var list []store.CourseSummary
	if s := c.js("GET", "/api/v1/courses", nil, &list); s != 200 {
		t.Fatalf("list courses: %d", s)
	}
	for _, x := range list {
		if x.Title == title {
			return x
		}
	}
	t.Fatalf("course %q not in %+v", title, list)
	return store.CourseSummary{}
}

func tree(t *testing.T, c *client, id uint64) store.CourseTree {
	t.Helper()
	var tr store.CourseTree
	if s := c.js("GET", "/api/v1/courses/"+strconv.FormatUint(id, 10), nil, &tr); s != 200 {
		t.Fatalf("course tree: %d", s)
	}
	return tr
}

func item(t *testing.T, tr store.CourseTree, title string) store.Item {
	t.Helper()
	for _, s := range tr.Sections {
		for _, it := range s.Items {
			if it.Title == title {
				return it
			}
		}
	}
	t.Fatalf("item %q not found", title)
	return store.Item{}
}

func itemPath(it store.Item, suffix string) string {
	return "/api/v1/items/" + strconv.FormatUint(it.ID, 10) + suffix
}

// ---- the tests ----

func TestFullFlow(t *testing.T) {
	e := newEnv(t)
	ann := e.client()

	t.Run("health and setup", func(t *testing.T) {
		r := ann.req("GET", "/healthz", nil)
		expectStatus(t, r, 200, "")
		var s struct {
			NeedsSetup bool `json:"needs_setup"`
		}
		ann.js("GET", "/api/v1/setup", nil, &s)
		if !s.NeedsSetup {
			t.Fatal("fresh database must need setup")
		}
		expectStatus(t, ann.req("GET", "/api/v1/courses", nil), 401, "unauthorized")
		expectStatus(t, ann.req("GET", "/api/v1/admin/users", nil), 401, "unauthorized")
		expectStatus(t, ann.req("GET", "/api/v1/nope", nil), 404, "not_found")
	})

	t.Run("create the first admin", func(t *testing.T) {
		expectStatus(t, ann.req("POST", "/api/v1/setup", map[string]string{"username": "ann", "password": "short"}), 400, "bad_request")
		expectStatus(t, ann.req("POST", "/api/v1/setup", map[string]string{"username": "a b", "password": "long enough pw"}), 400, "bad_request")
		expectStatus(t, ann.req("POST", "/api/v1/setup", `{"username":"ann","password":"long enough pw","extra":1}`), 400, "bad_request")

		r := ann.req("POST", "/api/v1/setup", map[string]string{"username": "ann", "password": "long enough pw", "display_name": "Ann Lee"})
		expectStatus(t, r, 201, "")
		var cookie *http.Cookie
		for _, c := range r.Cookies() {
			if c.Name == cookieName {
				cookie = c
			}
		}
		if cookie == nil || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" || cookie.Value == "" {
			t.Fatalf("session cookie not set safely: %+v", cookie)
		}
		var me meResponse
		ann.js("GET", "/api/v1/me", nil, &me)
		if me.User.Username != "ann" || me.User.Role != "admin" || me.User.DisplayName != "Ann Lee" || me.Settings.ResumeMode != "ask" {
			t.Fatalf("me = %+v", me)
		}
		if bytes.Contains(r.body, []byte("pw_hash")) || bytes.Contains(r.body, []byte("pbkdf2")) {
			t.Error("password hash leaked in JSON")
		}
		expectStatus(t, e.client().req("POST", "/api/v1/setup", map[string]string{"username": "eve", "password": "long enough pw"}), 409, "already_set_up")

		// the raw token is never stored
		var stored int
		e.st.DB().QueryRow("SELECT COUNT(*) FROM sessions WHERE HEX(id) = UPPER(?)", cookie.Value).Scan(&stored)
		if stored != 0 {
			t.Error("session token stored in clear")
		}
	})

	var goBasics store.CourseSummary
	var tr store.CourseTree
	t.Run("catalog", func(t *testing.T) {
		var list []store.CourseSummary
		ann.js("GET", "/api/v1/courses", nil, &list)
		if len(list) != 2 {
			t.Fatalf("courses = %+v", list)
		}
		ann.js("GET", "/api/v1/courses?q=podcast", nil, &list)
		if len(list) != 1 || list[0].Title != "Podcasts" {
			t.Fatalf("search = %+v", list)
		}
		ann.js("GET", "/api/v1/courses?q=%25", nil, &list) // LIKE wildcards are escaped
		if len(list) != 0 {
			t.Fatalf("wildcard search matched %d courses", len(list))
		}
		goBasics = findCourse(t, ann, "Go Basics")
		tr = tree(t, ann, goBasics.ID)
		if len(tr.Sections) != 3 || tr.Resume != nil || tr.Completed {
			t.Fatalf("tree: %d sections, resume %+v", len(tr.Sections), tr.Resume)
		}
		if goBasics.ItemCount != 10 || goBasics.DoneCount != 0 {
			t.Fatalf("summary: %+v", goBasics)
		}
		expectStatus(t, ann.req("GET", "/api/v1/courses/999999", nil), 404, "not_found")
		expectStatus(t, ann.req("GET", "/api/v1/courses/abc", nil), 404, "not_found")
		var d store.ItemDetail
		ann.js("GET", itemPath(item(t, tr, "Welcome"), ""), nil, &d)
		if d.CourseTitle != "Go Basics" || d.SectionTitle != "Intro" || len(d.Item.Subtitles) != 1 {
			t.Fatalf("item detail: %+v", d)
		}
	})

	welcome := item(t, tr, "Welcome")
	t.Run("streaming with ranges", func(t *testing.T) {
		want := fakeVideo(200_000)
		r := ann.req("GET", itemPath(welcome, "/stream"), nil)
		expectStatus(t, r, 200, "")
		if r.Header.Get("Accept-Ranges") != "bytes" || r.Header.Get("Content-Type") != "video/mp4" || !bytes.Equal(r.body, want) {
			t.Fatalf("full stream wrong: %v", r.Header)
		}
		if r.Header.Get("X-Content-Type-Options") != "nosniff" {
			t.Error("nosniff missing")
		}
		r = ann.req("GET", itemPath(welcome, "/stream"), nil, "Range", "bytes=10-19")
		expectStatus(t, r, 206, "")
		if r.Header.Get("Content-Range") != "bytes 10-19/200000" || !bytes.Equal(r.body, want[10:20]) {
			t.Fatalf("range wrong: %q %q", r.Header.Get("Content-Range"), r.body)
		}
		r = ann.req("GET", itemPath(welcome, "/stream"), nil, "Range", "bytes=199990-")
		if r.StatusCode != 206 || !bytes.Equal(r.body, want[199990:]) {
			t.Fatalf("open-ended range: %d", r.StatusCode)
		}
		r = ann.req("GET", itemPath(welcome, "/stream"), nil, "Range", "bytes=999999999-")
		if r.StatusCode != 416 {
			t.Fatalf("unsatisfiable range: %d", r.StatusCode)
		}
		etag := ann.req("HEAD", itemPath(welcome, "/stream"), nil).Header.Get("ETag")
		if etag == "" {
			t.Fatal("no ETag")
		}
		if r = ann.req("GET", itemPath(welcome, "/stream"), nil, "If-None-Match", etag); r.StatusCode != 304 {
			t.Fatalf("conditional request: %d", r.StatusCode)
		}
		// media needs a session
		expectStatus(t, e.client().req("GET", itemPath(welcome, "/stream"), nil), 401, "unauthorized")
	})

	t.Run("safe serving of untrusted files", func(t *testing.T) {
		svg := ann.req("GET", itemPath(item(t, tr, "Diagram"), "/stream"), nil)
		if svg.Header.Get("Content-Type") != "image/svg+xml" || !strings.Contains(svg.Header.Get("Content-Security-Policy"), "sandbox") {
			t.Errorf("svg must be sandboxed: %v", svg.Header)
		}
		html := ann.req("GET", itemPath(item(t, tr, "Page"), "/stream"), nil)
		if !strings.HasPrefix(html.Header.Get("Content-Type"), "text/plain") || html.Header.Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("html must be shown as plain text: %v", html.Header)
		}
		md := ann.req("GET", itemPath(item(t, tr, "README"), "/stream"), nil)
		if !strings.HasPrefix(md.Header.Get("Content-Type"), "text/plain") || string(md.body) != "# Hello\n" {
			t.Errorf("markdown: %v %q", md.Header, md.body)
		}
		docx := ann.req("GET", itemPath(item(t, tr, "Handout"), "/stream"), nil)
		if !strings.HasPrefix(docx.Header.Get("Content-Disposition"), "attachment") {
			t.Errorf("office files must download, got %q", docx.Header.Get("Content-Disposition"))
		}
		pdf := ann.req("GET", itemPath(item(t, tr, "Slides"), "/stream"), nil)
		if pdf.Header.Get("Content-Type") != "application/pdf" || !strings.HasPrefix(pdf.Header.Get("Content-Disposition"), "inline") {
			t.Errorf("pdf: %v", pdf.Header)
		}
		dl := ann.req("GET", itemPath(item(t, tr, "Données"), "/download"), nil)
		cd := dl.Header.Get("Content-Disposition")
		if !strings.HasPrefix(cd, "attachment") || !strings.Contains(cd, "filename*=") || !strings.Contains(cd, "Donn%C3%A9es") {
			t.Errorf("download disposition = %q", cd)
		}
		// a link has nothing to stream
		expectStatus(t, ann.req("GET", itemPath(item(t, tr, "Docs"), "/stream"), nil), 400, "bad_request")
		if item(t, tr, "Docs").LinkURL != "https://go.dev/doc/" {
			t.Error("link url missing from tree")
		}
	})

	t.Run("pictures beside a page", func(t *testing.T) {
		png := []byte("\x89PNG\r\n\x1a\nnot really a picture")
		put(t, e.root, "Go Basics/01 - Intro/images/pic.png", png)
		put(t, e.root, "Go Basics/01 - Intro/images/My Pic.JPG", png)
		put(t, e.root, "Go Basics/01 - Intro/images/vector.svg", []byte("<svg/>"))
		put(t, e.root, "Go Basics/01 - Intro/images/notes.txt", []byte("text"))
		put(t, e.root, "Go Basics/top.png", png)
		put(t, e.root, "Podcasts/other.png", png)
		// the library is scanned again further down: leave it as it was
		t.Cleanup(func() {
			os.RemoveAll(filepath.Join(e.root, "Go Basics", "01 - Intro", "images"))
			os.Remove(filepath.Join(e.root, "Go Basics", "top.png"))
			os.Remove(filepath.Join(e.root, "Podcasts", "other.png"))
		})
		page := item(t, tr, "Page")
		asset := func(ref string) string { return itemPath(page, "/asset?path="+url.QueryEscape(ref)) }

		for _, ref := range []string{
			"images/pic.png", "./images/pic.png", `images\pic.png`, "images/pic.png?v=2#top",
			"images/My Pic.JPG", "images/../images/pic.png",
			"../top.png", // the course folder is as far up as it goes
			"/top.png",   // from the course folder
		} {
			r := ann.req("GET", asset(ref), nil)
			if r.StatusCode != 200 || !bytes.Equal(r.body, png) {
				t.Errorf("%q: %d %q", ref, r.StatusCode, r.body)
				continue
			}
			if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "image/") || r.Header.Get("X-Content-Type-Options") != "nosniff" {
				t.Errorf("%q: headers %v", ref, r.Header)
			}
		}
		for _, ref := range []string{
			"", "images/missing.png", "images/vector.svg", "images/notes.txt", "images", "05 - Page.html",
			"../../Podcasts/other.png", "../../../etc/passwd.png", "/../../Podcasts/other.png", "images/../../../Podcasts/other.png",
			"https://example.com/pic.png", "//example.com/pic.png", "images/pic.png\x00.txt", "data:image/png;base64,AAAA",
		} {
			if r := ann.req("GET", asset(ref), nil); r.StatusCode != 404 || bytes.Contains(r.body, png) {
				t.Errorf("%q should be refused: %d", ref, r.StatusCode)
			}
		}
		expectStatus(t, e.client().req("GET", asset("images/pic.png"), nil), 401, "unauthorized")
		// a link has no folder to look in
		if r := ann.req("GET", itemPath(item(t, tr, "Docs"), "/asset?path=pic.png"), nil); r.StatusCode != 404 {
			t.Errorf("link item: %d", r.StatusCode)
		}
	})

	t.Run("path escapes are refused", func(t *testing.T) {
		secret := filepath.Join(filepath.Dir(e.root), "secret.txt")
		os.WriteFile(secret, []byte("TOP SECRET"), 0o600)
		txt := item(t, tr, "Page")
		for _, evil := range []string{"../../secret.txt", "../" + filepath.Base(e.root) + "/../secret.txt", "/etc/passwd"} {
			if _, err := e.st.DB().Exec("UPDATE items SET rel_path = ? WHERE id = ?", evil, txt.ID); err != nil {
				t.Fatal(err)
			}
			r := ann.req("GET", itemPath(txt, "/stream"), nil)
			if r.StatusCode == 200 || bytes.Contains(r.body, []byte("TOP SECRET")) {
				t.Fatalf("path %q escaped the library: %d %q", evil, r.StatusCode, r.body)
			}
		}
		// a symlink swapped in after the scan is refused as well
		link := filepath.Join(e.root, "Go Basics", "01 - Intro", "05 - Page.html")
		os.Remove(link)
		if err := os.Symlink(secret, link); err == nil {
			e.st.DB().Exec("UPDATE items SET rel_path = ? WHERE id = ?", "01 - Intro/05 - Page.html", txt.ID)
			r := ann.req("GET", itemPath(txt, "/stream"), nil)
			if r.StatusCode == 200 || bytes.Contains(r.body, []byte("TOP SECRET")) {
				t.Fatalf("symlink escape served: %d", r.StatusCode)
			}
		}
	})

	t.Run("subtitles", func(t *testing.T) {
		var d store.ItemDetail
		ann.js("GET", itemPath(welcome, ""), nil, &d)
		r := ann.req("GET", "/api/v1/subtitles/"+strconv.FormatUint(d.Item.Subtitles[0].ID, 10), nil)
		expectStatus(t, r, 200, "")
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "text/vtt") ||
			!strings.HasPrefix(string(r.body), "WEBVTT\n\n") || !strings.Contains(string(r.body), "00:00:01.500 --> 00:00:03.000") {
			t.Fatalf("vtt = %q (%v)", r.body, r.Header)
		}
	})

	t.Run("progress, resume and completion", func(t *testing.T) {
		post := func(it store.Item, body any) (int, store.Progress) {
			var out struct {
				Progress store.Progress `json:"progress"`
			}
			s := ann.js("POST", itemPath(it, "/progress"), body, &out)
			return s, out.Progress
		}
		now := time.Now().UnixMilli()
		s, p := post(welcome, map[string]any{"event": "heartbeat", "position": 30.5, "duration": 100, "client_ts": now})
		if s != 200 || p.State != "started" || p.Position != 30.5 || p.Duration == nil || *p.Duration != 100 {
			t.Fatalf("heartbeat: %d %+v", s, p)
		}
		// a late, older report must not rewind
		if s, p = post(welcome, map[string]any{"event": "heartbeat", "position": 5, "duration": 100, "client_ts": now - 60_000}); s != 200 || p.Position != 30.5 {
			t.Fatalf("stale report applied: %d %+v", s, p)
		}
		// sendBeacon sends text/plain
		if s = ann.js("POST", itemPath(welcome, "/progress"), `{"event":"pause","position":40,"duration":100}`, nil, "Content-Type", "text/plain;charset=UTF-8"); s != 200 {
			t.Fatalf("beacon style post: %d", s)
		}
		for _, bad := range []any{
			map[string]any{"event": "explode"},
			map[string]any{"position": -1},
			map[string]any{"position": 1e9},
			map[string]any{"duration": 0},
			map[string]any{"page": 0},
			`{"position":"abc"}`,
			`not json`,
		} {
			if s, _ := post(welcome, bad); s != 400 {
				t.Errorf("bad progress %v accepted: %d", bad, s)
			}
		}
		expectStatus(t, ann.req("POST", "/api/v1/items/999999/progress", map[string]any{"position": 1}), 404, "not_found")

		tr = tree(t, ann, goBasics.ID)
		if tr.Resume == nil || tr.Resume.Reason != "continue" || tr.Resume.ItemID != welcome.ID || tr.Resume.Position != 40 {
			t.Fatalf("resume = %+v", tr.Resume)
		}
		// finishing an item moves the resume point to the next unfinished one
		if s, p = post(welcome, map[string]any{"event": "end", "position": 100, "duration": 100}); s != 200 || p.State != "done" {
			t.Fatalf("end: %d %+v", s, p)
		}
		tr = tree(t, ann, goBasics.ID)
		if tr.Resume == nil || tr.Resume.Reason != "next" || tr.Resume.ItemID == welcome.ID {
			t.Fatalf("resume after finishing = %+v", tr.Resume)
		}
		// done is sticky: later playback from the start does not undo it
		if s, p = post(welcome, map[string]any{"event": "heartbeat", "position": 1, "duration": 100}); p.State != "done" {
			t.Fatalf("done reverted: %+v", p)
		}
		// pdf completes on the last page; manual toggle works both ways
		slides := item(t, tr, "Slides")
		if s, p = post(slides, map[string]any{"event": "page", "page": 2, "pages": 3}); p.State != "started" || p.Page == nil || *p.Page != 2 {
			t.Fatalf("pdf page 2: %+v", p)
		}
		if s, p = post(slides, map[string]any{"event": "page", "page": 3, "pages": 3}); p.State != "done" {
			t.Fatalf("pdf last page: %+v", p)
		}
		if s, p = post(slides, map[string]any{"event": "done", "done": false}); p.State == "done" {
			t.Fatalf("manual un-done: %+v", p)
		}
		// opening a text-like file counts as done by default
		if s, p = post(item(t, tr, "README"), map[string]any{"event": "open"}); p.State != "done" {
			t.Fatalf("open readme: %+v", p)
		}
		var cont []store.ContinueEntry
		ann.js("GET", "/api/v1/continue", nil, &cont)
		if len(cont) != 1 || cont[0].Course.Title != "Go Basics" || cont[0].Item == nil {
			t.Fatalf("continue = %+v", cont)
		}
		if got := findCourse(t, ann, "Go Basics"); got.DoneCount != 2 {
			t.Fatalf("done_count = %d", got.DoneCount)
		}
	})

	var bob *client
	t.Run("admin manages users, progress stays private", func(t *testing.T) {
		expectStatus(t, ann.req("POST", "/api/v1/admin/users", map[string]string{"username": "bob", "password": "bobs password", "role": "wizard"}), 400, "bad_request")
		expectStatus(t, ann.req("POST", "/api/v1/admin/users", map[string]string{"username": "bob", "password": "x"}), 400, "bad_request")
		var u store.User
		if s := ann.js("POST", "/api/v1/admin/users", map[string]string{"username": "bob", "password": "bobs password", "display_name": "Bob"}, &u); s != 201 || u.Role != "user" {
			t.Fatalf("create bob: %d %+v", s, u)
		}
		expectStatus(t, ann.req("POST", "/api/v1/admin/users", map[string]string{"username": "BOB", "password": "bobs password"}), 409, "username_taken")

		bob = e.client()
		if s := bob.login("bob", "bobs password"); s != 200 {
			t.Fatalf("bob login: %d", s)
		}
		// bob sees the same courses but none of ann's progress
		bt := tree(t, bob, goBasics.ID)
		if bt.Resume != nil || item(t, bt, "Welcome").Progress != nil {
			t.Fatalf("bob sees ann's progress: %+v", bt.Resume)
		}
		var cont []store.ContinueEntry
		bob.js("GET", "/api/v1/continue", nil, &cont)
		if len(cont) != 0 {
			t.Fatalf("bob's continue list: %+v", cont)
		}
		if s := bob.js("POST", itemPath(welcome, "/progress"), map[string]any{"event": "heartbeat", "position": 7, "duration": 100}, nil); s != 200 {
			t.Fatal(s)
		}
		at := tree(t, ann, goBasics.ID)
		if p := item(t, at, "Welcome").Progress; p == nil || p.State != "done" || p.Position == 7 {
			t.Fatalf("bob's progress leaked into ann's: %+v", p)
		}
		// bob is not an admin
		expectStatus(t, bob.req("GET", "/api/v1/admin/users", nil), 403, "forbidden")
		expectStatus(t, bob.req("POST", "/api/v1/admin/scan", nil), 403, "forbidden")
		expectStatus(t, bob.req("GET", "/api/v1/admin/events", nil), 403, "forbidden")
	})

	t.Run("csrf protection", func(t *testing.T) {
		body := map[string]string{"display_name": "Hacked"}
		expectStatus(t, ann.req("PATCH", "/api/v1/me", body, "Origin", "http://evil.example"), 403, "cross_site")
		expectStatus(t, ann.req("PATCH", "/api/v1/me", body, "Origin", "null"), 403, "cross_site")
		expectStatus(t, ann.req("PATCH", "/api/v1/me", body, "Sec-Fetch-Site", "cross-site"), 403, "cross_site")
		expectStatus(t, ann.req("PATCH", "/api/v1/me", body, "Sec-Fetch-Site", "same-site"), 403, "cross_site")
		expectStatus(t, ann.req("PATCH", "/api/v1/me", body, "Origin", e.ts.URL), 200, "")
		expectStatus(t, ann.req("PATCH", "/api/v1/me", body, "Sec-Fetch-Site", "same-origin"), 200, "")
		expectStatus(t, ann.req("PATCH", "/api/v1/me", body), 200, "") // curl and friends
		expectStatus(t, e.client().req("POST", "/api/v1/auth/login", map[string]string{"username": "x", "password": "y"}, "Origin", "http://evil.example"), 403, "cross_site")
		// GET never changes state, so cross-site GETs are not blocked here
		expectStatus(t, ann.req("GET", "/api/v1/me", nil, "Origin", "http://evil.example"), 200, "")
		// explicitly allowed origins
		e.srv.Cfg.AllowedOrigins = []string{"http://localhost:5173"}
		expectStatus(t, ann.req("PATCH", "/api/v1/me", body, "Origin", "http://localhost:5173"), 200, "")
		e.srv.Cfg.AllowedOrigins = nil
		var me meResponse
		ann.js("PATCH", "/api/v1/me", map[string]string{"display_name": "Ann Lee"}, nil)
		ann.js("GET", "/api/v1/me", nil, &me)
		if me.User.DisplayName != "Ann Lee" {
			t.Fatal("display name not restored")
		}
	})

	t.Run("settings", func(t *testing.T) {
		var st store.Settings
		ann.js("GET", "/api/v1/me/settings", nil, &st)
		if st.ResumeMode != "ask" || !st.AutoplayNext || st.PlaybackRate != 1 {
			t.Fatalf("defaults: %+v", st)
		}
		expectStatus(t, ann.req("PUT", "/api/v1/me/settings", map[string]any{"resume_mode": "sometimes"}), 400, "bad_request")
		expectStatus(t, ann.req("PUT", "/api/v1/me/settings", map[string]any{"playback_rate": 9}), 400, "bad_request")
		if s := ann.js("PUT", "/api/v1/me/settings", map[string]any{"resume_mode": "auto", "playback_rate": 1.5}, &st); s != 200 || st.ResumeMode != "auto" || st.PlaybackRate != 1.5 || !st.AutoplayNext {
			t.Fatalf("update: %d %+v", s, st)
		}
		ann.js("GET", "/api/v1/me/settings", nil, &st)
		if st.ResumeMode != "auto" {
			t.Fatal("settings not persisted")
		}
		// settings are per user
		var bs store.Settings
		bob.js("GET", "/api/v1/me/settings", nil, &bs)
		if bs.ResumeMode != "ask" {
			t.Fatalf("bob inherited ann's settings: %+v", bs)
		}
		ann.js("PUT", "/api/v1/me/settings", map[string]any{"resume_mode": "ask", "playback_rate": 1}, nil)
	})

	t.Run("admin scan, events and libraries", func(t *testing.T) {
		var libs []store.Library
		ann.js("GET", "/api/v1/admin/libraries", nil, &libs)
		if len(libs) != 1 || libs[0].Label != "Main" || libs[0].CourseCount != 2 {
			t.Fatalf("libraries = %+v", libs)
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go e.runner.Run(ctx)

		// live events
		req, _ := http.NewRequestWithContext(ctx, "GET", e.ts.URL+"/api/v1/admin/events", nil)
		for _, c := range ann.hc.Jar.Cookies(req.URL) {
			req.AddCookie(c)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		if res.StatusCode != 200 || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") {
			t.Fatalf("events: %d %v", res.StatusCode, res.Header)
		}
		lines := make(chan string, 64)
		go func() {
			sc := bufio.NewScanner(res.Body)
			for sc.Scan() {
				lines <- sc.Text()
			}
			close(lines)
		}()
		waitFor := func(substr string) string {
			deadline := time.After(10 * time.Second)
			for {
				select {
				case l, ok := <-lines:
					if !ok {
						t.Fatalf("event stream closed while waiting for %q", substr)
					}
					if strings.Contains(l, substr) {
						return l
					}
				case <-deadline:
					t.Fatalf("timed out waiting for %q", substr)
				}
			}
		}
		waitFor("event: scan")

		// let the startup scan finish, then trigger another
		deadline := time.Now().Add(10 * time.Second)
		for e.runner.Status().Running || e.runner.Status().FinishedAt == nil {
			if time.Now().After(deadline) {
				t.Fatal("startup scan did not finish")
			}
			time.Sleep(20 * time.Millisecond)
		}
		put(t, e.root, "Podcasts/ep2.mp3", fakeVideo(11_000))
		if s := ann.js("POST", "/api/v1/admin/scan", nil, nil); s != 202 {
			t.Fatalf("start scan: %d", s)
		}
		waitFor(`"running":false`)
		var status scan.Status
		deadline = time.Now().Add(10 * time.Second)
		for {
			ann.js("GET", "/api/v1/admin/scan", nil, &status)
			if !status.Running && status.Last != nil && status.Last.ItemsAdded == 1 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("scan result never showed the new file: %+v", status)
			}
			time.Sleep(20 * time.Millisecond)
		}
		if pod := findCourse(t, ann, "Podcasts"); pod.ItemCount != 2 {
			t.Fatalf("new file not visible: %+v", pod)
		}
		cancel()
	})

	t.Run("password change signs out other devices", func(t *testing.T) {
		bob2 := e.client()
		if s := bob2.login("bob", "bobs password"); s != 200 {
			t.Fatal(s)
		}
		expectStatus(t, bob.req("PUT", "/api/v1/me/password", map[string]string{"current_password": "nope", "new_password": "a brand new one"}), 403, "wrong_password")
		expectStatus(t, bob.req("PUT", "/api/v1/me/password", map[string]string{"current_password": "bobs password", "new_password": "tiny"}), 400, "bad_request")
		expectStatus(t, bob.req("PUT", "/api/v1/me/password", map[string]string{"current_password": "bobs password", "new_password": "a brand new one"}), 204, "")
		expectStatus(t, bob.req("GET", "/api/v1/me", nil), 200, "")
		expectStatus(t, bob2.req("GET", "/api/v1/me", nil), 401, "unauthorized")
		if s := e.client().login("bob", "bobs password"); s != 401 {
			t.Fatalf("old password still works: %d", s)
		}
		if s := e.client().login("bob", "a brand new one"); s != 200 {
			t.Fatalf("new password rejected: %d", s)
		}
	})

	t.Run("admin edits and removes users", func(t *testing.T) {
		var users []store.User
		ann.js("GET", "/api/v1/admin/users", nil, &users)
		var bobID, annID uint64
		for _, u := range users {
			switch u.Username {
			case "bob":
				bobID = u.ID
			case "ann":
				annID = u.ID
			}
		}
		if len(users) != 2 || bobID == 0 {
			t.Fatalf("users = %+v", users)
		}
		bp := "/api/v1/admin/users/" + strconv.FormatUint(bobID, 10)
		ap := "/api/v1/admin/users/" + strconv.FormatUint(annID, 10)

		expectStatus(t, ann.req("PATCH", ap, map[string]any{"disabled": true}), 400, "bad_request")
		expectStatus(t, ann.req("PATCH", ap, map[string]any{"role": "user"}), 400, "bad_request")
		expectStatus(t, ann.req("DELETE", ap, nil), 400, "bad_request")
		expectStatus(t, ann.req("PATCH", bp, map[string]any{}), 400, "bad_request")
		expectStatus(t, ann.req("PATCH", "/api/v1/admin/users/99999", map[string]any{"role": "user"}), 404, "not_found")

		var u store.User
		if s := ann.js("PATCH", bp, map[string]any{"role": "admin", "display_name": "Robert"}, &u); s != 200 || u.Role != "admin" || u.DisplayName != "Robert" {
			t.Fatalf("promote: %d %+v", s, u)
		}
		// disabling signs him out and blocks login
		if s := ann.js("PATCH", bp, map[string]any{"disabled": true}, nil); s != 200 {
			t.Fatal(s)
		}
		expectStatus(t, bob.req("GET", "/api/v1/me", nil), 401, "unauthorized")
		r := e.client().req("POST", "/api/v1/auth/login", map[string]string{"username": "bob", "password": "a brand new one"})
		expectStatus(t, r, 403, "account_disabled")
		// admin password reset
		ann.js("PATCH", bp, map[string]any{"disabled": false, "password": "reset by admin"}, nil)
		if s := e.client().login("bob", "reset by admin"); s != 200 {
			t.Fatalf("reset password login: %d", s)
		}
		// delete him; his progress goes with him
		expectStatus(t, ann.req("DELETE", bp, nil), 204, "")
		var n int
		e.st.DB().QueryRow("SELECT COUNT(*) FROM progress WHERE user_id = ?", bobID).Scan(&n)
		if n != 0 {
			t.Errorf("%d progress rows left for a deleted user", n)
		}
	})

	t.Run("logout", func(t *testing.T) {
		c := e.client()
		if s := c.login("ann", "long enough pw"); s != 200 {
			t.Fatal(s)
		}
		expectStatus(t, c.req("POST", "/api/v1/auth/logout", nil), 204, "")
		expectStatus(t, c.req("GET", "/api/v1/me", nil), 401, "unauthorized")
		// logging out twice (or without a session) is fine
		expectStatus(t, c.req("POST", "/api/v1/auth/logout", nil), 204, "")
	})

	// last: this locks the test client's address out of logging in
	t.Run("login rate limit", func(t *testing.T) {
		c := e.client()
		if s := c.login("ann", "wrong password"); s != 401 {
			t.Fatalf("wrong password: %d", s)
		}
		if s := c.login("nobody-here", "whatever pw"); s != 401 {
			t.Fatalf("unknown user: %d", s)
		}
		var limited resp
		for i := 0; i < 15; i++ {
			r := c.req("POST", "/api/v1/auth/login", map[string]string{"username": "ann", "password": "wrong password"})
			if r.StatusCode == 429 {
				limited = r
				break
			}
		}
		if limited.Response == nil {
			t.Fatal("no rate limit after 15 bad attempts")
		}
		if ra, _ := strconv.Atoi(limited.Header.Get("Retry-After")); ra <= 0 {
			t.Errorf("Retry-After = %q", limited.Header.Get("Retry-After"))
		}
		expectStatus(t, c.req("POST", "/api/v1/auth/login", map[string]string{"username": "ann", "password": "long enough pw"}), 429, "rate_limited")
	})
}

func TestMissingFileAndUnmountedLibrary(t *testing.T) {
	e := newEnv(t)
	c := e.client()
	if r := c.req("POST", "/api/v1/setup", map[string]string{"username": "ann", "password": "long enough pw"}); r.StatusCode != 201 {
		t.Fatal(r.StatusCode)
	}
	tr := tree(t, c, findCourse(t, c, "Go Basics").ID)
	it := item(t, tr, "Lesson")
	os.Remove(filepath.Join(e.root, "Go Basics", "02 - Next", "01 - Lesson.mp4"))
	r := c.req("GET", itemPath(it, "/stream"), nil)
	expectStatus(t, r, 404, "file_missing")
	// after a rescan the item is gone from the course
	if _, err := e.app.ScanLibrary(context.Background(), e.lib, nil); err != nil {
		t.Fatal(err)
	}
	expectStatus(t, c.req("GET", itemPath(it, "/stream"), nil), 404, "not_found")
	expectStatus(t, c.req("POST", itemPath(it, "/progress"), map[string]any{"position": 1}), 404, "not_found")
}
