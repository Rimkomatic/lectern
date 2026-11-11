package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"lectern/internal/store"
)

// apiError is an error with an HTTP status and a stable machine-readable code.
type apiError struct {
	Status  int
	Code    string
	Message string
	Headers map[string]string // extra response headers, e.g. Retry-After
}

func (e *apiError) Error() string { return e.Message }

func badRequest(msg string) *apiError {
	return &apiError{Status: http.StatusBadRequest, Code: "bad_request", Message: msg}
}
func notFound() *apiError {
	return &apiError{Status: http.StatusNotFound, Code: "not_found", Message: "not found"}
}
func conflict(code, msg string) *apiError {
	return &apiError{Status: http.StatusConflict, Code: code, Message: msg}
}

var (
	errUnauthorized = &apiError{Status: http.StatusUnauthorized, Code: "unauthorized", Message: "sign in required"}
	errForbidden    = &apiError{Status: http.StatusForbidden, Code: "forbidden", Message: "you are not allowed to do that"}
)

type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeAPIError(w http.ResponseWriter, e *apiError) {
	for k, v := range e.Headers {
		w.Header().Set(k, v)
	}
	var b errorBody
	b.Error.Code, b.Error.Message = e.Code, e.Message
	writeJSON(w, e.Status, b)
}

// fail turns any error into a JSON error response. Unknown errors are logged
// and reported as a generic 500 so internals never leak.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	var ae *apiError
	switch {
	case errors.As(err, &ae):
	case errors.Is(err, store.ErrNotFound):
		ae = notFound()
	case errors.Is(err, store.ErrConflict):
		ae = conflict("conflict", "that already exists")
	case errors.Is(err, store.ErrLastAdmin):
		ae = conflict("last_admin", "there must be at least one enabled admin")
	case errors.Is(err, context.Canceled):
		return // the client went away
	default:
		s.Log.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
		ae = &apiError{Status: http.StatusInternalServerError, Code: "internal", Message: "something went wrong"}
	}
	writeAPIError(w, ae)
}

// decodeJSON reads exactly one JSON object from the body: size-limited, no
// unknown fields, no trailing data.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any, limit int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var tooBig *http.MaxBytesError
		switch {
		case errors.As(err, &tooBig):
			return &apiError{Status: http.StatusRequestEntityTooLarge, Code: "too_large", Message: "request body is too large"}
		case errors.Is(err, io.EOF):
			return badRequest("the request body is empty")
		}
		return badRequest("the request body is not valid for this endpoint")
	}
	if _, err := dec.Token(); err != io.EOF {
		return badRequest("unexpected data after the JSON body")
	}
	return nil
}

// pathID parses a numeric {name} path segment. Garbage is reported as 404.
func pathID(r *http.Request, name string) (uint64, error) {
	id, err := strconv.ParseUint(r.PathValue(name), 10, 64)
	if err != nil || id == 0 {
		return 0, notFound()
	}
	return id, nil
}

// emptyIfNil keeps JSON arrays as [] instead of null.
func emptyIfNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
