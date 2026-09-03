package postgres_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
)

func TestBudgetPolicyAcceptanceConcurrentAppend(t *testing.T) {
	s := testStore(t)
	user := newUser(t, s)
	today := date.MustNew(2026, 9, 3)
	err := s.SetBudgetConfiguration(t.Context(), app.BudgetConfiguration{UserID: user, Currency: "USD", MonthlyMinor: 1000, PayDay: 1, OpeningMinor: 700, OpeningAsOf: today, Funding: &app.BudgetFunding{CashThrough: "2026-09-03", MonthlyMinor: 500, SpentMinor: 400}})
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Budget(t.Context(), user)
	if err != nil {
		t.Fatal(err)
	}
	p := app.BudgetPolicy{EffectiveFrom: "2026-09-15", MonthlyMinor: 1500, CarryRule: "carry_cash", ReleasedPaymentRule: "roll_all"}
	outcomes := make(chan error, 2)
	var group sync.WaitGroup
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := s.AppendBudgetPolicy(t.Context(), user, "USD", b.Version, p)
			outcomes <- err
		}()
	}
	group.Wait()
	close(outcomes)
	passed, conflicts := 0, 0
	for err := range outcomes {
		switch {
		case err == nil:
			passed++
		case errors.Is(err, app.ErrConflict):
			conflicts++
		default:
			t.Fatal(err)
		}
	}
	if passed != 1 || conflicts != 1 {
		t.Fatalf("success=%d conflicts=%d", passed, conflicts)
	}
	got, err := s.Budget(t.Context(), user)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != b.Version+1 || len(got.Policies) != 1 || got.Policies[0].Version != got.Version {
		t.Fatal("aggregate version/history lost")
	}
	if got.Opening.Minor() != 700 || got.Funding.MonthlyMinor != 500 || got.Funding.SpentMinor != 400 || got.Funding.CashThrough != "2026-09-03" {
		t.Fatal("cash facts changed")
	}
	stranger := newUser(t, s)
	if _, err := s.AppendBudgetPolicy(t.Context(), stranger, "USD", got.Version, p); !errors.Is(err, app.ErrConflict) {
		t.Fatal("foreign budget mutated")
	}
	if err := s.SetBudget(t.Context(), user, "USD", 9999, 1); !errors.Is(err, app.ErrConflict) {
		t.Fatal("legacy command overwrote policy")
	}
}

type budgetCycleTestClock struct{}

func (budgetCycleTestClock) Now() time.Time { return time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC) }
func TestBudgetCycleReconciliationAcrossCalendarBoundary(t *testing.T) {
	s := testStore(t)
	owner := newUser(t, s)
	d := draft(owner, t)
	d.Contract.Type = model.Annuity
	loan, err := s.CreateLoan(t.Context(), d)
	if err != nil {
		t.Fatal(err)
	}
	today := date.MustNew(2026, 9, 3)
	if err = s.SetBudgetConfiguration(t.Context(), app.BudgetConfiguration{UserID: owner, Currency: "AMD", MonthlyMinor: 100000, PayDay: 1, OpeningAsOf: today, OpeningMinor: 80000, Funding: &app.BudgetFunding{MonthlyMinor: 100000}}); err != nil {
		t.Fatal(err)
	}
	b, err := s.Budget(t.Context(), owner)
	if err != nil {
		t.Fatal(err)
	}
	version, err := s.AppendBudgetPolicy(t.Context(), owner, "AMD", b.Version, app.BudgetPolicy{EffectiveFrom: "2026-08-15", CycleDay: 15, MonthlyMinor: 100000, CarryRule: "carry_cash", ReleasedPaymentRule: "roll_all"})
	if err != nil {
		t.Fatal(err)
	}
	service := app.PaymentService{Store: s, Clock: budgetCycleTestClock{}}
	first, err := service.Record(t.Context(), owner, app.PaymentCommand{LoanID: loan, Key: uuid.NewString(), AmountMinor: 20000, TransactionDate: "2026-08-20", ValueDate: "2026-08-20"})
	if err != nil {
		t.Fatal(err)
	}
	paid, err := service.Record(t.Context(), owner, app.PaymentCommand{LoanID: loan, Key: uuid.NewString(), ExpectedVersion: first.Version, AmountMinor: 10000, TransactionDate: "2026-09-02", ValueDate: "2026-09-02"})
	if err != nil {
		t.Fatal(err)
	}
	c := app.ReconciliationCommand{LoanID: loan, Key: uuid.NewString(), ExpectedVersion: paid.Version, BudgetVersion: version, AsOf: today.String(), PrincipalMinor: 60000, NextDue: "2026-09-15", NextPaymentMinor: 30000, CashMinor: 60000, SpentMinor: 10000, SpentPeriodStart: "2026-08-15", IncludePosted: true}
	if _, err = service.Reconcile(t.Context(), owner, c); !errors.Is(err, app.ErrPaymentInvalid) {
		t.Fatalf("previous-month payment omitted: %v", err)
	}
	c.SpentMinor = 30000
	c.SpentPeriodStart = "2026-09-01"
	if _, err = service.Reconcile(t.Context(), owner, c); !errors.Is(err, app.ErrPaymentInvalid) {
		t.Fatal("wrong period accepted", err)
	}
	c.SpentPeriodStart = "2026-08-15"
	receipt, err := service.Reconcile(t.Context(), owner, c)
	if err != nil {
		t.Fatal(err)
	}
	again, err := service.Reconcile(t.Context(), owner, c)
	if err != nil || receipt != again {
		t.Fatal("retry changed receipt", err)
	}
	b, err = s.Budget(t.Context(), owner)
	if err != nil {
		t.Fatal(err)
	}
	if b.Funding.SpentMinor != 30000 || b.Funding.SpentPeriodStart != "2026-08-15" || b.Funding.CashThrough != "2026-09-03" || b.Opening.Minor() != 60000 {
		t.Fatal("cycle statement lost")
	}
	cp := b.CashPlan(today)
	if cp.Spending.RuleError != "" || cp.Spending.Spent.Minor() != 30000 {
		t.Fatal("cycle statement not consumed")
	}
}
