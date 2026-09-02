package plan

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

func capInput(t *testing.T, n int) Input {
	t.Helper()
	v := date.MustNew(2026, 1, 15)
	cur := money.MustLookup("AMD")
	in := Input{ValuationDate: v, Cash: CashPlan{Monthly: money.FromMinor(10_000_000, cur), PayDay: 15}}
	for i := 0; i < n; i++ {
		l := honestyLoan(t, fmt.Sprint(i), money.Actual365, v)
		l.Balance = money.FromMinor(int64(i+1)*100_000, cur)
		l.Contract.NominalRate = money.RateFromPercent(int64(i+1)*3, 0)
		l.Contract.MaturityDate = date.MustNew(2026, 3, 15)
		in.Loans = append(in.Loans, l)
	}
	return in
}

func TestSearchCapAfterPermutationFallback(t *testing.T) {
	in := capInput(t, 4)
	cur := in.Cash.Monthly.Currency()
	// Two distinct named orders, 16 effects, 16 timings, nine batches:
	// 4608 candidates remain even AFTER dropping all permutations.
	batches := []money.Amount{{}}
	for i := range in.Loans {
		for j := 0; j < 2; j++ {
			a := money.FromMinor(int64(2*i+j+1)*100, cur)
			in.Loans[i].Contract.Prepayment.Charges = append(in.Loans[i].Contract.Prepayment.Charges,
				model.PrepaymentCharge{FreeAllowance: a, PercentBP: 1})
			batches = append(batches, a)
		}
	}
	u, err := Explore(in)
	if err != nil {
		t.Fatal(err)
	}
	// Build the expected prefix without the search's axis constructors,
	// cap constant, counters or candidate traversal. Every candidate here
	// is feasible; matching IDs checks actual simulations and their order.
	var want []string
	for _, o := range []struct {
		name string
		idx  []int
	}{{"avalanche", []int{3, 2, 1, 0}}, {"snowball", []int{0, 1, 2, 3}}} {
		for e := 0; e < 16; e++ {
			for tm := 0; tm < 16; tm++ {
				for _, b := range batches {
					p := Policy{Name: o.name, Order: o.idx, Rollover: RollFreed, MinPrepay: b}
					for i := 0; i < 4; i++ {
						effect := model.PrepayShortenTerm
						if e&(1<<i) != 0 {
							effect = model.PrepayReduceInstalment
						}
						timing := OnDue
						if tm&(1<<i) != 0 {
							timing = OnReceipt
						}
						p.Effect = append(p.Effect, effect)
						p.Timing = append(p.Timing, timing)
					}
					want = append(want, p.ID())
				}
			}
		}
	}
	if len(want) != 4608 {
		t.Fatalf("fixture has %d candidates", len(want))
	}
	var got []string
	for _, r := range u.Results {
		got = append(got, r.Policy.ID())
	}
	if !reflect.DeepEqual(got, want[:4096]) {
		t.Fatalf("simulated prefix differs: got %d, want 4096", len(got))
	}
	rep, err := u.Rank(Goal{Kind: LeastInterest})
	if err != nil {
		t.Fatal(err)
	}
	if c := rep.Certificate; c.Policies != 4096 || c.FeasiblePolicies != 4096 || c.Strength != BoundedHeuristic ||
		!strings.Contains(c.Truncation, "permutations dropped") || !strings.Contains(c.Truncation, "candidate prefix only") {
		t.Fatalf("capped certificate: %+v", c)
	}
	relief, err := u.Rank(Goal{Kind: Relief, Cap: in.Cash.Monthly})
	if err != nil {
		t.Fatal(err)
	}
	if c := relief.Certificate; c.Policies != 8192 || c.FeasiblePolicies != 8192 {
		t.Fatalf("counts must accumulate both rollovers: %+v", c)
	}
	for i, r := range u.Results[4096:] {
		p := r.Policy
		p.Rollover = RollFreed
		if p.ID() != want[i] {
			t.Fatalf("rollover prefix differs at %d", i)
		}
	}
	again, err := u.Rank(Goal{Kind: LeastInterest})
	if err != nil {
		t.Fatal(err)
	}
	if again.Certificate.Policies != 8192 || again.Certificate.FeasiblePolicies != 8192 {
		t.Fatal("ranking an explored rollover repeated simulations")
	}

	// The same candidate axes with no income make EVERY attempt infeasible.
	// A success-count cap would run all 4608; a Results-based certificate
	// would incorrectly say zero attempts. Establish infeasibility directly.
	failed := *u
	failed.Input.Cash.Monthly = money.Zero(cur)
	failed.Results, failed.explored, failed.attempted = nil, nil, 0
	failed.cache = cache{}
	if _, err := Run(failed.Input, u.Results[0].Policy); !isInfeasible(err) {
		t.Fatalf("expected independent infeasibility check, got %v", err)
	}
	for _, rollover := range []Rollover{RollFreed, KeepFreed, RollFreed} {
		if err := failed.explore(rollover); err != nil {
			t.Fatal(err)
		}
	}
	c := failed.certificate(rep.Goal, rep)
	if c.Policies != 8192 || c.FeasiblePolicies != 0 || len(failed.Results) != 0 {
		t.Fatalf("infeasible attempts must consume the cap: %+v", c)
	}
}

