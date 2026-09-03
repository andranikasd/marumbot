//go:build oracle

package plan_test

import (
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

// reducedOracle is an independent FORWARD exhaustive frontier. It uses no
// planner transition, schedule builder, accrual, rounding or priority primitive.
// Tiny amounts make these integer products provably fit int64. Each frontier
// belongs to one explicit calendar event; its key retains cash and both balances.
// Production uses backward recursion and memoized tails; this oracle retains
// the cheapest prefix reaching each exact state. Hold and every split are tried.
func reducedOracle(balances [2]int64, rates [2]int64, budget int64, horizon int) (int64, bool) {
	type state struct {
		balance [2]int64
		cash    int64
	}
	frontier := map[state]int64{{balance: balances}: 0}
	best := int64(1 << 60)
	days := []int64{31, 28, 31, 30}
	for month := 0; month < horizon; month++ {
		next := map[state]int64{}
		for s, cost := range frontier {
			remaining := s.balance
			available := s.cash + budget
			interest := int64(0)
			for i, b := range s.balance {
				// Half-up to one minor unit; ppb rate, actual/365. No money.Accrue.
				numerator := b * rates[i] * days[month]
				denominator := int64(365000000000)
				accrued := (numerator + denominator/2) / denominator
				owed := b + accrued
				required := min(int64(2), owed)
				remaining[i] = owed - required
				available -= required
				interest += accrued
			}
			if available < 0 {
				continue
			}
			for a := int64(0); a <= min(available, remaining[0]); a++ {
				for b := int64(0); b <= min(available-a, remaining[1]); b++ {
					ns := state{balance: [2]int64{remaining[0] - a, remaining[1] - b}, cash: available - a - b}
					nc := cost + interest
					if ns.balance == [2]int64{} {
						if nc < best {
							best = nc
						}
						continue
					}
					if old, ok := next[ns]; !ok || nc < old {
						next[ns] = nc
					}
				}
			}
		}
		frontier = next
	}
	return best, best < 1<<60
}

func TestDynamicMatchesIndependentReducedOracle(t *testing.T) {
	for _, budget := range []int64{6, 8, 10} {
		for _, rate := range []int64{240, 600} {
			ls := []plan.Position{pos("x", "X", 1, rate, 2), pos("y", "Y", 1, 120, 2)}
			for i := range ls {
				ls[i].Balance = money.FromMinor(int64(4+2*i), amd)
				ls[i].Contract.HasScheduled = true
				ls[i].Contract.ScheduledPayment = money.FromMinor(2, amd)
				ls[i].Contract.Prepayment.Effect = model.PrepayShortenTerm
				ls[i].Contract.Rounding = money.Policy{Mode: money.HalfUp, Unit: 1}
			}
			in := plan.Input{ValuationDate: date.MustNew(2026, 1, 15), Loans: ls, Cash: plan.CashPlan{Monthly: money.FromMinor(budget, amd)}}
			got, err := plan.SearchDynamic(plan.DynamicRequest{Input: in, Horizon: 4, MaxStates: 100000})
			if err != nil {
				t.Fatal(err)
			}
			if !got.Complete {
				t.Fatalf("oracle fixture unexpectedly capped: budget=%d rate=%d", budget, rate)
			}
			want, feasible := reducedOracle([2]int64{4, 6}, [2]int64{int64(ls[0].Contract.NominalRate), int64(ls[1].Contract.NominalRate)}, budget, 4)
			if feasible != (got.Cost != nil) || (feasible && want != got.Cost.Minor()) {
				t.Fatalf("budget=%d rate=%d got=%v oracle=%d feasible=%v", budget, rate, got.Cost, want, feasible)
			}
		}
	}
}
