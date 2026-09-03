package plan

import (
	"errors"
	"reflect"
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

func comparisonInput(t *testing.T) Input {
	t.Helper()
	v := date.MustNew(2026, 1, 15)
	cur := money.MustLookup("AMD")
	a, b := honestyLoan(t, "a", money.Actual365, v), honestyLoan(t, "b", money.Actual365, v)
	a.Balance = money.FromMinor(100000, cur)
	b.Balance = money.FromMinor(200000, cur)
	a.Contract.NominalRate = money.RateFromPercent(20, 0)
	b.Contract.NominalRate = money.RateFromPercent(10, 0)
	a.Contract.HasScheduled = true
	b.Contract.HasScheduled = true
	a.Contract.ScheduledPayment = money.FromMinor(10000, cur)
	b.Contract.ScheduledPayment = money.FromMinor(30000, cur)
	a.Contract.Prepayment.Effect = model.PrepayShortenTerm
	b.Contract.Prepayment.Effect = model.PrepayShortenTerm
	return Input{ValuationDate: v, Cash: CashPlan{Monthly: money.FromMinor(60000, cur)}, Loans: []Position{a, b}}
}

func TestNamedStrategyPriorities(t *testing.T) {
	in := comparisonInput(t)
	cases := []struct {
		id   StrategyID
		want []int
	}{{StrategyHighestRate, []int{0, 1}}, {StrategySnowball, []int{0, 1}}, {StrategyHybrid, []int{0, 1}}, {StrategyHighestRequired, []int{1, 0}}, {StrategyHighestInterest, []int{0, 1}}, {StrategyCashflowIndex, []int{1, 0}}}
	for _, tt := range cases {
		t.Run(string(tt.id), func(t *testing.T) {
			got, err := StrategyOrder(in, tt.id, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("%v want %v", got, tt.want)
			}
		})
	}
	// Fees make payoff priority differ from principal-only snowball.
	in.Loans[0].Contract.Prepayment.Charges = []model.PrepaymentCharge{{Fixed: money.FromMinor(200000, in.Cash.Monthly.Currency())}}
	got, err := StrategyOrder(in, StrategySnowball, nil)
	if err != nil || got[0] != 1 {
		t.Fatalf("payoff includes fee: %v %v", got, err)
	}
	if _, err = StrategyOrder(in, StrategyAvalanche, nil); err == nil {
		t.Fatal("fee-bearing avalanche silently used nominal rate")
	}
	if _, err = StrategyOrder(in, StrategyHighestRate, nil); err != nil {
		t.Fatal(err)
	}
}

func TestStrategyStableIDsAndRefusals(t *testing.T) {
	in := comparisonInput(t)
	in.Loans[1].Contract.NominalRate = in.Loans[0].Contract.NominalRate
	in.Loans[0], in.Loans[1] = in.Loans[1], in.Loans[0]
	got, err := StrategyOrder(in, StrategyHighestRate, nil)
	if err != nil || in.Loans[got[0]].ID != "a" {
		t.Fatalf("stable tie: %v %v", got, err)
	}
	for _, ids := range [][]string{{"a"}, {"a", "a"}, {"a", "missing"}} {
		if _, err = StrategyOrder(in, StrategyCustom, ids); err == nil {
			t.Fatalf("accepted %v", ids)
		}
	}
	got, err = StrategyOrder(in, StrategyCustom, []string{"a", "b"})
	if err != nil || in.Loans[got[0]].ID != "a" {
		t.Fatal(got, err)
	}
	in.Loans[0].OptionalExcluded = true
	if _, err = StrategyOrder(in, StrategyCustom, []string{"a"}); err != nil {
		t.Fatal(err)
	}
	if _, err = StrategyOrder(in, StrategyCustom, []string{"a", "b"}); err == nil {
		t.Fatal("accepted excluded ID")
	}
	in.Loans[1].Contract.NominalRate = 0
	for _, id := range []StrategyID{StrategyHybrid, StrategyUtilisation, StrategyID("typo")} {
		_, err = StrategyOrder(in, id, nil)
		var unsupported *UnsupportedError
		if !errors.As(err, &unsupported) {
			t.Fatalf("%s: %v", id, err)
		}
	}
}

func TestCompareSimulatesCanonicalPoliciesOnce(t *testing.T) {
	in := comparisonInput(t)
	rep, err := Compare(ComparisonRequest{Input: in})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Results) != 6 || rep.Simulations != 2 || len(rep.PairwiseDeltas) != 15 {
		t.Fatalf("rows=%d runs=%d deltas=%d", len(rep.Results), rep.Simulations, len(rep.PairwiseDeltas))
	}
	if rep.Results[0].CanonicalID != rep.Results[1].CanonicalID {
		t.Fatal("aliases not canonical")
	}
	for _, d := range rep.PairwiseDeltas {
		want, e := rep.Results[d.A].Result.Cost().Sub(rep.Results[d.B].Result.Cost())
		if e != nil || d.Cost != want {
			t.Fatal("delta direction", e)
		}
	}
	req := ComparisonRequest{Input: in, StrategyIDs: []StrategyID{StrategyHighestRate, StrategyUtilisation}, OptimizedGoals: []Goal{{Kind: LeastInterest}, {Kind: Fastest}, {Kind: Relief, Cap: money.FromMinor(20000, in.Cash.Monthly.Currency())}}}
	rep, err = Compare(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Optimized) != 3 || rep.Results[1].Refusal == nil || rep.Results[1].Result != nil {
		t.Fatal("missing explicit refusal/ranking")
	}
	for _, g := range rep.Optimized {
		for _, r := range g.Ranked {
			if (g.Goal.Kind == Relief) != (r.Policy.Rollover == KeepFreed) {
				t.Fatal("mixed incompatible rollover")
			}
		}
	}
	for _, d := range rep.PairwiseDeltas {
		if rep.Results[d.A].Rollover != rep.Results[d.B].Rollover {
			t.Fatal("incompatible delta")
		}
	}
}

