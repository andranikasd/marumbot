// Package identity encrypts the Telegram identifiers that link an account to a
// person, and derives the deterministic lookup keys used to find them again.
//
// Two separate operations, deliberately, because they need opposite properties:
//
//	Seal  is randomised, so the same chat id encrypts differently every time and
//	      a stolen table reveals nothing by comparing rows.
//	Tag   is deterministic, so an incoming update can find its existing account
//	      without decrypting anything.
//
// Both derive their keys from the same master secret through HKDF with distinct
// info strings, so the encryption key and the tagging key cannot be substituted
// for one another even though there is one secret to manage.
//
// The point of all this: identities is the only table that knows who a borrower
// is. Loans, events and balances live elsewhere and carry only a user uuid, so a
// leak of the financial data does not also say whose it is.
package identity

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"

	"golang.org/x/crypto/hkdf"
)

// ErrKey is returned when the configured key is unusable. It is deliberately
// fatal at startup rather than at first use: a service that runs without being
// able to encrypt identifiers would happily store them in the clear.
var ErrKey = errors.New("identity: unusable key")

// KeyVersion is stamped on every row so a future rotation can tell which secret
// a ciphertext was sealed with. Rotating means adding a version, re-sealing in
// the background, and only then retiring the old one.
const KeyVersion = 1

// Cipher seals and tags Telegram identifiers.
type Cipher struct {
	aead cipher.AEAD
	tag  []byte
}

// New builds a Cipher from a base64-encoded 32-byte key, as produced by
// `openssl rand -base64 32`.
func New(encoded string) (*Cipher, error) {
	if encoded == "" {
		return nil, fmt.Errorf("%w: no key configured", ErrKey)
	}
	master, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("%w: not base64: %w", ErrKey, err)
	}
	if len(master) != 32 {
		return nil, fmt.Errorf("%w: want 32 bytes, got %d", ErrKey, len(master))
	}

	encKey, err := derive(master, "marum/identity/encryption/v1")
	if err != nil {
		return nil, err
	}
	tagKey, err := derive(master, "marum/identity/tag/v1")
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(encKey)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrKey, err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrKey, err)
	}
	return &Cipher{aead: aead, tag: tagKey}, nil
}

func derive(master []byte, info string) ([]byte, error) {
	out := make([]byte, 32)
	// No salt: the master secret is already a uniformly random 32 bytes, which
	// is the case HKDF's RFC explicitly allows a salt to be omitted for.
	if _, err := io.ReadFull(hkdf.New(sha256.New, master, nil, []byte(info)), out); err != nil {
		return nil, fmt.Errorf("%w: deriving %s: %w", ErrKey, info, err)
	}
	return out, nil
}

// Seal encrypts a Telegram identifier. The nonce is random and prepended, so
// two rows holding the same id are not recognisably equal.
func (c *Cipher) Seal(id int64) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("identity: nonce: %w", err)
	}
	return c.aead.Seal(nonce, nonce, []byte(strconv.FormatInt(id, 10)), nil), nil
}

// Open decrypts an identifier sealed by Seal.
func (c *Cipher) Open(box []byte) (int64, error) {
	n := c.aead.NonceSize()
	if len(box) < n {
		return 0, errors.New("identity: ciphertext too short")
	}
	plain, err := c.aead.Open(nil, box[:n], box[n:], nil)
	if err != nil {
		return 0, fmt.Errorf("identity: open: %w", err)
	}
	return strconv.ParseInt(string(plain), 10, 64)
}

// Tag is the deterministic lookup key for an identifier.
//
// HMAC rather than a bare hash: a plain SHA-256 of a Telegram id is trivially
// reversible by enumeration, since ids are small integers. Keying it means an
// attacker holding the table still cannot test a guess.
func (c *Cipher) Tag(id int64) string {
	m := hmac.New(sha256.New, c.tag)
	m.Write([]byte(strconv.FormatInt(id, 10)))
	return hex.EncodeToString(m.Sum(nil))
}
