package postgres_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"testing"

	"github.com/google/uuid"

	"github.com/andranikasd/marumbot/internal/adapter/out/postgres"
	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/internal/identity"
)

func adminSecretCipher(t *testing.T, keyByte byte) *identity.SecretCipher {
	t.Helper()
	c, err := identity.NewSecretCipher(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{keyByte}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func saveAdminSecret(t *testing.T, s *postgres.Store, secret string) app.AdminIdentity {
	t.Helper()
	ctx := context.Background()
	id := app.AdminIdentity{ID: uuid.NewString(), Username: "secret-test-" + uuid.NewString(), PasswordHash: "test-hash", TOTPSecret: secret, Enabled: true, Version: 1}
	tx, err := s.BeginAdmin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := tx.SaveIdentity(ctx, id, 0); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestAdminSecretsAreEncryptedAndBoundToIdentity(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	cipher := adminSecretCipher(t, 42)
	s.WithAdminSecrets(cipher)
	id := saveAdminSecret(t, s, "totp-secret-test-value")
	got, err := s.AdminIdentityByUsername(ctx, id.Username)
	if err != nil || got.TOTPSecret != id.TOTPSecret {
		t.Fatalf("encrypted credential round trip: %v", err)
	}
	tx, err := s.BeginAdmin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got, err = tx.Identity(ctx, id.ID)
	_ = tx.Rollback(ctx)
	if err != nil || got.TOTPSecret != id.TOTPSecret {
		t.Fatalf("transaction credential round trip: %v", err)
	}
	wrong := testStore(t)
	wrong.WithAdminSecrets(adminSecretCipher(t, 43))
	if got, err := wrong.AdminIdentityByUsername(ctx, id.Username); err == nil || got.TOTPSecret != "" {
		t.Fatal("wrong key decrypted stored credential")
	}
	raw := testStore(t)
	if _, err := raw.AdminIdentityByUsername(ctx, id.Username); err == nil {
		t.Fatal("credential was readable without encryption key")
	}
	foreign, err := cipher.Seal(uuid.NewString(), "swapped-secret")
	if err != nil {
		t.Fatal(err)
	}
	swapped := saveAdminSecret(t, raw, foreign)
	if got, err := s.AdminIdentityByUsername(ctx, swapped.Username); err == nil || got.TOTPSecret != "" {
		t.Fatal("credential from another identity accepted")
	}
}

func TestLegacyAdminSecretMigrationPreservesCredential(t *testing.T) {
	ctx := context.Background()
	raw := testStore(t)
	id := saveAdminSecret(t, raw, "legacy-totp-secret-test-value")
	s := testStore(t)
	s.WithAdminSecrets(adminSecretCipher(t, 42))
	if err := app.ProtectAdminSecrets(ctx, s); err != nil {
		t.Fatal(err)
	}
	got, err := s.AdminIdentityByUsername(ctx, id.Username)
	if err != nil || got.TOTPSecret != id.TOTPSecret {
		t.Fatalf("migrated credential round trip: %v", err)
	}
	legacy, err := s.LegacyAdminSecrets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range legacy {
		if row.ID == id.ID {
			t.Fatal("migrated credential remains plaintext")
		}
	}
	if _, err := raw.AdminIdentityByUsername(ctx, id.Username); err == nil {
		t.Fatal("migrated credential readable without key")
	}
	if err := app.ProtectAdminSecrets(ctx, s); err != nil {
		t.Fatalf("repeated migration: %v", err)
	}
}
