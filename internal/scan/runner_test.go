package scan

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
	_ "time/tzdata" // the zones below must load on any machine
)

func zone(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func at(loc *time.Location, y int, m time.Month, d, h, min, s int) time.Time {
	return time.Date(y, m, d, h, min, s, 0, loc)
}

func TestScheduleNextDaily(t *testing.T) {
	utc := time.UTC
	s := Schedule{Daily: true, Hour: 3, Loc: utc}
	for _, tc := range []struct{ now, want time.Time }{
		{at(utc, 2026, 11, 12, 2, 0, 0), at(utc, 2026, 11, 12, 3, 0, 0)}, // before today's time
		{at(utc, 2026, 11, 12, 3, 0, 0), at(utc, 2026, 11, 13, 3, 0, 0)}, // exactly at it: the next one
		{at(utc, 2026, 11, 12, 3, 0, 1), at(utc, 2026, 11, 13, 3, 0, 0)}, // just after
		{at(utc, 2026, 11, 12, 23, 59, 59), at(utc, 2026, 11, 13, 3, 0, 0)},
		{at(utc, 2026, 1, 31, 12, 0, 0), at(utc, 2026, 2, 1, 3, 0, 0)},  // month end
		{at(utc, 2026, 12, 31, 12, 0, 0), at(utc, 2027, 1, 1, 3, 0, 0)}, // year end
		{at(utc, 2028, 2, 28, 12, 0, 0), at(utc, 2028, 2, 29, 3, 0, 0)}, // leap day
	} {
		if got := s.Next(tc.now); !got.Equal(tc.want) {
			t.Errorf("Next(%v) = %v, want %v", tc.now, got, tc.want)
		}
	}

	// the time of day is read in the schedule's zone, whatever zone the argument is in
	ist := zone(t, "Asia/Kolkata")
	k := Schedule{Daily: true, Hour: 3, Loc: ist}
	now := at(ist, 2026, 11, 12, 13, 1, 0) // when this feature was asked for
	want := at(ist, 2026, 11, 13, 3, 0, 0)
	if got := k.Next(now.UTC()); !got.Equal(want) || got.Location() != ist {
		t.Errorf("Kolkata: Next = %v, want %v", got, want)
	}
	if want.UTC() != at(utc, 2026, 11, 12, 21, 30, 0) {
		t.Errorf("03:00 IST should be 21:30 UTC the evening before, got %v", want.UTC())
	}
	if got := (Schedule{Daily: true, Hour: 23, Minute: 30, Loc: utc}).Next(at(utc, 2026, 6, 1, 23, 29, 0)); !got.Equal(at(utc, 2026, 6, 1, 23, 30, 0)) {
		t.Errorf("minutes ignored: %v", got)
	}
}

func TestScheduleNextAcrossClockChanges(t *testing.T) {
	ny := zone(t, "America/New_York")

	// Spring forward, 8 March 2026: 02:00 became 03:00. "03:00" stays 03:00 on
	// the wall clock, so that night is 23 hours long, not 24.
	s := Schedule{Daily: true, Hour: 3, Loc: ny}
	before := at(ny, 2026, 3, 7, 3, 0, 1)
	next := s.Next(before)
	if h, m, _ := next.Clock(); next.Day() != 8 || h != 3 || m != 0 {
		t.Errorf("spring: Next = %v", next)
	}
	if d := next.Sub(before); d != 23*time.Hour-time.Second {
		t.Errorf("spring: that night is %v long, want 22h59m59s", d)
	}

	// Fall back, 1 November 2026: 01:00-02:00 happens twice. A 01:30 scan runs
	// once, not twice.
	f := Schedule{Daily: true, Hour: 1, Minute: 30, Loc: ny}
	first := f.Next(at(ny, 2026, 10, 31, 12, 0, 0))
	if first.UTC() != at(time.UTC, 2026, 11, 1, 5, 30, 0) { // 01:30 EDT, the first one
		t.Fatalf("fall: first = %v (%v)", first, first.UTC())
	}
	second := f.Next(first)
	if second.UTC() != at(time.UTC, 2026, 11, 2, 6, 30, 0) { // 01:30 EST on the 2nd
		t.Errorf("fall: after the first 01:30 the next is %v (%v), want the 2nd", second, second.UTC())
	}

	// A time that does not exist that day (02:30 on 8 March 2026) still gives a
	// later moment within the day, and never gets stuck.
	g := Schedule{Daily: true, Hour: 2, Minute: 30, Loc: ny}
	now := at(ny, 2026, 3, 7, 2, 30, 1)
	n1 := g.Next(now)
	if !n1.After(now) || n1.Sub(now) > 26*time.Hour {
		t.Fatalf("gap: Next = %v", n1)
	}
	if n2 := g.Next(n1); !n2.After(n1) || n2.Sub(n1) > 26*time.Hour {
		t.Errorf("gap: Next(Next) = %v after %v", n2, n1)
	}
}

func TestScheduleNextInterval(t *testing.T) {
	now := at(time.UTC, 2026, 11, 12, 14, 0, 0)
	if got := (Schedule{Every: 15 * time.Minute}).Next(now); !got.Equal(now.Add(15 * time.Minute)) {
		t.Errorf("interval Next = %v", got)
	}
	if got := (Schedule{}).Next(now); !got.IsZero() {
		t.Errorf("an empty schedule never scans, got %v", got)
	}
	if (Schedule{}).Enabled() || !(Schedule{Daily: true}).Enabled() || !(Schedule{Every: time.Second}).Enabled() {
		t.Error("Enabled wrong")
	}
}

func TestScheduleInfo(t *testing.T) {
	ist := zone(t, "Asia/Kolkata")
	now := at(ist, 2026, 11, 12, 13, 1, 0)
	if i := (Schedule{Daily: true, Hour: 3, Loc: ist}).info(now); i != (ScheduleInfo{Mode: "daily", At: "03:00", Zone: "IST, UTC+05:30"}) {
		t.Errorf("daily info = %+v", i)
	}
	if i := (Schedule{Daily: true, Hour: 3, Minute: 5, Loc: time.UTC}).info(now); i != (ScheduleInfo{Mode: "daily", At: "03:05", Zone: "UTC"}) {
		t.Errorf("utc info = %+v", i)
	}
	if i := (Schedule{Every: 90 * time.Second}).info(now); i != (ScheduleInfo{Mode: "interval", Every: 90}) {
		t.Errorf("interval info = %+v", i)
	}
	if i := (Schedule{}).info(now); i != (ScheduleInfo{Mode: "off"}) {
		t.Errorf("off info = %+v", i)
	}
	// zones with no letter abbreviation are written as their offset
	if got := zoneLabel(at(zone(t, "America/Sao_Paulo"), 2026, 6, 1, 12, 0, 0)); got != "UTC-03:00" {
		t.Errorf("zoneLabel = %q", got)
	}
	if got := (Schedule{Daily: true, Hour: 3, Loc: ist}).String(); got != "daily at 03:00 (IST, UTC+05:30)" {
		t.Errorf("String = %q", got)
	}
}

// ---- the loop, on a clock the test moves ----

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Set(t time.Time) {
	c.mu.Lock()
	c.t = t
	c.mu.Unlock()
}

type scanLog struct {
	mu  sync.Mutex
	why []string
}

func (l *scanLog) add(w string) {
	l.mu.Lock()
	l.why = append(l.why, w)
	l.mu.Unlock()
}

func (l *scanLog) all() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.why...)
}

