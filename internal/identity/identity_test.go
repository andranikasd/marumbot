package identity

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"testing"
)

func testKey(t *testing.T) string {
	t.Helper()
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func TestRoundTrip(t *testing.T) {
	c, err := New(testKey(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{1, 8982118495, -1001234567890, 1 << 62} {
		box, err := c.Seal(id)
		if err != nil {
			t.Fatalf("Seal(%d): %v", id, err)
		}
		got, err := c.Open(box)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		if got != id {
			t.Errorf("round trip gave %d, want %d", got, id)
		}
	}
}

// The same id must NOT produce the same ciphertext twice. If it did, anyone
// holding the table could tell which rows belong to the same person without
// decrypting anything.
func TestSealIsRandomised(t *testing.T) {
	c, err := New(testKey(t))
	if err != nil {
		t.Fatal(err)
	}
	a, _ := c.Seal(42)
	b, _ := c.Seal(42)
	if bytes.Equal(a, b) {
		t.Error("sealing the same id twice produced identical ciphertext")
	}
}

// The tag must be stable, or an incoming update could never find its account.
func TestTagIsDeterministicAndKeyed(t *testing.T) {
	k := testKey(t)
	c1, _ := New(k)
	c2, _ := New(k)
	if c1.Tag(42) != c2.Tag(42) {
		t.Error("the same key produced different tags")
	}
	if c1.Tag(42) == c1.Tag(43) {
		t.Error("different ids produced the same tag")
	}
	other, _ := New(testKey(t))
	if c1.Tag(42) == other.Tag(42) {
		t.Error("a different key produced the same tag; the tag is not keyed")
	}
}

// The encryption key and the tagging key are derived from one secret with
// different info strings. Neither may be usable in place of the other.
func TestDerivedKeysDiffer(t *testing.T) {
	c, err := New(testKey(t))
	if err != nil {
		t.Fatal(err)
	}
	box, _ := c.Seal(7)
	if bytes.Contains(box, c.tag) {
		t.Error("the tagging key appears inside a ciphertext")
	}
}

// Tampering must fail loudly. GCM authenticates, so a flipped bit is a decrypt
// error rather than a plausible wrong answer.
func TestTamperedCiphertextIsRejected(t *testing.T) {
	c, _ := New(testKey(t))
	box, _ := c.Seal(99)
	box[len(box)-1] ^= 0x01
	if _, err := c.Open(box); err == nil {
		t.Error("a tampered ciphertext decrypted without error")
	}
}

// A bad key must be fatal at construction. Starting without usable encryption
// would mean storing identifiers in the clear.
func TestBadKeysAreRejected(t *testing.T) {
	for name, key := range map[string]string{
		"empty":      "",
		"not base64": "!!!!",
		"too short":  base64.StdEncoding.EncodeToString(make([]byte, 16)),
		"too long":   base64.StdEncoding.EncodeToString(make([]byte, 64)),
	} {
		if _, err := New(key); err == nil {
			t.Errorf("%s: expected an error, got none", name)
		}
	}
}

func TestShortCiphertextIsRejected(t *testing.T) {
	c, _ := New(testKey(t))
	if _, err := c.Open([]byte{1, 2, 3}); err == nil {
		t.Error("expected an error for a truncated ciphertext")
	}
}
