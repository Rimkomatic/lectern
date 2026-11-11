// Package db opens the MySQL connection pool and applies embedded migrations.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"time"

	"github.com/go-sql-driver/mysql"

	"lectern/internal/config"
)

// DSN builds a go-sql-driver DSN with the settings the app depends on
// (UTC session time zone, utf8mb4, parseTime). multi enables multi-statement
// execution and is only used for migrations.
func DSN(c config.DB, multi bool) (string, error) {
	var cfg *mysql.Config
	if c.DSN != "" {
		var err error
		if cfg, err = mysql.ParseDSN(c.DSN); err != nil {
			return "", fmt.Errorf("parse LECTERN_DB_DSN: %w", err)
		}
	} else {
		cfg = mysql.NewConfig()
		cfg.User = c.User
		cfg.Passwd = c.Password
		cfg.Net = "tcp"
		cfg.Addr = net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
		cfg.DBName = c.Name
	}
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	cfg.Collation = "utf8mb4_unicode_ci"
	cfg.MultiStatements = multi
	if cfg.Timeout == 0 {
		cfg.Timeout = 5 * time.Second
	}
	if cfg.ReadTimeout == 0 {
		cfg.ReadTimeout = 60 * time.Second
	}
	if cfg.WriteTimeout == 0 {
		cfg.WriteTimeout = 60 * time.Second
	}
	if cfg.Params == nil {
		cfg.Params = map[string]string{}
	}
	cfg.Params["time_zone"] = "'+00:00'"
	return cfg.FormatDSN(), nil
}

// Open connects to MySQL, retrying until it answers or wait elapses. This lets
// the app start before the database container is ready.
func Open(ctx context.Context, c config.DB, wait time.Duration, log *slog.Logger) (*sql.DB, error) {
	dsn, err := DSN(c, false)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	deadline := time.Now().Add(wait)
	for {
		pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err = db.PingContext(pctx)
		cancel()
		if err == nil {
			return db, nil
		}
		if ctx.Err() != nil || time.Now().After(deadline) {
			_ = db.Close()
			return nil, fmt.Errorf("connect to mysql: %w", err)
		}
		log.Warn("waiting for mysql", "err", err)
		select {
		case <-ctx.Done():
			_ = db.Close()
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}
