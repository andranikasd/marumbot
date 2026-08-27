package plan_test

import (
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/allocation"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

func reducing() []plan.Position {
	ls := three()
	for i := range ls {
		ls[i].Excess = allocation.ExcessReducePrincipal
	}
	return ls
}

// Paying the surplus on payday must cost less than paying it with the
// instalment, and splitting it must land between the two. This is the
// monotonicity claim the whole optimiser rests on, measured.
func TestEarlyPaymentSavesInterestAndSplitNeverWins(t *testing.T) {
	ls := reducing()
	cash := plan.Cash{Monthly: amt(200_000), Day: 1}
	run := func(tm plan.Timing) plan.Result {
		r, err := plan.Run(ls, cash, plan.Policy{Name: "avalanche", Order: []int{2, 0, 1}, Timing: tm})
		if err != nil {
			t.Fatalf("%s: %v", tm, err)
		}
		return r
	}
	due, early, split := run(plan.OnDue), run(plan.OnReceipt), run(plan.SplitHalf)
	if early.TotalInterest.Cmp(due.TotalInterest) >= 0 {
		t.Fatalf("on receipt %s is not cheaper than on due %s", early.TotalInterest, due.TotalInterest)
	}
	if split.TotalInterest.Cmp(early.TotalInterest) < 0 || split.TotalInterest.Cmp(due.TotalInterest) > 0 {
		t.Fatalf("split %s is not between early %s and due %s", split.TotalInterest, early.TotalInterest, due.TotalInterest)
	}
	if !early.TimingCredited {
		t.Fatal("early payment was not credited on a reduce_principal lender")
	}
}

// A lender that does not reduce principal on the day of payment gives early
// timing no effect, and the engine must say so rather than invent a saving.
func TestUnknownExcessRuleGetsNoTimingCredit(t *testing.T) {
	cash := plan.Cash{Monthly: amt(200_000), Day: 1}
	pol := plan.Policy{Name: "avalanche", Order: []int{2, 0, 1}}
	due, err := plan.Run(three(), cash, pol)
	if err != nil {
		t.Fatal(err)
	}
	pol.Timing = plan.OnReceipt
	early, err := plan.Run(three(), cash, pol)
	if err != nil {
		t.Fatal(err)
	}
	if early.TimingCredited || early.TotalInterest.Cmp(due.TotalInterest) != 0 {
		t.Fatalf("early %s credited=%v, want equal to due %s and not credited",
			early.TotalInterest, early.TimingCredited, due.TotalInterest)
	}
	rep, err := plan.Search(three(), cash, plan.PayLeastInterest)
	if err != nil {
		t.Fatal(err)
	}
	if rep.TimingSaving.Sign() != 0 || rep.Best.Policy.Timing != plan.OnDue {
		t.Fatalf("search credited timing on an unknown rule: %+v", rep.Best.Policy)
	}
}

// With no fees and every minimum met, no priority order beats the avalanche on
// interest. The search must find that, and must have actually looked.
func TestSearchExhaustiveNeverBeatsAvalancheByMoreThanRounding(t *testing.T) {
	rep, err := plan.Search(reducing(), plan.Cash{Monthly: amt(200_000), Day: 1}, plan.PayLeastInterest)
	if err != nil {
		t.Fatal(err)
	}
	// 6 orders × 3 timings × 2 prepayment effects, the contracts leaving the
	// effect to the borrower.
	if !rep.Exhaustive || rep.Evaluated != 6*3*2 {
		t.Fatalf("expected 36 exhaustive candidates, got exhaustive=%v evaluated=%d", rep.Exhaustive, rep.Evaluated)
	}
	if rep.Best.Policy.Name != "avalanche" || rep.Best.Policy.Timing != plan.OnReceipt {
		t.Fatalf("best policy is %s, want avalanche/on_receipt", rep.Best.Policy)
	}
	if rep.TimingSaving.Sign() <= 0 {
		t.Fatalf("timing saving is %s", rep.TimingSaving)
	}
	if rep.Best.TotalInterest.Cmp(rep.Avalanche.TotalInterest) >= 0 {
		t.Fatalf("best %s not below avalanche on due %s", rep.Best.TotalInterest, rep.Avalanche.TotalInterest)
	}
	if rep.Snowball.TotalInterest.Cmp(rep.Avalanche.TotalInterest) < 0 {
		t.Fatalf("snowball %s cheaper than avalanche %s", rep.Snowball.TotalInterest, rep.Avalanche.TotalInterest)
	}
}

// The first month must be spelled out as dated payments, and the early extra
// must carry the interest it saves.
func TestFirstMonthActionsAreDated(t *testing.T) {
	rep, err := plan.Search(reducing(), plan.Cash{Monthly: amt(200_000), Day: 1}, plan.PayLeastInterest)
	if err != nil {
		t.Fatal(err)
	}
	var extras, required int
	for _, a := range rep.Best.Actions {
		if a.On.IsZero() || a.Amount.Sign() <= 0 {
			t.Errorf("malformed action %+v", a)
		}
		if a.Extra {
			extras++
			if a.Saves.Sign() <= 0 {
				t.Errorf("early extra %+v saves nothing", a)
			}
		} else {
			required++
		}
	}
	if required != 3 || extras < 1 {
		t.Fatalf("got %d required and %d extra actions", required, extras)
	}
	for i := 1; i < len(rep.Best.Actions); i++ {
		if rep.Best.Actions[i].On.Before(rep.Best.Actions[i-1].On) {
			t.Fatal("actions are not in date order")
		}
	}
}

// Without a known payday the search must not propose early payment.
func TestSearchWithoutPaydayStaysOnDue(t *testing.T) {
	rep, err := plan.Search(reducing(), plan.Cash{Monthly: amt(200_000)}, plan.FinishSoonest)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Evaluated != 6*2 || rep.Best.Policy.Timing != plan.OnDue {
		t.Fatalf("evaluated=%d best=%s", rep.Evaluated, rep.Best.Policy)
	}
}

// Ranking must be total: running twice gives the same best policy.
func TestSearchIsDeterministic(t *testing.T) {
	cash := plan.Cash{Monthly: amt(200_000), Day: 5}
	for _, g := range []plan.Goal{plan.PayLeastInterest, plan.FinishSoonest, plan.FreeUpMonthly} {
		a, err := plan.Search(reducing(), cash, g)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := plan.Search(reducing(), cash, g)
		if a.Best.Policy.String() != b.Best.Policy.String() {
			t.Errorf("%s: %s vs %s", g, a.Best.Policy, b.Best.Policy)
		}
	}
}

// Interest must accrue in every month of a run, not only the first. A
// projection anchored on a payment date used to include that date as a
// zero-day row, so every later month showed no interest at all; the
// timeline is where that would show, so the timeline is what is checked.
func TestInterestAccruesEveryMonth(t *testing.T) {
	r, err := plan.Run(reducing(), plan.Cash{Monthly: amt(200_000), Day: 1},
		plan.Policy{Name: "avalanche", Order: []int{2, 0, 1}})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Timeline) != r.Months {
		t.Fatalf("timeline has %d months, run took %d", len(r.Timeline), r.Months)
	}
	for _, m := range r.Timeline[:r.Months-1] {
		if m.Interest.Sign() <= 0 {
			t.Fatalf("month %d accrued %s", m.Month, m.Interest)
		}
	}
}

