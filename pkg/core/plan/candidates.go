package plan

// The candidate axes: the priority orders, prepayment effects, payment
// timings and batch thresholds whose product is the policy space a search
// explores. Enumeration only -- nothing here simulates or ranks.

import (
	"fmt"
	"sort"

	"github.com/andranikasd/marumbot/pkg/core/allocation"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// effectVectors enumerates the prepayment effect per loan whose contract
// leaves the choice to the borrower, up to maxVectorLoans free loans;
// beyond that only the two uniform vectors are tried.
func effectVectors(loans []Position) [][]model.PrepaymentEffect {
	n := len(loans)
	var free []int
	for i, l := range loans {
		if l.Contract.Prepayment.Effect == model.PrepayBorrowerChooses && l.Contract.Type == model.Annuity && l.Balance.Sign() > 0 {
			free = append(free, i)
		}
	}
	if len(free) == 0 {
		return [][]model.PrepaymentEffect{uniform(n, model.PrepayBorrowerChooses)}
	}
	if len(free) > maxVectorLoans {
		return [][]model.PrepaymentEffect{uniform(n, model.PrepayShortenTerm), uniform(n, model.PrepayReduceInstalment)}
	}
	var out [][]model.PrepaymentEffect
	for mask := 0; mask < 1<<len(free); mask++ {
		v := uniform(n, model.PrepayBorrowerChooses)
		for k, i := range free {
			if mask&(1<<k) != 0 {
				v[i] = model.PrepayReduceInstalment
			} else {
				v[i] = model.PrepayShortenTerm
			}
		}
		out = append(out, v)
	}
	return out
}

// timingVectors enumerates on_receipt/on_due per loan that a lender would
// credit early, when a payday is known. Loans that cannot be credited early
// are always on_due.
//
// Without fees or thresholds the per-loan mixes are dominated: interest is
// non-increasing in each payment's date, so on_receipt for every creditable
// loan weakly beats every mix. Only the two uniform vectors are run then —
// the second so the report can price the payday. With fees the mixes are
// enumerated, because a fee can make waiting right for one loan and wrong
// for another.
func timingVectors(in Input, loans []Position) [][]Timing {
	n := len(loans)
	var free []int
	fees := false
	if in.Cash.PayDay > 0 {
		for i, l := range loans {
			if l.Excess == allocation.ExcessReducePrincipal && l.Balance.Sign() > 0 {
				free = append(free, i)
			}
			if len(l.Contract.Prepayment.Charges) > 0 || l.Contract.Prepayment.FeeBP > 0 || l.Contract.Prepayment.MinAmount.Sign() > 0 {
				fees = true
			}
		}
	}
	if len(free) == 0 {
		return [][]Timing{uniform(n, OnDue)}
	}
	if len(free) > maxVectorLoans || !fees {
		all := uniform(n, OnDue)
		for _, i := range free {
			all[i] = OnReceipt
		}
		return [][]Timing{uniform(n, OnDue), all}
	}
	var out [][]Timing
	for mask := 0; mask < 1<<len(free); mask++ {
		v := uniform(n, OnDue)
		for k, i := range free {
			if mask&(1<<k) != 0 {
				v[i] = OnReceipt
			}
		}
		out = append(out, v)
	}
	return out
}

// batchThresholds are the amounts worth waiting for when fees apply: each
// loan's free allowance, and the amount at which a fixed fee is one per
// cent of the payment. They are contract breakpoints, not a grid.
func batchThresholds(in Input) []money.Amount {
	cur := in.Cash.Monthly.Currency()
	seen := map[int64]bool{}
	var out []money.Amount
	add := func(a money.Amount) {
		if a.Sign() > 0 && !seen[a.Minor()] {
			seen[a.Minor()] = true
			out = append(out, a)
		}
	}
	for _, l := range in.Loans {
		for _, r := range l.Contract.Prepayment.Charges {
			add(r.FreeAllowance)
			if r.Fixed.Sign() > 0 {
				add(money.FromMinor(r.Fixed.Minor()*100, cur))
			}
		}
		add(l.Contract.Prepayment.MinAmount)
	}
	return out
}

// Named strategies. Anything else in the candidate set is a bare permutation.
const (
	nameAvalanche = "avalanche"
	nameSnowball  = "snowball"
	namePermuted  = "order"
)

type order struct {
	name string
	idx  []int
	also []string
}

func namedOrders(loans []Position) []order {
	av := identity(len(loans))
	sort.SliceStable(av, func(a, b int) bool {
		ra, rb := loans[av[a]].Contract.NominalRate, loans[av[b]].Contract.NominalRate
		if ra != rb {
			return ra > rb
		}
		if c := loans[av[a]].Balance.Cmp(loans[av[b]].Balance); c != 0 {
			return c < 0
		}
		return loans[av[a]].ID < loans[av[b]].ID
	})
	sn := identity(len(loans))
	sort.SliceStable(sn, func(a, b int) bool {
		c := loans[sn[a]].Balance.Cmp(loans[sn[b]].Balance)
		if c != 0 {
			return c < 0
		}
		if r1, r2 := loans[sn[a]].Contract.NominalRate, loans[sn[b]].Contract.NominalRate; r1 != r2 {
			return r1 > r2
		}
		return loans[sn[a]].ID < loans[sn[b]].ID
	})
	name := nameAvalanche
	if avalancheDomain(Input{Loans: loans}) != "" {
		name = string(StrategyHighestRate)
	}
	return []order{{name: name, idx: av}, {name: nameSnowball, idx: sn}}
}

func permutations(n int) []order {
	var out []order
	cur := identity(n)
	var rec func(k int)
	rec = func(k int) {
		if k == n {
			cp := make([]int, n)
			copy(cp, cur)
			out = append(out, order{name: namePermuted, idx: cp})
			return
		}
		for i := k; i < n; i++ {
			cur[k], cur[i] = cur[i], cur[k]
			rec(k + 1)
			cur[k], cur[i] = cur[i], cur[k]
		}
	}
	rec(0)
	return out
}

func dedupeOrders(os []order) []order {
	seen := map[string]int{}
	var out []order
	for _, o := range os {
		k := fmt.Sprint(o.idx)
		if i, ok := seen[k]; ok {
			out[i].also = append(out[i].also, o.name)
			continue
		}
		seen[k] = len(out)
		o.also = []string{o.name}
		out = append(out, o)
	}
	return out
}
