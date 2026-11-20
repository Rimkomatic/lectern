package api

import (
	"strings"
	"testing"
)

// Lectern is private: every answer asks search engines to stay away.
func TestNotForSearchEngines(t *testing.T) {
	e := newEnv(t)
	c := e.client()
	for _, path := range []string{"/healthz", "/api/v1/setup", "/api/v1/courses", "/api/v1/nope"} {
		r := c.req("GET", path, nil)
		if tag := r.Header.Get("X-Robots-Tag"); !strings.Contains(tag, "noindex") {
			t.Errorf("%s: X-Robots-Tag = %q", path, tag)
		}
	}
}
