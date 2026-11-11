package testdb

import "testing"

func TestCheckWipeable(t *testing.T) {
	for name, ok := range map[string]bool{
		"lectern_test": true,
		"LECTERN_TEST": true,
		"test":         true,
		"ci_testing_7": true,
		"lectern":      false, // the real database from .env
		"Lectern":      false,
		"":             false, // no database selected
		"prod":         false,
	} {
		err := checkWipeable(name)
		if ok && err != nil {
			t.Errorf("%q should be allowed: %v", name, err)
		}
		if !ok && err == nil {
			t.Errorf("%q must be refused", name)
		}
	}
}
