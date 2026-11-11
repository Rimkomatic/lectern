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

// Status is what the admin page shows about scanning.
type Status struct {
	Running    bool       `json:"running"`
	Trigger    string     `json:"trigger,omitempty"` // startup | schedule | manual
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Library    string     `json:"library,omitempty"`
	Done       int        `json:"done"`
	Total      int        `json:"total"`
	Current    string     `json:"current,omitempty"`
	Errors     []string   `json:"errors,omitempty"`
	Last       *Counts    `json:"last,omitempty"` // result of the latest finished scan
}

// Runner scans all libraries once at start, then on a timer and on demand.
// Only one scan runs at a time.
type Runner struct {
	app       *Applier
	interval  time.Duration
	retention time.Duration
	log       *slog.Logger

	mu      sync.Mutex
	status  Status
	pending bool
	subs    map[chan Status]struct{}
	lastPub time.Time
	trigger chan string
}

// NewRunner creates a runner. interval <= 0 disables the timer (scans still
// happen at startup and on demand); retention <= 0 keeps missing items forever.
func NewRunner(app *Applier, interval, retention time.Duration, log *slog.Logger) *Runner {
	if log == nil {
		log = app.logger()
	}
	return &Runner{
		app: app, interval: interval, retention: retention, log: log,
		subs:    map[chan Status]struct{}{},
		trigger: make(chan string, 1),
	}
}

// Run blocks until ctx ends. Call it in its own goroutine.
func (r *Runner) Run(ctx context.Context) {
	var tick <-chan time.Time
	if r.interval > 0 {
		t := time.NewTicker(r.interval)
		defer t.Stop()
		tick = t.C
	}
	r.scan(ctx, "startup")
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick:
			r.scan(ctx, "schedule")
		case why := <-r.trigger:
			r.scan(ctx, why)
		}
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
	start := time.Now().UTC()
	r.mu.Lock()
	r.pending = false
	r.status = Status{Running: true, Trigger: why, StartedAt: &start, Last: r.status.Last}
	r.publish(true)
	r.mu.Unlock()

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

	end := time.Now().UTC()
	r.mu.Lock()
	r.status.Running, r.status.FinishedAt = false, &end
	r.status.Library, r.status.Current = "", ""
	r.status.Errors = errs
	r.status.Last = &total
	r.publish(true)
	r.mu.Unlock()
	r.log.Info("scan finished", "trigger", why, "took", end.Sub(start).Round(time.Millisecond),
		"courses", total.Courses, "items", total.Items, "added", total.ItemsAdded,
		"updated", total.ItemsUpdated, "moved", total.ItemsMoved, "missing", total.ItemsMissing)
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