// testRunner starts a Runner on a fake clock whose scans just record why they
// ran. onScan, if set, runs inside each scan (to move the clock "while it runs").
func testRunner(t *testing.T, sched Schedule, start time.Time, onScan func(why string)) (*Runner, *fakeClock, *scanLog) {
	t.Helper()
	clk := &fakeClock{t: start}
	rec := &scanLog{}
	r := NewRunner(nil, sched, 0, slog.New(slog.NewTextHandler(io.Discard, nil)))
	r.now = clk.Now
	r.poll = 2 * time.Millisecond
	r.work = func(ctx context.Context) (Counts, []string) {
		why := r.Status().Trigger
		rec.add(why)
		if onScan != nil {
			onScan(why)
		}
		return Counts{}, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.Run(ctx); close(done) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("Run did not stop")
		}
	})
	return r, clk, rec
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for: %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

// settle gives the loop time to (wrongly) act on a clock change.
func settle() { time.Sleep(40 * time.Millisecond) }

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func nextOf(r *Runner) time.Time {
	if n := r.Status().NextAt; n != nil {
		return *n
	}
	return time.Time{}
}

func TestRunnerDailyScan(t *testing.T) {
	utc := time.UTC
	r, clk, rec := testRunner(t, Schedule{Daily: true, Hour: 3, Loc: utc}, at(utc, 2026, 11, 12, 14, 0, 0), nil)

	// start-up scan first; the nightly one is planned for the coming 03:00
	eventually(t, "start-up scan", func() bool { return len(rec.all()) == 1 && !r.Status().Running })
	if got := rec.all()[0]; got != "startup" {
		t.Fatalf("first scan was %q", got)
	}
	if got, want := nextOf(r), at(utc, 2026, 11, 13, 3, 0, 0); !got.Equal(want) {
		t.Fatalf("next = %v, want %v", got, want)
	}
	if i := r.Status().Schedule; i.Mode != "daily" || i.At != "03:00" {
		t.Errorf("schedule = %+v", i)
	}

	// a second before 03:00: nothing happens
	clk.Set(at(utc, 2026, 11, 13, 2, 59, 59))
	settle()
	if n := len(rec.all()); n != 1 {
		t.Fatalf("scanned before its time: %v", rec.all())
	}

	// 03:00: the nightly scan, once, and the next one is a day later
	clk.Set(at(utc, 2026, 11, 13, 3, 0, 0))
	eventually(t, "the 03:00 scan", func() bool { return equal(rec.all(), []string{"startup", "schedule"}) && !r.Status().Running })
	settle()
	if n := len(rec.all()); n != 2 {
		t.Fatalf("the nightly scan ran more than once: %v", rec.all())
	}
	if got, want := nextOf(r), at(utc, 2026, 11, 14, 3, 0, 0); !got.Equal(want) {
		t.Fatalf("next = %v, want %v", got, want)
	}

	// "Scan now" in the afternoon does not move the nightly scan
	clk.Set(at(utc, 2026, 11, 13, 15, 0, 0))
	if !r.Trigger() {
		t.Fatal("Trigger refused")
	}
	eventually(t, "the manual scan", func() bool { return len(rec.all()) == 3 && !r.Status().Running })
	if got := rec.all()[2]; got != "manual" {
		t.Errorf("third scan was %q", got)
	}
	if got, want := nextOf(r), at(utc, 2026, 11, 14, 3, 0, 0); !got.Equal(want) {
		t.Errorf("a manual scan moved the nightly one to %v", got)
	}

	// a machine that slept for days wakes up: one catch-up scan, not one per missed night
	clk.Set(at(utc, 2026, 11, 20, 10, 0, 0))
	eventually(t, "the catch-up scan", func() bool { return len(rec.all()) == 4 && !r.Status().Running })
	settle()
	if n := len(rec.all()); n != 4 {
		t.Fatalf("catch-up scanned %d times: %v", n-3, rec.all())
	}
	if got, want := nextOf(r), at(utc, 2026, 11, 21, 3, 0, 0); !got.Equal(want) {
		t.Errorf("after the catch-up next = %v, want %v", got, want)
	}
}

