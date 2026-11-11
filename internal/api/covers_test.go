package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strconv"
	"strings"
	"testing"
	"time"

	"lectern/internal/store"
)

func makePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{R: uint8(40 * x), G: uint8(40 * y), B: 200, A: 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func makeJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, w, h)), nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestSniffImage(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want string
	}{
		{"jpeg", []byte("\xff\xd8\xff\xe0rest"), "image/jpeg"},
		{"png", []byte("\x89PNG\r\n\x1a\nrest"), "image/png"},
		{"webp", []byte("RIFF\x10\x00\x00\x00WEBPVP8 "), "image/webp"},
		{"avif", []byte("\x00\x00\x00\x1cftypavif\x00\x00\x00\x00"), "image/avif"},
		{"avif sequence", []byte("\x00\x00\x00\x1cftypavis\x00\x00\x00\x00"), "image/avif"},
		{"heic is not accepted", []byte("\x00\x00\x00\x1cftypheic\x00\x00\x00\x00"), ""},
		{"wav is not webp", []byte("RIFF\x10\x00\x00\x00WAVEfmt "), ""},
		{"gif is not accepted", []byte("GIF89a\x01\x00"), ""},
		{"svg is not accepted", []byte(`<svg xmlns="http://www.w3.org/2000/svg" onload="x()"></svg>`), ""},
		{"html is not accepted", []byte("<!doctype html><script>x()</script>"), ""},
		{"too short", []byte("\xff\xd8"), ""},
		{"empty", nil, ""},
	}
	for _, c := range cases {
		if got := sniffImage(c.data); got != c.want {
			t.Errorf("%s: sniffImage = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestEtagMatches(t *testing.T) {
	const tag = `"cover-7"`
	for header, want := range map[string]bool{
		`"cover-7"`:               true,
		`W/"cover-7"`:             true,
		`"a", "cover-7"`:          true,
		`"a" , W/"cover-7" , "b"`: true,
		`*`:                       true,
		`"cover-8"`:               false,
		``:                        false,
		`cover-7`:                 false,
	} {
		if got := etagMatches(header, tag); got != want {
			t.Errorf("etagMatches(%q) = %v, want %v", header, got, want)
		}
	}
}

func TestCourseCovers(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	ann := e.client()
	expectStatus(t, ann.req("POST", "/api/v1/setup", map[string]string{"username": "ann", "password": "long enough pw"}), 201, "")
	expectStatus(t, ann.req("POST", "/api/v1/admin/users", map[string]string{"username": "bob", "password": "long enough pw", "role": "user"}), 201, "")
	bob := e.client()
	if s := bob.login("bob", "long enough pw"); s != 200 {
		t.Fatalf("bob login: %d", s)
	}
	anon := e.client()

	goBasics := findCourse(t, ann, "Go Basics")
	podcasts := findCourse(t, ann, "Podcasts")
	coverURL := func(c store.CourseSummary) string { return fmt.Sprintf("/api/v1/courses/%d/cover", c.ID) }
	adminURL := func(c store.CourseSummary) string { return fmt.Sprintf("/api/v1/admin/courses/%d/cover", c.ID) }
	stateOf := func(t *testing.T, r resp) store.CoverState {
		t.Helper()
		expectStatus(t, r, 200, "")
		var st store.CoverState
		if err := json.Unmarshal(r.body, &st); err != nil {
			t.Fatalf("bad body %q: %v", r.body, err)
		}
		return st
	}

	t.Run("no cover to begin with", func(t *testing.T) {
		if goBasics.HasCover || goBasics.CoverV != 0 {
			t.Fatalf("summary: %+v", goBasics)
		}
		expectStatus(t, ann.req("GET", coverURL(goBasics), nil), 404, "not_found")
	})

	pic := makePNG(t, 4, 3)
	var v1 int64

	t.Run("only admins may change covers", func(t *testing.T) {
		expectStatus(t, bob.req("PUT", adminURL(goBasics), string(pic)), 403, "forbidden")
		expectStatus(t, bob.req("DELETE", adminURL(goBasics), nil), 403, "forbidden")
		expectStatus(t, anon.req("PUT", adminURL(goBasics), string(pic)), 401, "unauthorized")
		expectStatus(t, anon.req("DELETE", adminURL(goBasics), nil), 401, "unauthorized")
		expectStatus(t, ann.req("GET", coverURL(goBasics), nil), 404, "not_found") // nothing slipped through
	})

	t.Run("an admin uploads a cover", func(t *testing.T) {
		st := stateOf(t, ann.req("PUT", adminURL(goBasics), string(pic), "Content-Type", "image/png"))
		if !st.HasCover || st.Version <= 0 {
			t.Fatalf("state after upload: %+v", st)
		}
		v1 = st.Version

		c := findCourse(t, ann, "Go Basics")
		if !c.HasCover || c.CoverV != v1 {
			t.Fatalf("course list after upload: %+v", c)
		}
		if tr := tree(t, ann, goBasics.ID); !tr.Course.HasCover || tr.Course.CoverV != v1 {
			t.Fatalf("course page after upload: %+v", tr.Course)
		}
		if other := findCourse(t, ann, "Podcasts"); other.HasCover || other.CoverV != 0 {
			t.Fatalf("another course got the cover: %+v", other)
		}
	})

	t.Run("everyone can see it, and browsers can cache it", func(t *testing.T) {
		for name, c := range map[string]*client{"admin": ann, "user": bob} {
			r := c.req("GET", coverURL(goBasics), nil)
			expectStatus(t, r, 200, "")
			if !bytes.Equal(r.body, pic) || r.Header.Get("Content-Type") != "image/png" {
				t.Fatalf("%s got %q (%d bytes)", name, r.Header.Get("Content-Type"), len(r.body))
			}
			if r.Header.Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(r.Header.Get("Content-Security-Policy"), "sandbox") {
				t.Errorf("%s: missing hardening headers: %v", name, r.Header)
			}
			if r.Header.Get("Cache-Control") != "private, no-cache" {
				t.Errorf("without ?v the picture must be revalidated, got %q", r.Header.Get("Cache-Control"))
			}
		}
		versioned := coverURL(goBasics) + "?v=" + strconv.FormatInt(v1, 10)
		r := bob.req("GET", versioned, nil)
		expectStatus(t, r, 200, "")
		if cc := r.Header.Get("Cache-Control"); !strings.Contains(cc, "immutable") || !strings.HasPrefix(cc, "private") {
			t.Errorf("current version should be cached for good, got %q", cc)
		}
		etag := r.Header.Get("ETag")
		if etag == "" {
			t.Fatal("no ETag")
		}
		r = bob.req("GET", versioned, nil, "If-None-Match", etag)
		expectStatus(t, r, 304, "")
		if len(r.body) != 0 {
			t.Errorf("304 carried a body of %d bytes", len(r.body))
		}
		// an out-of-date ?v= is not trusted to be immutable
		r = bob.req("GET", coverURL(goBasics)+"?v=1", nil)
		expectStatus(t, r, 200, "")
		if strings.Contains(r.Header.Get("Cache-Control"), "immutable") {
			t.Errorf("stale version marked immutable: %q", r.Header.Get("Cache-Control"))
		}
		expectStatus(t, anon.req("GET", coverURL(goBasics), nil), 401, "unauthorized")
		if h := ann.req("HEAD", coverURL(goBasics), nil); h.StatusCode != 200 || len(h.body) != 0 {
			t.Errorf("HEAD: %d with %d bytes", h.StatusCode, len(h.body))
		}
	})

	t.Run("replacing it changes the version", func(t *testing.T) {
		time.Sleep(3 * time.Millisecond) // versions are milliseconds
		jpg := makeJPEG(t, 8, 8)
		st := stateOf(t, ann.req("PUT", adminURL(goBasics), string(jpg)))
		if st.Version <= v1 {
			t.Fatalf("version did not advance: %d -> %d", v1, st.Version)
		}
		r := bob.req("GET", coverURL(goBasics)+"?v="+strconv.FormatInt(st.Version, 10), nil)
		if r.Header.Get("Content-Type") != "image/jpeg" || !bytes.Equal(r.body, jpg) {
			t.Fatalf("replacement not served: %q", r.Header.Get("Content-Type"))
		}
		// the old ETag no longer matches
		oldTag := fmt.Sprintf(`"cover-%d"`, v1)
		expectStatus(t, bob.req("GET", coverURL(goBasics), nil, "If-None-Match", oldTag), 200, "")
		v1 = st.Version
	})

	t.Run("bad uploads are refused", func(t *testing.T) {
		expectStatus(t, ann.req("PUT", adminURL(goBasics), ""), 400, "bad_request")
		expectStatus(t, ann.req("PUT", adminURL(goBasics), "just some text"), 415, "unsupported_image")
		expectStatus(t, ann.req("PUT", adminURL(goBasics), `<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"/>`, "Content-Type", "image/svg+xml"), 415, "unsupported_image")
		expectStatus(t, ann.req("PUT", adminURL(goBasics), "<script>alert(1)</script>", "Content-Type", "image/png"), 415, "unsupported_image") // the header is not trusted
		big := string(pic) + strings.Repeat("x", maxCoverBytes)
		expectStatus(t, ann.req("PUT", adminURL(goBasics), big), 413, "too_large")
		expectStatus(t, ann.req("PUT", "/api/v1/admin/courses/999999/cover", string(pic)), 404, "not_found")
		expectStatus(t, ann.req("PUT", "/api/v1/admin/courses/abc/cover", string(pic)), 404, "not_found")
		expectStatus(t, ann.req("DELETE", "/api/v1/admin/courses/999999/cover", nil), 404, "not_found")
		// none of that disturbed the cover
		if c := findCourse(t, ann, "Go Basics"); c.CoverV != v1 {
			t.Fatalf("cover changed by failed uploads: %+v", c)
		}
	})

	t.Run("a cover just under the limit is accepted", func(t *testing.T) {
		edge := append(append([]byte{}, pic...), bytes.Repeat([]byte{0}, maxCoverBytes-len(pic))...)
		if len(edge) != maxCoverBytes {
			t.Fatalf("test data is %d bytes", len(edge))
		}
		st := stateOf(t, ann.req("PUT", adminURL(podcasts), string(edge)))
		if !st.HasCover {
			t.Fatalf("state: %+v", st)
		}
		r := ann.req("GET", coverURL(podcasts), nil)
		if len(r.body) != maxCoverBytes {
			t.Fatalf("stored %d bytes", len(r.body))
		}
		stateOf(t, ann.req("DELETE", adminURL(podcasts), nil))
	})

	t.Run("cross-site requests cannot change covers", func(t *testing.T) {
		expectStatus(t, ann.req("PUT", adminURL(goBasics), string(pic), "Origin", "https://evil.example"), 403, "cross_site")
		expectStatus(t, ann.req("DELETE", adminURL(goBasics), nil, "Origin", "https://evil.example"), 403, "cross_site")
		expectStatus(t, ann.req("DELETE", adminURL(goBasics), nil, "Sec-Fetch-Site", "cross-site"), 403, "cross_site")
		if c := findCourse(t, ann, "Go Basics"); c.CoverV != v1 {
			t.Fatalf("cover changed by a cross-site request: %+v", c)
		}
	})

	t.Run("the uploaded cover beats the folder image, which is the fallback", func(t *testing.T) {
		folder := makeJPEG(t, 6, 6)
		put(t, e.root, "Go Basics/cover.jpg", folder)
		if _, err := e.app.ScanLibrary(ctx, e.lib, nil); err != nil {
			t.Fatal(err)
		}
		c := findCourse(t, ann, "Go Basics")
		if !c.HasCover || c.CoverV != v1 {
			t.Fatalf("after adding a folder image: %+v", c)
		}
		if r := ann.req("GET", coverURL(goBasics), nil); bytes.Equal(r.body, folder) {
			t.Fatal("served the folder image although a cover was uploaded")
		}

		st := stateOf(t, ann.req("DELETE", adminURL(goBasics), nil))
		if !st.HasCover || st.Version != 0 {
			t.Fatalf("after removing: %+v (the folder image should remain)", st)
		}
		r := ann.req("GET", coverURL(goBasics), nil)
		expectStatus(t, r, 200, "")
		if !bytes.Equal(r.body, folder) || r.Header.Get("Content-Type") != "image/jpeg" {
			t.Fatalf("folder image not served after removing the upload: %q", r.Header.Get("Content-Type"))
		}
		if c := findCourse(t, ann, "Go Basics"); !c.HasCover || c.CoverV != 0 {
			t.Fatalf("list after removing: %+v", c)
		}
	})

	t.Run("removing is repeatable and ends in no cover", func(t *testing.T) {
		stateOf(t, ann.req("PUT", adminURL(podcasts), string(pic)))
		st := stateOf(t, ann.req("DELETE", adminURL(podcasts), nil))
		if st.HasCover || st.Version != 0 {
			t.Fatalf("after removing: %+v", st)
		}
		expectStatus(t, ann.req("GET", coverURL(podcasts), nil), 404, "not_found")
		st = stateOf(t, ann.req("DELETE", adminURL(podcasts), nil)) // nothing to remove: still fine
		if st.HasCover {
			t.Fatalf("second removal: %+v", st)
		}
	})

	t.Run("a vanished course takes its cover along", func(t *testing.T) {
		stateOf(t, ann.req("PUT", adminURL(podcasts), string(pic)))
		count := func() int {
			var n int
			if err := e.st.DB().QueryRow(`SELECT COUNT(*) FROM course_covers WHERE course_id = ?`, podcasts.ID).Scan(&n); err != nil {
				t.Fatal(err)
			}
			return n
		}
		if count() != 1 {
			t.Fatal("cover row missing")
		}
		// the folder disappears: hidden at once, forgotten once it has been gone long enough
		seen := map[uint64]bool{goBasics.ID: true}
		if _, err := e.st.MarkCoursesMissing(ctx, e.lib.ID, seen); err != nil {
			t.Fatal(err)
		}
		expectStatus(t, ann.req("GET", coverURL(podcasts), nil), 404, "not_found")
		expectStatus(t, ann.req("PUT", adminURL(podcasts), string(pic)), 404, "not_found")
		if _, err := e.st.PurgeMissing(ctx, time.Now().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		if count() != 0 {
			t.Fatal("cover row survived its course")
		}
	})
}
