package api

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lectern/internal/scan"
	"lectern/internal/store"
)

type chaptersReply struct {
	Chapters []store.Chapter `json:"chapters"`
	Progress *store.Progress `json:"progress"`
}

func TestChaptersAndSingleFileCourses(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	// A long video with a timestamp file, a video straight in the library root,
	// and a folder of loose videos.
	put(t, e.root, "Long Talk.mp4", fakeVideo(120_000))
	put(t, e.root, "Long Talk.chapters.txt", []byte("0:00 Intro\n10:00 Middle\n20:00 End\n"))
	put(t, e.root, "Long Talk.en.srt", []byte("1\n00:00:01,000 --> 00:00:02,000\nHello\n"))
	put(t, e.root, "Flat Course/01 - One.mp4", fakeVideo(20_000))
	put(t, e.root, "Flat Course/02 - Two.mp4", fakeVideo(21_000))
	if _, err := e.app.ScanLibrary(ctx, e.lib, nil); err != nil {
		t.Fatal(err)
	}

	ann := e.client()
	expectStatus(t, ann.req("POST", "/api/v1/setup", map[string]string{"username": "ann", "password": "long enough pw"}), 201, "")
	expectStatus(t, ann.req("POST", "/api/v1/admin/users", map[string]string{"username": "bob", "password": "long enough pw", "role": "user"}), 201, "")
	bob := e.client()
	if s := bob.login("bob", "long enough pw"); s != 200 {
		t.Fatalf("bob login: %d", s)
	}
	anon := e.client()

	t.Run("a video in the library root is a course of its own", func(t *testing.T) {
		c := findCourse(t, ann, "Long Talk")
		if c.ItemCount != 3 || c.DoneCount != 0 {
			t.Fatalf("summary = %+v (three chapters count as three lessons)", c)
		}
		tr := tree(t, ann, c.ID)
		if len(tr.Sections) != 1 || !tr.Sections[0].Loose || len(tr.Sections[0].Items) != 1 {
			t.Fatalf("sections = %+v", tr.Sections)
		}
		it := tr.Sections[0].Items[0]
		if it.Kind != "video" || len(it.Chapters) != 3 || len(it.Subtitles) != 1 {
			t.Fatalf("item = %+v", it)
		}
		// it streams like any other, with range support
		r := ann.req("GET", itemPath(it, "/stream"), nil, "Range", "bytes=10-19")
		expectStatus(t, r, 206, "")
		if got, want := string(r.body), string(fakeVideo(20)[10:20]); got != want {
			t.Errorf("range body %q, want %q", got, want)
		}
		expectStatus(t, ann.req("GET", fmt.Sprintf("/api/v1/subtitles/%d", it.Subtitles[0].ID), nil), 200, "")
	})

	t.Run("a folder of loose videos has a loose section", func(t *testing.T) {
		c := findCourse(t, ann, "Flat Course")
		tr := tree(t, ann, c.ID)
		if len(tr.Sections) != 1 || !tr.Sections[0].Loose || len(tr.Sections[0].Items) != 2 {
			t.Fatalf("sections = %+v", tr.Sections)
		}
		// whereas a course with real sections is not loose
		gb := tree(t, ann, findCourse(t, ann, "Go Basics").ID)
		for _, s := range gb.Sections {
			if s.Loose != (s.Title == scan.RootSectionTitle) {
				t.Errorf("section %q loose=%v", s.Title, s.Loose)
			}
		}
	})

	talk := findCourse(t, ann, "Long Talk")
	it := tree(t, ann, talk.ID).Sections[0].Items[0]
	url := itemPath(it, "/chapters")
	post := func(c *client, body any) (resp, chaptersReply) {
		t.Helper()
		r := c.req("POST", url, body)
		var out chaptersReply
		if r.StatusCode == 200 {
			if err := json.Unmarshal(r.body, &out); err != nil {
				t.Fatalf("bad body %q: %v", r.body, err)
			}
		}
		return r, out
	}

	t.Run("who may mark chapters", func(t *testing.T) {
		expectStatus(t, anon.req("POST", url, map[string]any{"done": []int{0}}), 401, "unauthorized")
		r := ann.req("POST", url, map[string]any{"done": []int{0}}, "Origin", "https://evil.example")
		expectStatus(t, r, 403, "cross_site")
	})

	t.Run("bad requests", func(t *testing.T) {
		for name, body := range map[string]any{
			"negative":      `{"done":[-1]}`,
			"fraction":      `{"done":[1.5]}`,
			"text":          `{"done":["a"]}`,
			"unknown field": `{"finished":[0]}`,
			"too many":      `{"done":[` + strings.TrimSuffix(strings.Repeat("1,", 2001), ",") + `]}`,
		} {
			r, _ := post(ann, body)
			if r.StatusCode != 400 {
				t.Errorf("%s: status %d, want 400 (%s)", name, r.StatusCode, r.body)
			}
		}
		expectStatus(t, ann.req("POST", "/api/v1/items/99999999/chapters", map[string]any{"done": []int{0}}), 404, "not_found")
		plain := tree(t, ann, findCourse(t, ann, "Go Basics").ID)
		welcome := itemPath(item(t, plain, "Welcome"), "/chapters")
		expectStatus(t, ann.req("POST", welcome, map[string]any{"done": []int{0}}), 404, "not_found") // no chapters there
	})

	t.Run("chapters are finished one by one and the item follows", func(t *testing.T) {
		r, out := post(ann, map[string]any{"done": []int{0}})
		expectStatus(t, r, 200, "")
		if len(out.Chapters) != 3 || !out.Chapters[0].Done || out.Chapters[1].Done || out.Progress.State != "started" {
			t.Fatalf("after the first: %+v %+v", out.Chapters, out.Progress)
		}
		if c := findCourse(t, ann, "Long Talk"); c.DoneCount != 1 || c.ItemCount != 3 {
			t.Errorf("summary = %+v", c)
		}
		// repeating is harmless; starts that are not chapters are ignored
		_, out = post(ann, map[string]any{"done": []int{0, 7, 4000000000}})
		if !out.Chapters[0].Done || out.Chapters[2].Done {
			t.Fatalf("repeat: %+v", out.Chapters)
		}
		_, out = post(ann, map[string]any{"done": []int{600, 1200}})
		if out.Progress.State != "done" || out.Progress.DoneAt == nil {
			t.Fatalf("all finished: %+v", out.Progress)
		}
		got := tree(t, ann, talk.ID)
		if !got.Completed || got.Resume != nil || got.Course.DoneCount != 3 {
			t.Errorf("completed course: completed=%v resume=%+v summary=%+v", got.Completed, got.Resume, got.Course)
		}
		for _, ch := range got.Sections[0].Items[0].Chapters {
			if !ch.Done {
				t.Errorf("chapter %+v is not done", ch)
			}
		}
	})

	t.Run("each person has their own", func(t *testing.T) {
		bt := tree(t, bob, talk.ID)
		if bt.Course.DoneCount != 0 || bt.Course.ItemCount != 3 {
			t.Fatalf("bob's summary = %+v", bt.Course)
		}
		for _, ch := range bt.Sections[0].Items[0].Chapters {
			if ch.Done {
				t.Errorf("bob sees %+v as done", ch)
			}
		}
		var d store.ItemDetail
		if s := bob.js("GET", itemPath(it, ""), nil, &d); s != 200 || d.Item.Progress != nil || len(d.Item.Chapters) != 3 || d.Item.Chapters[0].Done {
			t.Fatalf("bob's item: %d %+v", s, d.Item)
		}
	})

	t.Run("playing to the end does not finish a video with chapters", func(t *testing.T) {
		// bob watches the whole thing without finishing a single chapter
		r := bob.req("POST", itemPath(it, "/progress"), map[string]any{"event": "end", "position": 1200, "duration": 1200})
		expectStatus(t, r, 200, "")
		var out struct {
			Progress store.Progress `json:"progress"`
		}
		_ = json.Unmarshal(r.body, &out)
		if out.Progress.State != "started" || out.Progress.Position != 1200 {
			t.Fatalf("progress = %+v", out.Progress)
		}
		// the same report for a video without chapters finishes it
		plain := item(t, tree(t, bob, findCourse(t, bob, "Go Basics").ID), "Lesson")
		r = bob.req("POST", itemPath(plain, "/progress"), map[string]any{"event": "end", "position": 40, "duration": 40})
		_ = json.Unmarshal(r.body, &out)
		if out.Progress.State != "done" {
			t.Fatalf("an ordinary video should finish: %+v", out.Progress)
		}
	})

	t.Run("marking the whole item and taking it back", func(t *testing.T) {
		// ann: item is done through its chapters; mark not done clears them all
		r := ann.req("POST", itemPath(it, "/progress"), map[string]any{"event": "done", "done": false})
		expectStatus(t, r, 200, "")
		got := tree(t, ann, talk.ID)
		if got.Course.DoneCount != 0 {
			t.Fatalf("after mark-not-done: %+v", got.Course)
		}
		for _, ch := range got.Sections[0].Items[0].Chapters {
			if ch.Done {
				t.Errorf("chapter still done: %+v", ch)
			}
		}
		// bob marks the whole thing done
		expectStatus(t, bob.req("POST", itemPath(it, "/progress"), map[string]any{"event": "done", "done": true}), 200, "")
		bt := tree(t, bob, talk.ID)
		if bt.Course.DoneCount != 3 || !bt.Completed {
			t.Fatalf("bob after mark-done: %+v completed=%v", bt.Course, bt.Completed)
		}
	})

	t.Run("a removed timestamp file turns it back into a plain video", func(t *testing.T) {
		if err := os.Remove(filepath.Join(e.root, "Long Talk.chapters.txt")); err != nil {
			t.Fatal(err)
		}
		if _, err := e.app.ScanLibrary(ctx, e.lib, nil); err != nil {
			t.Fatal(err)
		}
		c := findCourse(t, bob, "Long Talk")
		if c.ItemCount != 1 {
			t.Fatalf("summary = %+v", c)
		}
		if got := tree(t, bob, talk.ID).Sections[0].Items[0]; len(got.Chapters) != 0 || got.Progress == nil || got.Progress.State != "done" {
			t.Errorf("item = %+v", got)
		}
	})
}