func TestComparisonHashIncludesMoneyDatesPermissionsAndMapOrder(t *testing.T) {
	in := comparisonInput(t)
	a := inputHash(in)
	in.Cash.Monthly = money.FromMinor(in.Cash.Monthly.Minor()+1, in.Cash.Monthly.Currency())
	if inputHash(in) == a {
		t.Fatal("hash dropped money")
	}
	a = inputHash(in)
	in.ValuationDate = date.MustNew(2026, 1, 16)
	if inputHash(in) == a {
		t.Fatal("hash dropped date")
	}
	in.Cash.Spending = &SpendingPlan{Monthly: in.Cash.Monthly}
	a = inputHash(in)
	copyPlan := *in.Cash.Spending
	in.Cash.Spending = &copyPlan
	if inputHash(in) != a {
		t.Fatal("hash uses pointer address")
	}
	in.Cash.Spending.Spent = in.Cash.Monthly
	if inputHash(in) == a {
		t.Fatal("hash dropped permission consumption")
	}
	in.Cash.MonthlyOverrides = map[string]money.Amount{"2026-02": in.Cash.Monthly, "2026-03": in.Cash.Monthly}
	a = inputHash(in)
	in.Cash.MonthlyOverrides = map[string]money.Amount{"2026-03": in.Cash.Monthly, "2026-02": in.Cash.Monthly}
	if inputHash(in) != a {
		t.Fatal("map iteration affects hash")
	}
}

func TestCompareCompatibleGoalsReuseSimulations(t *testing.T) {
	in := comparisonInput(t)
	one, err := Compare(ComparisonRequest{Input: in, OptimizedGoals: []Goal{{Kind: LeastInterest}}})
	if err != nil {
		t.Fatal(err)
	}
	many, err := Compare(ComparisonRequest{Input: in, OptimizedGoals: []Goal{{Kind: LeastInterest}, {Kind: Fastest}, {Kind: FirstWin}}})
	if err != nil {
		t.Fatal(err)
	}
	if one.Simulations != many.Simulations || one.Simulations != 2 {
		t.Fatalf("same policies rerun across goals: %d/%d", one.Simulations, many.Simulations)
	}
	for _, g := range many.Optimized {
		if g.Certificate.Policies != many.Simulations {
			t.Fatal("certificate counts candidates instead of attempts")
		}
	}
}

func TestSearchFeeBaselineDoesNotClaimAvalanche(t *testing.T) {
	in := comparisonInput(t)
	in.Loans[0].Contract.Prepayment.FeeBP = 100
	rep, err := Search(in, Goal{Kind: LeastInterest})
	if err != nil {
		t.Fatal(err)
	}
	if rep.AvalancheUnsupported == "" || rep.HighestRate.Policy.Name != "highest_rate" || rep.Avalanche.Policy.Name != "" {
		t.Fatal("fee-bearing nominal priority mislabeled avalanche")
	}
	if rep.Certificate.LowerBound != nil || rep.Certificate.Gap != nil {
		t.Fatal("winner's interest is not a lower bound")
	}
}
