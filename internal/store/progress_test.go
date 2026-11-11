package store

import (
	"math"
	"testing"
	"time"
)

var t0 = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func f64(v float64) *float64 { return &v }
func intp(v int) *int        { return &v }
func boolp(v bool) *bool     { return &v }

func merge(prev *Progress, in ProgressInput, kind string) Progress {
	return MergeProgress(prev, in, kind, 0.95, t0)
}

func TestMergeProgressMediaPosition(t *testing.T) {
	p := merge(nil, ProgressInput{Event: "heartbeat", Position: f64(30), Duration: f64(100), ClientTS: 10}, "video")
	if p.State != "started" || p.Position != 30 || p.Furthest != 30 || p.Duration == nil || *p.Duration != 100 {
		t.Fatalf("unexpected %+v", p)
	}
	// seeking back moves the position but not the furthest point
	p = merge(&p, ProgressInput{Event: "seek", Position: f64(10), ClientTS: 20}, "video")
	if p.Position != 10 || p.Furthest != 30 {
		t.Fatalf("position/furthest after seek back: %+v", p)
	}
	// a position beyond the duration is clamped
	p = merge(&p, ProgressInput{Event: "heartbeat", Position: f64(500), ClientTS: 30}, "video")
	if p.Position != 100 {
		t.Fatalf("not clamped: %+v", p)
	}
}

func TestMergeProgressStaleReportsDoNotMoveBack(t *testing.T) {
	p := merge(nil, ProgressInput{Event: "heartbeat", Position: f64(60), Duration: f64(100), ClientTS: 200}, "video")
	// an older report arriving late: position is ignored, duration is still learnt
	p = merge(&p, ProgressInput{Event: "heartbeat", Position: f64(5), Duration: f64(120), ClientTS: 100}, "video")
	if p.Position != 60 || p.ClientTS != 200 {
		t.Fatalf("stale report applied: %+v", p)
	}
	if p.Duration == nil || *p.Duration != 120 {
		t.Fatalf("duration should still update: %+v", p)
	}
	// equal timestamps are accepted (the same client can report twice in one ms)
	p = merge(&p, ProgressInput{Event: "pause", Position: f64(61), ClientTS: 200}, "video")
	if p.Position != 61 {
		t.Fatalf("equal ts rejected: %+v", p)
	}
}

func TestMergeProgressCompletion(t *testing.T) {
	// below the threshold: not done
	p := merge(nil, ProgressInput{Event: "heartbeat", Position: f64(94), Duration: f64(100), ClientTS: 1}, "video")
	if p.State != "started" {
		t.Fatalf("94%% should not complete: %+v", p)
	}
	// at the threshold: done
	p = merge(&p, ProgressInput{Event: "heartbeat", Position: f64(95), ClientTS: 2}, "video")
	if p.State != "done" || p.DoneAt == nil {
		t.Fatalf("95%% should complete: %+v", p)
	}
	// "end" completes even without a known duration
	q := merge(nil, ProgressInput{Event: "end", Position: f64(12), ClientTS: 1}, "audio")
	if q.State != "done" {
		t.Fatalf("end should complete: %+v", q)
	}
	// a custom threshold of 1.0 means "only on end"
	r := MergeProgress(nil, ProgressInput{Event: "heartbeat", Position: f64(99), Duration: f64(100), ClientTS: 1}, "video", 1.0, t0)
	if r.State != "started" {
		t.Fatalf("threshold 1.0: %+v", r)
	}
}

func TestMergeProgressDoneIsSticky(t *testing.T) {
	p := merge(nil, ProgressInput{Event: "end", Position: f64(100), Duration: f64(100), ClientTS: 1}, "video")
	doneAt := *p.DoneAt
	later := t0.Add(time.Hour)
	p = MergeProgress(&p, ProgressInput{Event: "seek", Position: f64(3), ClientTS: 2}, "video", 0.95, later)
	if p.State != "done" || !p.DoneAt.Equal(doneAt) {
		t.Fatalf("re-watching must not un-complete or re-stamp: %+v", p)
	}
	if p.Position != 3 {
		t.Fatalf("position should still follow: %+v", p)
	}
}

