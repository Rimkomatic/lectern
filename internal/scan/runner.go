package scan

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path"
	"sync"
	"time"

	"lectern/internal/fsutil"
)

// Schedule says when libraries are scanned by themselves, besides the scan at
// start-up and the ones an admin asks for. The zero value means never.
type Schedule struct {
	// Daily scans once a day at Hour:Minute. The time is a wall-clock time in
	// Loc, so "03:00" stays 03:00 across the change to and from summer time.
	Daily        bool
	Hour, Minute int
	// Every scans this long after the previous scan ended. It is used when
	// Daily is off.
	Every time.Duration
	// Loc is the time zone of the daily time; nil means the server's (TZ).
	Loc *time.Location
}

// Enabled reports whether any scan is planned by the clock.
func (s Schedule) Enabled() bool { return s.Daily || s.Every > 0 }

func (s Schedule) location() *time.Location {
	if s.Loc != nil {
		return s.Loc
	}
	return time.Local
}

// Next is the first moment after t at which the schedule calls for a scan, or
// the zero time if it never does.
func (s Schedule) Next(t time.Time) time.Time {
	switch {
	case s.Daily:
		t = t.In(s.location())
		y, m, d := t.Date()
		next := time.Date(y, m, d, s.Hour, s.Minute, 0, 0, t.Location())
		// Today's time may be past; a day with a clock change can also put it
		// at or before t. The next day's is always later, so this ends at once.
		for i := 0; i < 3 && !next.After(t); i++ {
			d++
			next = time.Date(y, m, d, s.Hour, s.Minute, 0, 0, t.Location())
		}
		if !next.After(t) {
			next = t.Add(24 * time.Hour)
		}
		return next
	case s.Every > 0:
		return t.Add(s.Every)
	}
	return time.Time{}
}

// ScheduleInfo is the schedule as the admin page shows it.
type ScheduleInfo struct {
	Mode  string `json:"mode"`                    // daily | interval | off
	At    string `json:"at,omitempty"`            // daily: the time of day, "03:00"
	Every int64  `json:"every_seconds,omitempty"` // interval: seconds between scans
	Zone  string `json:"zone,omitempty"`          // daily: the time zone the time is in
}

func (s Schedule) info(now time.Time) ScheduleInfo {
	switch {
	case s.Daily:
		return ScheduleInfo{Mode: "daily", At: fmt.Sprintf("%02d:%02d", s.Hour, s.Minute), Zone: zoneLabel(now.In(s.location()))}
	case s.Every > 0:
		return ScheduleInfo{Mode: "interval", Every: int64(s.Every / time.Second)}
	}
	return ScheduleInfo{Mode: "off"}
}

// String describes the schedule for the log.
func (s Schedule) String() string {
	switch i := s.info(time.Now()); i.Mode {
	case "daily":
		return "daily at " + i.At + " (" + i.Zone + ")"
	case "interval":
		return "every " + s.Every.String()
	}
	return "off (start-up and Scan now only)"
}

// zoneLabel names the zone of t: "IST, UTC+05:30", or "UTC".
func zoneLabel(t time.Time) string {
	name, off := t.Zone()
	sign := '+'
	if off < 0 {
		sign, off = '-', -off
	}
	if off == 0 && (name == "UTC" || name == "") {
		return "UTC"
	}
	utc := fmt.Sprintf("UTC%c%02d:%02d", sign, off/3600, off%3600/60)
	if name == "" || name[0] == '+' || name[0] == '-' { // zones without a letter abbreviation
		return utc
	}
	return name + ", " + utc
}

// Status is what the admin page shows about scanning.
type Status struct {
	Running    bool         `json:"running"`
	Trigger    string       `json:"trigger,omitempty"` // startup | schedule | manual
	StartedAt  *time.Time   `json:"started_at,omitempty"`
	FinishedAt *time.Time   `json:"finished_at,omitempty"`
	Library    string       `json:"library,omitempty"`
	Done       int          `json:"done"`
	Total      int          `json:"total"`
	Current    string       `json:"current,omitempty"`
	Errors     []string     `json:"errors,omitempty"`
	Last       *Counts      `json:"last,omitempty"` // result of the latest finished scan
	Schedule   ScheduleInfo `json:"schedule"`
	NextAt     *time.Time   `json:"next_at,omitempty"` // when the next scan by the clock is due
}

