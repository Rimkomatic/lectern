// Package config loads runtime settings from environment variables (and an
// optional .env file, see LoadDotEnv). Names that the MySQL container also
// understands (MYSQL_*) are shared so one .env file serves both.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Library struct {
	Label string
	Path  string
}

type DB struct {
	DSN      string // optional full go-sql-driver DSN, overrides the fields below
	Host     string
	Port     int
	Name     string
	User     string
	Password string
}

type Config struct {
	ListenAddr       string
	DB               DB
	DBWait           time.Duration
	Libraries        []Library
	ScanInterval     time.Duration // 0 disables periodic scans (boot + manual only)
	MissingRetention time.Duration // how long vanished items keep their progress
	SessionTTL       time.Duration
	CookieSecure     string // auto | true | false
	TrustProxy       bool   // honour X-Forwarded-* headers
	AllowedOrigins   []string
	FollowSymlinks   bool
	DoneThreshold    float64 // fraction of a video/audio that counts as finished
	AdminUser        string
	AdminPassword    string
	LogLevel         string
}

func Load() (*Config, error) {
	c := &Config{}
	var err error

	c.ListenAddr = env("LECTERN_LISTEN", ":8080")
	c.DB = DB{
		DSN:      env("LECTERN_DB_DSN", ""),
		Host:     env("MYSQL_HOST", "127.0.0.1"),
		Name:     env("MYSQL_DATABASE", "lectern"),
		User:     env("MYSQL_USER", "lectern"),
		Password: env("MYSQL_PASSWORD", ""),
	}
	if c.DB.Port, err = envInt("MYSQL_PORT", 3306); err != nil {
		return nil, err
	}
	if c.DBWait, err = envDuration("LECTERN_DB_WAIT", 60*time.Second); err != nil {
		return nil, err
	}
	if c.DB.DSN == "" && c.DB.Password == "" {
		return nil, errors.New("MYSQL_PASSWORD (or LECTERN_DB_DSN) must be set")
	}

	if c.Libraries, err = ParseLibraries(env("LECTERN_LIBRARIES", "/library")); err != nil {
		return nil, err
	}
	if c.ScanInterval, err = envDuration("LECTERN_SCAN_INTERVAL", 15*time.Minute); err != nil {
		return nil, err
	}
	if c.MissingRetention, err = envDuration("LECTERN_MISSING_RETENTION", 30*24*time.Hour); err != nil {
		return nil, err
	}
	if c.SessionTTL, err = envDuration("LECTERN_SESSION_TTL", 30*24*time.Hour); err != nil {
		return nil, err
	}
	if c.SessionTTL <= 0 {
		return nil, errors.New("LECTERN_SESSION_TTL must be positive")
	}

	c.CookieSecure = strings.ToLower(env("LECTERN_COOKIE_SECURE", "auto"))
	switch c.CookieSecure {
	case "auto", "true", "false":
	default:
		return nil, fmt.Errorf("LECTERN_COOKIE_SECURE must be auto, true or false (got %q)", c.CookieSecure)
	}
	c.TrustProxy = envBool("LECTERN_TRUST_PROXY", false)
	for _, o := range strings.Split(env("LECTERN_ALLOWED_ORIGINS", ""), ",") {
		if o = strings.TrimSpace(o); o != "" {
			c.AllowedOrigins = append(c.AllowedOrigins, o)
		}
	}
	c.FollowSymlinks = envBool("LECTERN_FOLLOW_SYMLINKS", false)

	th := env("LECTERN_DONE_THRESHOLD", "0.95")
	if c.DoneThreshold, err = strconv.ParseFloat(th, 64); err != nil || c.DoneThreshold < 0.5 || c.DoneThreshold > 1 {
		return nil, fmt.Errorf("LECTERN_DONE_THRESHOLD must be between 0.5 and 1 (got %q)", th)
	}

	c.AdminUser = env("LECTERN_ADMIN_USER", "")
	c.AdminPassword = env("LECTERN_ADMIN_PASSWORD", "")
	c.LogLevel = strings.ToLower(env("LECTERN_LOG_LEVEL", "info"))
	return c, nil
}

// ParseLibraries parses "label=/path;/other/path". The label is optional.
func ParseLibraries(s string) ([]Library, error) {
	var out []Library
	seen := map[string]bool{}
	for _, part := range strings.Split(s, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		label, p := "", part
		if i := strings.Index(part, "="); i > 0 && !strings.ContainsAny(part[:i], `/\`) {
			label, p = strings.TrimSpace(part[:i]), strings.TrimSpace(part[i+1:])
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, fmt.Errorf("library path %q: %w", p, err)
		}
		if seen[abs] {
			continue
		}
		seen[abs] = true
		if label == "" {
			label = filepath.Base(abs)
			if label == "/" || label == "." || label == string(filepath.Separator) {
				label = "Library"
			}
		}
		out = append(out, Library{Label: label, Path: abs})
	}
	if len(out) == 0 {
		return nil, errors.New("LECTERN_LIBRARIES is empty")
	}
	return out, nil
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return def
}

func envInt(key string, def int) (int, error) {
	v := env(key, "")
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return n, nil
}

func envBool(key string, def bool) bool {
	switch strings.ToLower(env(key, "")) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return def
}

func envDuration(key string, def time.Duration) (time.Duration, error) {
	v := env(key, "")
	if v == "" {
		return def, nil
	}
	d, err := ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return d, nil
}

// ParseDuration is time.ParseDuration plus "d" (days) and the words
// 0 / off / disabled / never, which all mean zero.
func ParseDuration(s string) (time.Duration, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "0", "off", "disabled", "never", "false":
		return 0, nil
	}
	if strings.HasSuffix(s, "d") {
		n, err := strconv.ParseFloat(strings.TrimSuffix(s, "d"), 64)
		if err != nil {
			return 0, fmt.Errorf("invalid duration %q", s)
		}
		return time.Duration(n * float64(24*time.Hour)), nil
	}
	return time.ParseDuration(s)
}
