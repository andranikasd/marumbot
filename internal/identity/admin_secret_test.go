package identity

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func testSecretCipher(t *testing.T, keyByte byte) *SecretCipher {
	t.Helper()
	c, err := NewSecretCipher(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{keyByte}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestAdminSecretRoundTripAndRandomNonce(t *testing.T) {
	c := testSecretCipher(t, 1)
	const id, secret = "administrator", "opaque-totp-secret"
	first, err := c.Seal(id, secret)
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.Seal(id, secret)
	if err != nil {
		t.Fatal(err)
	}
	if first == second || !strings.HasPrefix(first, "v1:") || strings.Contains(first, secret) {
		t.Fatal("ciphertext must be versioned, randomized, and opaque")
	}
	for _, box := range []string{first, second} {
		got, err := c.Open(id, box)
		if err != nil || got != secret {
			t.Fatalf("round trip failed: %v", err)
		}
	}
	box, err := c.Seal(id, "")
	if err != nil || box != "" {
		t.Fatal("empty secret did not remain empty")
	}
	plain, err := c.Open(id, box)
	if err != nil || plain != "" {
		t.Fatal("empty ciphertext did not remain empty")
	}
}

func TestAdminSecretRejectsWrongKeyRowAndTampering(t *testing.T) {
	c := testSecretCipher(t, 1)
	wrongKey := testSecretCipher(t, 2)
	const id, secret = "administrator", "opaque-totp-secret"
	sealed, err := c.Seal(id, secret)
	if err != nil {
		t.Fatal(err)
	}
	box, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(sealed, "v1:"))
	if err != nil {
		t.Fatal(err)
	}
	box[len(box)-1] ^= 1
	for _, tc := range []struct {
		name, id, sealed string
		cipher           *SecretCipher
	}{
		{"wrong key", id, sealed, wrongKey},
		{"wrong row", "another-administrator", sealed, c},
		{"missing row", "", sealed, c},
		{"tampered", id, "v1:" + base64.StdEncoding.EncodeToString(box), c},
		{"plaintext", id, secret, c},
		{"unknown version", id, "v2:" + strings.TrimPrefix(sealed, "v1:"), c},
		{"invalid encoding", id, "v1:" + secret, c},
		{"short", id, "v1:AA==", c},
		{"noncanonical encoding", id, sealed + "\n", c},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plain, err := tc.cipher.Open(tc.id, tc.sealed)
			if !errors.Is(err, ErrAdminSecret) || plain != "" {
				t.Fatal("invalid ciphertext accepted")
			}
			if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), id) {
				t.Fatal("error disclosed secret or administrator")
			}
		})
	}
}

func TestAdminSecretRejectsInvalidMasterKey(t *testing.T) {
	for _, key := range []string{"", "private-key-material", base64.StdEncoding.EncodeToString(make([]byte, 31))} {
		if _, err := NewSecretCipher(key); !errors.Is(err, ErrKey) {
			t.Fatal("invalid master key accepted")
		}
	}
}