func TestMergeProgressExplicitDone(t *testing.T) {
	p := merge(nil, ProgressInput{Event: "done", Done: boolp(true), ClientTS: 1}, "pdf")
	if p.State != "done" || p.DoneAt == nil {
		t.Fatalf("explicit done: %+v", p)
	}
	// explicit done ignores everything else in the report
	p2 := merge(&p, ProgressInput{Event: "done", Done: boolp(true), Position: f64(50), ClientTS: 2}, "video")
	if p2.Position != 0 {
		t.Fatalf("explicit done must not move the position: %+v", p2)
	}
	// marking done twice keeps the first timestamp
	later := MergeProgress(&p, ProgressInput{Event: "done", Done: boolp(true), ClientTS: 3}, "pdf", 0.95, t0.Add(time.Hour))
	if !later.DoneAt.Equal(*p.DoneAt) {
		t.Fatalf("done_at re-stamped: %+v vs %+v", later.DoneAt, p.DoneAt)
	}
	// explicit not-done clears it and keeps the position
	v := merge(nil, ProgressInput{Event: "end", Position: f64(90), Duration: f64(90), ClientTS: 1}, "video")
	v = merge(&v, ProgressInput{Event: "done", Done: boolp(false), ClientTS: 2}, "video")
	if v.State != "started" || v.DoneAt != nil || v.Position != 90 {
		t.Fatalf("explicit not-done: %+v", v)
	}
}

func TestMergeProgressPDF(t *testing.T) {
	p := merge(nil, ProgressInput{Event: "open", Page: intp(1), Pages: intp(3), DoneOnOpen: true, ClientTS: 1}, "pdf")
	if p.State != "started" || *p.Page != 1 || *p.Pages != 3 {
		t.Fatalf("opening page 1 of 3: %+v", p)
	}
	p = merge(&p, ProgressInput{Event: "page", Page: intp(2), ClientTS: 2}, "pdf")
	if p.State != "started" || *p.Page != 2 {
		t.Fatalf("page 2: %+v", p)
	}
	p = merge(&p, ProgressInput{Event: "page", Page: intp(3), ClientTS: 3}, "pdf")
	if p.State != "done" {
		t.Fatalf("last page completes: %+v", p)
	}
	// a one-page pdf is done as soon as it is shown
	one := merge(nil, ProgressInput{Event: "open", Page: intp(1), Pages: intp(1), ClientTS: 1}, "pdf")
	if one.State != "done" {
		t.Fatalf("one-page pdf: %+v", one)
	}
	// "open" without a page (a resume prompt is showing) must not touch the saved page
	saved := merge(nil, ProgressInput{Event: "page", Page: intp(7), Pages: intp(10), ClientTS: 1}, "pdf")
	saved = merge(&saved, ProgressInput{Event: "open", Pages: intp(10), ClientTS: 2}, "pdf")
	if *saved.Page != 7 || saved.State != "started" {
		t.Fatalf("open without page changed the page: %+v", saved)
	}
}

func TestMergeProgressOtherKinds(t *testing.T) {
	for _, kind := range []string{"image", "markdown", "text", "csv", "sheet", "link", "other"} {
		opened := merge(nil, ProgressInput{Event: "open", DoneOnOpen: true, ClientTS: 1}, kind)
		if opened.State != "done" {
			t.Errorf("%s: open with done_on_open should complete: %+v", kind, opened)
		}
		off := merge(nil, ProgressInput{Event: "open", DoneOnOpen: false, ClientTS: 1}, kind)
		if off.State != "started" {
			t.Errorf("%s: open without done_on_open should not complete: %+v", kind, off)
		}
		hb := merge(nil, ProgressInput{Event: "heartbeat", DoneOnOpen: true, ClientTS: 1}, kind)
		if hb.State != "started" {
			t.Errorf("%s: only the open event completes: %+v", kind, hb)
		}
	}
	// position and page are meaningless for non-media / non-pdf items
	img := merge(nil, ProgressInput{Event: "heartbeat", Position: f64(50), Page: intp(4), ClientTS: 1}, "image")
	if img.Position != 0 || img.Page != nil {
		t.Errorf("image kept media/pdf fields: %+v", img)
	}
	vid := merge(nil, ProgressInput{Event: "heartbeat", Page: intp(4), ClientTS: 1}, "video")
	if vid.Page != nil {
		t.Errorf("video kept a page: %+v", vid)
	}
}

func TestMergeProgressRejectsBadNumbers(t *testing.T) {
	prev := merge(nil, ProgressInput{Event: "heartbeat", Position: f64(20), Duration: f64(100), ClientTS: 1}, "video")
	for _, bad := range []float64{math.NaN(), math.Inf(1), -1, 1e9} {
		p := merge(&prev, ProgressInput{Event: "heartbeat", Position: f64(bad), Duration: f64(bad), Pages: intp(-5), ClientTS: 2}, "video")
		if p.Position != 20 || *p.Duration != 100 || p.Pages != nil {
			t.Errorf("bad value %v accepted: %+v", bad, p)
		}
	}
}

