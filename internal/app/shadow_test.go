package app

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/andranikasd/marumbot/pkg/core/allocation"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

type shadowFakes struct {
	loans   []UserLoan
	budget  Budget
	stored  []ShadowRecommendation
	users   []string
	written map[string]bool // user|day|goal already stored
}

func (f *shadowFakes) LoansForUser(context.Context, string, int32) ([]UserLoan, error) {
	return f.loans, nil
}

func (f *shadowFakes) Budget(context.Context, string) (Budget, error) { return f.budget, nil }
func (f *shadowFakes) SetBudget(context.Context, string, string, int64, int) error {
	return nil
}

func (f *shadowFakes) ActiveLoanUsers(context.Context, int32) ([]string, error) {
	return f.users, nil
}

func (f *shadowFakes) RecordShadow(_ context.Context, r ShadowRecommendation) (bool, error) {
	key := r.UserID + "|" + r.ComputedOn + "|" + r.Goal
	if f.written == nil {
		f.written = map[string]bool{}
	}
	if f.written[key] {
		return false, nil
	}
	f.written[key] = true
	f.stored = append(f.stored, r)
	return true, nil
}

type fixedClock struct{ at time.Time }

func (c *fixedClock) Now() time.Time { return c.at }

func shadowLoan(t *testing.T) UserLoan {
	t.Helper()
	amd := money.MustLookup("AMD")
	v := date.MustNew(2026, 1, 15)
	return UserLoan{
		ID: "loan-a", Name: "Car",
		Contract: model.Contract{
			LoanID: "loan-a", Version: 1, Currency: amd, EffectiveFrom: v,
			NominalRate: money.RateFromPercent(21, 0), DayCount: money.Actual365,
			Type: model.Annuity, StartDate: v, MaturityDate: date.MustNew(2028, 1, 15),
			PaymentDay: 15, Rounding: money.DefaultPolicy(amd),
		},
		Balance: money.FromMinor(120_000_000, amd),
		AsOf:    v,
		Excess:  allocation.ExcessReducePrincipal,
		Trust:   "user_entered",
	}
}

func shadowWorker(t *testing.T, f *shadowFakes) *Worker {
	t.Helper()
	return &Worker{
		Loans:           f,
		Budgets:         f,
		Shadow:          f,
		Clock:           &fixedClock{at: time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC)},
		Log:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		DefaultCurrency: money.MustLookup("AMD"),
	}
}

func TestTickShadowStoresSilently(t *testing.T) {
	amd := money.MustLookup("AMD")
	f := &shadowFakes{
		loans:  []UserLoan{shadowLoan(t)},
		budget: Budget{Currency: "AMD", Monthly: money.FromMinor(25_000_000, amd), Set: true, PayDay: 1},
		users:  []string{"user-1", "user-2"},
	}
	w := shadowWorker(t, f)

	n, err := w.TickShadow(context.Background(), f)
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if n != 2 || len(f.stored) != 2 {
		t.Fatalf("want 2 recommendations stored, got n=%d stored=%d", n, len(f.stored))
	}
	r := f.stored[0]
	if r.ComputedOn != "2026-01-15" {
		t.Errorf("computed_on = %q, want the valuation date", r.ComputedOn)
	}
	if r.Engine == "" || r.Fingerprint == "" || len(r.Sheet) == 0 {
		t.Errorf("recommendation missing evidence fields: %+v", r)
	}
	if r.Goal != "least_interest" {
		t.Errorf("goal = %q, want the default least_interest", r.Goal)
	}

	// The same tick within the gate window is a no-op: nothing recomputes,
	// nothing is stored twice.
	n, err = w.TickShadow(context.Background(), f)
	if err != nil || n != 0 {
		t.Fatalf("second tick inside the gate: n=%d err=%v", n, err)
	}
}

func TestTickShadowSkipsAccountsThatCannotPlan(t *testing.T) {
	f := &shadowFakes{
		loans:  nil, // no loans: PlanSheet answers ErrNotFound
		budget: Budget{},
		users:  []string{"user-1"},
	}
	w := shadowWorker(t, f)
	n, err := w.TickShadow(context.Background(), f)
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if n != 0 || len(f.stored) != 0 {
		t.Fatalf("an unplannable account produced evidence: n=%d stored=%d", n, len(f.stored))
	}
}

func TestTickShadowWithoutAStoreIsOff(t *testing.T) {
	f := &shadowFakes{users: []string{"user-1"}}
	w := shadowWorker(t, f)
	w.Shadow = nil
	n, err := w.TickShadow(context.Background(), f)
	if err != nil || n != 0 {
		t.Fatalf("shadow without a store: n=%d err=%v", n, err)
	}
}
