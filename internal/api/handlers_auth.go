package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"lectern/internal/auth"
	"lectern/internal/store"
)

type meResponse struct {
	User     *store.User    `json:"user"`
	Settings store.Settings `json:"settings"`
}

func (s *Server) getSetup(w http.ResponseWriter, r *http.Request) error {
	n, err := s.Store.CountUsers(r.Context())
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"needs_setup": n == 0, "version": s.Version})
	return nil
}

// startSession creates a session row and sets the cookie.
func (s *Server) startSession(w http.ResponseWriter, r *http.Request, u *store.User) error {
	token, idHash, err := auth.NewToken()
	if err != nil {
		return err
	}
	exp := time.Now().Add(s.Cfg.SessionTTL)
	ip := auth.ClientIP(r, s.Cfg.TrustProxy)
	if err := s.Store.CreateSession(r.Context(), idHash, u.ID, exp, r.UserAgent(), ip); err != nil {
		return err
	}
	if err := s.Store.TouchLogin(r.Context(), u.ID); err != nil {
		s.Log.Warn("could not record login time", "err", err)
	}
	s.setCookie(w, r, token, exp)
	return nil
}

func (s *Server) postSetup(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
	}
	if err := decodeJSON(w, r, &in, 8<<10); err != nil {
		return err
	}
	in.Username = strings.TrimSpace(in.Username)
	if err := auth.ValidateUsername(in.Username); err != nil {
		return badRequest(err.Error())
	}
	if err := auth.ValidatePassword(in.Password); err != nil {
		return badRequest(err.Error())
	}
	if n, err := s.Store.CountUsers(r.Context()); err != nil {
		return err
	} else if n > 0 {
		return conflict("already_set_up", "setup has already been completed")
	}
	name := trimName(in.DisplayName, 64)
	if name == "" {
		name = in.Username
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return err
	}
	u, err := s.Store.CreateFirstAdmin(r.Context(), in.Username, name, hash)
	if errors.Is(err, store.ErrConflict) {
		return conflict("already_set_up", "setup has already been completed")
	}
	if err != nil {
		return err
	}
	if err := s.startSession(w, r, u); err != nil {
		return err
	}
	settings, _ := s.Store.GetSettings(r.Context(), u.ID)
	writeJSON(w, http.StatusCreated, meResponse{User: u, Settings: settings})
	return nil
}

func (s *Server) postLogin(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &in, 4<<10); err != nil {
		return err
	}
	in.Username = strings.TrimSpace(in.Username)
	if in.Username == "" || in.Password == "" || len(in.Username) > 64 || len(in.Password) > 512 {
		return &apiError{Status: http.StatusUnauthorized, Code: "invalid_credentials", Message: "wrong username or password"}
	}

	ip := auth.ClientIP(r, s.Cfg.TrustProxy)
	keys := []string{"ip:" + ip, "user:" + strings.ToLower(in.Username)}
	for _, k := range keys {
		if ok, wait := s.login.Allowed(k); !ok {
			secs := int(wait.Seconds()) + 1
			return &apiError{
				Status: http.StatusTooManyRequests, Code: "rate_limited",
				Message: "too many failed attempts; try again in a few minutes",
				Headers: map[string]string{"Retry-After": strconv.Itoa(secs)},
			}
		}
	}
	bad := &apiError{Status: http.StatusUnauthorized, Code: "invalid_credentials", Message: "wrong username or password"}

	u, err := s.Store.UserByUsername(r.Context(), in.Username)
	if errors.Is(err, store.ErrNotFound) {
		auth.BurnPassword(in.Password)
		for _, k := range keys {
			s.login.Fail(k)
		}
		return bad
	}
	if err != nil {
		return err
	}
	ok, rehash := auth.VerifyPassword(u.PwHash, in.Password)
	if !ok {
		for _, k := range keys {
			s.login.Fail(k)
		}
		return bad
	}
	if u.Disabled {
		return &apiError{Status: http.StatusForbidden, Code: "account_disabled", Message: "this account has been disabled"}
	}
	s.login.Reset(keys[1])
	if rehash {
		if h, err := auth.HashPassword(in.Password); err == nil {
			if err := s.Store.SetPassword(r.Context(), u.ID, h); err != nil {
				s.Log.Warn("could not upgrade password hash", "err", err)
			}
		}
	}
	if err := s.startSession(w, r, u); err != nil {
		return err
	}
	settings, err := s.Store.GetSettings(r.Context(), u.ID)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, meResponse{User: u, Settings: settings})
	return nil
}

func (s *Server) postLogout(w http.ResponseWriter, r *http.Request) error {
	if c, err := r.Cookie(cookieName); err == nil {
		if id, ok := auth.HashToken(c.Value); ok {
			if err := s.Store.DeleteSession(r.Context(), id); err != nil {
				return err
			}
		}
	}
	s.clearCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) getMe(w http.ResponseWriter, r *http.Request, a *authCtx) error {
	settings, err := s.Store.GetSettings(r.Context(), a.User.ID)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, meResponse{User: a.User, Settings: settings})
	return nil
}

func (s *Server) patchMe(w http.ResponseWriter, r *http.Request, a *authCtx) error {
	var in struct {
		DisplayName *string `json:"display_name"`
	}
	if err := decodeJSON(w, r, &in, 4<<10); err != nil {
		return err
	}
	if in.DisplayName == nil {
		return badRequest("nothing to change")
	}
	name := trimName(*in.DisplayName, 64)
	if name == "" {
		return badRequest("display name cannot be empty")
	}
	u, err := s.Store.UpdateUser(r.Context(), a.User.ID, store.UserPatch{DisplayName: &name})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, u)
	return nil
}

func (s *Server) putPassword(w http.ResponseWriter, r *http.Request, a *authCtx) error {
	var in struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
	}
	if err := decodeJSON(w, r, &in, 4<<10); err != nil {
		return err
	}
	key := "pw:" + strconv.FormatUint(a.User.ID, 10)
	if ok, wait := s.login.Allowed(key); !ok {
		return &apiError{
			Status: http.StatusTooManyRequests, Code: "rate_limited",
			Message: "too many failed attempts; try again later",
			Headers: map[string]string{"Retry-After": strconv.Itoa(int(wait.Seconds()) + 1)},
		}
	}
	if ok, _ := auth.VerifyPassword(a.User.PwHash, in.Current); !ok {
		s.login.Fail(key)
		return &apiError{Status: http.StatusForbidden, Code: "wrong_password", Message: "the current password is wrong"}
	}
	s.login.Reset(key)
	if err := auth.ValidatePassword(in.New); err != nil {
		return badRequest(err.Error())
	}
	hash, err := auth.HashPassword(in.New)
	if err != nil {
		return err
	}
	if err := s.Store.SetPassword(r.Context(), a.User.ID, hash); err != nil {
		return err
	}
	// Changing the password signs out every other device.
	if err := s.Store.DeleteOtherUserSessions(r.Context(), a.User.ID, a.IDHash); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request, a *authCtx) error {
	st, err := s.Store.GetSettings(r.Context(), a.User.ID)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, st)
	return nil
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request, a *authCtx) error {
	st, err := s.Store.GetSettings(r.Context(), a.User.ID) // start from what is stored: partial updates are fine
	if err != nil {
		return err
	}
	if err := decodeJSON(w, r, &st, 4<<10); err != nil {
		return err
	}
	if err := st.Validate(); err != nil {
		return badRequest(err.Error())
	}
	if err := s.Store.PutSettings(r.Context(), a.User.ID, st); err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, st)
	return nil
}