// Runner scans all libraries once at start, then by its schedule and on
// demand. Only one scan runs at a time.
type Runner struct {
	app       *Applier
	sched     Schedule
	retention time.Duration
	log       *slog.Logger

	mu      sync.Mutex
	status  Status
	pending bool
	nextAt  time.Time // when the schedule next calls for a scan; zero if never
	subs    map[chan Status]struct{}
	lastPub time.Time
	trigger chan string

	// The clock and the scan itself, replaceable so the schedule can be tested.
	now  func() time.Time
	poll time.Duration // longest sleep: the clock is looked at least this often
	work func(ctx context.Context) (Counts, []string)
}

// NewRunner creates a runner. With a Schedule that is not Enabled, scans still
// happen at start-up and on demand; retention <= 0 keeps missing items forever.
func NewRunner(app *Applier, sched Schedule, retention time.Duration, log *slog.Logger) *Runner {
	if log == nil {
		log = app.logger()
	}
	r := &Runner{
		app: app, sched: sched, retention: retention, log: log,
		subs:    map[chan Status]struct{}{},
		trigger: make(chan string, 1),
		now:     time.Now,
		poll:    30 * time.Second,
	}
	r.work = r.scanLibraries
	return r
}

// Run blocks until ctx ends. Call it in its own goroutine.
func (r *Runner) Run(ctx context.Context) {
	r.scan(ctx, "startup")
	for {
		var timer *time.Timer
		var tick <-chan time.Time
		if wait, planned := r.sleepFor(); planned {
			timer = time.NewTimer(wait)
			tick = timer.C
		}
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return
		case <-tick:
			if r.due() {
				r.scan(ctx, "schedule")
			}
		case why := <-r.trigger:
			if timer != nil {
				timer.Stop()
			}
			r.scan(ctx, why)
		}
	}
}

// sleepFor is how long to wait before looking at the clock again: until the
// next scan is due, but never longer than r.poll. A wall clock can jump (a
// machine that slept, a corrected clock) and a timer set hours ahead would not
// notice, so the clock is checked every so often instead of trusting one timer.
func (r *Runner) sleepFor() (time.Duration, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.nextAt.IsZero() {
		return 0, false
	}
	d := r.nextAt.Sub(r.now())
	if d > r.poll {
		d = r.poll
	}
	if d < time.Millisecond {
		d = time.Millisecond
	}
	return d, true
}

func (r *Runner) due() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return !r.nextAt.IsZero() && !r.now().Before(r.nextAt)
}

// rearm plans the next scan by the clock, after one has ended at `end`. Call
// with r.mu held.
func (r *Runner) rearm(end time.Time) {
	switch {
	case r.sched.Daily:
		// A scan that ends after the due time has done that scan's work (even a
		// manual one that happened to run across it). One that ended before it,
		// say "Scan now" in the afternoon, leaves the nightly scan in place.
		if r.nextAt.IsZero() || !end.Before(r.nextAt) {
			r.nextAt = r.sched.Next(end)
		}
	case r.sched.Every > 0:
		r.nextAt = r.sched.Next(end)
	}
}

// Trigger asks for a scan now. It returns false if one is already running or queued.
func (r *Runner) Trigger() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.status.Running || r.pending {
		return false
	}
	select {
	case r.trigger <- "manual":
		r.pending = true
		return true
	default:
		return false
	}
}

// Status returns a copy of the current state.
func (r *Runner) Status() Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.snapshot()
}

func (r *Runner) snapshot() Status {
	s := r.status
	if s.Errors != nil {
		s.Errors = append([]string(nil), s.Errors...)
	}
	s.Schedule = r.sched.info(r.now())
	if r.sched.Enabled() && !r.nextAt.IsZero() {
		t := r.nextAt.UTC()
		s.NextAt = &t
	}
	return s
}

// Subscribe returns a channel of status updates and a function to stop it.
func (r *Runner) Subscribe() (<-chan Status, func()) {
	ch := make(chan Status, 8)
	r.mu.Lock()
	r.subs[ch] = struct{}{}
	r.mu.Unlock()
	return ch, func() {
		r.mu.Lock()
		delete(r.subs, ch)
		r.mu.Unlock()
	}
}

