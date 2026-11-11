package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"lectern/internal/auth"
	"lectern/internal/scan"
	"lectern/internal/store"
)

func (s *Server) adminLibraries(w http.ResponseWriter, r *http.Request, a *authCtx) error {
	libs, err := s.Store.ListLibraries(r.Context(), false)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, emptyIfNil(libs))
	return nil
}

func (s *Server) adminScanStatus(w http.ResponseWriter, r *http.Request, a *authCtx) error {
	writeJSON(w, http.StatusOK, s.Runner.Status())
	return nil
}

func (s *Server) adminScanStart(w http.ResponseWriter, r *http.Request, a *authCtx) error {
	if !s.Runner.Trigger() {
		return conflict("scan_running", "a scan is already running")
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"started": true})
	return nil
}

// adminEvents streams scan status as server-sent events.
func (s *Server) adminEvents(w http.ResponseWriter, r *http.Request, a *authCtx) error {
	rc := http.NewResponseController(w)
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no") // do not buffer behind nginx
	w.WriteHeader(http.StatusOK)

	ch, cancel := s.Runner.Subscribe()
	defer cancel()
	send := func(st scan.Status) error {
		b, err := json.Marshal(st)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "event: scan\ndata: %s\n\n", b); err != nil {
			return err
		}
		return rc.Flush()
	}
	if err := send(s.Runner.Status()); err != nil {
		return nil
	}
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return nil
		case <-s.shutdown:
			return nil
		case st := <-ch:
			if send(st) != nil {
				return nil
			}
		case <-ping.C:
			if _, err := io.WriteString(w, ": ping\n\n"); err != nil {
				return nil
			}
			if rc.Flush() != nil {
				return nil
			}
		}
	}
}

// ---- users ----

func (s *Server) adminListUsers(w http.ResponseWriter, r *http.Request, a *authCtx) error {
	users, err := s.Store.ListUsers(r.Context())
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, emptyIfNil(users))
	return nil
}

func validRole(r string) bool { return r == "admin" || r == "user" }

func (s *Server) adminCreateUser(w http.ResponseWriter, r *http.Request, a *authCtx) error {
	var in struct {
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Password    string `json:"password"`
		Role        string `json:"role"`
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
	if in.Role == "" {
		in.Role = "user"
	}
	if !validRole(in.Role) {
		return badRequest("role must be admin or user")
	}
	name := trimName(in.DisplayName, 64)
	if name == "" {
		name = in.Username
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return err
	}
	u, err := s.Store.CreateUser(r.Context(), in.Username, name, hash, in.Role)
	if errors.Is(err, store.ErrConflict) {
		return conflict("username_taken", "that username is already taken")
	}
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, u)
	return nil
}

func (s *Server) adminPatchUser(w http.ResponseWriter, r *http.Request, a *authCtx) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		DisplayName *string `json:"display_name"`
		Role        *string `json:"role"`
		Disabled    *bool   `json:"disabled"`
		Password    *string `json:"password"`
	}
	if err := decodeJSON(w, r, &in, 8<<10); err != nil {
		return err
	}
	self := id == a.User.ID
	if self && ((in.Disabled != nil && *in.Disabled) || (in.Role != nil && *in.Role != "admin")) {
		return badRequest("you cannot disable or demote your own account")
	}
	patch := store.UserPatch{Role: in.Role, Disabled: in.Disabled}
	if in.Role != nil && !validRole(*in.Role) {
		return badRequest("role must be admin or user")
	}
	if in.DisplayName != nil {
		name := trimName(*in.DisplayName, 64)
		if name == "" {
			return badRequest("display name cannot be empty")
		}
		patch.DisplayName = &name
	}
	var newHash string
	if in.Password != nil {
		if err := auth.ValidatePassword(*in.Password); err != nil {
			return badRequest(err.Error())
		}
		if newHash, err = auth.HashPassword(*in.Password); err != nil {
			return err
		}
	}
	if patch.DisplayName == nil && patch.Role == nil && patch.Disabled == nil && in.Password == nil {
		return badRequest("nothing to change")
	}

	u, err := s.Store.UpdateUser(r.Context(), id, patch)
	if err != nil {
		return err
	}
	if in.Password != nil {
		if err := s.Store.SetPassword(r.Context(), id, newHash); err != nil {
			return err
		}
		if self {
			err = s.Store.DeleteOtherUserSessions(r.Context(), id, a.IDHash)
		} else {
			err = s.Store.DeleteUserSessions(r.Context(), id)
		}
		if err != nil {
			return err
		}
	}
	if in.Disabled != nil && *in.Disabled {
		if err := s.Store.DeleteUserSessions(r.Context(), id); err != nil {
			return err
		}
	}
	writeJSON(w, http.StatusOK, u)
	return nil
}

func (s *Server) adminDeleteUser(w http.ResponseWriter, r *http.Request, a *authCtx) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	if id == a.User.ID {
		return badRequest("you cannot delete your own account")
	}
	if err := s.Store.DeleteUser(r.Context(), id); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
