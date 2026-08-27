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
	if !rep.Exhaustive || rep.Evaluated != 6*3 {
		t.Fatalf("expected 18 exhaustive candidates, got exhaustive=%v evaluated=%d", rep.Exhaustive, rep.Evaluated)
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
	if rep.Evaluated != 6 || rep.Best.Policy.Timing != plan.OnDue {
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