// Paying only the minimum must run to the longest maturity: three loans of
// two, three and ten years clear in 120 months, not sooner.
func TestMinimumRunsToMaturity(t *testing.T) {
	rep, err := plan.Search(reducing(), plan.Cash{Monthly: amt(200_000), Day: 1}, plan.PayLeastInterest)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Minimum.Months != 120 {
		t.Fatalf("minimum cleared in %d months, want 120", rep.Minimum.Months)
	}
	if rep.Best.TotalInterest.Cmp(rep.Minimum.TotalInterest) >= 0 {
		t.Fatalf("best %s is not cheaper than the minimum %s", rep.Best.TotalInterest, rep.Minimum.TotalInterest)
	}
}

// The relief goal must actually lower the monthly outflow, and say when, and
// it must cost more interest than the cheapest plan — that is the trade.
func TestReliefLowersTheOutflowAtACost(t *testing.T) {
	cash := plan.Cash{Monthly: amt(250_000), Day: 1}
	relief, err := plan.Search(reducing(), cash, plan.FreeUpMonthly)
	if err != nil {
		t.Fatal(err)
	}
	cheap, err := plan.Search(reducing(), cash, plan.PayLeastInterest)
	if err != nil {
		t.Fatal(err)
	}
	b := relief.Best
	if b.ReliefMonth == 0 || b.FinalMonthly.Cmp(b.PeakMonthly) >= 0 {
		t.Fatalf("no relief: month %d, peak %s, final %s", b.ReliefMonth, b.PeakMonthly, b.FinalMonthly)
	}
	if b.TotalInterest.Cmp(cheap.Best.TotalInterest) <= 0 {
		t.Fatalf("relief %s not dearer than cheapest %s", b.TotalInterest, cheap.Best.TotalInterest)
	}
	if b.Months <= cheap.Best.Months {
		t.Fatalf("relief %d months not longer than cheapest %d", b.Months, cheap.Best.Months)
	}
}

