// Package auth holds password hashing, session tokens and the login rate limiter.
package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Iterations is the PBKDF2-HMAC-SHA256 work factor for new hashes
// (OWASP's current recommendation). Tests lower it to stay fast.
var Iterations = 600_000

const (
	maxIterations = 5_000_000
	saltLen       = 16
	keyLen        = 32
	hashPrefix    = "pbkdf2-sha256"
)

var b64 = base64.RawStdEncoding

// HashPassword returns "pbkdf2-sha256$iterations$salt$key" (base64, no padding).
func HashPassword(pw string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, pw, salt, Iterations, keyLen)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s$%d$%s$%s", hashPrefix, Iterations, b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

// VerifyPassword checks pw against an encoded hash in constant time.
// needsRehash is true when the stored hash uses fewer iterations than today's.
func VerifyPassword(encoded, pw string) (ok, needsRehash bool) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != hashPrefix {
		return false, false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter < 1 || iter > maxIterations {
		return false, false
	}
	salt, err := b64.DecodeString(parts[2])
	if err != nil || len(salt) == 0 {
		return false, false
	}
	want, err := b64.DecodeString(parts[3])
	if err != nil || len(want) == 0 {
		return false, false
	}
	got, err := pbkdf2.Key(sha256.New, pw, salt, iter, len(want))
	if err != nil {
		return false, false
	}
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return false, false
	}
	return true, iter < Iterations
}

var (
	dummyOnce sync.Once
	dummyHash string
)

// BurnPassword does the same work as a failed verification. Call it when the
// user does not exist so response time does not reveal valid usernames.
func BurnPassword(pw string) {
	dummyOnce.Do(func() { dummyHash, _ = HashPassword("lectern-dummy-password") })
	VerifyPassword(dummyHash, pw)
}

// ValidatePassword enforces the (deliberately light) password policy.
func ValidatePassword(pw string) error {
	switch {
	case utf8.RuneCountInString(pw) < 8:
		return errors.New("password must be at least 8 characters")
	case len(pw) > 512:
		return errors.New("password is too long")
	}
	return nil
}

// ValidateUsername allows 3-32 letters, digits, dots, dashes and underscores.
func ValidateUsername(u string) error {
	if len(u) < 3 || len(u) > 32 {
		return errors.New("username must be 3 to 32 characters")
	}
	for _, r := range u {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
		default:
			return errors.New("username may only contain letters, digits, '.', '-' and '_'")
		}
	}
	return nil
}

// NewToken makes a session token (sent to the browser) and the hash that is
// stored in the database. A stolen database never reveals usable tokens.
func NewToken() (token string, idHash []byte, err error) {
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return "", nil, err
	}
	sum := sha256.Sum256(raw)
	return base64.RawURLEncoding.EncodeToString(raw), sum[:], nil
}

// HashToken maps a presented token to its database id, or false if malformed.
func HashToken(token string) ([]byte, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != 32 {
		return nil, false
	}
	sum := sha256.Sum256(raw)
	return sum[:], true
}

// ClientIP is the address used for rate limiting and the session list. Behind
// a reverse proxy you trust (LECTERN_TRUST_PROXY=true) it comes from X-Real-IP
// or the last X-Forwarded-For entry; otherwise it is the TCP peer.
func ClientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if v := strings.TrimSpace(r.Header.Get("X-Real-IP")); v != "" {
			if ip := net.ParseIP(v); ip != nil {
				return ip.String()
			}
		}
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if ip := net.ParseIP(strings.TrimSpace(parts[len(parts)-1])); ip != nil {
				return ip.String()
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// Limiter counts failures per key in a fixed window.
type Limiter struct {
	mu      sync.Mutex
	max     int
	window  time.Duration
	entries map[string]*limitEntry
	now     func() time.Time
}

type limitEntry struct {
	count int
	start time.Time
}

const maxLimiterKeys = 50_000

func NewLimiter(max int, window time.Duration) *Limiter {
	return &Limiter{max: max, window: window, entries: map[string]*limitEntry{}, now: time.Now}
}

// Allowed reports whether key may try again, and if not, for how long it is blocked.
func (l *Limiter) Allowed(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e := l.entries[key]
	if e == nil {
		return true, 0
	}
	now := l.now()
	if now.Sub(e.start) >= l.window {
		delete(l.entries, key)
		return true, 0
	}
	if e.count >= l.max {
		return false, e.start.Add(l.window).Sub(now)
	}
	return true, 0
}

// Fail records a failed attempt.
func (l *Limiter) Fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if len(l.entries) >= maxLimiterKeys {
		for k, e := range l.entries {
			if now.Sub(e.start) >= l.window {
				delete(l.entries, k)
			}
		}
		if len(l.entries) >= maxLimiterKeys {
			l.entries = map[string]*limitEntry{} // under attack: favour availability
		}
	}
	e := l.entries[key]
	if e == nil || now.Sub(e.start) >= l.window {
		l.entries[key] = &limitEntry{count: 1, start: now}
		return
	}
	e.count++
}

// Reset forgets a key (after a successful login).
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	delete(l.entries, key)
	l.mu.Unlock()
}
