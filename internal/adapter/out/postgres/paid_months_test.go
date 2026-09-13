package postgres_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
)

func TestPaidMonthStatementSourceAndDurableRetry(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	user := newUser(t, s)
	clock := &preferenceClock{at: time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)}
	service := app.LoanCommands{Store: s, Clock: clock, Users: s}
	d := draft(user, t)
	d.Contract.Type = model.Annuity
	created, err := service.Create(ctx, uuid.NewString(), d)
	if err != nil {
		t.Fatal(err)
	}
	input := app.PaidMonthStatement{Month: "2026-09", AsOf: "2026-09-13", PrincipalMinor: 60000, NextPaymentMinor: 30000, Confirmed: true}
	key := uuid.NewString()
	receipt, err := service.MarkMonthsPaid(ctx, user, created.ID, key, created.Version, input)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Version <= created.Version {
		t.Fatal("source statement did not advance mutation version")
	}
	loan, err := s.LoanForUser(ctx, created.ID, user)
	if err != nil {
		t.Fatal(err)
	}
	if loan.Balance.Minor() != 60000 || loan.Trust != "user_entered" || loan.AsOf.String() != input.AsOf || loan.Contract.NotBeforeDue.String() != "2026-10-15" || loan.Contract.ScheduledPayment.Minor() != 30000 || loan.UnreconciledPayments {
		t.Fatal("bank source statement not retained")
	}
	// This loan-only action does not require a budget or invent an expenditure.
	payments, err := s.PaymentContext(ctx, created.ID, user)
	if err != nil {
		t.Fatal(err)
	}
	if payments.Version != 0 {
		t.Fatal("paid month fabricated a payment event")
	}
	facts, err := s.BorrowerActivity(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 2 {
		t.Fatalf("expected original and new bank statement, got %d", len(facts))
	}
	changed := input
	changed.PrincipalMinor++
	if _, err = service.MarkMonthsPaid(ctx, user, created.ID, key, created.Version, changed); !errors.Is(err, app.ErrConflict) {
		t.Fatal("key payload conflict lost", err)
	}
	if _, err = service.MarkMonthsPaid(ctx, user, created.ID, uuid.NewString(), created.Version, input); !errors.Is(err, app.ErrConflict) {
		t.Fatal("stale version accepted", err)
	}
	if _, err = service.MarkMonthsPaid(ctx, newUser(t, s), created.ID, uuid.NewString(), receipt.Version, input); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("ownership violated", err)
	}
	// The receipt remains replayable after midnight and after archival.
	clock.at = clock.at.Add(24 * time.Hour)
	if _, err = service.Archive(ctx, user, created.ID, uuid.NewString(), receipt.Version); err != nil {
		t.Fatal(err)
	}
	restarted := app.LoanCommands{Store: s, Clock: clock, Users: s}
	again, err := restarted.MarkMonthsPaid(ctx, user, created.ID, key, created.Version, input)
	if err != nil || again != receipt {
		t.Fatal("durable next-day archived retry failed", err)
	}
	facts, err = s.BorrowerActivity(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 2 {
		t.Fatal("retry appended another statement")
	}
}

func TestPaidMonthStatementRejectsInvalidAndPendingSources(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	user := newUser(t, s)
	clock := &preferenceClock{at: time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)}
	service := app.LoanCommands{Store: s, Clock: clock, Users: s}
	d := draft(user, t)
	d.Contract.Type = model.Annuity
	created, err := service.Create(ctx, uuid.NewString(), d)
	if err != nil {
		t.Fatal(err)
	}
	valid := app.PaidMonthStatement{Month: "2026-09", AsOf: "2026-09-13", PrincipalMinor: 60000, NextPaymentMinor: 30000, Confirmed: true}
	for _, tt := range []struct {
		name string
		edit func(*app.PaidMonthStatement)
	}{
		{"unconfirmed", func(s *app.PaidMonthStatement) { s.Confirmed = false }},
		{"stale balance", func(s *app.PaidMonthStatement) { s.AsOf = "2026-09-12" }},
		{"future balance", func(s *app.PaidMonthStatement) { s.AsOf = "2026-09-14" }},
		{"future month", func(s *app.PaidMonthStatement) { s.Month = "2026-10" }},
		{"overdue remains", func(s *app.PaidMonthStatement) { s.Month = "2026-07" }},
		{"negative balance", func(s *app.PaidMonthStatement) { s.PrincipalMinor = -1 }},
		{"unsafe precision", func(s *app.PaidMonthStatement) { s.PrincipalMinor = 9007199254740992 }},
		{"missing instalment", func(s *app.PaidMonthStatement) { s.NextPaymentMinor = 0 }},
		{"instalment cannot repay", func(s *app.PaidMonthStatement) { s.NextPaymentMinor = 1 }},
		{"settled with instalment", func(s *app.PaidMonthStatement) { s.PrincipalMinor = 0 }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			input := valid
			tt.edit(&input)
			if _, err := service.MarkMonthsPaid(ctx, user, created.ID, uuid.NewString(), created.Version, input); !errors.Is(err, app.ErrPaymentInvalid) {
				t.Fatal("invalid source accepted", err)
			}
		})
	}
	payments := app.PaymentService{Store: s, Clock: clock, Users: s}
	pending, err := payments.Record(ctx, user, app.PaymentCommand{LoanID: created.ID, Key: uuid.NewString(), AmountMinor: 100, TransactionDate: "2026-09-13"})
	if err != nil {
		t.Fatal(err)
	}
	loan, err := s.LoanForUser(ctx, created.ID, user)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.MarkMonthsPaid(ctx, user, created.ID, uuid.NewString(), loan.MutationVersion, valid); !errors.Is(err, app.ErrPaymentReconciliation) {
		t.Fatal("pending posting bypassed reconciliation", err)
	}
	// Correcting the pending transfer into a posted payment still requires
	// a reconciliation that explicitly covers the event, not this shortcut.
	_, err = payments.Record(ctx, user, app.PaymentCommand{LoanID: created.ID, Key: uuid.NewString(), ExpectedVersion: pending.Version, Replaces: pending.ID, AmountMinor: 100, TransactionDate: "2026-09-13", ValueDate: "2026-09-13"})
	if err != nil {
		t.Fatal(err)
	}
	loan, err = s.LoanForUser(ctx, created.ID, user)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.MarkMonthsPaid(ctx, user, created.ID, uuid.NewString(), loan.MutationVersion, valid); !errors.Is(err, app.ErrPaymentReconciliation) {
		t.Fatal("posted transfer bypassed reconciliation", err)
	}
}

func TestPaidMonthStatementSettlesLoan(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	user := newUser(t, s)
	clock := &preferenceClock{at: time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)}
	service := app.LoanCommands{Store: s, Clock: clock, Users: s}
	d := draft(user, t)
	d.Contract.Type = model.Annuity
	created, err := service.Create(ctx, uuid.NewString(), d)
	if err != nil {
		t.Fatal(err)
	}
	input := app.PaidMonthStatement{Month: "2026-09", AsOf: "2026-09-13", Confirmed: true}
	if _, err = service.MarkMonthsPaid(ctx, user, created.ID, uuid.NewString(), created.Version, input); err != nil {
		t.Fatal(err)
	}
	loan, err := s.LoanForUser(ctx, created.ID, user)
	if err != nil {
		t.Fatal(err)
	}
	if loan.Balance.Sign() != 0 || !loan.Contract.NotBeforeDue.IsZero() {
		t.Fatal("settled bank statement retained debt")
	}
	if loan.AsOf != date.MustNew(2026, 9, 13) {
		t.Fatal("settlement moved date")
	}
}
