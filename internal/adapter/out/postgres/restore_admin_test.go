package postgres

import (
	"context"
	"encoding/base64"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/internal/identity"
)

func TestRestoreAdminSecretsChecksEveryPage(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := t.Context()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	setup, err := os.ReadFile("../../../../queries/tests/restore_admin_setup.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, string(setup)); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	cipher, err := identity.NewSecretCipher(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatal(err)
	}
	writer := adminTransaction{tx: tx, secrets: cipher}
	var last app.AdminIdentity
	for i := 0; i < 105; i++ {
		id := uuid.NewString()
		row := app.AdminIdentity{ID: id, Username: id, PasswordHash: "fixture", TOTPSecret: "fixture-secret", Version: 1, Enabled: false}
		if err := writer.SaveIdentity(ctx, row, 0); err != nil {
			t.Fatal(err)
		}
		if row.ID > last.ID {
			last = row
		}
	}
	if err := verifyAdminSecrets(ctx, tx.Query, cipher); err != nil {
		t.Fatal(err)
	}
	key[0] = 1
	wrong, err := identity.NewSecretCipher(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyAdminSecrets(ctx, tx.Query, wrong); err == nil {
		t.Fatal("wrong key accepted")
	}
	// Corruption in the final page must fail even for a disabled administrator.
	last.TOTPSecret = "v1:invalid"
	last.Version = 2
	raw := adminTransaction{tx: tx}
	if err := raw.SaveIdentity(ctx, last, 1); err != nil {
		t.Fatal(err)
	}
	if err := verifyAdminSecrets(ctx, tx.Query, cipher); err == nil {
		t.Fatal("corrupt last-page credential accepted")
	}
}
