package db

import (
	"strings"
	"testing"
)

func TestCheckNotNewer(t *testing.T) {
	known := []migration{{version: 1}, {version: 2}, {version: 3}}
	set := func(vs ...int) map[int]bool {
		m := map[int]bool{}
		for _, v := range vs {
			m[v] = true
		}
		return m
	}
	for name, applied := range map[string]map[int]bool{
		"a new database":        set(),
		"behind: will migrate":  set(1),
		"up to date":            set(1, 2, 3),
		"a gap is not a future": set(1, 3),
	} {
		if err := checkNotNewer(applied, known); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	for name, applied := range map[string]map[int]bool{
		"one step ahead":  set(1, 2, 3, 4),
		"far ahead":       set(1, 2, 3, 40),
		"only the future": set(9),
	} {
		err := checkNotNewer(applied, known)
		if err == nil {
			t.Errorf("%s: a newer database must be refused", name)
			continue
		}
		if msg := err.Error(); !strings.Contains(msg, "newer version of Lectern") || !strings.Contains(msg, "only knows up to 3") || !strings.Contains(msg, "backup") {
			t.Errorf("%s: unhelpful message: %v", name, err)
		}
	}
}
