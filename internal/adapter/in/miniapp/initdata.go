// Package miniapp serves the Telegram Mini App and authenticates its calls.
//
// A Mini App is an ordinary web page in a Telegram webview, which means the
// browser it runs in is not trusted and neither is anything it sends. The only
// thing that proves a request came from a real Telegram user is initData: a
// query string Telegram signs with a key derived from the bot token. Without
// checking it, anyone who learns the URL could file a loan against any account
// by editing a number.
package miniapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ErrInitData is returned when initData is absent, malformed, unsigned, forged
// or stale. It is deliberately one error: telling a caller which of those it
// was tells an attacker which part to fix.
var ErrInitData = errors.New("miniapp: initData rejected")

// MaxAge is how long a signed initData stays acceptable.
//
// Telegram recommends checking auth_date but names no window. A day is long
// enough that a user who opens the form, goes to find a loan agreement and
// comes back is not logged out, and short enough that a leaked URL from a
// screenshot or a shared link stops working before it is useful.
const MaxAge = 24 * time.Hour

// User is the part of initData Marum acts on.
type User struct {
	ID           int64  `json:"id"`
	LanguageCode string `json:"language_code"`
	IsBot        bool   `json:"is_bot"`
}

// Verified is a proven-authentic initData.
type Verified struct {
	User     User
	AuthDate time.Time
	Raw      url.Values
}

// Verify checks initData against the bot token and returns the user it names.
//
//	secret_key = HMAC_SHA256(key: "WebAppData", data: bot_token)
//	hash       = hex(HMAC_SHA256(key: secret_key, data: data_check_string))
//
// The argument order is the trap: the constant is the key and the token is the
// message, which is the reverse of the intuitive reading. Swapping them
// produces a validator that is self-consistent, passes a round-trip test
// written against itself, and accepts nothing Telegram ever sends.
func Verify(initData, botToken string, now time.Time) (Verified, error) {
	if initData == "" || botToken == "" {
		return Verified{}, fmt.Errorf("%w: empty", ErrInitData)
	}
	values, err := url.ParseQuery(initData)
	if err != nil {
		return Verified{}, fmt.Errorf("%w: unparseable", ErrInitData)
	}
	got := values.Get("hash")
	if got == "" {
		return Verified{}, fmt.Errorf("%w: no hash", ErrInitData)
	}

	// The data-check-string is every field except the hash, sorted by key, as
	// key=value joined by newlines. The values are the DECODED ones.
	pairs := make([]string, 0, len(values))
	for k, v := range values {
		if k == "hash" {
			continue
		}
		pairs = append(pairs, k+"="+v[0])
	}
	sort.Strings(pairs)
	check := strings.Join(pairs, "\n")

	secret := hmacSHA256([]byte("WebAppData"), []byte(botToken))
	want := hex.EncodeToString(hmacSHA256(secret, []byte(check)))

	// Constant time: a byte-by-byte comparison leaks how much of a forged hash
	// was correct, which is enough to build the rest one byte at a time.
	if !hmac.Equal([]byte(want), []byte(got)) {
		return Verified{}, fmt.Errorf("%w: signature mismatch", ErrInitData)
	}

	authUnix, err := strconv.ParseInt(values.Get("auth_date"), 10, 64)
	if err != nil {
		return Verified{}, fmt.Errorf("%w: no auth_date", ErrInitData)
	}
	authDate := time.Unix(authUnix, 0)
	// Reject the future as well as the past: a clock-skewed or replayed value
	// dated forwards would otherwise never expire.
	if now.Sub(authDate) > MaxAge || authDate.After(now.Add(5*time.Minute)) {
		return Verified{}, fmt.Errorf("%w: stale", ErrInitData)
	}

	var u User
	if raw := values.Get("user"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &u); err != nil {
			return Verified{}, fmt.Errorf("%w: undecodable user", ErrInitData)
		}
	}
	if u.ID == 0 {
		return Verified{}, fmt.Errorf("%w: no user", ErrInitData)
	}
	if u.IsBot {
		return Verified{}, fmt.Errorf("%w: bot", ErrInitData)
	}
	return Verified{User: u, AuthDate: authDate, Raw: values}, nil
}

func hmacSHA256(key, data []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(data)
	return m.Sum(nil)
}

// Sign produces initData as Telegram would. It exists so tests can build a
// genuinely valid payload rather than asserting that Verify agrees with itself.
func Sign(values url.Values, botToken string) string {
	pairs := make([]string, 0, len(values))
	for k, v := range values {
		if k == "hash" {
			continue
		}
		pairs = append(pairs, k+"="+v[0])
	}
	sort.Strings(pairs)
	secret := hmacSHA256([]byte("WebAppData"), []byte(botToken))
	h := hex.EncodeToString(hmacSHA256(secret, []byte(strings.Join(pairs, "\n"))))

	out := url.Values{}
	for k, v := range values {
		out.Set(k, v[0])
	}
	out.Set("hash", h)
	return out.Encode()
}
