package api

import (
	"context"
	"testing"
	"time"
)

// The setup page lets whoever opens it create the administrator, so it is only
// open for a while after Lectern starts (LECTERN_SETUP_WINDOW). These tests
// move the server's start time instead of waiting.
func TestSetupWindow(t *testing.T) {
	e := newEnv(t)
	c := e.client()

	type setup struct {
		NeedsSetup  bool `json:"needs_setup"`
		SetupClosed bool `json:"setup_closed"`
	}
	status := func() setup {
		t.Helper()
		var s setup
		c.js("GET", "/api/v1/setup", nil, &s)
		return s
	}
	startedAgo := func(window, ago time.Duration) {
		e.srv.Cfg.SetupWindow = window
		e.srv.started = time.Now().Add(-ago)
	}
	users := func() int {
		n, err := e.st.CountUsers(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	create := map[string]string{"username": "ann", "password": "long enough pw"}

	startedAgo(15*time.Minute, 2*time.Minute)
	if s := status(); !s.NeedsSetup || s.SetupClosed {
		t.Fatalf("2 minutes into a 15 minute window: %+v", s)
	}
	startedAgo(time.Hour, 59*time.Minute)
	if s := status(); !s.NeedsSetup || s.SetupClosed {
		t.Fatalf("59 minutes into a one hour window: %+v", s)
	}

	startedAgo(15*time.Minute, 16*time.Minute)
	if s := status(); !s.NeedsSetup || !s.SetupClosed {
		t.Fatalf("after the window: %+v", s)
	}
	r := c.req("POST", "/api/v1/setup", create)
	expectStatus(t, r, 403, "setup_closed")
	if len(r.Cookies()) != 0 {
		t.Error("a refused setup must not sign anybody in")
	}
	if users() != 0 {
		t.Fatal("a closed setup page created an account")
	}

	// "forever" keeps it open however long Lectern has been running.
	startedAgo(0, 90*24*time.Hour)
	if s := status(); !s.NeedsSetup || s.SetupClosed {
		t.Fatalf("with no limit: %+v", s)
	}
	expectStatus(t, c.req("POST", "/api/v1/setup", create), 201, "")
	if users() != 1 {
		t.Fatal("setup should have created the administrator")
	}

	// With an account, the window no longer matters: nothing is "closed", and
	// the answer to another attempt is the usual one.
	startedAgo(15*time.Minute, 16*time.Minute)
	if s := status(); s.NeedsSetup || s.SetupClosed {
		t.Fatalf("after setup: %+v", s)
	}
	expectStatus(t, e.client().req("POST", "/api/v1/setup", map[string]string{"username": "eve", "password": "long enough pw"}), 409, "already_set_up")
}
