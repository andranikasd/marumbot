package app

import (
	"context"
	"errors"
	"fmt"
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
	listed  []string
	visited []string
	onLoan  func(string)
	written map[string]bool // user|day|goal already stored
}

func (f *shadowFakes) LoansForUser(ctx context.Context, id string, _ int32) ([]UserLoan, error) {
	f.visited = append(f.visited, id)
	if f.onLoan != nil {
		f.onLoan(id)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return f.loans, nil
}

func (f *shadowFakes) Budget(context.Context, string) (Budget, error) { return f.budget, nil }
func (f *shadowFakes) SetBudget(context.Context, string, string, int64, int) error {
	return nil
}

func (f *shadowFakes) ActiveLoanUsers(_ context.Context, after string, limit int32) ([]string, error) {
	f.listed = append(f.listed, after)
	var ids []string
	for _, id := range f.users {
		if id > after && len(ids) < int(limit) {
			ids = append(ids, id)
		}
	}
	return ids, nil
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
		budget: Budget{Currency: "AMD", Monthly: money.FromMinor(25_000_000, amd), Set: true, PayDay: 1, Funding: &BudgetFunding{MonthlyMinor: 25_000_000}},
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

func TestTickShadowResumesInterruptedFinalPage(t *testing.T) {
	f := &shadowFakes{users: []string{"user-1", "user-2", "user-3"}}
	w := shadowWorker(t, f)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.onLoan = func(id string) {
		if id == "user-2" {
			cancel()
		}
	}
	if _, err := w.TickShadow(ctx, f); !errors.Is(err, context.Canceled) {
		t.Fatalf("interrupted tick: %v", err)
	}
	f.onLoan = nil
	if _, err := w.TickShadow(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if len(f.listed) != 2 || f.listed[1] != "user-1" {
		t.Fatalf("resume cursors = %v; want interrupted account retried after user-1", f.listed)
	}
	want := []string{"user-1", "user-2", "user-2", "user-3"}
	if fmt.Sprint(f.visited) != fmt.Sprint(want) {
		t.Fatalf("visits = %v; want %v", f.visited, want)
	}
	if _, err := w.TickShadow(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if len(f.listed) != 2 {
		t.Fatal("completed walk did not observe six-hour pause")
	}
	w.Clock.(*fixedClock).at = w.Clock.Now().Add(shadowEvery)
	if _, err := w.TickShadow(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if len(f.listed) != 3 || f.listed[2] != "" {
		t.Fatalf("new walk cursors = %v", f.listed)
	}
}

func TestTickShadowContinuesFullPageWithoutPause(t *testing.T) {
	f := &shadowFakes{}
	for i := 0; i < shadowWalkLimit+1; i++ {
		f.users = append(f.users, fmt.Sprintf("user-%04d", i))
	}
	w := shadowWorker(t, f)
	for i := 0; i < 3; i++ {
		if _, err := w.TickShadow(context.Background(), f); err != nil {
			t.Fatal(err)
		}
	}
	if len(f.visited) != shadowWalkLimit+1 {
		t.Fatalf("visited %d accounts", len(f.visited))
	}
	if len(f.listed) != 2 || f.listed[1] != f.users[shadowWalkLimit-1] {
		t.Fatalf("page cursors = %v", f.listed)
	}
}
