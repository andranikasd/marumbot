package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/pkg/core/date"
)

type budgetCommandClock struct{ at time.Time }

func (c budgetCommandClock) Now() time.Time { return c.at }

func TestBudgetCommandRetryAndConcurrentWriters(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	user := newUser(t, s)
	clock := budgetCommandClock{time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)}
	svc := app.BudgetCommands{Store: s, Clock: clock}
	zero := int64(0)
	command := app.BudgetConfiguration{Key: uuid.NewString(), ExpectedVersion: &zero, UserID: user, Currency: "AMD", MonthlyMinor: 100000, PayDay: 5, OpeningAsOf: date.MustNew(2026, 8, 1), Funding: &app.BudgetFunding{MonthlyMinor: 100000}}
	const callers = 4
	var wg sync.WaitGroup
	errs := make(chan error, callers)
	for range callers {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- svc.SetBudgetConfiguration(ctx, command) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	b, err := s.Budget(ctx, user)
	if err != nil || b.Version != 1 {
		t.Fatalf("single declaration: version=%d err=%v", b.Version, err)
	}
	conflict := command
	conflict.MonthlyMinor++
	if err = svc.SetBudgetConfiguration(ctx, conflict); !errors.Is(err, app.ErrConflict) {
		t.Fatal("changed retry accepted", err)
	}
	changed := command
	changed.Key = uuid.NewString()
	changed.ExpectedVersion = &b.Version
	changed.MonthlyMinor = 120000
	if err = svc.SetBudgetConfiguration(ctx, changed); err != nil {
		t.Fatal(err)
	}
	// The original receipt survives later edits and a day boundary.
	svc.Clock = budgetCommandClock{clock.at.Add(24 * time.Hour)}
	if err = svc.SetBudgetConfiguration(ctx, command); err != nil {
		t.Fatal("original retry", err)
	}
	b, err = s.Budget(ctx, user)
	if err != nil || b.Version != 2 || b.Monthly.Minor() != 120000 {
		t.Fatal("retry rewrote newer source", err)
	}
	command.Key = uuid.NewString()
	command.ExpectedVersion = &b.Version
	if err = svc.SetBudgetConfiguration(ctx, command); !errors.Is(err, app.ErrConflict) {
		t.Fatal("old first-time statement accepted", err)
	}
}

func TestBudgetFundingAndPolicyReceiptsPreserveFacts(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	user := newUser(t, s)
	clock := budgetCommandClock{time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)}
	svc := app.BudgetCommands{Store: s, Clock: clock}
	zero := int64(0)
	c := app.BudgetConfiguration{Key: uuid.NewString(), ExpectedVersion: &zero, UserID: user, Currency: "AMD", MonthlyMinor: 100000, PayDay: 5, OpeningMinor: 50000, ReserveMinor: 10000, OpeningAsOf: date.MustNew(2026, 8, 1), Funding: &app.BudgetFunding{MonthlyMinor: 100000, SpentMinor: 20000, CashThrough: "2026-08-01"}}
	if err := svc.SetBudgetConfiguration(ctx, c); err != nil {
		t.Fatal(err)
	}
	p := app.BudgetPolicy{EffectiveFrom: "2026-08-01", MonthlyMinor: 100000, CarryRule: "carry_cash", ReleasedPaymentRule: "roll_all"}
	key := uuid.NewString()
	v, err := svc.SavePolicy(ctx, user, "AMD", 1, key, p)
	if err != nil {
		t.Fatal(err)
	}
	in := app.BudgetFundingUpdate{Key: uuid.NewString(), Currency: "AMD", ExpectedVersion: v, PayDay: 10, MonthlyMinor: 150000, Events: []app.BudgetCashEvent{{On: "2026-08-01", Minor: 1000}}}
	if err = svc.UpdateBudgetFunding(ctx, user, in); err != nil {
		t.Fatal(err)
	}
	svc.Clock = budgetCommandClock{clock.at.Add(24 * time.Hour)}
	if err = svc.UpdateBudgetFunding(ctx, user, in); err != nil {
		t.Fatal("funding retry after midnight", err)
	}
	if got, err := svc.SavePolicy(ctx, user, "AMD", 1, key, p); err != nil || got != v {
		t.Fatal("policy retry", err)
	}
	b, err := s.Budget(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	if b.Version != 3 || len(b.Policies) != 1 || b.Opening.Minor() != 50000 || b.Reserve.Minor() != 10000 || b.Funding.SpentMinor != 20000 || b.Funding.CashThrough != "2026-08-01" || b.Funding.MonthlyMinor != 150000 {
		t.Fatal("funding update rewrote other facts")
	}
	other := c
	other.Key = uuid.NewString()
	other.Currency = "USD"
	other.OpeningAsOf = date.MustNew(2026, 8, 2)
	other.OpeningMinor = 900000
	other.ExpectedVersion = &zero
	if err = svc.SetBudgetConfiguration(ctx, other); err != nil {
		t.Fatal(err)
	}
	// The newest USD statement must not authorize retained AMD cash.
	in.Key = uuid.NewString()
	in.ExpectedVersion = b.Version
	in.Events = nil
	if err = svc.UpdateBudgetFunding(ctx, user, in); !errors.Is(err, app.ErrConflict) {
		t.Fatal("another currency supplied retained cash context", err)
	}
	in.Key = uuid.NewString()
	in.ExpectedVersion = b.Version
	in.Events = nil
	in.Currency = "USD"
	if err = svc.UpdateBudgetFunding(ctx, user, in); !errors.Is(err, app.ErrConflict) {
		t.Fatal("wrong currency accepted", err)
	}
}

// A reconciliation holding a loan/budget must be able to insert source history
// while another command owns the user mutex. FOR UPDATE would block its FK check
// and form a lock cycle when that command subsequently requests the loan.
func TestCommandUserMutexAllowsSourceHistoryForeignKey(t *testing.T) {
	s := testStore(t)
	user := newUser(t, s)
	ctx := t.Context()
	if err := s.SetBudget(ctx, user, "AMD", 10000, 1); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"budget and activation", "loan"} {
		t.Run(kind, func(t *testing.T) {
			bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			if kind == "loan" {
				tx, err := s.BeginLoanCommand(bounded)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = tx.Rollback(ctx) }()
				if err = tx.LockUser(bounded, user); err != nil {
					t.Fatal(err)
				}
			} else {
				tx, err := s.BeginBudgetCommand(bounded)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = tx.Rollback(ctx) }()
				if err = tx.LockBudgetUser(bounded, user); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.SetBudget(bounded, user, "AMD", 20000, 1); err != nil {
				t.Fatal("user mutex blocked source-history foreign key", err)
			}
		})
	}
}
