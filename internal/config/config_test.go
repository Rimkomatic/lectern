package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseLibraries(t *testing.T) {
	libs, err := ParseLibraries("Main=/srv/courses; /mnt/extra ;/srv/courses;")
	if err != nil {
		t.Fatal(err)
	}
	if len(libs) != 2 {
		t.Fatalf("want 2 libraries (dedupe), got %d: %+v", len(libs), libs)
	}
	if libs[0].Label != "Main" || libs[0].Path != "/srv/courses" {
		t.Errorf("first library wrong: %+v", libs[0])
	}
	if libs[1].Label != "extra" || libs[1].Path != "/mnt/extra" {
		t.Errorf("second library wrong: %+v", libs[1])
	}
	if _, err := ParseLibraries(" ; "); err == nil {
		t.Error("empty list should be an error")
	}
}

func TestParseDuration(t *testing.T) {
	cases := map[string]time.Duration{
		"15m": 15 * time.Minute, "30d": 30 * 24 * time.Hour, "0": 0, "off": 0, "1.5h": 90 * time.Minute,
	}
	for in, want := range cases {
		got, err := ParseDuration(in)
		if err != nil || got != want {
			t.Errorf("ParseDuration(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if _, err := ParseDuration("soon"); err == nil {
		t.Error("expected error for garbage")
	}
}

func TestLoadDotEnv(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, ".env")
	content := "# comment\nA_TEST_KEY=hello # inline\nexport B_TEST_KEY=\"quoted value\"\nC_TEST_KEY='single'\nEXISTING_TEST_KEY=fromfile\n"
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EXISTING_TEST_KEY", "fromenv")
	for _, k := range []string{"A_TEST_KEY", "B_TEST_KEY", "C_TEST_KEY"} {
		os.Unsetenv(k)
		t.Cleanup(func() { os.Unsetenv(k) })
	}
	if err := LoadDotEnv(p); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"A_TEST_KEY": "hello", "B_TEST_KEY": "quoted value", "C_TEST_KEY": "single", "EXISTING_TEST_KEY": "fromenv"}
	for k, v := range want {
		if got := os.Getenv(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
	if err := LoadDotEnv(filepath.Join(dir, "missing.env")); err != nil {
		t.Errorf("missing file should be ignored: %v", err)
	}
}
