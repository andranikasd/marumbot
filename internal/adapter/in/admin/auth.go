// Package admin serves the private web interface: one operator, one browser,
// bound to loopback. It is read-mostly, and the one write it exists for is
// recording a lender's payment-allocation policy - data a person reads off a
// real contract, with no other surface that can capture it.
package admin

import (
	"crypto/hmac"
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
)

const (
	cookieName   = "marum_admin"
	sessionTTL   = 12 * time.Hour
	pbkdf2Rounds = 210_000 // OWASP guidance for PBKDF2-HMAC-SHA256
	keyLen       = 32
)

// HashPassword produces the value for MARUM_ADMIN_PASSWORD_HASH. The format is
// self-describing so the cost can be raised later without invalidating the
// meaning of existing hashes.
//
// The fields are colon-separated rather than the conventional '$', because a
// '$' in an environment value is interpolated by Docker Compose, by shells and
// by systemd, and the resulting hash silently loses characters.
//
//	pbkdf2-sha256:<rounds>:<salt-b64>:<key-b64>
func HashPassword(password string) (string, error) {
	if len(password) < 12 {
		return "", errors.New("admin password must be at least 12 characters")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, pbkdf2Rounds, keyLen)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("pbkdf2-sha256:%d:%s:%s", pbkdf2Rounds,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

// verifyPassword is constant-time in the comparison. It returns false rather
// than an error for a malformed hash, so a misconfigured deployment fails
// closed instead of letting anyone in.
func verifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, ":")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	rounds, err := strconv.Atoi(parts[1])
	if err != nil || rounds < 1000 || rounds > 1000000 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(want) != keyLen || len(salt) < 16 {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, rounds, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

// sessionKey derives the cookie-signing key from the password hash, so
// changing the password invalidates every existing session without needing a
// second secret to configure and forget.
func sessionKey(passwordHash string) []byte {
	m := hmac.New(sha256.New, []byte(passwordHash))
	m.Write([]byte("marum-admin-session-v1"))
	return m.Sum(nil)
}

// issue returns a signed, expiring token. There is no server-side session
// table: one operator does not need one, and a stateless token survives a
// restart.
func issue(key []byte, now time.Time) string {
	payload := strconv.FormatInt(now.Add(sessionTTL).Unix(), 10)
	m := hmac.New(sha256.New, key)
	m.Write([]byte(payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// throttle slows down repeated failures from one address. It is deliberately
// simple: a single operator on loopback does not need a distributed limiter,
// but an unthrottled login form is an invitation regardless of who can reach
// it.
type throttle struct {
	mu       sync.Mutex
	failures map[string]int
	until    map[string]time.Time
	seen     map[string]time.Time
}

func newThrottle() *throttle {
	return &throttle{
		failures: map[string]int{},
		until:    map[string]time.Time{},
		seen:     map[string]time.Time{},
	}
}

// throttleRetention is how long a failing address is remembered. Longer than
// the maximum backoff, so pruning never shortens a block; short enough that
// addresses which only ever fail cannot grow the maps without bound.
const throttleRetention = 15 * time.Minute

func (t *throttle) blocked(addr string, now time.Time) (bool, time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if u, ok := t.until[addr]; ok && now.Before(u) {
		return true, u.Sub(now).Round(time.Second)
	}
	return false, 0
}

func (t *throttle) fail(addr string, now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for a, s := range t.seen {
		if now.Sub(s) > throttleRetention {
			delete(t.failures, a)
			delete(t.until, a)
			delete(t.seen, a)
		}
	}
	t.failures[addr]++
	t.seen[addr] = now
	if n := t.failures[addr]; n >= 5 {
		// Back off geometrically, capped, so a forgotten password is annoying
		// rather than a lockout.
		delay := time.Duration(1<<min(n-5, 6)) * time.Second
		t.until[addr] = now.Add(delay)
	}
}

func (t *throttle) succeed(addr string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.failures, addr)
	delete(t.until, addr)
	delete(t.seen, addr)
}

func clientAddr(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
