package plan

import (
	"errors"
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/allocation"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// The adjustable budget: per-month overrides and opening cash flow through
// the search and change the answer the way money changes it.

func overrideInput(t *testing.T) Input {
	t.Helper()
	amd := money.MustLookup("AMD")
	v := date.MustNew(2026, 1, 15)
	return Input{
		ValuationDate: v,
		Cash:          CashPlan{Monthly: money.FromMinor(30_000_000, amd), PayDay: 5},
		Loans: []Position{{
			ID: "a", Name: "Car",
			Contract: model.Contract{
				LoanID: "a", Version: 1, Currency: amd, EffectiveFrom: v,
				NominalRate: money.RateFromPercent(18, 0), DayCount: money.Actual365,
				Type: model.Annuity, StartDate: v, MaturityDate: date.MustNew(2028, 1, 15),
				PaymentDay: 20, Rounding: money.DefaultPolicy(amd),
			},
			Balance: money.FromMinor(500_000_000, amd), From: v,
			Excess: allocation.ExcessReducePrincipal, Trust: "user_entered",
		}},
	}
}

func TestMonthKey(t *testing.T) {
	if k := MonthKey(date.MustNew(2026, 3, 7)); k != "2026-03" {
		t.Fatalf("MonthKey = %q", k)
	}
}

func TestOverridesChangeTheAnswer(t *testing.T) {
	amd := money.MustLookup("AMD")
	goal := Goal{Kind: LeastInterest}

	base, err := Search(overrideInput(t), goal)
	if err != nil {
		t.Fatalf("base search: %v", err)
	}

	// A generous March: more money that month must not cost more interest,
	// and the freed-up cash should shorten or cheapen the path.
	richer := overrideInput(t)
	richer.Cash.MonthlyOverrides = map[string]money.Amount{
		"2026-03": money.FromMinor(60_000_000, amd),
	}
	rich, err := Search(richer, goal)
	if err != nil {
		t.Fatalf("override search: %v", err)
	}
	if rich.Best.TotalInterest.Cmp(base.Best.TotalInterest) >= 0 {
		t.Errorf("a richer month did not reduce interest: %s vs %s",
			rich.Best.TotalInterest, base.Best.TotalInterest)
	}

	// A tighter March, still above the instalment: costs more, never less.
	tighter := overrideInput(t)
	tighter.Cash.MonthlyOverrides = map[string]money.Amount{
		"2026-03": money.FromMinor(27_000_000, amd),
	}
	tight, err := Search(tighter, goal)
	if err != nil {
		t.Fatalf("tight search: %v", err)
	}
	if tight.Best.TotalInterest.Cmp(base.Best.TotalInterest) < 0 {
		t.Errorf("a tighter month reduced interest: %s vs %s",
			tight.Best.TotalInterest, base.Best.TotalInterest)
	}
}

func TestOverrideBelowInstalmentRefusesWithTheDate(t *testing.T) {
	amd := money.MustLookup("AMD")
	in := overrideInput(t)
	in.Cash.MonthlyOverrides = map[string]money.Amount{
		"2026-03": money.FromMinor(1_000_00, amd), // cannot cover the instalment
	}
	_, err := Search(in, Goal{Kind: LeastInterest})
	var inf *InfeasibleError
	if !errors.As(err, &inf) {
		t.Fatalf("want InfeasibleError, got %v", err)
	}
	if MonthKey(inf.On) != "2026-03" {
		t.Errorf("refusal names %s, want the override month", inf.On)
	}
}

func TestOpeningCashReducesInterest(t *testing.T) {
	amd := money.MustLookup("AMD")
	goal := Goal{Kind: LeastInterest}

	base, err := Search(overrideInput(t), goal)
	if err != nil {
		t.Fatalf("base search: %v", err)
	}
	funded := overrideInput(t)
	funded.Cash.OpeningCash = money.FromMinor(100_000_000, amd)
	rich, err := Search(funded, goal)
	if err != nil {
		t.Fatalf("funded search: %v", err)
	}
	if rich.Best.TotalInterest.Cmp(base.Best.TotalInterest) >= 0 {
		t.Errorf("opening cash did not reduce interest: %s vs %s",
			rich.Best.TotalInterest, base.Best.TotalInterest)
	}
}

func TestOverrideValidation(t *testing.T) {
	amd := money.MustLookup("AMD")
	for name, over := range map[string]map[string]money.Amount{
		"bad key":        {"March": money.FromMinor(1, amd)},
		"bad month":      {"2026-13": money.FromMinor(1, amd)},
		"wrong currency": {"2026-03": money.FromMinor(1, money.MustLookup("USD"))},
	} {
		in := overrideInput(t)
		in.Cash.MonthlyOverrides = over
		if err := in.Validate(); err == nil {
			t.Errorf("%s: validated", name)
		}
	}
	ok := overrideInput(t)
	ok.Cash.MonthlyOverrides = map[string]money.Amount{"2026-03": money.Zero(amd)}
	if err := ok.Validate(); err != nil {
		t.Errorf("a zero override is a real statement and must validate: %v", err)
	}
}
