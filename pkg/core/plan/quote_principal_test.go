package plan_test

import (
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

func TestPartialPayoffNeverCreditsMoreThanPrincipal(t *testing.T) {
	// Independent arithmetic fixture: 100,000 AMD at 36.5% ACT/365 accrues
	// exactly 100 AMD/day. Jan 15-Feb 1 books 1,700 AMD; retiring all
	// principal Feb 1 saves at most 1,400 AMD before the Feb 15 due date.
	// Opening cash exceeds principal but cannot settle principal+interest.
	for _, opening := range []int64{100000, 101000} {
		p := pos("a", "Loan", 100000, 0, 1)
		p.Contract.NominalRate = money.RateFromPercent(36, 500000)
		in := input([]plan.Position{p}, 20000, 1)
		in.ValuationDate = date.MustNew(2026, 2, 1)
		in.Cash.OpeningCash = amt(opening)
		in.Cash.Spending = &plan.SpendingPlan{Monthly: amt(200000)}
		pol := plan.Policy{Order: []int{0}, Timing: []plan.Timing{plan.OnReceipt}, Effect: []model.PrepaymentEffect{model.PrepayShortenTerm}}
		result, actions, err := plan.PaymentTimeline(in, pol)
		if err != nil {
			t.Fatal(err)
		}
		if len(actions) != 2 || actions[0].Amount.Minor() != amt(100000).Minor() || actions[0].Saves.Minor() != amt(1400).Minor() || actions[1].Amount.Minor() != amt(1700).Minor() {
			t.Fatalf("opening %d: principal credit and accrued-interest settlement: %+v", opening, actions)
		}
		if result.TotalInterest.Minor() != amt(1700).Minor() || result.TotalPaid.Minor() != amt(101700).Minor() {
			t.Fatalf("opening %d: independent payoff totals: %+v", opening, result)
		}
	}
}
