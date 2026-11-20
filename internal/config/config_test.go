package config

import (
	"os"
	"path/filepath"
	"strings"
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

func TestParseClock(t *testing.T) {
	good := map[string][2]int{"03:00": {3, 0}, "3:00": {3, 0}, "00:00": {0, 0}, "23:59": {23, 59}, " 04:30 ": {4, 30}}
	for in, want := range good {
		h, m, err := ParseClock(in)
		if err != nil || h != want[0] || m != want[1] {
			t.Errorf("ParseClock(%q) = %d, %d, %v; want %v", in, h, m, err, want)
		}
	}
	for _, in := range []string{"", "3", "3pm", "03h00", "24:00", "12:60", "12:5", "123:00", "-1:00", "+3:00", "03:00:00", "ab:cd", "3:0a", "03.00", ":30"} {
		if _, _, err := ParseClock(in); err == nil {
			t.Errorf("ParseClock(%q) should fail", in)
		}
	}
}

func TestLoadScanSchedule(t *testing.T) {
	load := func(at, every string) (*Config, error) {
		t.Setenv("MYSQL_PASSWORD", "secret")
		t.Setenv("LECTERN_SCAN_AT", at)
		t.Setenv("LECTERN_SCAN_INTERVAL", every)
		return Load()
	}
	type plan struct {
		daily     bool
		hour, min int
		every     time.Duration
	}
	for _, tc := range []struct {
		name, at, every string
		want            plan
	}{
		{"default is 03:00 every day", "", "", plan{daily: true, hour: 3}},
		{"another time", "04:15", "", plan{daily: true, hour: 4, min: 15}},
		{"a time without the leading zero", "5:05", "", plan{daily: true, hour: 5, min: 5}},
		{"switched off", "off", "", plan{}},
		{"switched off with never", "Never", "", plan{}},
		{"the older setting still works", "", "30m", plan{every: 30 * time.Minute}},
		{"the older setting, days", "", "1d", plan{every: 24 * time.Hour}},
		{"the older way to switch off", "", "0", plan{}},
		{"the older way to switch off, as a word", "", "off", plan{}},
	} {
		c, err := load(tc.at, tc.every)
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		got := plan{c.ScanDaily, c.ScanAtHour, c.ScanAtMinute, c.ScanInterval}
		if got != tc.want {
			t.Errorf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
	}
	for _, tc := range []struct{ name, at, every, mention string }{
		{"both settings", "03:00", "15m", "not both"},
		{"both, one of them off", "off", "15m", "not both"},
		{"a bad time", "3am", "", "LECTERN_SCAN_AT"},
		{"a time that does not exist", "25:00", "", "HH:MM"},
		{"a bad interval", "", "soon", "LECTERN_SCAN_INTERVAL"},
		{"a negative interval", "", "-5m", "negative"},
	} {
		_, err := load(tc.at, tc.every)
		if err == nil || !strings.Contains(err.Error(), tc.mention) {
			t.Errorf("%s: error = %v, want one mentioning %q", tc.name, err, tc.mention)
		}
	}
}

func TestParseSetupWindow(t *testing.T) {
	for in, want := range map[string]time.Duration{
		"":          DefaultSetupWindow,
		"   ":       DefaultSetupWindow,
		"15m":       15 * time.Minute,
		"2h":        2 * time.Hour,
		"1d":        24 * time.Hour,
		"90s":       90 * time.Second,
		"forever":   0,
		"Forever":   0,
		" always ":  0,
		"unlimited": 0,
	} {
		got, err := ParseSetupWindow(in)
		if err != nil || got != want {
			t.Errorf("ParseSetupWindow(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if DefaultSetupWindow != 15*time.Minute {
		t.Errorf("the documented default is 15 minutes, got %v", DefaultSetupWindow)
	}
	// Words that could mean "no limit" or "never open" must not be guessed at.
	for _, in := range []string{"0", "off", "Never", "disabled", "false"} {
		if d, err := ParseSetupWindow(in); err == nil || !strings.Contains(err.Error(), "two opposite things") {
			t.Errorf("ParseSetupWindow(%q) = %v, %v; want the ambiguity error", in, d, err)
		}
	}
	for _, in := range []string{"soon", "15", "-5m", "0s", "0m"} {
		if d, err := ParseSetupWindow(in); err == nil || !strings.Contains(err.Error(), "LECTERN_SETUP_WINDOW") {
			t.Errorf("ParseSetupWindow(%q) = %v, %v; want an error that names the setting", in, d, err)
		}
	}
}

func TestLoadSetupWindow(t *testing.T) {
	load := func(v string) (*Config, error) {
		t.Setenv("MYSQL_PASSWORD", "secret")
		t.Setenv("LECTERN_SETUP_WINDOW", v)
		return Load()
	}
	if c, err := load(""); err != nil || c.SetupWindow != DefaultSetupWindow {
		t.Errorf("default: %v, %v", c, err)
	}
	if c, err := load("30m"); err != nil || c.SetupWindow != 30*time.Minute {
		t.Errorf("30m: %v, %v", c, err)
	}
	if c, err := load("forever"); err != nil || c.SetupWindow != 0 {
		t.Errorf("forever: %v, %v", c, err)
	}
	if _, err := load("never"); err == nil {
		t.Error("an ambiguous value must stop Lectern from starting")
	}
}
