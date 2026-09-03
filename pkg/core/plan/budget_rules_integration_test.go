package plan_test

import (
	"errors"
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

func TestBudgetDatedIncreaseDoesNotResetSpent(t *testing.T) {
	in, p := fundedFixture(500, 150)
	in.Cash.Spending.Spent = amt(50)
	in.Cash.Spending.Changes = []plan.SpendingChange{
		{On: date.MustNew(2026, 2, 1), Limit: amt(150)},
		{On: date.MustNew(2026, 2, 15), Limit: amt(200)},
	}
	r, err := plan.Run(in, p)
	if err != nil {
		t.Fatal(err)
	}
	// Already spent 50 + required 100 + optional 50 = permission 200.
	first := r.Timeline[0]
	if first.Required.Minor() != amt(100).Minor() || first.Extra.Minor() != amt(50).Minor() {
		t.Fatalf("dated increase reset spent: %+v", first)
	}
	for _, a := range r.Actions {
		if a.Kind == plan.Extra && a.On.Before(date.MustNew(2026, 2, 15)) {
			t.Fatal("future permission spent early")
		}
	}
}

func TestBudgetDatedDecreaseCannotUnspend(t *testing.T) {
	in, p := fundedFixture(500, 100)
	in.Cash.Spending.Changes = []plan.SpendingChange{{On: date.MustNew(2026, 2, 15), Limit: amt(50)}}
	_, err := plan.Run(in, p)
	var short *plan.InfeasibleError
	if !errors.As(err, &short) || !short.On.Equal(date.MustNew(2026, 2, 15)) || short.Shortfall.Minor() != amt(50).Minor() || short.Constraint != "spending_limit" {
		t.Fatalf("want dated 50 shortfall after 100 spent: %v", err)
	}
}

func TestBudgetUserCycleReservesNextCalendarMonth(t *testing.T) {
	in, p := fundedFixture(500, 100)
	in.ValuationDate = date.MustNew(2026, 2, 15)
	in.Loans[0].From = date.MustNew(2026, 2, 1)
	in.Cash.OpeningCash = amt(500)
	in.Cash.Spending.CycleDay = 15
	r, err := plan.Run(in, p)
	if err != nil {
		t.Fatal(err)
	}
	if r.Timeline[0].Required.Minor() != amt(100).Minor() || r.Timeline[0].Extra.Sign() != 0 {
		t.Fatalf("March obligation not reserved in February cycle: %+v", r.Timeline[0])
	}
}

func TestBudgetCycleClampsWithoutDrifting(t *testing.T) {
	p := plan.SpendingPlan{CycleDay: 31}
	for _, tc := range []struct{ on, want string }{
		{"2028-02-29", "2028-02-29"}, {"2028-03-30", "2028-02-29"}, {"2028-03-31", "2028-03-31"}, {"2027-03-01", "2027-02-28"},
	} {
		on, _ := date.Parse(tc.on)
		if got := p.PeriodStart(on).String(); got != tc.want {
			t.Fatalf("%s: got %s want %s", tc.on, got, tc.want)
		}
	}
}

func TestBudgetNoCarryKeepsHouseholdCashAccounted(t *testing.T) {
	in, p := fundedFixture(500, 100)
	in.Cash.Spending.CarryRule = "no_carry"
	r, err := plan.Run(in, p)
	if err != nil {
		t.Fatal(err)
	}
	// 12 receipts of 500, twelve required payments of 100. Eleven completed
	// periods transfer 400 each; the final 400 is still in its current period.
	if r.HouseholdCash.Minor() != amt(4400).Minor() || r.TotalPaid.Minor() != amt(1200).Minor() || r.Timeline[len(r.Timeline)-1].Cash.Minor() != amt(400).Minor() {
		t.Fatalf("cash not conserved: %+v", r)
	}
	for _, m := range r.Timeline[:11] {
		if m.HouseholdCash.Minor() != amt(400).Minor() {
			t.Fatal("period transfer missing")
		}
	}
}

func TestBudgetCarryDateWakesWithoutFunding(t *testing.T) {
	in, p := fundedFixture(500, 500)
	in.Cash.Spending.CarryRule = "carry_to_date"
	in.Cash.Spending.CarryUntil = date.MustNew(2026, 2, 15)
	r, err := plan.Run(in, p)
	if err != nil {
		t.Fatal(err)
	}
	saw := false
	for _, a := range r.Actions {
		if a.Kind == plan.Extra {
			if a.On.Before(date.MustNew(2026, 2, 15)) {
				t.Fatal("spent before carry date")
			}
			if a.On.Equal(date.MustNew(2026, 2, 15)) {
				saw = true
			}
		}
	}
	if !saw {
		t.Fatal("carry date did not trigger allocation")
	}
	if r.Timeline[0].Required.Minor() != amt(100).Minor() {
		t.Fatal("carry held mandatory payment")
	}
}

func TestBudgetBatchThresholdDoesNotInventPermission(t *testing.T) {
	in, p := fundedFixture(200, 200)
	in.Cash.Spending.CarryRule = "batch_until"
	in.Cash.Spending.CarryMinimum = amt(250)
	r, err := plan.Run(in, p)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range r.Timeline {
		if m.Required.Minor()+m.Extra.Minor()+m.Fees.Minor() > amt(200).Minor() {
			t.Fatal("carried cash increased permission")
		}
	}
	if r.Timeline[0].Extra.Sign() != 0 {
		t.Fatal("optional amount below threshold spent")
	}
}

func TestBudgetPaydayTenCycleFifteenDatedReduction(t *testing.T) {
	for _, changed := range []date.Date{date.MustNew(2026, 2, 25), date.MustNew(2026, 3, 12)} {
		t.Run(changed.String(), func(t *testing.T) {
			in, p := fundedFixture(500, 100)
			in.ValuationDate = date.MustNew(2026, 2, 10)
			in.Cash.PayDay = 10
			in.Cash.Spending.CycleDay = 15
			in.Loans[0].Contract.PaymentDay = 20
			in.Loans[0].Contract.MaturityDate = date.MustNew(2027, 1, 20)
			in.Loans[0].Contract.HasScheduled = true
			in.Loans[0].Contract.ScheduledPayment = amt(100)
			p.RequiredOnly = true
			in.Cash.Spending.Changes = []plan.SpendingChange{{On: changed, Limit: amt(50)}}
			_, actions, err := plan.PaymentTimeline(in, p)
			var short *plan.InfeasibleError
			if !errors.As(err, &short) || !short.On.Equal(changed) || short.Required.Minor() != amt(100).Minor() || short.Shortfall.Minor() != amt(50).Minor() || short.Constraint != "spending_limit" {
				t.Fatalf("100 paid on February 20 must leave exact 50 deficit on %s, including after March 10 income: %v", changed, err)
			}
			paid := int64(0)
			for _, action := range actions {
				if action.On.Equal(date.MustNew(2026, 2, 20)) {
					paid += action.Amount.Minor()
				}
			}
			if paid != amt(100).Minor() {
				t.Fatalf("February 20 outflow = %d, want 100 AMD", paid)
			}
		})
	}
}
