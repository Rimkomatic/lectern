package main

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"lectern/internal/config"
	"lectern/internal/store"
	"lectern/internal/testdb"
)

func TestDescribeDuration(t *testing.T) {
	for in, want := range map[time.Duration]string{
		15 * time.Minute:        "15 minutes",
		time.Minute:             "1 minute",
		time.Hour:               "1 hour",
		2 * time.Hour:           "2 hours",
		36 * time.Hour:          "36 hours",
		24 * time.Hour:          "1 day",
		48 * time.Hour:          "2 days",
		90 * time.Second:        "1m30s",
		45 * time.Second:        "45s",
		90 * time.Minute:        "90 minutes",
		1500 * time.Millisecond: "2s",
	} {
		if got := describeDuration(in); got != want {
			t.Errorf("describeDuration(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestAnnounceSetup(t *testing.T) {
	conn, _ := testdb.Open(t)
	st, ctx := store.New(conn), context.Background()

	announce := func(window time.Duration) string {
		var buf bytes.Buffer
		log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
		announceSetup(ctx, &config.Config{SetupWindow: window}, st, log)
		return buf.String()
	}

	out := announce(15 * time.Minute)
	if !strings.Contains(out, "level=WARN") || !strings.Contains(out, "within 15 minutes") || !strings.Contains(out, "restarted") {
		t.Errorf("a limited window: %q", out)
	}
	out = announce(0)
	if !strings.Contains(out, "level=WARN") || !strings.Contains(out, "anyone who can reach") {
		t.Errorf("no limit: %q", out)
	}

	if _, err := st.CreateUser(ctx, "ann", "Ann", "x", "admin"); err != nil {
		t.Fatal(err)
	}
	if out := announce(15 * time.Minute); out != "" {
		t.Errorf("once there is an account the log stays quiet: %q", out)
	}
}
