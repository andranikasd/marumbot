// Package plan allocates a monthly budget across several loans.
//
// The rule that matters is not which loan to target. It is that the surplus
// goes to ONE loan.
//
// Left alone, people balance-match: they split repayment in proportion to each
// debt's share of total balances. Gathergood, Mahoney, Stewart and Weber (AER
// 109(3), 2019) found this captures more than half the predictable variation in
// how people repay multiple cards, that it does not improve as the stakes rise,
// and that it does not improve with experience. It is strictly dominated, and
// the loss grows with the number of debts -- which is exactly the situation a
// planner is used in.
//
// Concentrating the surplus beats spreading it on cost. Kettle and colleagues
// (2016) found separately that spreading it is also the least motivating
// arrangement. Both dimensions point the same way, which makes concentration
// the one recommendation here with no trade-off attached.
//
// Which loan to concentrate on is a second-order question worth a few per cent,
// and for many borrowers the orderings coincide and it is worth nothing at all.
// So it is offered as a goal rather than decided by the engine.
package plan

import (
	"errors"
	"fmt"
	"sort"

	"github.com/andranikasd/marumbot/pkg/core/money"
)

// ErrBudget is returned when a budget cannot cover the contractual minimums.
var ErrBudget = errors.New("plan: budget below required payments")

// Goal is what the borrower is optimising for.
type Goal uint8

// The goals Marum offers.
const (
	// PayLeastInterest targets the highest rate first. This is the avalanche,
	// and it minimises total interest when every loan's minimum is met and no
	// prepayment fee applies -- both of which this package checks rather than
	// assumes.
	PayLeastInterest Goal = iota

	// FinishSoonest targets the smallest balance first, clearing whole loans
	// early. It costs slightly more interest than the avalanche and removes
	// obligations sooner, which is the snowball.
	FinishSoonest

	// FreeUpMonthly targets whichever loan frees the most required payment per
	// dram repaid. For a borrower whose problem is this month rather than this
	// decade, that is a different loan from either of the above.
	FreeUpMonthly
)

func (g Goal) String() string {
	switch g {
	case FinishSoonest:
		return "finish_soonest"
	case FreeUpMonthly:
		return "free_up_monthly"
	default:
		return "pay_least_interest"
	}
}

// Loan is one debt as the planner sees it.
type Loan struct {
	ID       string
	Balance  money.Amount
	Rate     money.Rate   // annual nominal, for ordering only
	Required money.Amount // the contractual instalment, which must be paid
	// PrepaymentFeeBP is charged on any amount above the required payment, in
	// basis points. Armenian consumer credit forbids this outright -- Consumer
	// Lending Law Article 10 makes any agreement to the contrary void -- but
	// mortgages may charge it in the first three years, so it is a field rather
	// than an assumption.
	PrepaymentFeeBP int
}

// Allocation is what one loan receives this month.
type Allocation struct {
	LoanID   string
	Required money.Amount
	Extra    money.Amount
	Total    money.Amount
}

// Plan is a month's decision.
type Plan struct {
	Allocations []Allocation
	Target      string       // the loan the surplus went to, empty if none
	Surplus     money.Amount // what was left after the required payments
	Unspent     money.Amount // surplus deliberately not applied, and why below
	Note        string
}

