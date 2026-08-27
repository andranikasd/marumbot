package plan_test

import (
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

func pos(id, name string, major int64, ratePct int64, years int) plan.Position {
	start := date.MustNew(2026, 1, 15)
	return plan.Position{
		ID: id, Name: name,
		Contract: model.Contract{
			LoanID: model.ID(id), Version: 1, Currency: amd,
			NominalRate: money.RateFromPercent(ratePct, 0),
			DayCount:    money.Actual365, Type: model.Annuity,
			StartDate: start, MaturityDate: date.MustNew(2026+years, 1, 15),
			PaymentDay: 15, Rounding: money.DefaultPolicy(amd),
		},
		Balance: amt(major), From: start,
	}
}

func three() []plan.Position {
	return []plan.Position{
		pos("a", "Car", 1_200_000, 21, 3),
		pos("b", "Home", 4_000_000, 11, 10),
		pos("c", "Phone", 300_000, 26, 2),
	}
}

// The simulation must actually finish, and finishing is the number a borrower
// asks for first.
func TestSimulateClearsEveryLoan(t *testing.T) {
	for _, g := range []plan.Goal{plan.PayLeastInterest, plan.FinishSoonest, plan.FreeUpMonthly} {
		o, err := plan.Simulate(three(), amt(200_000), g)
		if err != nil {
			t.Fatalf("%s: %v", g, err)
		}
		if o.Months <= 0 || o.Months > 240 {
			t.Errorf("%s: cleared in %d months, which is not plausible", g, o.Months)
		}
		if o.TotalInterest.Sign() <= 0 {
			t.Errorf("%s: total interest is %s", g, o.TotalInterest)
		}
		if o.ClearedFirst == "" {
			t.Errorf("%s: no loan was ever cleared", g)
		}
	}
}

// Paying the highest rate first must cost the least interest. This is the whole
// claim behind offering the choice, and it should be measurable rather than
// asserted in a comment.
func TestAvalancheCostsLeastInterest(t *testing.T) {
	outs, err := plan.CompareAll(three(), amt(200_000))
	if err != nil {
		t.Fatal(err)
	}
	var cheapest plan.Outcome
	for i, o := range outs {
		if i == 0 || o.TotalInterest.Cmp(cheapest.TotalInterest) < 0 {
			cheapest = o
		}
	}
	if cheapest.Goal != plan.PayLeastInterest {
		t.Errorf("cheapest goal was %s at %s; expected pay_least_interest",
			cheapest.Goal, cheapest.TotalInterest)
	}
}

// Clearing the smallest balance first must remove a loan sooner than the
// avalanche does, or the goal is not doing what it says.
func TestSnowballClearsSomethingSoonest(t *testing.T) {
	fast, err := plan.Simulate(three(), amt(200_000), plan.FinishSoonest)
	if err != nil {
		t.Fatal(err)
	}
	cheap, err := plan.Simulate(three(), amt(200_000), plan.PayLeastInterest)
	if err != nil {
		t.Fatal(err)
	}
	if fast.ClearedMonth > cheap.ClearedMonth {
		t.Errorf("finish_soonest cleared its first loan in month %d, "+
			"later than pay_least_interest at %d", fast.ClearedMonth, cheap.ClearedMonth)
	}
}

// A bigger budget must not cost more interest or take longer. If it does, the
// simulation is wrong somewhere that no single case would reveal.
func TestMoreBudgetIsNeverWorse(t *testing.T) {
	small, err := plan.Simulate(three(), amt(200_000), plan.PayLeastInterest)
	if err != nil {
		t.Fatal(err)
	}
	large, err := plan.Simulate(three(), amt(400_000), plan.PayLeastInterest)
	if err != nil {
		t.Fatal(err)
	}
	if large.Months > small.Months {
		t.Errorf("a larger budget took longer: %d vs %d months", large.Months, small.Months)
	}
	if large.TotalInterest.Cmp(small.TotalInterest) > 0 {
		t.Errorf("a larger budget cost more interest: %s vs %s",
			large.TotalInterest, small.TotalInterest)
	}
}

// The report has to answer "what will remain", so the figure must be there and
// must be below what is owed now.
func TestNextMonthOwedIsReported(t *testing.T) {
	o, err := plan.Simulate(three(), amt(200_000), plan.PayLeastInterest)
	if err != nil {
		t.Fatal(err)
	}
	total := amt(1_200_000 + 4_000_000 + 300_000)
	if o.NextMonthOwed.Sign() <= 0 || o.NextMonthOwed.Cmp(total) >= 0 {
		t.Errorf("next month owed = %s, want between zero and %s", o.NextMonthOwed, total)
	}
}

// A budget below the required payments is a real answer, not a crash.
func TestBudgetBelowMinimumsIsReported(t *testing.T) {
	if _, err := plan.Simulate(three(), amt(10_000), plan.PayLeastInterest); err == nil {
		t.Error("expected an error for a budget below the required payments")
	}
}
