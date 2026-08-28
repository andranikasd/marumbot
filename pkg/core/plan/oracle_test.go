//go:build oracle

// The oracle takes minutes by design. Run it with: go test -tags oracle -run Oracle ./pkg/core/plan/

package plan_test

import (
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/amortisation"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

// The oracle is an independent, deliberately slow exhaustive search over
// DYNAMIC allocations: every month it tries every split of the surplus
// across the loans in coarse steps, including keeping cash for later, and
// follows the whole tree. It shares the single-loan arithmetic (Accrue,
// Solve) with the engine but none of the portfolio logic, and it knows
// nothing about priority orders. On fee-free immediate-credit loans with a
// fixed instalment the static-order search must reach its cost; a gap would
// be a static-order assumption failing.

type oLoan struct {
	balance    int64 // minor
	instalment int64 // fixed at the start: shorten_term semantics
	rate       money.Rate
	dc         money.DayCount
	pol        money.Policy
	day        int
	from       date.Date
}

func (l oLoan) accrue(to date.Date) int64 {
	i, _ := money.Accrue(money.FromMinor(l.balance, amd), l.rate, int64(date.DaysBetween(l.from, to)), l.dc, l.pol)
	return i.Minor()
}

// oracleStep pays one month: the instalment on the due date, plus `extra`
// on the same date (payday = due day, so there is no timing dimension).
// Returns interest paid and the loan after the payment.
func oracleStep(l oLoan, extra int64) (int64, oLoan) {
	due := date.Occurrence(l.from, l.day, 1)
	interest := l.accrue(due)
	owed := l.balance + interest
	pay := l.instalment + extra
	if pay > owed {
		pay = owed
	}
	l.balance = owed - pay
	l.from = due
	return interest, l
}

func oracleBest(loans []oLoan, budget, carry int64, step int64, depth int, memo map[[5]int64]int64) int64 {
	open := 0
	var key [5]int64
	for i, l := range loans {
		if l.balance > 0 {
			open++
		}
		if i < 4 {
			key[i] = l.balance
		}
	}
	if open == 0 {
		return 0
	}
	if depth == 0 {
		// Horizon: a cost floor that cannot favour leaving debt open, so the
		// tree is compared on equal footing — the remaining interest if the
		// budget is spent on the highest rate first from here is not known
		// to the oracle; it simply refuses to leave loans open.
		return 1 << 60
	}
	key[4] = carry*1000 + int64(depth)
	if v, ok := memo[key]; ok {
		return v
	}
	required := int64(0)
	for _, l := range loans {
		if l.balance > 0 {
			due := date.Occurrence(l.from, l.day, 1)
			owed := l.balance + l.accrue(due)
			r := l.instalment
			if r > owed {
				r = owed
			}
			required += r
		}
	}
	surplus := budget + carry - required
	if surplus < 0 {
		memo[key] = 1 << 60
		return 1 << 60
	}
	best := int64(1 << 60)
	n := len(loans)
	split := make([]int64, n)
	var rec func(i int, left int64)
	rec = func(i int, left int64) {
		if i == n {
			interest := int64(0)
			next := make([]oLoan, n)
			for j, l := range loans {
				if l.balance <= 0 {
					next[j] = l
					continue
				}
				in, nl := oracleStep(l, split[j])
				interest += in
				next[j] = nl
			}
			rest := oracleBest(next, budget, left, step, depth-1, memo)
			if rest < 1<<59 && interest+rest < best {
				best = interest + rest
			}
			return
		}
		if loans[i].balance <= 0 {
			split[i] = 0
			rec(i+1, left)
			return
		}
		for a := int64(0); a <= left; a += step {
			split[i] = a
			rec(i+1, left-a)
		}
		if left%step != 0 {
			split[i] = left // the remainder, so a payoff is always reachable
			rec(i+1, 0)
		}
	}
	rec(0, surplus)
	memo[key] = best
	return best
}

func TestStaticOrderMatchesDynamicOracleWithoutFees(t *testing.T) {
	ls := []plan.Position{
		pos("x", "X", 120_000, 24, 1),
		pos("y", "Y", 180_000, 12, 1),
	}
	for i := range ls {
		ls[i].Contract.Prepayment.Effect = model.PrepayShortenTerm
	}
	in := input(ls, 60_000, 15) // payday on the due day: no timing dimension
	rep, err := plan.Search(in, plan.Goal{Kind: plan.LeastInterest})
	if err != nil {
		t.Fatal(err)
	}

	var ol []oLoan
	for _, l := range ls {
		inst, err := amortisation.Solve(l.Contract, l.Balance, l.From)
		if err != nil {
			t.Fatal(err)
		}
		ol = append(ol, oLoan{
			balance: l.Balance.Minor(), instalment: inst.Minor(), rate: l.Contract.NominalRate,
			dc: l.Contract.DayCount, pol: l.Contract.Rounding, day: l.Contract.PaymentDay, from: l.From,
		})
	}
	oracle := oracleBest(ol, amt(60_000).Minor(), 0, amt(10_000).Minor(), 7, map[[5]int64]int64{})
	if oracle >= 1<<59 {
		t.Fatal("oracle found no complete policy within the horizon")
	}
	got := rep.Best.TotalInterest.Minor()
	t.Logf("search %d, oracle %d (minor units)", got, oracle)
	if got > oracle {
		t.Fatalf("search found %s, oracle found %s", rep.Best.TotalInterest, money.FromMinor(oracle, amd))
	}
}