func TestMergeProgressDoesNotMutateInput(t *testing.T) {
	prev := merge(nil, ProgressInput{Event: "heartbeat", Position: f64(20), Duration: f64(100), ClientTS: 1}, "video")
	snapshot := prev
	_ = merge(&prev, ProgressInput{Event: "end", Position: f64(100), ClientTS: 2}, "video")
	if prev.State != snapshot.State || prev.Position != snapshot.Position {
		t.Fatalf("MergeProgress changed its input: %+v vs %+v", prev, snapshot)
	}
}

// ---- ComputeResume ----

func prog(state string) *Progress { return &Progress{State: state} }

func course() []Section {
	return []Section{
		{ID: 1, Title: "A", Items: []Item{
			{ID: 11, Title: "a1", Kind: "video"},
			{ID: 12, Title: "a2", Kind: "pdf"},
		}},
		{ID: 2, Title: "B", Items: []Item{
			{ID: 21, Title: "b1", Kind: "video"},
			{ID: 22, Title: "b2", Kind: "link"},
		}},
	}
}

func setState(secs []Section, id uint64, p *Progress) {
	for si := range secs {
		for ii := range secs[si].Items {
			if secs[si].Items[ii].ID == id {
				secs[si].Items[ii].Progress = p
			}
		}
	}
}

func TestComputeResume(t *testing.T) {
	t.Run("empty course", func(t *testing.T) {
		if r, done := ComputeResume(nil, 0); r != nil || done {
			t.Fatalf("got %+v %v", r, done)
		}
	})
	t.Run("never opened", func(t *testing.T) {
		if r, done := ComputeResume(course(), 0); r != nil || done {
			t.Fatalf("got %+v %v", r, done)
		}
	})
	t.Run("continue the unfinished last item", func(t *testing.T) {
		s := course()
		setState(s, 11, prog("done"))
		setState(s, 12, &Progress{State: "started", Page: intp(4)})
		r, done := ComputeResume(s, 12)
		if done || r == nil || r.ItemID != 12 || r.Reason != "continue" || r.SectionID != 1 || r.Page == nil || *r.Page != 4 {
			t.Fatalf("got %+v %v", r, done)
		}
	})
	t.Run("position is carried for a started video", func(t *testing.T) {
		s := course()
		setState(s, 11, &Progress{State: "started", Position: 77})
		r, _ := ComputeResume(s, 11)
		if r == nil || r.Position != 77 {
			t.Fatalf("got %+v", r)
		}
	})
	t.Run("last item done: next unfinished in order", func(t *testing.T) {
		s := course()
		setState(s, 11, prog("done"))
		setState(s, 12, prog("done"))
		r, done := ComputeResume(s, 12)
		if done || r == nil || r.ItemID != 21 || r.Reason != "next" || r.SectionID != 2 {
			t.Fatalf("got %+v %v", r, done)
		}
		if r.Position != 0 || r.Page != nil {
			t.Fatalf("a 'next' item must not carry a position: %+v", r)
		}
	})
	t.Run("skips finished items and wraps around", func(t *testing.T) {
		s := course()
		setState(s, 22, prog("done"))
		setState(s, 21, prog("done"))
		r, _ := ComputeResume(s, 21)
		if r == nil || r.ItemID != 11 || r.Reason != "next" {
			t.Fatalf("expected to wrap to the first unfinished, got %+v", r)
		}
	})
	t.Run("everything done", func(t *testing.T) {
		s := course()
		for _, id := range []uint64{11, 12, 21, 22} {
			setState(s, id, prog("done"))
		}
		r, done := ComputeResume(s, 22)
		if r != nil || !done {
			t.Fatalf("got %+v %v", r, done)
		}
	})
	t.Run("remembered item vanished but progress exists", func(t *testing.T) {
		s := course()
		setState(s, 11, prog("done"))
		r, done := ComputeResume(s, 999)
		if done || r == nil || r.ItemID != 12 || r.Reason != "next" {
			t.Fatalf("got %+v %v", r, done)
		}
	})
	t.Run("remembered item vanished and nothing is done", func(t *testing.T) {
		s := course()
		setState(s, 11, prog("started"))
		if r, done := ComputeResume(s, 999); r != nil || done {
			t.Fatalf("got %+v %v", r, done)
		}
	})
	t.Run("items marked done by hand without ever opening the course", func(t *testing.T) {
		s := course()
		setState(s, 11, prog("done"))
		r, _ := ComputeResume(s, 0)
		if r == nil || r.ItemID != 12 || r.Reason != "next" {
			t.Fatalf("got %+v", r)
		}
	})
}
