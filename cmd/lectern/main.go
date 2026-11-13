// Command lectern is a self-hosted course server: point it at folders of
// videos, PDFs and files and it turns them into courses with per-user progress.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata" // zone rules inside the binary: the image has no zoneinfo files, and TZ=Europe/Berlin must work

	"lectern/internal/api"
	"lectern/internal/auth"
	"lectern/internal/config"
	"lectern/internal/db"
	"lectern/internal/scan"
	"lectern/internal/store"
	"lectern/internal/webui"
)

// version is set at build time: -ldflags "-X main.version=1.2.3".
var version = "dev"

func main() {
	healthcheck := flag.Bool("healthcheck", false, "probe the local /healthz endpoint and exit (for container health checks)")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}
	if err := config.LoadDotEnv(".env"); err != nil {
		fmt.Fprintln(os.Stderr, "reading .env:", err)
		os.Exit(2)
	}
	if *healthcheck {
		os.Exit(runHealthcheck())
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "configuration error:", err)
		os.Exit(2)
	}
	log := newLogger(cfg.LogLevel)
	if err := run(cfg, log); err != nil {
		log.Error("lectern stopped", "err", err)
		os.Exit(1)
	}
}

func newLogger(level string) *slog.Logger {
	var l slog.Level
	switch level {
	case "debug":
		l = slog.LevelDebug
	case "warn", "warning":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: l}))
}

func run(cfg *config.Config, log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log.Info("starting lectern", "version", version)

	conn, err := db.Open(ctx, cfg.DB, cfg.DBWait, log)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer conn.Close()
	if err := db.Migrate(ctx, cfg.DB, log); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	st := store.New(conn)

	specs := make([]store.LibrarySpec, len(cfg.Libraries))
	for i, l := range cfg.Libraries {
		specs[i] = store.LibrarySpec{Label: l.Label, Path: l.Path}
		if fi, err := os.Stat(l.Path); err != nil || !fi.IsDir() {
			log.Warn("library folder is not available yet", "library", l.Label, "path", l.Path, "err", err)
		}
	}
	if err := st.SyncLibraries(ctx, specs); err != nil {
		return fmt.Errorf("libraries: %w", err)
	}
	if err := bootstrapAdmin(ctx, cfg, st, log); err != nil {
		return err
	}

	app := &scan.Applier{Store: st, Log: log, Follow: cfg.FollowSymlinks}
	sched := scan.Schedule{Daily: cfg.ScanDaily, Hour: cfg.ScanAtHour, Minute: cfg.ScanAtMinute, Every: cfg.ScanInterval}
	runner := scan.NewRunner(app, sched, cfg.MissingRetention, log)
	log.Info("automatic scans", "schedule", sched.String())
	go runner.Run(ctx)
	go purgeSessions(ctx, st, log)

	srv := api.New(cfg, st, runner, webui.Handler(), version, log)
	httpSrv := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: srv.Handler(),
		// No ReadTimeout/WriteTimeout: they would cut off long video streams.
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}
	httpSrv.RegisterOnShutdown(srv.Close)

	errc := make(chan error, 1)
	go func() { errc <- httpSrv.ListenAndServe() }()
	log.Info("listening", "addr", cfg.ListenAddr, "libraries", len(cfg.Libraries))

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		_ = httpSrv.Close()
	}
	return nil
}

// bootstrapAdmin creates the first admin from LECTERN_ADMIN_USER/PASSWORD when
// the database has no users yet. Otherwise the web setup page does it.
func bootstrapAdmin(ctx context.Context, cfg *config.Config, st *store.Store, log *slog.Logger) error {
	if cfg.AdminUser == "" && cfg.AdminPassword == "" {
		return nil
	}
	n, err := st.CountUsers(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	if cfg.AdminUser == "" || cfg.AdminPassword == "" {
		return errors.New("set both LECTERN_ADMIN_USER and LECTERN_ADMIN_PASSWORD, or neither")
	}
	if err := auth.ValidateUsername(cfg.AdminUser); err != nil {
		return fmt.Errorf("LECTERN_ADMIN_USER: %w", err)
	}
	if err := auth.ValidatePassword(cfg.AdminPassword); err != nil {
		return fmt.Errorf("LECTERN_ADMIN_PASSWORD: %w", err)
	}
	hash, err := auth.HashPassword(cfg.AdminPassword)
	if err != nil {
		return err
	}
	if _, err := st.CreateFirstAdmin(ctx, cfg.AdminUser, cfg.AdminUser, hash); err != nil && !errors.Is(err, store.ErrConflict) {
		return err
	}
	log.Info("created the first admin from the environment", "user", cfg.AdminUser)
	return nil
}

func purgeSessions(ctx context.Context, st *store.Store, log *slog.Logger) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if n, err := st.PurgeExpiredSessions(ctx); err != nil {
				log.Warn("could not purge expired sessions", "err", err)
			} else if n > 0 {
				log.Debug("purged expired sessions", "count", n)
			}
		}
	}
}

// runHealthcheck asks the locally running server for /healthz. It needs only
// LECTERN_LISTEN, so it works in a minimal container health check.
func runHealthcheck() int {
	addr := os.Getenv("LECTERN_LISTEN")
	if addr == "" {
		addr = ":8080"
	}
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		if strings.HasPrefix(addr, ":") {
			port = strings.TrimPrefix(addr, ":")
		} else {
			fmt.Fprintln(os.Stderr, "bad LECTERN_LISTEN:", err)
			return 1
		}
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://" + net.JoinHostPort("127.0.0.1", port) + "/healthz")
	if err != nil {
		fmt.Fprintln(os.Stderr, "unhealthy:", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "unhealthy: status", resp.StatusCode)
		return 1
	}
	return 0
}