// More money per month must clear sooner and cost less, rung by rung.
func TestLadderIsMonotone(t *testing.T) {
	rep, err := plan.Search(reducing(), plan.Cash{Monthly: amt(250_000), Day: 1}, plan.FinishSoonest)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Ladder) < 3 {
		t.Fatalf("ladder has %d rungs", len(rep.Ladder))
	}
	for i := 1; i < len(rep.Ladder); i++ {
		a, b := rep.Ladder[i-1], rep.Ladder[i]
		if b.Months > a.Months || b.Interest.Cmp(a.Interest) > 0 {
			t.Fatalf("rung %s (%d, %s) is not better than %s (%d, %s)", b.Budget, b.Months, b.Interest, a.Budget, a.Months, a.Interest)
		}
	}
}

// The inverse solver must return a budget that meets the target and whose
// next lower settlement unit does not.
func TestBudgetForMeetsTheTargetExactly(t *testing.T) {
	ls := reducing()
	cash := plan.Cash{Monthly: amt(250_000), Day: 1}
	pol := plan.Policy{Name: "avalanche", Order: []int{2, 0, 1}, Timing: plan.OnReceipt}
	b, err := plan.BudgetFor(ls, cash, pol, 12)
	if err != nil {
		t.Fatal(err)
	}
	r, err := plan.Run(ls, plan.Cash{Monthly: b, Day: 1}, pol)
	if err != nil || r.Months > 12 {
		t.Fatalf("budget %s clears in %d months (%v)", b, r.Months, err)
	}
	less, err := plan.Run(ls, plan.Cash{Monthly: amt(b.Minor()/100 - 1000), Day: 1}, pol)
	if err == nil && less.Months <= 12 {
		t.Fatalf("a smaller budget also clears in %d months", less.Months)
	}
}

// A lump sum in a given month must shorten the run.
func TestLumpSumShortensTheRun(t *testing.T) {
	ls := reducing()
	pol := plan.Policy{Name: "avalanche", Order: []int{2, 0, 1}}
	base, err := plan.Run(ls, plan.Cash{Monthly: amt(200_000)}, pol)
	if err != nil {
		t.Fatal(err)
	}
	lump, err := plan.Run(ls, plan.Cash{Monthly: amt(200_000), Lumps: []plan.Lump{{Month: 3, Amount: amt(1_000_000)}}}, pol)
	if err != nil {
		t.Fatal(err)
	}
	if lump.Months >= base.Months || lump.TotalInterest.Cmp(base.TotalInterest) >= 0 {
		t.Fatalf("lump: %d months %s vs base %d months %s", lump.Months, lump.TotalInterest, base.Months, base.TotalInterest)
	}
}
