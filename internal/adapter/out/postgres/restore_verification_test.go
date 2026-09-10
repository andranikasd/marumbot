package postgres_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"

	"github.com/andranikasd/marumbot/internal/adapter/out/postgres"
	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/internal/identity"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

type restoreFixtureUsers struct{ id string }

// This test scopes borrower verification to its fixture. The admin verifier has
// separate coverage; the release rehearsal uses the real store for both.
func (u restoreFixtureUsers) VerifyAdminSecrets(context.Context) error { return nil }

func (u restoreFixtureUsers) MenuUsers(_ context.Context, after string, _ int32) ([]app.MenuUser, error) {
	if after != "" {
		return nil, nil
	}
	return []app.MenuUser{{ID: u.id, Locale: "hy"}}, nil
}

// This fixture can populate an otherwise empty database for an actual dump,
// restore and -verify-restore rehearsal. The all-zero master is test-only.
func TestRestoreVerificationFixture(t *testing.T) {
	s := testStore(t)
	s.WithAdminSecrets(adminSecretCipher(t, 0))
	saveAdminSecret(t, s, "restore-fixture-totp-secret")
	ctx := context.Background()
	master := base64.StdEncoding.EncodeToString(make([]byte, 32))
	cipher, err := identity.New(master)
	if err != nil {
		t.Fatal(err)
	}
	in := freshUpsert()
	telegramID, chatID := randUpdateID(t), randUpdateID(t)
	in.UserTag, in.ChatTag = cipher.Tag(telegramID), cipher.Tag(chatID)
	in.UserSealed, err = cipher.Seal(telegramID)
	if err != nil {
		t.Fatal(err)
	}
	in.ChatSealed, err = cipher.Seal(chatID)
	if err != nil {
		t.Fatal(err)
	}
	in.KeyVersion = identity.KeyVersion
	account, err := s.UpsertByTelegram(ctx, in)
	if err != nil || !account.Created {
		t.Fatalf("fresh encrypted account: %v", err)
	}
	if _, err := s.CreateLoan(ctx, draft(account.ID, t)); err != nil {
		t.Fatal(err)
	}
	if err := s.SetBudgetConfiguration(ctx, app.BudgetConfiguration{UserID: account.ID, Currency: "AMD", MonthlyMinor: 600_000_00, PayDay: 5, OpeningAsOf: mustDate(t, "2026-08-01"), Funding: &app.BudgetFunding{MonthlyMinor: 600_000_00}}); err != nil {
		t.Fatal(err)
	}
	w := app.Worker{Users: s, Loans: s, Budgets: s, Plans: s, History: s, Clock: historyClock{}, DefaultCurrency: money.MustLookup("AMD"), Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	sheet, err := w.PlanSheet(ctx, account.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.ActivateProposal(ctx, account.ID, app.PlanActivationCommand{Proposal: sheet.Proposal, Key: uuid.NewString(), ExpectedRevision: sheet.ActiveRevision}); err != nil {
		t.Fatal(err)
	}
	versions, _, err := s.PlanHistory(ctx, account.ID)
	if err != nil || len(versions) != 1 {
		t.Fatalf("fixture must include one approved manifest: %v", err)
	}
	users := restoreFixtureUsers{id: account.ID}
	if err := app.VerifyRestoredData(ctx, users, postgres.ChatLookup{Store: s, Cipher: cipher}, s, users); err != nil {
		t.Fatalf("restore verification failed: %v", err)
	}
	wrong, err := identity.New(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	if err := app.VerifyRestoredData(ctx, users, postgres.ChatLookup{Store: s, Cipher: wrong}, s, users); err == nil {
		t.Fatal("restore verification accepted wrong identity key")
	}
}
