// Package testdb gives tests a clean, migrated MySQL database.
//
// It reads LECTERN_TEST_DB_HOST / _PORT / _NAME / _USER / _PASSWORD. The
// defaults (a database, a user and a password that are all called
// "lectern_test" on 127.0.0.1:3306) are what `make db-test` creates inside the
// docker-compose MySQL. If MySQL cannot be reached the test is skipped, unless
// LECTERN_TEST_REQUIRE_DB=1 is set, in which case it fails. Every test empties
// the database first, so only databases with "test" in their name are
// accepted. Tests that use it must not run in parallel with other packages:
// run `go test -p 1 ./...` (the Makefile does).
package testdb

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"lectern/internal/config"
	"lectern/internal/db"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// Config is the connection used by tests.
func Config() config.DB {
	port, _ := strconv.Atoi(env("LECTERN_TEST_DB_PORT", "3306"))
	return config.DB{
		Host:     env("LECTERN_TEST_DB_HOST", "127.0.0.1"),
		Port:     port,
		Name:     env("LECTERN_TEST_DB_NAME", "lectern_test"),
		User:     env("LECTERN_TEST_DB_USER", "lectern_test"),
		Password: env("LECTERN_TEST_DB_PASSWORD", "lectern_test"),
	}
}

// Open drops everything in the test database, applies the migrations and
// returns a connection pool.
func Open(t testing.TB) (*sql.DB, config.DB) {
	t.Helper()
	cfg := Config()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := db.Open(ctx, cfg, 3*time.Second, log)
	if err != nil {
		if os.Getenv("LECTERN_TEST_REQUIRE_DB") == "1" {
			t.Fatalf("MySQL not reachable: %v", err)
		}
		t.Skipf("MySQL not reachable (%v); start docker compose or set LECTERN_TEST_DB_*", err)
	}
	t.Cleanup(func() { conn.Close() })

	if err := Wipe(ctx, conn); err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(ctx, cfg, log); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return conn, cfg
}

// checkWipeable refuses database names that do not look like a throw-away
// test database, so a mistyped LECTERN_TEST_DB_NAME (or the real MYSQL_DATABASE
// copied from .env) can never lose data.
func checkWipeable(name string) error {
	if !strings.Contains(strings.ToLower(name), "test") {
		return fmt.Errorf("refusing to empty database %q: only databases with \"test\" in their name may be wiped", name)
	}
	return nil
}

// Wipe drops every table in the database the pool is connected to. It is what
// makes each test (and the browser smoke test in e2e/) start from nothing, so
// it only works on databases with "test" in their name.
func Wipe(ctx context.Context, conn *sql.DB) error {
	c, err := conn.Conn(ctx)
	if err != nil {
		return err
	}
	defer c.Close()
	var name sql.NullString
	if err := c.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&name); err != nil {
		return err
	}
	if err := checkWipeable(name.String); err != nil {
		return err
	}
	if _, err := c.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 0"); err != nil {
		return err
	}
	rows, err := c.QueryContext(ctx, "SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE()")
	if err != nil {
		return err
	}
	var tables []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			return err
		}
		tables = append(tables, n)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, n := range tables {
		if _, err := c.ExecContext(ctx, "DROP TABLE `"+n+"`"); err != nil {
			return err
		}
	}
	_, err = c.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 1")
	return err
}
