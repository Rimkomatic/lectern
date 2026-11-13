package api

import (
	"strconv"
	"testing"

	"lectern/internal/store"
)

func TestDropCourse(t *testing.T) {
	e := newEnv(t)
	ann := e.client()
	expectStatus(t, ann.req("POST", "/api/v1/setup", map[string]string{"username": "ann", "password": "long enough pw"}), 201, "")
	expectStatus(t, ann.req("POST", "/api/v1/admin/users", map[string]string{"username": "bob", "password": "long enough pw", "role": "user"}), 201, "")
	bob := e.client()
	if s := bob.login("bob", "long enough pw"); s != 200 {
		t.Fatalf("bob login: %d", s)
	}
	anon := e.client()

	gb, pods := findCourse(t, ann, "Go Basics"), findCourse(t, ann, "Podcasts")
	gbTree := tree(t, ann, gb.ID)
	welcome, docs, ep1 := item(t, gbTree, "Welcome"), item(t, gbTree, "Docs"), item(t, tree(t, ann, pods.ID), "ep1")
	drop := func(c *client, id uint64, hdr ...string) resp {
		return c.req("DELETE", "/api/v1/courses/"+strconv.FormatUint(id, 10)+"/progress", nil, hdr...)
	}
	report := func(c *client, it store.Item, body map[string]any) store.Progress {
		t.Helper()
		var out struct {
			Progress store.Progress `json:"progress"`
		}
		if s := c.js("POST", itemPath(it, "/progress"), body, &out); s != 200 {
			t.Fatalf("progress of %q: %d", it.Title, s)
		}
		return out.Progress
	}
	continueIDs := func(c *client) []uint64 {
		t.Helper()
		var cont []store.ContinueEntry
		if s := c.js("GET", "/api/v1/continue", nil, &cont); s != 200 {
			t.Fatalf("continue: %d", s)
		}
		var ids []uint64
		for _, x := range cont {
			ids = append(ids, x.Course.ID)
		}
		return ids
	}

	// Both have watched a little of both courses; Ann also marked a link done by hand.
	for _, c := range []*client{ann, bob} {
		report(c, welcome, map[string]any{"event": "heartbeat", "position": 40, "duration": 100})
		report(c, ep1, map[string]any{"event": "heartbeat", "position": 5, "duration": 50})
	}
	report(ann, docs, map[string]any{"event": "done", "done": true})
	if before := findCourse(t, ann, "Go Basics"); before.DoneCount != 1 || before.LastSeenAt == nil {
		t.Fatalf("before: %+v", before)
	}
	if ids := continueIDs(ann); len(ids) != 2 {
		t.Fatalf("continue before: %v", ids)
	}

	t.Run("only signed-in people, only from this site", func(t *testing.T) {
		expectStatus(t, drop(anon, gb.ID), 401, "unauthorized")
		expectStatus(t, drop(ann, gb.ID, "Origin", "http://evil.example"), 403, "cross_site")
		expectStatus(t, drop(ann, gb.ID, "Sec-Fetch-Site", "cross-site"), 403, "cross_site")
		if findCourse(t, ann, "Go Basics").LastSeenAt == nil {
			t.Fatal("a refused request deleted progress")
		}
	})

	t.Run("bad and unknown courses", func(t *testing.T) {
		expectStatus(t, ann.req("DELETE", "/api/v1/courses/abc/progress", nil), 404, "not_found")
		expectStatus(t, drop(ann, 987654), 404, "not_found")
	})

	t.Run("deletes the caller's progress in that course", func(t *testing.T) {
		expectStatus(t, drop(ann, gb.ID), 204, "")
		after := findCourse(t, ann, "Go Basics")
		if after.DoneCount != 0 || after.LastSeenAt != nil {
			t.Fatalf("summary after: %+v", after)
		}
		tr := tree(t, ann, gb.ID)
		if tr.Resume != nil || tr.Completed {
			t.Fatalf("tree after: resume=%+v completed=%v", tr.Resume, tr.Completed)
		}
		for _, s := range tr.Sections {
			for _, it := range s.Items {
				if it.Progress != nil {
					t.Fatalf("%q still has progress %+v", it.Title, it.Progress)
				}
			}
		}
		if len(tr.Sections) != len(gbTree.Sections) || tr.Course.ItemCount != gbTree.Course.ItemCount {
			t.Fatalf("the course itself changed: %d sections, %d items", len(tr.Sections), tr.Course.ItemCount)
		}
	})

	t.Run("nothing else is touched", func(t *testing.T) {
		if ids := continueIDs(ann); len(ids) != 1 || ids[0] != pods.ID {
			t.Fatalf("ann's continue row = %v, want only Podcasts (%d)", ids, pods.ID)
		}
		if findCourse(t, ann, "Podcasts").LastSeenAt == nil {
			t.Fatal("ann's other course lost its progress")
		}
		if ids := continueIDs(bob); len(ids) != 2 {
			t.Fatalf("bob's continue row = %v, want both courses", ids)
		}
		d := store.ItemDetail{}
		if s := bob.js("GET", itemPath(welcome, ""), nil, &d); s != 200 || d.Item.Progress == nil || d.Item.Progress.Position != 40 {
			t.Fatalf("bob's progress: %d %+v", s, d.Item.Progress)
		}
	})

	t.Run("can be repeated", func(t *testing.T) {
		expectStatus(t, drop(ann, gb.ID), 204, "")
		expectStatus(t, drop(ann, gb.ID), 204, "")
	})

	t.Run("watching again starts from nothing", func(t *testing.T) {
		p := report(ann, welcome, map[string]any{"event": "heartbeat", "position": 3, "duration": 100})
		if p.Position != 3 || p.Furthest != 3 || p.State != "started" {
			t.Fatalf("progress after dropping and watching again: %+v", p)
		}
		if ids := continueIDs(ann); len(ids) != 2 {
			t.Fatalf("continue row = %v, want both courses again", ids)
		}
	})
}
