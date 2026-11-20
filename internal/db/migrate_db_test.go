package db_test

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"lectern/internal/db"
	"lectern/internal/testdb"
)

// A database that a newer Lectern has migrated must make an older one stop
// before it touches anything.
func TestMigrateRefusesANewerDatabase(t *testing.T) {
	conn, cfg := testdb.Open(t)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	if err := db.Migrate(ctx, cfg, log); err != nil {
		t.Fatalf("migrating a database that is up to date must work, twice: %v", err)
	}
	count := func() (n int) {
		if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&n); err != nil {
			t.Fatal(err)
		}
		return
	}
	before := count()

	if _, err := conn.ExecContext(ctx,
		"INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)",
		999, "0999_from_the_future.sql", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	err := db.Migrate(ctx, cfg, log)
	if err == nil || !strings.Contains(err.Error(), "newer version of Lectern") || !strings.Contains(err.Error(), "999") {
		t.Fatalf("want a refusal that names the newer schema, got %v", err)
	}
	if got := count(); got != before+1 {
		t.Fatalf("a refused start must not change anything: %d rows, want %d", got, before+1)
	}
}
