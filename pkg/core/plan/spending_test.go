package plan_test

import (
	"errors"
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

// Independent zero-interest fixture: 1,200 AMD over 12 months, 100 AMD
// contractually due each month. No annuity formula supplies expected values.
func fundedFixture(cash, limit int64) (plan.Input, plan.Policy) {
	l := pos("a", "Loan", 1200, 0, 1)
	in := input([]plan.Position{l}, cash, 1)
	in.ValuationDate = date.MustNew(2026, 2, 1)
	in.Loans[0].From = valuation
	in.Cash.Spending = &plan.SpendingPlan{Monthly: amt(limit)}
	p := plan.Policy{Order: []int{0}, Timing: []plan.Timing{plan.OnReceipt}, Effect: []model.PrepaymentEffect{model.PrepayReduceInstalment}}
	return in, p
}

func TestSpendingAndFundingAreIndependent(t *testing.T) {
	for _, tc := range []struct {
		name        string
		cash, limit int64
		constraint  string
	}{{"cash", 50, 200, ""}, {"permission", 200, 50, "spending_limit"}} {
		t.Run(tc.name, func(t *testing.T) {
			in, p := fundedFixture(tc.cash, tc.limit)
			_, err := plan.Run(in, p)
			var e *plan.InfeasibleError
			if !errors.As(err, &e) || e.Constraint != tc.constraint || e.Required.Minor() != amt(100).Minor() || e.Shortfall.Minor() != amt(50).Minor() {
				t.Fatalf("want exact 100 required / 50 short %s, got %v", tc.constraint, err)
			}
		})
	}
}

func TestCashDoesNotIncreaseSpendingPermission(t *testing.T) {
	in, p := fundedFixture(500, 100)
	in.Cash.Lumps = []plan.CashEvent{{On: date.MustNew(2026, 2, 5), Amount: amt(300)}, {On: date.MustNew(2026, 2, 8), Amount: amt(400)}}
	r, err := plan.Run(in, p)
	if err != nil {
		t.Fatal(err)
	}
	if r.Months != 12 || r.TotalPaid.Minor() != amt(1200).Minor() {
		t.Fatalf("required-only fixture changed: %+v", r)
	}
	for _, m := range r.Timeline {
		if m.Required.Minor() != amt(100).Minor() || m.Extra.Sign() != 0 {
			t.Fatalf("permission exceeded: %+v", m)
		}
	}
}

func TestExpectedCashCannotFundBasePlan(t *testing.T) {
	in, p := fundedFixture(50, 100)
	in.Cash.Lumps = []plan.CashEvent{{On: date.MustNew(2026, 2, 5), Amount: amt(1000), Expected: true}}
	_, err := plan.Run(in, p)
	var e *plan.InfeasibleError
	if !errors.As(err, &e) || e.Shortfall.Minor() != amt(50).Minor() {
		t.Fatalf("expected cash funded base: %v", err)
	}
}

func TestExcludedLoanStillRequiresPayments(t *testing.T) {
	in, p := fundedFixture(500, 500)
	in.Loans[0].OptionalExcluded = true
	r, err := plan.Run(in, p)
	if err != nil {
		t.Fatal(err)
	}
	if r.Months != 12 || r.TotalPaid.Minor() != amt(1200).Minor() {
		t.Fatalf("excluded loan changed: %+v", r)
	}
	for _, m := range r.Timeline {
		if m.Extra.Sign() != 0 {
			t.Fatal("extra allocated to excluded debt")
		}
	}
}

func TestSpentAndOverrideGuardRequiredPayment(t *testing.T) {
	in, p := fundedFixture(500, 200)
	in.Cash.Spending.Spent = amt(150)
	_, err := plan.Run(in, p)
	var e *plan.InfeasibleError
	if !errors.As(err, &e) || e.Constraint != "spending_limit" || e.Shortfall.Minor() != amt(50).Minor() {
		t.Fatalf("already-spent not protected: %v", err)
	}
	in.Cash.Spending.Spent = money.Zero(amd)
	in.Cash.Spending.Overrides = map[string]money.Amount{"2026-02": amt(50)}
	_, err = plan.Run(in, p)
	if !errors.As(err, &e) || e.Constraint != "spending_limit" {
		t.Fatalf("override not protected: %v", err)
	}
}

func TestReserveCannotFundOptionalPayment(t *testing.T) {
	in, p := fundedFixture(100, 500)
	in.Cash.OpeningCash = amt(300)
	in.Cash.ReserveFloor = amt(300)
	r, err := plan.Run(in, p)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range r.Timeline {
		if m.Extra.Sign() != 0 || m.Cash.Cmp(amt(300)) != 0 {
			t.Fatalf("reserve spent: %+v", m)
		}
	}
}

func TestOptionalQuantumNeverRoundsPermissionUp(t *testing.T) {
	in, p := fundedFixture(200, 101)
	in.Cash.Spending.Monthly = money.FromMinor(10106, amd)
	r, err := plan.Run(in, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Actions) == 0 || r.Actions[0].Kind != plan.Extra || r.Actions[0].Amount.Minor() != 100 {
		t.Fatalf("106 minor allowance must pay 100, got %+v", r.Actions)
	}
	for _, m := range r.Timeline {
		paid := m.Required.Minor() + m.Extra.Minor() + m.Fees.Minor()
		if paid > 10106 {
			t.Fatalf("rounded permission up: %d", paid)
		}
	}
}
