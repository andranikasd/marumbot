package plan

import (
	"fmt"
	"sort"

	"github.com/andranikasd/marumbot/pkg/core/allocation"
	"github.com/andranikasd/marumbot/pkg/core/amortisation"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// This file is the optimiser. Simulate answers "what does this goal produce";
// Search answers "which way of paying produces the best result for this goal",
// by running every candidate policy through the same dated simulator and
// ranking what comes out.
//
// The search space is deliberately small, and the reason is a theorem rather
// than a shortcut. Under simple daily accrual on a declining balance, total
// interest is linear in each payment and non-increasing in each payment's
// date: a dram repaid on day t stops accruing from day t on, and nothing about
// a later payment can recover that. So for a lender that applies excess to
// principal on the day it is paid, the surplus is best paid on the earliest
// day it exists, and splitting it between an early date and the due date can
// only tie, never win. That removes the intra-month dimension from the search.
// What remains is which loan gets the surplus, and whether the lender credits
// an early payment at all — and those the search enumerates.
//
// The split variant is still simulated, on purpose: a borrower who suspects
// "a bit before, a bit after" is cleverer deserves to see the number, not a
// footnote.

// Timing is when within a cycle the surplus is paid.
type Timing uint8

const (
	// OnDue pays the surplus together with the required instalment.
	OnDue Timing = iota
	// OnReceipt pays the surplus on the day the budget becomes available,
	// before the due date, so the balance accrues less for the rest of the
	// cycle. Only lenders that reduce principal on the day of payment give
	// this any effect; for the others the simulator treats it as OnDue and
	// says so in the outcome.
	OnReceipt
	// SplitHalf pays half on receipt and half on the due date. It exists to be
	// measured against the other two, and it never beats OnReceipt.
	SplitHalf
)

func (t Timing) String() string {
	switch t {
	case OnReceipt:
		return "on_receipt"
	case SplitHalf:
		return "split_half"
	default:
		return "on_due"
	}
}

// Cash is what the borrower has to spend each cycle and when it arrives.
type Cash struct {
	Monthly money.Amount
	// Day is the day of the month the money is available, 1..31. Zero means
	// unknown, which disables the early-payment variants: the engine will not
	// credit a payment on a day it cannot name.
	Day int
}

// Named strategies. Anything else in the candidate set is a bare permutation.
const (
	nameAvalanche = "avalanche"
	nameSnowball  = "snowball"
	namePermuted  = "order"
)

// Policy is one way of spending the surplus: a priority order over the loans,
// and a timing. The order is by index into the positions the search was given.
type Policy struct {
	Name   string // avalanche, snowball, relief, or "order" for a permutation
	Order  []int
	Timing Timing
}

func (p Policy) String() string {
	return fmt.Sprintf("%s/%s", p.Name, p.Timing)
}

// Action is one payment the best policy makes in the first month, dated.
type Action struct {
	On     date.Date
	LoanID string
	Loan   string
	Amount money.Amount
	Extra  bool // false for the contractual instalment
	// Saves is the interest this cycle that paying on this date rather than
	// the due date avoids: amount × rate × days / basis. Zero for a payment
	// on the due date, and zero when the lender does not credit early
	// payment.
	Saves money.Amount
}

// Result is what one policy produces over the whole run.
type Result struct {
	Policy        Policy
	Months        int
	TotalInterest money.Amount
	NextMonthOwed money.Amount
	ClearedFirst  string
	ClearedMonth  int
	MonthlyFreed  money.Amount
	Actions       []Action // first month only
	// TimingCredited is false when the policy asked for early payment but no
	// loan's lender reduces principal on the day of payment, so the early
	// variant collapsed to paying on the due date.
	TimingCredited bool
}

// Report is the answer to "how should I pay": the best policy for the goal,
// the conventional strategies it was measured against, and how far the search
// went so the reader knows whether "best" means best of everything or best of
// the usual suspects.
type Report struct {
	Goal      Goal
	Best      Result
	Ranked    []Result // every distinct candidate, best first
	Avalanche Result   // highest rate first, paid on the due date
	Snowball  Result   // smallest balance first, paid on the due date
	// TimingSaving is the interest the best policy saves over the same order
	// paid on the due date. It is the price of ignoring the payday.
	TimingSaving money.Amount
	// Exhaustive is true when every priority order was tried. Above
	// maxExhaustive loans only the named strategies are.
	Exhaustive bool
	Evaluated  int
}

// maxExhaustive bounds the permutation search. Five loans is 5! × 3 timings =
// 360 runs, about a second with the step cache; six would be 2,160 and the
// borrower would wait. Beyond it only the named strategies are tried, and the
// report says so.
const maxExhaustive = 5

// Search runs every candidate policy and ranks the outcomes for the goal.
func Search(loans []Position, cash Cash, goal Goal) (Report, error) {
	if len(loans) == 0 {
		return Report{}, fmt.Errorf("plan: no loans")
	}
	if cash.Day < 0 || cash.Day > 31 {
		return Report{}, fmt.Errorf("plan: cash day %d out of range", cash.Day)
	}

	early := cash.Day > 0 && anyReducesOnPayment(loans)
	timings := []Timing{OnDue}
	if early {
		timings = append(timings, OnReceipt, SplitHalf)
	}

	orders := namedOrders(loans)
	exhaustive := len(loans) <= maxExhaustive
	if exhaustive {
		orders = append(orders, permutations(len(loans))...)
	}
	orders = dedupeOrders(orders)

	rep := Report{Goal: goal, Exhaustive: exhaustive}
	cache := stepCache{}
	for _, o := range orders {
		for _, t := range timings {
			r, err := run(loans, cash, Policy{Name: o.name, Order: o.idx, Timing: t}, cache)
			if err != nil {
				return Report{}, err
			}
			rep.Ranked = append(rep.Ranked, r)
			if t == OnDue {
				for _, n := range o.also {
					switch n {
					case nameAvalanche:
						rep.Avalanche = r
					case nameSnowball:
						rep.Snowball = r
					}
				}
			}
		}
	}
	rep.Evaluated = len(rep.Ranked)
	sort.SliceStable(rep.Ranked, func(i, j int) bool { return better(goal, rep.Ranked[i], rep.Ranked[j]) })
	rep.Best = rep.Ranked[0]

	// The saving from timing alone: the same order, paid on the due date.
	rep.TimingSaving = money.Zero(cash.Monthly.Currency())
	if rep.Best.Policy.Timing != OnDue {
		base, err := run(loans, cash, Policy{Name: rep.Best.Policy.Name, Order: rep.Best.Policy.Order, Timing: OnDue}, cache)
		if err != nil {
			return Report{}, err
		}
		if rep.TimingSaving, err = base.TotalInterest.Sub(rep.Best.TotalInterest); err != nil {
			return Report{}, err
		}
	}
	return rep, nil
}

// better orders two results for a goal. Ties fall through to the next
// criterion, and finally to the simplest policy, so the ranking is total and
// the answer does not depend on iteration order.
func better(goal Goal, a, b Result) bool {
	ic := a.TotalInterest.Cmp(b.TotalInterest)
	switch goal {
	case FinishSoonest:
		if a.Months != b.Months {
			return a.Months < b.Months
		}
		if ic != 0 {
			return ic < 0
		}
	case FreeUpMonthly:
		// Earliest relief first, then the most of it, then cost.
		am, bm := a.ClearedMonth, b.ClearedMonth
		if am == 0 {
			am = 1 << 30
		}
		if bm == 0 {
			bm = 1 << 30
		}
		if am != bm {
			return am < bm
		}
		if fc := a.MonthlyFreed.Cmp(b.MonthlyFreed); fc != 0 {
			return fc > 0
		}
		if ic != 0 {
			return ic < 0
		}
	default:
		if ic != 0 {
			return ic < 0
		}
		if a.Months != b.Months {
			return a.Months < b.Months
		}
	}
	// Prefer a named strategy over an anonymous permutation, then fewer
	// payments, so a tie reads as "the avalanche" rather than "order 3-1-2".
	if (a.Policy.Name == namePermuted) != (b.Policy.Name == namePermuted) {
		return a.Policy.Name != namePermuted
	}
	return a.Policy.Timing < b.Policy.Timing
}

// stepKey identifies one loan's state at the start of a cycle. Loans that are
// not receiving surplus follow the contractual path, which is the same under
// every policy, so a search memoises the projection by state and only the
// target loan's diverging states cost a fresh solve.
type stepKey struct {
	idx     int
	balance int64
	from    date.Date
}

type stepVal struct {
	due      date.Date
	interest money.Amount
	required money.Amount
}

type stepCache map[stepKey]stepVal

func (m stepCache) next(idx int, p Position) (stepVal, error) {
	k := stepKey{idx, p.Balance.Minor(), p.From}
	if v, ok := m[k]; ok {
		return v, nil
	}
	s, err := amortisation.Build(p.Contract, p.Balance, p.From)
	if err != nil || len(s.Rows) == 0 {
		return stepVal{}, fmt.Errorf("plan: projecting %s: %w", p.ID, err)
	}
	r := s.Rows[0]
	v := stepVal{due: r.Due, interest: r.Interest, required: r.Payment}
	m[k] = v
	return v, nil
}

// Run follows one policy month by month until every loan is clear.
//
// Each cycle the required instalment of every live loan is reserved first;
// the surplus then goes to the loans in the policy's order, each taking as
// much as it can absorb before the remainder moves to the next. That cascade
// is what lets a cleared loan's freed instalment flow to the next target in
// the same month rather than the one after.
func Run(loans []Position, cash Cash, pol Policy) (Result, error) {
	return run(loans, cash, pol, stepCache{})
}

func run(loans []Position, cash Cash, pol Policy, cache stepCache) (Result, error) {
	if len(pol.Order) != len(loans) {
		return Result{}, fmt.Errorf("plan: policy covers %d of %d loans", len(pol.Order), len(loans))
	}
	cur := cash.Monthly.Currency()
	res := Result{
		Policy:        pol,
		TotalInterest: money.Zero(cur),
		NextMonthOwed: money.Zero(cur),
		MonthlyFreed:  money.Zero(cur),
	}

	live := make([]Position, len(loans))
	copy(live, loans)

	for month := 1; month <= maxMonths; month++ {
		type cycle struct {
			idx      int
			due      date.Date
			payday   date.Date // zero when no early payment this cycle
			interest money.Amount
			required money.Amount
		}
		var cycles []cycle
		required := money.Zero(cur)

		for i := range live {
			if live[i].Balance.Sign() <= 0 {
				continue
			}
			r, err := cache.next(i, live[i])
			if err != nil {
				return res, err
			}
			c := cycle{idx: i, due: r.due, interest: r.interest, required: r.required}
			if pol.Timing != OnDue && live[i].Excess == allocation.ExcessReducePrincipal {
				c.payday = paydayIn(live[i].From, r.due, cash.Day)
			}
			cycles = append(cycles, c)
			if required, err = required.Add(r.required); err != nil {
				return res, err
			}
		}
		if len(cycles) == 0 {
			res.Months = month - 1
			return res, nil
		}

		surplus, err := cash.Monthly.Sub(required)
		if err != nil {
			return res, err
		}
		if surplus.Sign() < 0 {
			return res, fmt.Errorf("plan: month %d requires %s, budget is %s", month, required, cash.Monthly)
		}

		// Cascade the surplus down the priority order. A loan absorbs at most
		// what it would still owe after its required payment.
		extra := make(map[int]money.Amount, len(cycles))
		byIdx := make(map[int]cycle, len(cycles))
		for _, c := range cycles {
			byIdx[c.idx] = c
		}
		for _, idx := range pol.Order {
			c, ok := byIdx[idx]
			if !ok || surplus.Sign() <= 0 {
				continue
			}
			owed, err := live[idx].Balance.Add(c.interest)
			if err != nil {
				return res, err
			}
			room, err := owed.Sub(c.required)
			if err != nil {
				return res, err
			}
			if room.Sign() <= 0 {
				continue
			}
			take := surplus
			if take.Cmp(room) > 0 {
				take = room
			}
			extra[idx] = take
			if surplus, err = surplus.Sub(take); err != nil {
				return res, err
			}
		}

		for _, c := range cycles {
			l := &live[c.idx]
			e, hasExtra := extra[c.idx]
			interest := c.interest
			var earlyPart, earlySaves money.Amount
			earlyPart = money.Zero(cur)
			earlySaves = money.Zero(cur)

			if hasExtra && !c.payday.IsZero() {
				earlyPart = e
				if pol.Timing == SplitHalf {
					earlyPart = money.FromMinor(e.Minor()/2, cur)
				}
				if earlyPart.Sign() > 0 {
					res.TimingCredited = true
					// Interest for the cycle is what the full balance accrues
					// up to the payday plus what the reduced balance accrues
					// after it. The saving is the second term's complement.
					days := int64(date.DaysBetween(c.payday, c.due))
					ct := l.Contract
					earlySaves, err = money.Accrue(earlyPart, ct.NominalRate, days, ct.DayCount, ct.Rounding)
					if err != nil {
						return res, err
					}
					if interest, err = interest.Sub(earlySaves); err != nil {
						return res, err
					}
				}
			}

			pay := c.required
			if hasExtra {
				if pay, err = pay.Add(e); err != nil {
					return res, err
				}
			}
			owed, err := l.Balance.Add(interest)
			if err != nil {
				return res, err
			}
			if pay.Cmp(owed) > 0 {
				pay = owed
			}
			principal, err := pay.Sub(interest)
			if err != nil {
				return res, err
			}
			if l.Balance, err = l.Balance.Sub(principal); err != nil {
				return res, err
			}
			if res.TotalInterest, err = res.TotalInterest.Add(interest); err != nil {
				return res, err
			}
			l.From = c.due

			if month == 1 {
				res.Actions = appendActions(res.Actions, loans, c.idx, c.due, c.payday, c.required, e, earlyPart, earlySaves)
			}
			if l.Balance.Sign() <= 0 && res.ClearedFirst == "" {
				res.ClearedFirst = loans[c.idx].Name
				res.ClearedMonth = month
				res.MonthlyFreed = c.required
			}
		}

		if month == 1 {
			for i := range live {
				if live[i].Balance.Sign() > 0 {
					if res.NextMonthOwed, err = res.NextMonthOwed.Add(live[i].Balance); err != nil {
						return res, err
					}
				}
			}
			sort.SliceStable(res.Actions, func(i, j int) bool { return res.Actions[i].On.Before(res.Actions[j].On) })
		}
	}
	return res, fmt.Errorf("plan: still owing after %d months", maxMonths)
}

// appendActions records what the first month asks the borrower to do for one
// loan: the instalment on the due date, and the surplus on whichever day the
// policy paid it.
func appendActions(acts []Action, loans []Position, idx int, due, payday date.Date, required, extra, earlyPart, saves money.Amount) []Action {
	name, id := loans[idx].Name, loans[idx].ID
	acts = append(acts, Action{On: due, LoanID: id, Loan: name, Amount: required, Saves: money.Zero(required.Currency())})
	if extra.Sign() <= 0 {
		return acts
	}
	if earlyPart.Sign() > 0 {
		acts = append(acts, Action{On: payday, LoanID: id, Loan: name, Amount: earlyPart, Extra: true, Saves: saves})
	}
	if rest, err := extra.Sub(earlyPart); err == nil && rest.Sign() > 0 {
		acts = append(acts, Action{On: due, LoanID: id, Loan: name, Amount: rest, Extra: true, Saves: money.Zero(rest.Currency())})
	}
	return acts
}

// paydayIn returns the first occurrence of day strictly after from and strictly
// before due, or the zero date when the money arrives no earlier than the
// instalment does — in which case there is nothing to pay early.
func paydayIn(from, due date.Date, day int) date.Date {
	if day < 1 {
		return date.Date{}
	}
	for _, cand := range []date.Date{date.OnDayOfMonth(from, day), date.OnDayOfMonth(date.AddMonths(from, 1), day)} {
		if cand.After(from) && cand.Before(due) {
			return cand
		}
	}
	return date.Date{}
}

func anyReducesOnPayment(loans []Position) bool {
	for _, l := range loans {
		if l.Excess == allocation.ExcessReducePrincipal {
			return true
		}
	}
	return false
}

type order struct {
	name string
	idx  []int
	// also lists every name this order answers to. The avalanche and the
	// snowball coincide whenever the highest rate sits on the smallest
	// balance, and the report still needs both baselines.
	also []string
}

// namedOrders are the strategies people have names for. They are always in
// the candidate set, so the report can say how much the winner beats them by.
func namedOrders(loans []Position) []order {
	idx := func() []int {
		o := make([]int, len(loans))
		for i := range o {
			o[i] = i
		}
		return o
	}
	av := idx()
	sort.SliceStable(av, func(a, b int) bool {
		ra, rb := loans[av[a]].Contract.NominalRate, loans[av[b]].Contract.NominalRate
		if ra != rb {
			return ra > rb
		}
		return loans[av[a]].Balance.Cmp(loans[av[b]].Balance) < 0
	})
	sn := idx()
	sort.SliceStable(sn, func(a, b int) bool {
		c := loans[sn[a]].Balance.Cmp(loans[sn[b]].Balance)
		if c != 0 {
			return c < 0
		}
		return loans[sn[a]].Contract.NominalRate > loans[sn[b]].Contract.NominalRate
	})
	return []order{{name: nameAvalanche, idx: av}, {name: nameSnowball, idx: sn}}
}

// permutations lists every priority order of n loans, in lexicographic order.
func permutations(n int) []order {
	var out []order
	cur := make([]int, n)
	for i := range cur {
		cur[i] = i
	}
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

// dedupeOrders merges orders that coincide, keeping the first name for the
// ranking and every name for the baselines, so the same policy is not run
// twice and a tie still reads as "the avalanche".
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
