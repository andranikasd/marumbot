package plan_test

import (
	"errors"
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/allocation"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

func inverseFixture() (plan.Input, plan.Policy) {
	in := input([]plan.Position{pos("zero", "Zero interest", 1200, 0, 1)}, 100, 0)
	in.Loans[0].Contract.HasScheduled = true
	in.Loans[0].Contract.ScheduledPayment = amt(100)
	pol := plan.Policy{Order: []int{0}, Timing: []plan.Timing{plan.OnDue}, Effect: []model.PrepaymentEffect{model.PrepayShortenTerm}}
	return in, pol
}

func TestBudgetForZeroInterestFixture(t *testing.T) {
	in, pol := inverseFixture()
	by := date.MustNew(2026, 4, 15)
	budget, err := plan.BudgetFor(in, pol, by)
	if err != nil {
		t.Fatal(err)
	}
	// Three funded dates, no interest: 1200 / 3 = 400 AMD independently
	// of the simulator. 399 AMD leaves 3 AMD after the third payment.
	if budget != amt(400) {
		t.Fatalf("budget = %s, want 400 AMD", budget)
	}
	unit := money.DefaultPolicy(amd).Unit
	for _, tc := range []struct {
		minor  int64
		clears bool
	}{
		{budget.Minor(), true},
		{budget.Minor() - unit, false},
	} {
		in.Cash.Monthly = money.FromMinor(tc.minor, amd)
		r, runErr := plan.Run(in, pol)
		if runErr != nil {
			t.Fatal(runErr)
		}
		if got := !r.PayoffDate.After(by); got != tc.clears {
			t.Fatalf("budget %s: payoff %s, want clears=%v", in.Cash.Monthly, r.PayoffDate, tc.clears)
		}
	}
}

func TestBudgetForZeroInterestMonotonicity(t *testing.T) {
	in, pol := inverseFixture()
	previous := date.MustNew(9999, 12, 31)
	for budget := int64(100); budget <= 450; budget++ {
		in.Cash.Monthly = amt(budget)
		r, err := plan.Run(in, pol)
		if err != nil {
			t.Fatal(err)
		}
		if r.PayoffDate.After(previous) {
			t.Fatalf("increasing budget to %s delayed payoff", in.Cash.Monthly)
		}
		previous = r.PayoffDate
	}
}

func TestBudgetForDomainRefusal(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*plan.Input, *plan.Policy)
	}{
		{"positive interest", func(in *plan.Input, _ *plan.Policy) { in.Loans[0].Contract.NominalRate = money.RateFromPercent(1, 0) }},
		{"unaligned balance", func(in *plan.Input, _ *plan.Policy) { in.Loans[0].Balance = money.FromMinor(120001, amd) }},
		{"unaligned instalment", func(in *plan.Input, _ *plan.Policy) {
			in.Loans[0].Contract.ScheduledPayment = money.FromMinor(10001, amd)
		}},
		{"unsupplied instalment", func(in *plan.Input, _ *plan.Policy) { in.Loans[0].Contract.HasScheduled = false }},
		{"early timing", func(_ *plan.Input, pol *plan.Policy) { pol.Timing[0] = plan.OnReceipt }},
		{"separate income calendar", func(in *plan.Input, _ *plan.Policy) { in.Cash.PayDay = 1 }},
		{"flat fee", func(in *plan.Input, _ *plan.Policy) { in.Loans[0].Contract.Prepayment.FeeBP = 100 }},
		{"dated fee", func(in *plan.Input, _ *plan.Policy) {
			in.Loans[0].Contract.Prepayment.Charges = []model.PrepaymentCharge{{Fixed: amt(1)}}
		}},
		{"contract threshold", func(in *plan.Input, _ *plan.Policy) { in.Loans[0].Contract.Prepayment.MinAmount = amt(10) }},
		{"policy threshold", func(_ *plan.Input, pol *plan.Policy) { pol.MinPrepay = amt(10) }},
		{"budget growth", func(in *plan.Input, _ *plan.Policy) {
			in.Cash.MonthlyOverrides = map[string]money.Amount{"2026-03": amt(200)}
		}},
		{"spending", func(in *plan.Input, _ *plan.Policy) {
			in.Cash.PayDay = 1
			in.Cash.Spending = &plan.SpendingPlan{Monthly: amt(100)}
		}},
		{"opening cash", func(in *plan.Input, _ *plan.Policy) { in.Cash.OpeningCash = amt(100) }},
		{"reserve", func(in *plan.Input, _ *plan.Policy) { in.Cash.ReserveFloor = amt(10) }},
		{"lump", func(in *plan.Input, _ *plan.Policy) {
			in.Cash.Lumps = []plan.CashEvent{{On: date.MustNew(2026, 3, 15), Amount: amt(10)}}
		}},
		{"keep freed", func(_ *plan.Input, pol *plan.Policy) { pol.Rollover = plan.KeepFreed }},
		{"required only", func(_ *plan.Input, pol *plan.Policy) { pol.RequiredOnly = true }},
		{"reduce instalment", func(_ *plan.Input, pol *plan.Policy) { pol.Effect[0] = model.PrepayReduceInstalment }},
		{"contract effect", func(in *plan.Input, _ *plan.Policy) {
			in.Loans[0].Contract.Prepayment.Effect = model.PrepayReduceInstalment
		}},
		{"declining principal", func(in *plan.Input, _ *plan.Policy) { in.Loans[0].Contract.Type = model.DecliningPrincipal }},
		{"excluded", func(in *plan.Input, _ *plan.Policy) { in.Loans[0].OptionalExcluded = true }},
		{"held credit", func(in *plan.Input, _ *plan.Policy) { in.Loans[0].Excess = allocation.ExcessHoldAsAdvance }},
		{"bank request required", func(in *plan.Input, _ *plan.Policy) { in.Loans[0].Excess = allocation.ExcessRequiresBankRequest }},
		{"unknown excess", func(in *plan.Input, _ *plan.Policy) { in.Loans[0].Excess = allocation.ExcessUnknown }},
		{"rounding", func(in *plan.Input, _ *plan.Policy) { in.Loans[0].Contract.Rounding.Unit = 1 }},
		{"late payday", func(in *plan.Input, _ *plan.Policy) { in.Cash.PayDay = 20 }},
		{"missing dimension", func(_ *plan.Input, pol *plan.Policy) { pol.Timing = nil }},
		{"invalid priority", func(_ *plan.Input, pol *plan.Policy) { pol.Order[0] = 1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in, pol := inverseFixture()
			tc.change(&in, &pol)
			budget, err := plan.BudgetFor(in, pol, date.MustNew(2026, 4, 15))
			var refusal *plan.NonMonotoneError
			if !errors.As(err, &refusal) || refusal.Reason == "" || budget != (money.Amount{}) {
				t.Fatalf("budget %s, want typed refusal, got %v", budget, err)
			}
		})
	}
}

func TestBudgetForMultipleLoansRefused(t *testing.T) {
	in, pol := inverseFixture()
	other := in.Loans[0]
	other.ID = "second"
	other.Contract.LoanID = "second"
	in.Loans = append(in.Loans, other)
	pol.Order = []int{0, 1}
	pol.Timing = []plan.Timing{plan.OnDue, plan.OnDue}
	pol.Effect = []model.PrepaymentEffect{model.PrepayShortenTerm, model.PrepayShortenTerm}
	_, err := plan.BudgetFor(in, pol, date.MustNew(2026, 4, 15))
	var refusal *plan.NonMonotoneError
	if !errors.As(err, &refusal) {
		t.Fatalf("want typed refusal, got %v", err)
	}
}