func TestSearchVectorTruncationPreventsExhaustiveClaims(t *testing.T) {
	for _, tc := range []struct {
		name    string
		effects bool
		timing  bool
		minimum bool
	}{
		{name: "effects only", effects: true},
		{name: "timings with minimum", timing: true, minimum: true},
		{name: "both without fees", effects: true, timing: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := capInput(t, 5)
			if !tc.timing {
				in.Cash.PayDay = 0
			}
			for i := range in.Loans {
				if !tc.effects {
					in.Loans[i].Contract.Prepayment.Effect = model.PrepayShortenTerm
				}
				if tc.minimum {
					in.Loans[i].Contract.Prepayment.MinAmount = money.FromMinor(100, in.Cash.Monthly.Currency())
				}
			}
			u, err := Explore(in)
			if err != nil {
				t.Fatal(err)
			}
			for _, goal := range []Goal{{Kind: LeastInterest}, {Kind: Fastest}} {
				rep, err := u.Rank(goal)
				if err != nil {
					t.Fatal(err)
				}
				c := rep.Certificate
				if c.Strength != BoundedHeuristic || c.Eligibility != "" {
					t.Fatalf("truncated vectors claimed %s: %+v", c.Strength, c)
				}
				if strings.Contains(c.Truncation, "effects:") != tc.effects || strings.Contains(c.Truncation, "timings:") != tc.timing {
					t.Fatalf("missing or incorrect vector truncation: %s", c.Truncation)
				}
			}
		})
	}
}

func TestSearchProofExcludesNewSpendingDomain(t *testing.T) {
	for _, mode := range []string{"legacy", "spending", "optional excluded"} {
		t.Run(mode, func(t *testing.T) {
			in := capInput(t, 1)
			switch mode {
			case "spending":
				in.Cash.Spending = &SpendingPlan{Monthly: in.Cash.Monthly}
			case "optional excluded":
				in.Loans[0].OptionalExcluded = true
			}
			rep, err := Search(in, Goal{Kind: LeastInterest})
			if err != nil {
				t.Fatal(err)
			}
			if (rep.Certificate.Strength == ProvenOptimal) != (mode == "legacy") {
				t.Fatalf("unexpected proof strength: %+v", rep.Certificate)
			}
			if mode != "legacy" && rep.Certificate.Eligibility != "" {
				t.Fatal("excluded proof retained eligibility")
			}
			if rep.Certificate.EngineVersion != "plan/3" {
				t.Fatalf("wrong engine version: %s", rep.Certificate.EngineVersion)
			}
		})
	}
}

func TestMinimumPreservesExplicitFundingAndPermission(t *testing.T) {
	for _, mode := range []string{"confirmed lump", "no funding", "expected lump", "low permission"} {
		t.Run(mode, func(t *testing.T) {
			in := capInput(t, 1)
			cur := in.Cash.Monthly.Currency()
			in.Loans[0].Contract.NominalRate = 0
			in.Cash.Monthly = money.Zero(cur)
			in.Cash.Spending = &SpendingPlan{Monthly: money.FromMinor(100_000, cur)}
			if mode != "no funding" {
				in.Cash.Lumps = []CashEvent{{On: in.ValuationDate, Amount: money.FromMinor(200_000, cur), Expected: mode == "expected lump"}}
			}
			if mode == "low permission" {
				in.Cash.Spending.Monthly = money.FromMinor(100, cur)
			}
			got, err := minimum(in, cache{})
			if mode != "confirmed lump" {
				if !isInfeasible(err) {
					t.Fatalf("minimum fabricated funding or permission: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			want, err := Run(in, Policy{
				Name: "minimum", Order: []int{0}, Timing: []Timing{OnDue},
				Effect: []model.PrepaymentEffect{model.PrepayReduceInstalment}, Rollover: KeepFreed, RequiredOnly: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) || !got.PayoffDate.Equal(in.Loans[0].Contract.MaturityDate) {
				t.Fatal("minimum changed cash inputs or prepaid the loan")
			}
			for _, m := range got.Timeline {
				if m.Extra.Sign() != 0 {
					t.Fatal("required-only minimum allocated extra")
				}
			}
		})
	}
}

func TestMinimumLegacyLargeOpeningCashNeverPrepays(t *testing.T) {
	in := capInput(t, 1)
	in.Loans[0].Contract.NominalRate = 0
	in.Cash.OpeningCash = in.Cash.Monthly // one hundred times the balance
	r, err := minimum(in, cache{})
	if err != nil {
		t.Fatal(err)
	}
	var requiredMinor int64
	for _, m := range r.Timeline {
		if m.Extra.Sign() != 0 {
			t.Fatalf("minimum made an optional payment in cycle %d", m.Month)
		}
		for _, l := range m.Loans {
			if l.Extra.Sign() != 0 {
				t.Fatalf("minimum prepaid a loan in cycle %d", m.Month)
			}
		}
		requiredMinor += m.Required.Minor()
	}
	if !r.Policy.RequiredOnly || r.Prepayments != 0 || !r.PayoffDate.Equal(in.Loans[0].Contract.MaturityDate) {
		t.Fatal("minimum did not follow the required-only contractual schedule")
	}
	// Zero interest: the contractual payments must repay exactly the balance.
	if requiredMinor != in.Loans[0].Balance.Minor() || r.TotalPaid.Minor() != requiredMinor {
		t.Fatal("minimum changed the contractual payment total")
	}
}
