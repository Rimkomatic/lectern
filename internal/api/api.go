// Package api is Lectern's HTTP layer: JSON endpoints under /api/v1, media
// streaming with Range support, and the embedded web UI at /.
package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"lectern/internal/auth"
	"lectern/internal/config"
	"lectern/internal/scan"
	"lectern/internal/store"
)

type Server struct {
	Cfg     *config.Config
	Store   *store.Store
	Runner  *scan.Runner
	Log     *slog.Logger
	Static  http.Handler // the single-page app; may be nil in tests
	Version string

	login    *auth.Limiter
	shutdown chan struct{}
	once     sync.Once
}

func New(cfg *config.Config, st *store.Store, runner *scan.Runner, static http.Handler, version string, log *slog.Logger) *Server {
	return &Server{
		Cfg: cfg, Store: st, Runner: runner, Static: static, Version: version, Log: log,
		login:    auth.NewLimiter(8, 15*time.Minute),
		shutdown: make(chan struct{}),
	}
}

// Close ends long-lived responses (the admin event stream) so the HTTP
// server can shut down promptly. Register it with http.Server.RegisterOnShutdown.
func (s *Server) Close() { s.once.Do(func() { close(s.shutdown) }) }

// Handler builds the full route table.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.healthz)

	const v1 = "/api/v1"
	// public
	mux.HandleFunc("GET "+v1+"/setup", s.public(s.getSetup))
	mux.HandleFunc("POST "+v1+"/setup", s.public(s.postSetup))
	mux.HandleFunc("POST "+v1+"/auth/login", s.public(s.postLogin))
	mux.HandleFunc("POST "+v1+"/auth/logout", s.public(s.postLogout))

	// any signed-in user
	mux.HandleFunc("GET "+v1+"/me", s.authed(s.getMe))
	mux.HandleFunc("PATCH "+v1+"/me", s.authed(s.patchMe))
	mux.HandleFunc("PUT "+v1+"/me/password", s.authed(s.putPassword))
	mux.HandleFunc("GET "+v1+"/me/settings", s.authed(s.getSettings))
	mux.HandleFunc("PUT "+v1+"/me/settings", s.authed(s.putSettings))

	mux.HandleFunc("GET "+v1+"/courses", s.authed(s.listCourses))
	mux.HandleFunc("GET "+v1+"/courses/{id}", s.authed(s.getCourse))
	mux.HandleFunc("GET "+v1+"/courses/{id}/cover", s.authed(s.getCover))
	mux.HandleFunc("DELETE "+v1+"/courses/{id}/progress", s.authed(s.dropCourse))
	mux.HandleFunc("GET "+v1+"/continue", s.authed(s.getContinue))

	mux.HandleFunc("GET "+v1+"/items/{id}", s.authed(s.getItem))
	mux.HandleFunc("GET "+v1+"/items/{id}/stream", s.authed(s.streamItem))
	mux.HandleFunc("GET "+v1+"/items/{id}/download", s.authed(s.downloadItem))
	mux.HandleFunc("GET "+v1+"/items/{id}/asset", s.authed(s.getItemAsset))
	mux.HandleFunc("POST "+v1+"/items/{id}/progress", s.authed(s.postProgress))
	mux.HandleFunc("POST "+v1+"/items/{id}/chapters", s.authed(s.postChapters))
	mux.HandleFunc("GET "+v1+"/subtitles/{id}", s.authed(s.getSubtitle))

	// admins
	mux.HandleFunc("GET "+v1+"/admin/libraries", s.admin(s.adminLibraries))
	mux.HandleFunc("GET "+v1+"/admin/scan", s.admin(s.adminScanStatus))
	mux.HandleFunc("POST "+v1+"/admin/scan", s.admin(s.adminScanStart))
	mux.HandleFunc("GET "+v1+"/admin/events", s.admin(s.adminEvents))
	mux.HandleFunc("GET "+v1+"/admin/users", s.admin(s.adminListUsers))
	mux.HandleFunc("POST "+v1+"/admin/users", s.admin(s.adminCreateUser))
	mux.HandleFunc("PATCH "+v1+"/admin/users/{id}", s.admin(s.adminPatchUser))
	mux.HandleFunc("DELETE "+v1+"/admin/users/{id}", s.admin(s.adminDeleteUser))
	mux.HandleFunc("PUT "+v1+"/admin/courses/{id}/cover", s.admin(s.adminSetCover))
	mux.HandleFunc("DELETE "+v1+"/admin/courses/{id}/cover", s.admin(s.adminDeleteCover))

	// anything else under /api is a JSON 404, never the web page
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeAPIError(w, notFound())
	})
	if s.Static != nil {
		mux.Handle("/", s.Static)
	}

	var h http.Handler = mux
	h = s.originCheck(h)
	h = s.securityHeaders(h)
	h = s.logRequests(h)
	h = s.recoverer(h)
	return h
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.Store.DB().PingContext(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "database unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": s.Version})
}

func trimName(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > max {
		return string(r[:max])
	}
	return s
}