// Allocate splits a monthly budget across loans.
//
// Required payments come first, always. A plan that underpays one loan to
// accelerate another manufactures arrears, and arrears cost penalty interest
// that dwarfs whatever the acceleration saved -- in Armenia up to four times
// the Central Bank's rate, under Civil Code Article 372.
func Allocate(loans []Loan, budget money.Amount, goal Goal) (Plan, error) {
	if len(loans) == 0 {
		return Plan{}, errors.New("plan: no loans")
	}
	cur := budget.Currency()

	required := money.Zero(cur)
	for _, l := range loans {
		if l.Balance.Currency() != cur || l.Required.Currency() != cur {
			// Allocating a dram budget across a dollar loan needs an exchange
			// rate, and Marum has no validated source for one. Refusing is the
			// honest answer.
			return Plan{}, fmt.Errorf("plan: loan %s is in %s, budget in %s",
				l.ID, l.Balance.Currency(), cur)
		}
		var err error
		if required, err = required.Add(l.Required); err != nil {
			return Plan{}, err
		}
	}

	if budget.Cmp(required) < 0 {
		return Plan{}, fmt.Errorf("%w: %s required, %s offered", ErrBudget, required, budget)
	}

	surplus, err := budget.Sub(required)
	if err != nil {
		return Plan{}, err
	}

	p := Plan{
		Allocations: make([]Allocation, 0, len(loans)),
		Surplus:     surplus,
		Unspent:     money.Zero(cur),
	}
	for _, l := range loans {
		p.Allocations = append(p.Allocations, Allocation{
			LoanID: l.ID, Required: l.Required,
			Extra: money.Zero(cur), Total: l.Required,
		})
	}
	if surplus.Sign() == 0 {
		p.Note = "budget covers the required payments exactly"
		return p, nil
	}

	target, note := pick(loans, goal)
	if target < 0 {
		// Every loan charges a prepayment fee that outweighs the interest saved.
		// Holding the money is then the correct answer, and saying so is more
		// useful than spending it to look decisive.
		p.Unspent = surplus
		p.Note = note
		return p, nil
	}

	a := &p.Allocations[target]
	extra := surplus
	// Never pay more than the loan is worth. The remainder is genuinely spare.
	if extra.Cmp(loans[target].Balance) > 0 {
		extra = loans[target].Balance
		if p.Unspent, err = surplus.Sub(extra); err != nil {
			return Plan{}, err
		}
	}
	a.Extra = extra
	if a.Total, err = a.Required.Add(extra); err != nil {
		return Plan{}, err
	}
	p.Target = loans[target].ID
	p.Note = note
	return p, nil
}

// pick chooses the one loan the surplus goes to, or reports that none should
// receive it.
func pick(loans []Loan, goal Goal) (int, string) {
	type cand struct {
		i    int
		rate money.Rate
		bal  int64
		req  int64
	}
	var cs []cand
	for i, l := range loans {
		if l.Balance.Sign() <= 0 {
			continue // already settled
		}
		if l.PrepaymentFeeBP > 0 && !feeWorthPaying(l) {
			continue
		}
		cs = append(cs, cand{i, l.Rate, l.Balance.Minor(), l.Required.Minor()})
	}
	if len(cs) == 0 {
		return -1, "every loan charges a prepayment fee larger than the interest " +
			"an early payment would save, so the surplus is better kept"
	}

	switch goal {
	case FinishSoonest:
		// Smallest balance first: the fastest way to remove a whole obligation.
		sort.Slice(cs, func(a, b int) bool {
			if cs[a].bal != cs[b].bal {
				return cs[a].bal < cs[b].bal
			}
			return cs[a].rate > cs[b].rate
		})
		return cs[0].i, "clearing the smallest balance first removes an obligation soonest"

	case FreeUpMonthly:
		// Most required payment freed per dram repaid. Comparing req/bal as a
		// ratio without dividing: a*d > c*b is the same ordering as a/b > c/d
		// for positive values, and avoids a rounding step.
		sort.Slice(cs, func(a, b int) bool {
			l, r := cs[a].req*cs[b].bal, cs[b].req*cs[a].bal
			if l != r {
				return l > r
			}
			return cs[a].rate > cs[b].rate
		})
		return cs[0].i, "this loan frees the most monthly payment per dram repaid"

	default:
		// Highest rate first: the avalanche. Minimises total interest given the
		// minimums are met and no prepayment fee applies, both already ensured.
		sort.Slice(cs, func(a, b int) bool {
			if cs[a].rate != cs[b].rate {
				return cs[a].rate > cs[b].rate
			}
			return cs[a].bal < cs[b].bal
		})
		return cs[0].i, "paying down the highest rate first costs the least interest"
	}
}

// feeWorthPaying reports whether prepaying still helps once the fee is charged.
//
// A fee of f basis points is worth paying only if the interest avoided exceeds
// it. Repaying one dram early avoids interest for the remaining life of that
// dram, so the comparison is the annual rate against the fee: below roughly one
// year of interest, the fee eats the saving.
func feeWorthPaying(l Loan) bool {
	// Rate is parts per billion of a year; a basis point is 1e-4.
	feePPB := int64(l.PrepaymentFeeBP) * 100_000
	return int64(l.Rate) > feePPB
}