// publish sends the current status to subscribers. Call with r.mu held.
// Updates are throttled unless force is set.
func (r *Runner) publish(force bool) {
	now := time.Now()
	if !force && now.Sub(r.lastPub) < 250*time.Millisecond {
		return
	}
	r.lastPub = now
	s := r.snapshot()
	for ch := range r.subs {
		select {
		case ch <- s:
		default: // slow reader: it will get the next one
		}
	}
}

func (r *Runner) scan(ctx context.Context, why string) {
	start := r.now().UTC()
	r.mu.Lock()
	r.pending = false
	r.status = Status{Running: true, Trigger: why, StartedAt: &start, Last: r.status.Last}
	r.publish(true)
	r.mu.Unlock()

	total, errs := r.work(ctx)

	end := r.now().UTC()
	r.mu.Lock()
	r.status.Running, r.status.FinishedAt = false, &end
	r.status.Library, r.status.Current = "", ""
	r.status.Errors = errs
	r.status.Last = &total
	r.rearm(end)
	r.publish(true)
	r.mu.Unlock()
	r.log.Info("scan finished", "trigger", why, "took", end.Sub(start).Round(time.Millisecond),
		"courses", total.Courses, "items", total.Items, "added", total.ItemsAdded,
		"updated", total.ItemsUpdated, "moved", total.ItemsMoved, "missing", total.ItemsMissing,
		"subtitles_unmatched", total.SubsUnpaired)
}

// scanLibraries is the work of one scan: every library, then the clean-up.
func (r *Runner) scanLibraries(ctx context.Context) (Counts, []string) {
	var total Counts
	var errs []string
	libs, err := r.app.Store.ListLibraries(ctx, true)
	if err != nil {
		errs = append(errs, "listing libraries: "+err.Error())
	}
	for _, lib := range libs {
		if ctx.Err() != nil {
			break
		}
		r.mu.Lock()
		r.status.Library, r.status.Done, r.status.Total, r.status.Current = lib.Label, 0, 0, ""
		r.publish(true)
		r.mu.Unlock()

		c, err := r.app.ScanLibrary(ctx, lib, r.progress)
		total.Add(c)
		if err != nil && ctx.Err() == nil {
			r.log.Error("scan: library failed", "library", lib.Label, "err", err)
			errs = append(errs, fmt.Sprintf("%s: %v", lib.Label, err))
		}
	}
	if ctx.Err() == nil && r.retention > 0 {
		if n, err := r.app.Store.PurgeMissing(ctx, time.Now().Add(-r.retention)); err != nil {
			r.log.Warn("scan: purge failed", "err", err)
		} else if n > 0 {
			r.log.Info("scan: purged long-missing entries", "rows", n)
		}
	}
	r.fillFingerprints(ctx)
	return total, errs
}

func (r *Runner) progress(done, total int, current string) {
	r.mu.Lock()
	r.status.Done, r.status.Total, r.status.Current = done, total, current
	r.publish(done == total)
	r.mu.Unlock()
}

// fillFingerprints computes fingerprints for items that do not have one yet,
// a bounded batch per scan, so renames can be recognised later.
func (r *Runner) fillFingerprints(ctx context.Context) {
	deadline := time.Now().Add(2 * time.Minute)
	for batch := 0; batch < 20 && ctx.Err() == nil && time.Now().Before(deadline); batch++ {
		targets, err := r.app.Store.FingerprintTargets(ctx, 250)
		if err != nil || len(targets) == 0 {
			return
		}
		for _, t := range targets {
			if ctx.Err() != nil {
				return
			}
			fp := UnreadableFingerprint()
			if p, err := fsutil.ResolveUnder(t.LibraryPath, path.Join(t.CourseRel, t.ItemRel), r.app.Follow); err == nil {
				if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
					if got, err := Fingerprint(p, fi.Size()); err == nil {
						fp = got
					}
				}
			}
			if err := r.app.Store.SetFingerprint(ctx, t.ItemID, fp); err != nil {
				r.log.Warn("scan: storing fingerprint failed", "err", err)
				return
			}
		}
	}
}
