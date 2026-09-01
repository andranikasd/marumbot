package plan

import (
	"errors"
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/allocation"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// The honesty fixes: a stale anchor refuses instead of assuming, the proof
// checks everything its sentence claims, and the payday's worth compares
// best against best.

func honestyLoan(t *testing.T, id string, dc money.DayCount, from date.Date) Position {
	t.Helper()
	amd := money.MustLookup("AMD")
	return Position{
		ID: id, Name: id,
		Contract: model.Contract{
			LoanID: model.ID(id), Version: 1, Currency: amd, EffectiveFrom: from,
			NominalRate: money.RateFromPercent(18, 0), DayCount: dc,
			Type: model.Annuity, StartDate: from, MaturityDate: date.MustNew(from.Year()+2, from.Month(), from.Day()),
			PaymentDay: 15, Rounding: money.DefaultPolicy(amd),
		},
		Balance: money.FromMinor(200_000_000, amd), From: from,
		Excess: allocation.ExcessReducePrincipal, Trust: "user_entered",
	}
}

func TestNormalizeRefusesAStaleAnchor(t *testing.T) {
	amd := money.MustLookup("AMD")
	v := date.MustNew(2026, 6, 15)

	// Anchored five months back: five instalments would have to be assumed.
	in := Input{
		ValuationDate: v,
		Cash:          CashPlan{Monthly: money.FromMinor(15_000_000, amd), PayDay: 5},
		Loans:         []Position{honestyLoan(t, "old", money.Actual365, date.MustNew(2026, 1, 10))},
	}
	_, _, err := Normalize(in)
	var st *StaleBalanceError
	if !errors.As(err, &st) {
		t.Fatalf("want StaleBalanceError, got %v", err)
	}
	if st.LoanID != "old" || st.Assumed <= MaxAssumedInstalments {
		t.Errorf("refusal carries wrong facts: %+v", st)
	}

	// Anchored two months back: within the bound, assumed and reported.
	in.Loans = []Position{honestyLoan(t, "recent", money.Actual365, date.MustNew(2026, 4, 10))}
	_, assumed, err := Normalize(in)
	if err != nil {
		t.Fatalf("recent anchor refused: %v", err)
	}
	if assumed["recent"] == 0 || assumed["recent"] > MaxAssumedInstalments {
		t.Errorf("assumed count out of bounds: %+v", assumed)
	}
}

func TestProofRequiresOneDayCountBasis(t *testing.T) {
	amd := money.MustLookup("AMD")
	v := date.MustNew(2026, 1, 10)
	in := Input{
		ValuationDate: v,
		Cash:          CashPlan{Monthly: money.FromMinor(30_000_000, amd), PayDay: 5},
		Loans: []Position{
			honestyLoan(t, "a365", money.Actual365, v),
			honestyLoan(t, "b360", money.Actual360, v),
		},
	}
	rep, err := Search(in, Goal{Kind: LeastInterest})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if rep.Certificate.Strength == ProvenOptimal {
		t.Fatalf("proof claimed over mixed day-count bases; certificate: %+v", rep.Certificate.Strength)
	}

	// Same basis: the proof may fire when its winner is the avalanche.
	in.Loans = []Position{
		honestyLoan(t, "a", money.Actual365, v),
		honestyLoan(t, "b", money.Actual365, v),
	}
	rep, err = Search(in, Goal{Kind: LeastInterest})
	if err != nil {
		t.Fatalf("uniform search: %v", err)
	}
	if rep.Certificate.Strength == ProvenOptimal && rep.Certificate.Eligibility == "" {
		t.Error("a proof without its printed rule")
	}
}

func TestTimingSavingIsBestAgainstBest(t *testing.T) {
	amd := money.MustLookup("AMD")
	v := date.MustNew(2026, 1, 10)
	in := Input{
		ValuationDate: v,
		Cash:          CashPlan{Monthly: money.FromMinor(30_000_000, amd), PayDay: 5},
		Loans: []Position{
			honestyLoan(t, "a", money.Actual365, v),
			honestyLoan(t, "b", money.Actual365, v),
		},
	}
	rep, err := Search(in, Goal{Kind: LeastInterest})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if rep.TimingSaving.Sign() < 0 {
		t.Fatalf("negative payday value: %s", rep.TimingSaving)
	}
	// The saving must never exceed the gap to the best all-on-due plan in
	// the ranked list -- it IS that gap.
	for _, r := range rep.Ranked {
		if tm, ok := uniformTiming(r.Policy.Timing); ok && tm == OnDue {
			gap, err := r.Cost().Sub(rep.Best.Cost())
			if err != nil {
				t.Fatal(err)
			}
			if rep.TimingSaving.Cmp(gap) > 0 {
				t.Errorf("saving %s exceeds the best on-due gap %s", rep.TimingSaving, gap)
			}
			break
		}
	}
}
