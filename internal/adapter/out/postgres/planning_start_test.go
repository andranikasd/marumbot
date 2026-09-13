package postgres_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

func TestPlanningStartDurableRetryAndConfigurationPreservation(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	user := newUser(t, s)
	svc := app.BudgetCommands{Store: s, Clock: budgetCommandClock{time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)}}
	zero := int64(0)
	cfg := app.BudgetConfiguration{UserID: user, Key: uuid.NewString(), ExpectedVersion: &zero, Currency: "USD", MonthlyMinor: 10000, PayDay: 25, OpeningMinor: 5000, OpeningAsOf: date.MustNew(2026, 9, 20), Funding: &app.BudgetFunding{MonthlyMinor: 10000, SpentMinor: 2000, PlanningStartMonth: "2026-10"}}
	if err := svc.SetBudgetConfiguration(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	b, err := s.Budget(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	if b.Funding.PlanningStartMonth != "" {
		t.Fatal("configuration bypassed planning-start validation")
	}
	in := app.PlanningStartUpdate{Key: uuid.NewString(), ExpectedVersion: b.Version, Month: "2026-10"}
	version, err := svc.SetPlanningStart(ctx, user, in)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := svc.SetPlanningStart(ctx, user, in)
	if err != nil || retry != version {
		t.Fatal("retry not durable", err)
	}
	in.Month = ""
	if _, err = svc.SetPlanningStart(ctx, user, in); !errors.Is(err, app.ErrConflict) {
		t.Fatal("changed retry accepted", err)
	}
	b, err = s.Budget(ctx, user)
	if err != nil || b.Funding.PlanningStartMonth != "2026-10" || b.Funding.SpentMinor != 2000 || b.Opening.Minor() != 5000 {
		t.Fatal("preference changed cash facts", err)
	}
	cfg.Key = uuid.NewString()
	cfg.ExpectedVersion = &b.Version
	cfg.Funding.PlanningStartMonth = ""
	cfg.MonthlyMinor = 11000
	if err = svc.SetBudgetConfiguration(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	b, err = s.Budget(ctx, user)
	if err != nil || b.Funding.PlanningStartMonth != "2026-10" {
		t.Fatal("budget edit erased preference", err)
	}
	in.Key = uuid.NewString()
	in.ExpectedVersion = b.Version
	if _, err = svc.SetPlanningStart(ctx, user, in); err != nil {
		t.Fatal(err)
	}
	b, err = s.Budget(ctx, user)
	if err != nil || b.Funding.PlanningStartMonth != "" {
		t.Fatal("resume failed", err)
	}
}

func TestPlanningStartRequiresEveryOutstandingLoanMonthPaid(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	user := newUser(t, s)
	svc := app.BudgetCommands{Store: s, Clock: budgetCommandClock{time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)}}
	cfg := app.BudgetConfiguration{UserID: user, Currency: "AMD", MonthlyMinor: 100000, PayDay: 25, OpeningAsOf: date.MustNew(2026, 9, 20), Funding: &app.BudgetFunding{MonthlyMinor: 100000}}
	if err := s.SetBudgetConfiguration(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	paid := draft(user, t)
	paid.Contract.NotBeforeDue = date.MustNew(2026, 10, 1)
	paid.AsOf = date.MustNew(2026, 9, 20)
	if _, err := s.CreateLoan(ctx, paid); err != nil {
		t.Fatal(err)
	}
	b, err := s.Budget(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	cmd := app.PlanningStartUpdate{Key: uuid.NewString(), ExpectedVersion: b.Version, Month: "2026-10"}
	if _, err = svc.SetPlanningStart(ctx, user, cmd); err != nil {
		t.Fatal("paid loan blocked next month", err)
	}
	unpaid := draft(user, t)
	if _, err = s.CreateLoan(ctx, unpaid); err != nil {
		t.Fatal(err)
	}
	b, err = s.Budget(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	cmd.Key = uuid.NewString()
	cmd.ExpectedVersion = b.Version
	var unsupported *plan.UnsupportedError
	if _, err = svc.SetPlanningStart(ctx, user, cmd); !errors.As(err, &unsupported) || unsupported.Feature != "required payments remain before planning start" {
		t.Fatal("one unpaid loan was hidden", err)
	}
	for _, month := range []string{"2026-08", "2026-11", "bad"} {
		cmd.Key = uuid.NewString()
		cmd.Month = month
		if _, err = svc.SetPlanningStart(ctx, user, cmd); !errors.Is(err, app.ErrPaymentInvalid) {
			t.Fatal("invalid month accepted", month, err)
		}
	}
	cmd.Key = uuid.NewString()
	cmd.Month = ""
	if _, err = svc.SetPlanningStart(ctx, user, cmd); err != nil {
		t.Fatal("unpaid loan prevented resuming today", err)
	}
}
