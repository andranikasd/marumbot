package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

func TestLoanSetupAtomicBankFiguresAndRetry(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	user := newUser(t, s)
	d := draft(user, t)
	d.AsOf = mustDate(t, "2026-08-01")
	original := d.Principal.Minor()
	d.Balance = money.FromMinor(original/2, d.Contract.Currency)
	statement := app.LoanSetupStatement{Draft: d, NextDue: mustDate(t, "2026-09-15"), PaymentMinor: 10000, Confirmed: true}
	service := app.LoanCommands{Store: s, Clock: clock, Users: s}
	key := uuid.NewString()
	receipt, err := service.CreateSetup(ctx, key, statement)
	if err != nil {
		t.Fatal(err)
	}
	loan, err := s.LoanForUser(ctx, receipt.ID, user)
	if err != nil {
		t.Fatal(err)
	}
	if loan.OriginalPrincipal.Minor() != original || loan.Balance.Minor() != original/2 || loan.Contract.NotBeforeDue != statement.NextDue || loan.Contract.ScheduledPayment.Minor() != 10000 || !loan.InterestUnknown {
		t.Fatal("source figures not preserved")
	}
	if _, err = loan.Schedule(); !errors.Is(err, app.ErrLoanInterestUnknown) {
		t.Fatalf("unknown-rate forecast: %v", err)
	}
	listed, err := s.LoansForUser(ctx, user, 100)
	if err != nil || len(listed) != 1 || !listed[0].InterestUnknown {
		t.Fatal("list source mismatch", err)
	}
	statement.Draft.AsOf = mustDate(t, "2026-10-01")
	retry, err := service.CreateSetup(ctx, key, statement)
	if err != nil || retry != receipt {
		t.Fatal("next-day lost-response retry failed", err)
	}
	statement.InterestKnown = true
	if _, err = service.CreateSetup(ctx, key, statement); !errors.Is(err, app.ErrConflict) {
		t.Fatal("changed command accepted", err)
	}
}

func TestSetupSourceSurvivesBalanceOnlyAndAcceptsExplicitRules(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	user := newUser(t, s)
	d := draft(user, t)
	created, err := (app.LoanCommands{Store: s, Clock: clock, Users: s}).CreateSetup(ctx, uuid.NewString(), app.LoanSetupStatement{Draft: d, NextDue: mustDate(t, "2026-09-15"), PaymentMinor: 10000, Confirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	service := app.LoanCommands{Store: s, Clock: clock, Users: s}
	balance := d.Balance.Minor()
	receipt, err := service.Revise(ctx, user, created.ID, uuid.NewString(), created.Version, app.LoanEdit{BalanceOnly: true, BalanceMinor: &balance, BalanceAsOf: d.AsOf})
	if err != nil {
		t.Fatal(err)
	}
	loan, err := s.LoanForUser(ctx, created.ID, user)
	if err != nil {
		t.Fatal(err)
	}
	if !loan.InterestUnknown || loan.Contract.NotBeforeDue.IsZero() {
		t.Fatal("balance edit asserted zero interest or erased next payment")
	}
	confirmed := true
	edit := app.LoanEdit{Name: loan.Name, Description: loan.Description, NominalRate: loan.Contract.NominalRate, Type: loan.Contract.Type, StartDate: loan.Contract.StartDate, MaturityDate: loan.Contract.MaturityDate, PaymentDay: loan.Contract.PaymentDay, PrepayEffect: loan.Contract.Prepayment.Effect, ProjectionTermsConfirmed: &confirmed}
	if _, err = service.Revise(ctx, user, created.ID, uuid.NewString(), receipt.Version, edit); err != nil {
		t.Fatal(err)
	}
	loan, err = s.LoanForUser(ctx, created.ID, user)
	if err != nil || loan.InterestUnknown || !loan.ProjectionTermsConfirmed {
		t.Fatal("explicit rate/bankrules not recorded", err)
	}
}

func TestOpeningInterestIsTiedToExactBankSnapshot(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	user := newUser(t, s)
	d := draft(user, t)
	interest := int64(1234)
	service := app.LoanCommands{Store: s, Clock: clock, Users: s}
	created, err := service.CreateSetup(ctx, uuid.NewString(), app.LoanSetupStatement{Draft: d, NextDue: mustDate(t, "2026-09-15"), PaymentMinor: 10000, Confirmed: true, AccruedInterestMinor: &interest})
	if err != nil {
		t.Fatal(err)
	}
	loan, err := s.LoanForUser(ctx, created.ID, user)
	if err != nil || !loan.OpeningInterestKnown || loan.OpeningInterestMinor != interest {
		t.Fatal("opening interest source lost", err)
	}
	balance := d.Balance.Minor()
	revised, err := service.Revise(ctx, user, created.ID, uuid.NewString(), created.Version, app.LoanEdit{BalanceOnly: true, BalanceMinor: &balance, BalanceAsOf: d.AsOf})
	if err != nil {
		t.Fatal(err)
	}
	loan, err = s.LoanForUser(ctx, created.ID, user)
	if err != nil || loan.OpeningInterestKnown {
		t.Fatal("same-date new bank balance reused stale interest", err)
	}
	zero := int64(0)
	_, err = service.Revise(ctx, user, created.ID, uuid.NewString(), revised.Version, app.LoanEdit{BalanceOnly: true, BalanceMinor: &balance, BalanceAsOf: d.AsOf, BalanceInterestMinor: &zero})
	if err != nil {
		t.Fatal(err)
	}
	loan, err = s.LoanForUser(ctx, created.ID, user)
	if err != nil || !loan.OpeningInterestKnown || loan.OpeningInterestMinor != 0 || !loan.InterestUnknown {
		t.Fatal("explicit zero lost or unknown nominal rate changed", err)
	}
}

func TestPaidMonthsRefreshAccruedSourceWithUnknownRate(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	user := newUser(t, s)
	d := draft(user, t)
	service := app.LoanCommands{Store: s, Clock: &preferenceClock{at: time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)}, Users: s}
	created, err := service.CreateSetup(ctx, uuid.NewString(), app.LoanSetupStatement{Draft: d, NextDue: mustDate(t, "2026-09-15"), PaymentMinor: 10000, Confirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	zero := int64(0)
	_, err = service.MarkMonthsPaid(ctx, user, created.ID, uuid.NewString(), created.Version, app.PaidMonthStatement{Month: "2026-09", AsOf: "2026-09-13", PrincipalMinor: d.Balance.Minor(), NextPaymentMinor: 10000, Confirmed: true, AccruedInterestMinor: &zero})
	if err != nil {
		t.Fatal("unknown nominal rate blocked bank statement", err)
	}
	loan, err := s.LoanForUser(ctx, created.ID, user)
	if err != nil || !loan.InterestUnknown || !loan.OpeningInterestKnown || loan.OpeningInterestMinor != 0 || loan.Contract.NotBeforeDue.String() != "2026-10-15" {
		t.Fatal("paid-month interest source lost", err)
	}
}
