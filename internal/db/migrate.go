package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"lectern/internal/config"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

type migration struct {
	version int
	name    string
	sql     string
}

func loadMigrations() ([]migration, error) {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return nil, err
	}
	var out []migration
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}
		num, _, ok := strings.Cut(name, "_")
		if !ok {
			return nil, fmt.Errorf("migration %q: expected NNNN_name.sql", name)
		}
		v, err := strconv.Atoi(num)
		if err != nil {
			return nil, fmt.Errorf("migration %q: %w", name, err)
		}
		b, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return nil, err
		}
		out = append(out, migration{version: v, name: name, sql: string(b)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}

// Migrate applies pending migrations. A MySQL advisory lock serialises
// concurrent starts. DDL is not transactional in MySQL, so each migration file
// should be small and re-runnable (CREATE TABLE IF NOT EXISTS, ...).
func Migrate(ctx context.Context, c config.DB, log *slog.Logger) error {
	dsn, err := DSN(c, true)
	if err != nil {
		return err
	}
	mdb, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	defer mdb.Close()

	conn, err := mdb.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	var got sql.NullInt64
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK('lectern_migrate', 60)").Scan(&got); err != nil {
		return fmt.Errorf("migration lock: %w", err)
	}
	if !got.Valid || got.Int64 != 1 {
		return fmt.Errorf("could not acquire migration lock")
	}
	defer func() {
		_, _ = conn.ExecContext(context.Background(), "SELECT RELEASE_LOCK('lectern_migrate')")
	}()

	if _, err := conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INT NOT NULL,
		name       VARCHAR(255) NOT NULL,
		applied_at DATETIME(3) NOT NULL,
		PRIMARY KEY (version)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
		return err
	}

	applied := map[int]bool{}
	rows, err := conn.QueryContext(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return err
	}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		applied[v] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	migs, err := loadMigrations()
	if err != nil {
		return err
	}
	for _, m := range migs {
		if applied[m.version] {
			continue
		}
		log.Info("applying migration", "name", m.name)
		if _, err := conn.ExecContext(ctx, m.sql); err != nil {
			return fmt.Errorf("migration %s: %w", m.name, err)
		}
		if _, err := conn.ExecContext(ctx,
			"INSERT INTO schema_migrations (version, name, applied_at) VALUES (?,?,?)",
			m.version, m.name, time.Now().UTC()); err != nil {
			return err
		}
	}
	return nil
}
