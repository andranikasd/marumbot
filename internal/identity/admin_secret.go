package identity

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
)

const adminSecretPurpose = "marum/admin/totp/encryption/v1"

// ErrAdminSecret reports invalid ciphertext without disclosing its contents.
var ErrAdminSecret = errors.New("identity: invalid admin secret ciphertext")

// SecretCipher seals administrator TOTP secrets with a purpose-derived key.
// The administrator ID is authenticated so ciphertext cannot move between rows.
// Empty strings represent administrators who have not enrolled in TOTP.
type SecretCipher struct {
	aead cipher.AEAD
}

// NewSecretCipher uses the existing base64-encoded 32-byte master key, deriving
// a key distinct from both Telegram identity encryption and lookup tagging.
func NewSecretCipher(encoded string) (*SecretCipher, error) {
	master, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(master) != 32 {
		return nil, ErrKey
	}
	key, err := derive(master, adminSecretPurpose)
	if err != nil {
		return nil, ErrKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrKey
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, ErrKey
	}
	return &SecretCipher{aead: aead}, nil
}

// Seal encrypts an opaque secret using a fresh nonce and binds it to id.
func (c *SecretCipher) Seal(id, secret string) (string, error) {
	if secret == "" {
		return "", nil
	}
	if id == "" {
		return "", ErrAdminSecret
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", errors.New("identity: admin secret nonce unavailable")
	}
	box := c.aead.Seal(nonce, nonce, []byte(secret), []byte(adminSecretPurpose+"\x00"+id))
	return "v1:" + base64.StdEncoding.EncodeToString(box), nil
}

// Open accepts only the versioned encrypted format. Plaintext legacy values
// must be migrated explicitly; authentication never silently accepts them.
func (c *SecretCipher) Open(id, sealed string) (string, error) {
	if sealed == "" {
		return "", nil
	}
	encoded, ok := strings.CutPrefix(sealed, "v1:")
	if !ok || id == "" {
		return "", ErrAdminSecret
	}
	box, err := base64.StdEncoding.Strict().DecodeString(encoded)
	n := c.aead.NonceSize()
	if err != nil || base64.StdEncoding.EncodeToString(box) != encoded || len(box) < n+c.aead.Overhead() {
		return "", ErrAdminSecret
	}
	plain, err := c.aead.Open(nil, box[:n], box[n:], []byte(adminSecretPurpose+"\x00"+id))
	if err != nil {
		return "", ErrAdminSecret
	}
	return string(plain), nil
}
