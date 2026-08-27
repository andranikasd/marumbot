package miniapp

import (
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"
)

// A fabricated token. It must never be a real one: this file is public, and a
// bot token grants full control of the bot -- sending as it, reading everything
// users send it, and repointing its webhook.
const testToken = "1234567890:AAFakeTokenForTestsOnlyNotARealBot00"

// A vector computed by an independent implementation, not by this package.
//
// This matters more than it looks. Verify and Sign share their key derivation,
// so a round-trip test passes even when the HMAC arguments are the wrong way
// round -- and the wrong way round is the intuitive way. With the key and the
// message swapped the same input hashes to
// a different value entirely instead,
// and Telegram would never authenticate.
const (
	knownUser     = `{"id":42,"first_name":"Test","language_code":"hy"}`
	knownAuthDate = "1767225600"
	knownHash     = "58b70a001773fe0cde1a1bdf8235e2028e4231ec19b68a70f54d8c9b973dac43"
)

func knownInitData() string {
	v := url.Values{}
	v.Set("auth_date", knownAuthDate)
	v.Set("query_id", "AAF")
	v.Set("user", knownUser)
	v.Set("hash", knownHash)
	return v.Encode()
}

func knownTime() time.Time { return time.Unix(1767225600, 0).Add(time.Hour) }

func TestVerifyAcceptsAnIndependentlySignedPayload(t *testing.T) {
	got, err := Verify(knownInitData(), testToken, knownTime())
	if err != nil {
		t.Fatalf("Verify rejected a valid payload: %v", err)
	}
	if got.User.ID != 42 {
		t.Errorf("user id = %d, want 42", got.User.ID)
	}
	if got.User.LanguageCode != "hy" {
		t.Errorf("language = %q, want hy", got.User.LanguageCode)
	}
}

// The forgery cases. Each of these is somebody trying to file a loan against
// another person's account.
func TestVerifyRejectsForgeries(t *testing.T) {
	valid := knownInitData()
	cases := map[string]string{
		"tampered user id": strings.Replace(valid, "42", "43", 1),
		"tampered hash":    strings.Replace(valid, knownHash[:8], "deadbeef", 1),
		"hash removed":     strings.Replace(valid, "hash="+knownHash, "hash=", 1),
		"no hash field":    "auth_date=" + knownAuthDate + "&user=" + url.QueryEscape(knownUser),
		"empty":            "",
	}
	for name, in := range cases {
		if _, err := Verify(in, testToken, knownTime()); !errors.Is(err, ErrInitData) {
			t.Errorf("%s: got %v, want ErrInitData", name, err)
		}
	}
}

// A payload signed by a different bot must not authenticate against ours,
// or anyone able to run their own bot could act on Marum's users.
func TestVerifyRejectsAnotherBotsSignature(t *testing.T) {
	v := url.Values{}
	v.Set("auth_date", knownAuthDate)
	v.Set("user", knownUser)
	other := Sign(v, "1111111:OTHERBOTTOKEN")

	if _, err := Verify(other, testToken, knownTime()); !errors.Is(err, ErrInitData) {
		t.Error("a payload signed by another bot was accepted")
	}
	// ...and the same payload must verify against the bot that DID sign it,
	// otherwise this test would pass even if Verify rejected everything.
	if _, err := Verify(other, "1111111:OTHERBOTTOKEN", knownTime()); err != nil {
		t.Errorf("the signing bot could not verify its own payload: %v", err)
	}
}

func TestVerifyRejectsStaleAndFutureData(t *testing.T) {
	valid := knownInitData()
	base := time.Unix(1767225600, 0)

	if _, err := Verify(valid, testToken, base.Add(MaxAge+time.Minute)); !errors.Is(err, ErrInitData) {
		t.Error("expired initData was accepted")
	}
	// Just inside the window is fine.
	if _, err := Verify(valid, testToken, base.Add(MaxAge-time.Minute)); err != nil {
		t.Errorf("initData inside the window was rejected: %v", err)
	}
	// Dated far in the future: a replayed or skewed value that would otherwise
	// never expire.
	if _, err := Verify(valid, testToken, base.Add(-time.Hour)); !errors.Is(err, ErrInitData) {
		t.Error("initData dated in the future was accepted")
	}
}

func TestVerifyRejectsBotsAndMissingUsers(t *testing.T) {
	for name, user := range map[string]string{
		"a bot":       `{"id":9,"is_bot":true}`,
		"no id":       `{"first_name":"Nobody"}`,
		"broken json": `{not json`,
	} {
		v := url.Values{}
		v.Set("auth_date", knownAuthDate)
		v.Set("user", user)
		if _, err := Verify(Sign(v, testToken), testToken, knownTime()); !errors.Is(err, ErrInitData) {
			t.Errorf("%s: accepted", name)
		}
	}
	// No user field at all.
	v := url.Values{}
	v.Set("auth_date", knownAuthDate)
	if _, err := Verify(Sign(v, testToken), testToken, knownTime()); !errors.Is(err, ErrInitData) {
		t.Error("initData with no user was accepted")
	}
}

// Extra fields Telegram may add later must not break validation: they are part
// of the signed set, so they have to be included in the check string.
func TestVerifyHandlesUnknownFields(t *testing.T) {
	v := url.Values{}
	v.Set("auth_date", knownAuthDate)
	v.Set("user", knownUser)
	v.Set("chat_type", "private")
	v.Set("some_future_field", "value")
	if _, err := Verify(Sign(v, testToken), testToken, knownTime()); err != nil {
		t.Errorf("a payload with unknown fields was rejected: %v", err)
	}
}
