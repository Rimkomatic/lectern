package auth

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPasswordRoundTrip(t *testing.T) {
	old := Iterations
	Iterations = 1000
	defer func() { Iterations = old }()

	h, err := HashPassword("correct horse")
	if err != nil || !strings.HasPrefix(h, "pbkdf2-sha256$1000$") {
		t.Fatalf("hash: %q %v", h, err)
	}
	if ok, re := VerifyPassword(h, "correct horse"); !ok || re {
		t.Errorf("verify good: ok=%v rehash=%v", ok, re)
	}
	if ok, _ := VerifyPassword(h, "wrong"); ok {
		t.Error("wrong password accepted")
	}
	h2, _ := HashPassword("correct horse")
	if h == h2 {
		t.Error("hashes must be salted")
	}
	Iterations = 2000
	if ok, re := VerifyPassword(h, "correct horse"); !ok || !re {
		t.Errorf("old-iteration hash should verify and ask for a rehash: ok=%v rehash=%v", ok, re)
	}
	for _, bad := range []string{"", "x", "pbkdf2-sha256$0$AA$AA", "pbkdf2-sha256$99999999999$AA$AA", "md5$1$AA$AA", "pbkdf2-sha256$10$!!$!!", "a$b$c$d"} {
		if ok, _ := VerifyPassword(bad, "x"); ok {
			t.Errorf("malformed hash %q accepted", bad)
		}
	}
	BurnPassword("anything") // must not panic
}

func TestValidation(t *testing.T) {
	if ValidatePassword("short") == nil || ValidatePassword("long enough") != nil || ValidatePassword(strings.Repeat("a", 600)) == nil {
		t.Error("password policy wrong")
	}
	for _, u := range []string{"ann", "Ann.Lee-1_x", "abc"} {
		if err := ValidateUsername(u); err != nil {
			t.Errorf("%q rejected: %v", u, err)
		}
	}
	for _, u := range []string{"", "ab", "has space", "semi;colon", "ünï", strings.Repeat("a", 33), "a<b>c"} {
		if ValidateUsername(u) == nil {
			t.Errorf("%q accepted", u)
		}
	}
}

func TestTokens(t *testing.T) {
	tok, id, err := NewToken()
	if err != nil || len(id) != 32 || len(tok) != 43 {
		t.Fatalf("token %q id %d err %v", tok, len(id), err)
	}
	got, ok := HashToken(tok)
	if !ok || string(got) != string(id) {
		t.Error("HashToken must reproduce the stored id")
	}
	for _, bad := range []string{"", "short", tok + "x", strings.Repeat("!", 43)} {
		if _, ok := HashToken(bad); ok {
			t.Errorf("token %q accepted", bad)
		}
	}
	tok2, _, _ := NewToken()
	if tok == tok2 {
		t.Error("tokens must be random")
	}
}

func TestLimiter(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	l := NewLimiter(3, time.Minute)
	l.now = func() time.Time { return now }

	for i := 0; i < 3; i++ {
		if ok, _ := l.Allowed("k"); !ok {
			t.Fatalf("blocked too early at %d", i)
		}
		l.Fail("k")
	}
	ok, wait := l.Allowed("k")
	if ok || wait <= 0 || wait > time.Minute {
		t.Fatalf("should be blocked: ok=%v wait=%v", ok, wait)
	}
	if ok, _ := l.Allowed("other"); !ok {
		t.Error("other keys are independent")
	}
	now = now.Add(61 * time.Second)
	if ok, _ := l.Allowed("k"); !ok {
		t.Error("window should have expired")
	}
	l.Fail("k")
	l.Reset("k")
	if ok, _ := l.Allowed("k"); !ok {
		t.Error("reset failed")
	}
}

func TestClientIP(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.5:4242"
	r.Header.Set("X-Forwarded-For", "6.6.6.6, 203.0.113.9")
	if got := ClientIP(r, false); got != "10.0.0.5" {
		t.Errorf("untrusted proxy: %q", got)
	}
	if got := ClientIP(r, true); got != "203.0.113.9" {
		t.Errorf("trusted proxy should use the last hop: %q", got)
	}
	r.Header.Set("X-Real-IP", "198.51.100.7")
	if got := ClientIP(r, true); got != "198.51.100.7" {
		t.Errorf("X-Real-IP: %q", got)
	}
	r.Header.Set("X-Real-IP", "not-an-ip")
	r.Header.Del("X-Forwarded-For")
	if got := ClientIP(r, true); got != "10.0.0.5" {
		t.Errorf("garbage header must fall back to the peer: %q", got)
	}
}
