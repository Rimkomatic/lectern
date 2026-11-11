package api

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
	"time"

	"lectern/internal/auth"
	"lectern/internal/store"
)

const cookieName = "lectern_session"

// authCtx is what an authenticated handler gets to see.
type authCtx struct {
	User   *store.User
	IDHash []byte // hashed session token, used by logout and password change
}

type handlerFunc func(w http.ResponseWriter, r *http.Request) error
type authedFunc func(w http.ResponseWriter, r *http.Request, a *authCtx) error

func (s *Server) public(h handlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			s.fail(w, r, err)
		}
	}
}

// authed requires a valid session.
func (s *Server) authed(h authedFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a, err := s.authenticate(w, r)
		if err == nil {
			err = h(w, r, a)
		}
		if err != nil {
			s.fail(w, r, err)
		}
	}
}

// admin requires a valid session of an admin.
func (s *Server) admin(h authedFunc) http.HandlerFunc {
	return s.authed(func(w http.ResponseWriter, r *http.Request, a *authCtx) error {
		if a.User.Role != "admin" {
			return errForbidden
		}
		return h(w, r, a)
	})
}

func (s *Server) authenticate(w http.ResponseWriter, r *http.Request) (*authCtx, error) {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return nil, errUnauthorized
	}
	idHash, ok := auth.HashToken(c.Value)
	if !ok {
		s.clearCookie(w, r)
		return nil, errUnauthorized
	}
	sess, user, err := s.Store.SessionUser(r.Context(), idHash)
	if errors.Is(err, store.ErrNotFound) {
		s.clearCookie(w, r)
		return nil, errUnauthorized
	}
	if err != nil {
		return nil, err
	}
	// Sliding expiry, written at most every ten minutes.
	if now := time.Now(); now.Sub(sess.LastSeenAt) > 10*time.Minute {
		exp := now.Add(s.Cfg.SessionTTL)
		if err := s.Store.TouchSession(r.Context(), idHash, exp); err != nil {
			s.Log.Warn("could not extend session", "err", err)
		} else {
			s.setCookie(w, r, c.Value, exp)
		}
	}
	return &authCtx{User: user, IDHash: idHash}, nil
}

func (s *Server) secureCookie(r *http.Request) bool {
	switch s.Cfg.CookieSecure {
	case "true":
		return true
	case "false":
		return false
	}
	if r.TLS != nil {
		return true
	}
	return s.Cfg.TrustProxy && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func (s *Server) setCookie(w http.ResponseWriter, r *http.Request, token string, exp time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		Expires:  exp,
		MaxAge:   int(time.Until(exp).Seconds()),
		HttpOnly: true,
		Secure:   s.secureCookie(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: "", Path: "/", MaxAge: -1, Expires: time.Unix(0, 0),
		HttpOnly: true, Secure: s.secureCookie(r), SameSite: http.SameSiteLaxMode,
	})
}

// ---- global middleware ----

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				s.Log.Error("panic in handler", "path", r.URL.Path, "panic", v, "stack", string(debug.Stack()))
				writeAPIError(w, &apiError{Status: http.StatusInternalServerError, Code: "internal", Message: "something went wrong"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += int64(n)
	return n, err
}

// Unwrap lets http.ResponseController reach the real writer (Flush, deadlines).
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		level := slog.LevelDebug
		switch {
		case sw.status >= 500:
			level = slog.LevelError
		case sw.status >= 400 && sw.status != 401 && sw.status != 404 && sw.status != 416:
			level = slog.LevelInfo
		}
		s.Log.Log(r.Context(), level, "request",
			"method", r.Method, "path", r.URL.Path, "status", sw.status,
			"bytes", sw.bytes, "ms", time.Since(start).Milliseconds())
	})
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		next.ServeHTTP(w, r)
	})
}

// originCheck is the CSRF defence for cookie sessions: a state-changing
// request made by a browser must come from this site (or an allowed origin).
// Requests without browser headers (curl, scripts) are not a CSRF vector.
func (s *Server) originCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		if !s.sameSiteRequest(r) {
			writeAPIError(w, &apiError{Status: http.StatusForbidden, Code: "cross_site", Message: "cross-site request blocked"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) sameSiteRequest(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin != "" {
		return s.originAllowed(r, origin)
	}
	switch r.Header.Get("Sec-Fetch-Site") {
	case "", "same-origin", "none":
		return true
	}
	return false
}

func (s *Server) originAllowed(r *http.Request, origin string) bool {
	if origin == "null" {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	host := r.Host
	if s.Cfg.TrustProxy {
		if fh := r.Header.Get("X-Forwarded-Host"); fh != "" {
			host = strings.TrimSpace(strings.Split(fh, ",")[0])
		}
	}
	if strings.EqualFold(u.Host, host) {
		return true
	}
	for _, a := range s.Cfg.AllowedOrigins {
		if strings.EqualFold(strings.TrimRight(a, "/"), origin) {
			return true
		}
	}
	return false
}