func TestRunnerScanRunningAcrossTheDueTime(t *testing.T) {
	utc := time.UTC
	var clk *fakeClock
	r, c, rec := testRunner(t, Schedule{Daily: true, Hour: 3, Loc: utc}, at(utc, 2026, 11, 12, 14, 0, 0), func(why string) {
		if why == "manual" {
			clk.Set(at(utc, 2026, 11, 13, 3, 0, 5)) // the scan runs over 03:00
		}
	})
	clk = c
	eventually(t, "start-up scan", func() bool { return len(rec.all()) == 1 && !r.Status().Running })
	clk.Set(at(utc, 2026, 11, 13, 2, 58, 0))
	if !r.Trigger() {
		t.Fatal("Trigger refused")
	}
	eventually(t, "the long manual scan", func() bool { return len(rec.all()) == 2 && !r.Status().Running })
	settle()
	// that scan covered 03:00, so no second one starts right behind it
	if n := len(rec.all()); n != 2 {
		t.Fatalf("a scan ran right after one that had covered the due time: %v", rec.all())
	}
	if got, want := nextOf(r), at(utc, 2026, 11, 14, 3, 0, 0); !got.Equal(want) {
		t.Errorf("next = %v, want %v", got, want)
	}
}

func TestRunnerIntervalScan(t *testing.T) {
	utc := time.UTC
	r, clk, rec := testRunner(t, Schedule{Every: 10 * time.Minute}, at(utc, 2026, 11, 12, 14, 0, 0), nil)
	eventually(t, "start-up scan", func() bool { return len(rec.all()) == 1 && !r.Status().Running })
	if got, want := nextOf(r), at(utc, 2026, 11, 12, 14, 10, 0); !got.Equal(want) {
		t.Fatalf("next = %v, want %v", got, want)
	}
	if i := r.Status().Schedule; i.Mode != "interval" || i.Every != 600 {
		t.Errorf("schedule = %+v", i)
	}
	clk.Set(at(utc, 2026, 11, 12, 14, 9, 59))
	settle()
	if len(rec.all()) != 1 {
		t.Fatalf("scanned early: %v", rec.all())
	}
	clk.Set(at(utc, 2026, 11, 12, 14, 10, 0))
	eventually(t, "the interval scan", func() bool { return len(rec.all()) == 2 && !r.Status().Running })
	if got := rec.all()[1]; got != "schedule" {
		t.Errorf("second scan was %q", got)
	}
	// the interval counts from the end of the last scan, manual ones included
	clk.Set(at(utc, 2026, 11, 12, 14, 15, 0))
	if !r.Trigger() {
		t.Fatal("Trigger refused")
	}
	eventually(t, "the manual scan", func() bool { return len(rec.all()) == 3 && !r.Status().Running })
	if got, want := nextOf(r), at(utc, 2026, 11, 12, 14, 25, 0); !got.Equal(want) {
		t.Errorf("next after a manual scan = %v, want %v", got, want)
	}
}

func TestRunnerWithoutSchedule(t *testing.T) {
	utc := time.UTC
	r, clk, rec := testRunner(t, Schedule{}, at(utc, 2026, 11, 12, 14, 0, 0), nil)
	eventually(t, "start-up scan", func() bool { return len(rec.all()) == 1 && !r.Status().Running })
	if st := r.Status(); st.NextAt != nil || st.Schedule.Mode != "off" {
		t.Errorf("status = %+v", st)
	}
	clk.Set(at(utc, 2027, 1, 1, 3, 0, 0))
	settle()
	if len(rec.all()) != 1 {
		t.Fatalf("scanned with no schedule: %v", rec.all())
	}
	// "Scan now" still works
	if !r.Trigger() {
		t.Fatal("Trigger refused")
	}
	eventually(t, "the manual scan", func() bool { return len(rec.all()) == 2 && !r.Status().Running })
}
