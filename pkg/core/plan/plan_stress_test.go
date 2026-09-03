package plan

import (
	"reflect"
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

func stressFixture(t *testing.T) (Input, Policy) {
	t.Helper()
	in, pol := routingFixture(t)
	in.Loans = in.Loans[:1]
	in.Loans[0].Contract.PaymentDay = 2
	in.Loans[0].Contract.ScheduledPayment = routingAmount(in, 100)
	in.Loans[0].Balance = routingAmount(in, 100)
	in.Cash.Monthly = routingAmount(in, 100)
	in.Cash.PayDay = 31
	pol.Order = []int{0}
	pol.Timing = pol.Timing[:1]
	pol.Effect = pol.Effect[:1]
	pol.RequiredOnly = true
	return in, pol
}

// Golden arithmetic: Jan 31 funds Feb 2 in base; a three-day delay credits
// Feb 3, leaving the Feb 2 required 100 cents entirely unfunded.
func TestStressIncomeCrossMonthGolden(t *testing.T) {
	in, pol := stressFixture(t)
	got, err := StressCases(in, pol, StressOptions{})
	if err != nil {
		t.Fatal(err)
	}
	c := got.Cases[0]
	if got.Base.Status != StressPassed || got.Health != StressTight || c.Status != StressFailed || c.Failure == nil {
		t.Fatalf("%+v", got)
	}
	if c.Failure.On != date.MustNew(2026, 2, 2) || c.Failure.Shortfall.Minor() != 100 || c.Failure.Available.Minor() != 0 {
		t.Fatalf("%+v", c.Failure)
	}
}

// Golden: opening 100 pays Feb 2; nominal January 120 arrives Feb 3;
// March 2 leaves 20, nominal February 80 arrives March 3, April 2 clears 100.
func TestStressPreservesNominalMonthAndInput(t *testing.T) {
	in, pol := stressFixture(t)
	in.Loans[0].Balance = routingAmount(in, 300)
	in.Cash.OpeningCash = routingAmount(in, 100)
	in.Cash.MonthlyOverrides = map[string]money.Amount{"2026-01": routingAmount(in, 120), "2026-02": routingAmount(in, 80)}
	before := inputHash(in)
	policyBefore := pol.ID()
	got, err := StressCases(in, pol, StressOptions{RequiredIncreaseBP: 500})
	if err != nil {
		t.Fatal(err)
	}
	if got.Cases[0].Status != StressPassed || got.Cases[0].PayoffDate != date.MustNew(2026, 4, 2) {
		t.Fatalf("%+v", got.Cases[0])
	}
	if got.Health != StressUnknown || got.Cases[2].Reason != StressRequiredTermsMissing {
		t.Fatalf("%+v", got)
	}
	if !reflect.DeepEqual(before, inputHash(in)) || policyBefore != pol.ID() {
		t.Fatal("stress mutated original")
	}
}

func TestStressExpectedCashCannotRescueBase(t *testing.T) {
	in, pol := stressFixture(t)
	in.Cash.Monthly = routingAmount(in, 0)
	in.Cash.Lumps = []CashEvent{{On: date.MustNew(2026, 1, 31), Amount: routingAmount(in, 100), Expected: true}}
	got, err := StressCases(in, pol, StressOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Health != StressInfeasible || got.Cases[1].Status != StressFailed || got.Cases[1].Reason != StressExpectedExcluded {
		t.Fatalf("%+v", got)
	}
}
