package plan

import (
	"fmt"
	"sort"

	"github.com/andranikasd/marumbot/pkg/core/allocation"
	"github.com/andranikasd/marumbot/pkg/core/amortisation"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// This file is the optimiser. Search answers "which way of paying produces
// the best result for this goal", by running every candidate policy through
// the same dated simulator and ranking what comes out.
//
// Three facts shape the design, all provable under simple daily accrual on a
// declining balance and stated here so the code is not mistaken for a
// heuristic:
//
//  1. With a fixed monthly budget that is always spent, the month every loan
//     is clear is almost independent of the order: money is fungible, and
//     total debt divided by the budget is the answer. Order changes interest,
//     not time. So "finish soonest" is not a different ranking of the same
//     runs; it is a different question — how much sooner for how much more —
//     which the ladder and the inverse solver answer.
//  2. Interest is linear in each payment and non-increasing in each payment's
//     date. Paying the surplus on the earliest day it exists weakly dominates
//     every split, so the intra-month dimension collapses to "on payday" or
//     "on the due date". The split is still simulated so its cost is visible.
//  3. "Pay less per month" is only meaningful if the budget is allowed to
//     fall: clearing a loan frees its instalment, and the relief goal keeps
//     that money rather than redeploying it. That is a different simulation,
//     not a different sort key, and it costs interest — which the report
//     shows.

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

// AfterClear is what happens to a cleared loan's instalment.
type AfterClear uint8

const (
	// KeepBudget redeploys the freed instalment to the next loan: the
	// monthly outflow stays the same until everything is clear.
	KeepBudget AfterClear = iota
	// DropBudget keeps the freed money: the monthly outflow falls each time
	// a loan is cleared. This is what "pay less per month" means.
	DropBudget
)

func (a AfterClear) String() string {
	if a == DropBudget {
		return "drop_budget"
	}
	return "keep_budget"
}

// Lump is a one-off sum available in a given month, on top of the budget: a
// bonus, a thirteenth salary, a sale.
type Lump struct {
	Month  int // 1-based month of the plan
	Amount money.Amount
}

// Cash is what the borrower has to spend and when it arrives.
type Cash struct {
	Monthly money.Amount
	// Day is the day of the month the money is available, 1..31. Zero means
	// unknown, which disables the early-payment variants: the engine will not
	// credit a payment on a day it cannot name.
	Day   int
	Lumps []Lump
}

// Policy is one way of spending the surplus.
type Policy struct {
	Name   string // avalanche, snowball, minimum, or "order" for a permutation
	Order  []int  // priority, by index into the positions the search was given
	Timing Timing
	// Effect overrides what a prepayment does to each loan's schedule. Zero
	// keeps each contract's own term; the search sets it when a contract
	// leaves the choice to the borrower.
	Effect     model.PrepaymentEffect
	AfterClear AfterClear
}

func (p Policy) String() string {
	s := fmt.Sprintf("%s/%s", p.Name, p.Timing)
	if p.Effect != model.PrepayBorrowerChooses {
		s += "/" + p.Effect.String()
	}
	if p.AfterClear == DropBudget {
		s += "/" + p.AfterClear.String()
	}
	return s
}

// Action is one payment the policy makes in the first month, dated.
type Action struct {
	On     date.Date
	LoanID string
	Loan   string
	Amount money.Amount
	Extra  bool // false for the contractual instalment
	// Saves is the interest this cycle that paying on this date rather than
	// the due date avoids. Zero for a payment on the due date, and zero when
	// the lender does not credit early payment.
	Saves money.Amount
}

// MonthState is one month of a run, for timelines and charts.
type MonthState struct {
	Month    int
	Required money.Amount // contractual instalments due this month
	Paid     money.Amount // what the policy actually paid
	Interest money.Amount // accrued this month across all loans
	Owed     money.Amount // total balance after this month's payments
	Cleared  string       // a loan that reached zero this month, by name
}

// Result is what one policy produces over the whole run.
type Result struct {
	Policy        Policy
	Months        int
	TotalInterest money.Amount
	TotalPaid     money.Amount
	NextMonthOwed money.Amount
	ClearedFirst  string
	ClearedMonth  int
	MonthlyFreed  money.Amount
	Actions       []Action     // first month only
	Timeline      []MonthState // every month
	// PeakMonthly and FinalMonthly bracket the outflow: under KeepBudget they
	// are equal; under DropBudget the second is what the borrower pays once
	// the relief has landed.
	PeakMonthly  money.Amount
	FinalMonthly money.Amount
	// ReliefMonth is the first month the outflow falls below the starting
	// budget, or zero if it never does.
	ReliefMonth int
	// TimingCredited is true when at least one early payment was actually
	// credited by a lender that reduces principal on the day of payment.
	TimingCredited bool
}

// Rung is one step of the budget ladder: what a different monthly amount
// would do under the best policy.
type Rung struct {
	Budget   money.Amount
	Months   int
	Interest money.Amount
}

// Report is the answer to "how should I pay".
type Report struct {
	Goal      Goal
	Best      Result
	Ranked    []Result // every distinct candidate, best first
	Avalanche Result   // highest rate first, paid on the due date, budget kept
	Snowball  Result   // smallest balance first, paid on the due date, budget kept
	// Minimum is paying only what the contracts require, from the anchor.
	// It is the floor every plan is measured against.
	Minimum Result
	// TimingSaving is the interest the best policy saves over the same
	// policy paid on the due date.
	TimingSaving money.Amount
	// Ladder shows what more money per month buys, under the best policy.
	// Empty for the relief goal, where the question is the opposite.
	Ladder []Rung
	// Ties explains why candidates coincide when they do, so a report that
	// shows the same number three times can say why rather than look broken.
	Ties []string
	// Exhaustive is true when every priority order was tried.
	Exhaustive bool
	Evaluated  int
}

// maxExhaustive bounds the permutation search. Five loans is 5! orders; with
// three timings and up to two prepayment effects that is 720 runs, a second
// or so with the step cache. Beyond it only the named strategies are tried,
// and the report says so.
const maxExhaustive = 5

// Search runs every candidate policy and ranks the outcomes for the goal.
func Search(loans []Position, cash Cash, goal Goal) (Report, error) {
	if len(loans) == 0 {
		return Report{}, fmt.Errorf("plan: no loans")
	}
	if cash.Day < 0 || cash.Day > 31 {
		return Report{}, fmt.Errorf("plan: cash day %d out of range", cash.Day)
	}
	cur := cash.Monthly.Currency()

	early := cash.Day > 0 && anyReducesOnPayment(loans)
	timings := []Timing{OnDue}
	if early {
		timings = append(timings, OnReceipt, SplitHalf)
	}
	effects := []model.PrepaymentEffect{model.PrepayBorrowerChooses}
	if anyBorrowerChooses(loans) {
		effects = []model.PrepaymentEffect{model.PrepayShortenTerm, model.PrepayReduceInstalment}
	}
	after := KeepBudget
	if goal == FreeUpMonthly {
		after = DropBudget
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
		for _, e := range effects {
			for _, t := range timings {
				pol := Policy{Name: o.name, Order: o.idx, Timing: t, Effect: e, AfterClear: after}
				r, err := run(loans, cash, pol, cache)
				if err != nil {
					return Report{}, err
				}
				rep.Ranked = append(rep.Ranked, r)
			}
		}
	}
	rep.Evaluated = len(rep.Ranked)
	sort.SliceStable(rep.Ranked, func(i, j int) bool { return better(goal, rep.Ranked[i], rep.Ranked[j]) })
	rep.Best = rep.Ranked[0]

	// The baselines are always the conventional strategies with the budget
	// kept, so a relief plan is measured against what it gives up.
	base := rep.Best.Policy.Effect
	var err error
	for _, o := range orders {
		for _, n := range o.also {
			switch n {
			case nameAvalanche:
				rep.Avalanche, err = run(loans, cash, Policy{Name: n, Order: o.idx, Effect: base}, cache)
			case nameSnowball:
				rep.Snowball, err = run(loans, cash, Policy{Name: n, Order: o.idx, Effect: base}, cache)
			}
			if err != nil {
				return Report{}, err
			}
		}
	}
	if rep.Minimum, err = minimum(loans, cash, base, cache); err != nil {
		return Report{}, err
	}

	rep.TimingSaving = money.Zero(cur)
	if rep.Best.Policy.Timing != OnDue {
		p := rep.Best.Policy
		p.Timing = OnDue
		due, err := run(loans, cash, p, cache)
		if err != nil {
			return Report{}, err
		}
		if rep.TimingSaving, err = due.TotalInterest.Sub(rep.Best.TotalInterest); err != nil {
			return Report{}, err
		}
	}
	if goal != FreeUpMonthly {
		if rep.Ladder, err = ladder(loans, cash, rep.Best.Policy, cache); err != nil {
			return Report{}, err
		}
	}
	rep.Ties = ties(loans, cash, rep)
	return rep, nil
}

// minimum pays only what the contracts require: no surplus, no early
// payment. It is what happens if the borrower ignores the advice.
func minimum(loans []Position, cash Cash, effect model.PrepaymentEffect, cache stepCache) (Result, error) {
	req, err := requiredNow(loans, cash.Monthly.Currency(), cache)
	if err != nil {
		return Result{}, err
	}
	pol := Policy{Name: "minimum", Order: identity(len(loans)), Effect: effect, AfterClear: DropBudget}
	return run(loans, Cash{Monthly: req, Day: cash.Day}, pol, cache)
}

// requiredNow totals the first instalment of every live loan.
func requiredNow(loans []Position, cur money.Currency, cache stepCache) (money.Amount, error) {
	total := money.Zero(cur)
	for i, l := range loans {
		if l.Balance.Sign() <= 0 {
			continue
		}
		st, err := cache.next(i, l, model.PrepayReduceInstalment, money.Amount{})
		if err != nil {
			return money.Amount{}, err
		}
		if total, err = total.Add(st.required); err != nil {
			return money.Amount{}, err
		}
	}
	return total, nil
}

// ladder runs the best policy at a few larger budgets. The rungs are what a
// borrower asking "what if I found another 20,000 a month" needs, and they
// make the fungibility of money visible: months fall, interest falls, and
// neither depends much on which loan the extra goes to.
func ladder(loans []Position, cash Cash, pol Policy, cache stepCache) ([]Rung, error) {
	cur := cash.Monthly.Currency()
	var out []Rung
	for _, pct := range []int64{100, 110, 125, 150, 200} {
		b := money.Quantise(money.FromMinor(cash.Monthly.Minor()*pct/100, cur), money.DefaultPolicy(cur))
		r, err := run(loans, Cash{Monthly: b, Day: cash.Day, Lumps: cash.Lumps}, pol, cache)
		if err != nil {
			return nil, err
		}
		out = append(out, Rung{Budget: b, Months: r.Months, Interest: r.TotalInterest})
	}
	return out, nil
}

// BudgetFor finds the smallest monthly budget that clears every loan within
// the given number of months under the policy, by bisection on the budget.
// It is the inverse of Run, and the answer to "I want this over in a year".
func BudgetFor(loans []Position, cash Cash, pol Policy, months int) (money.Amount, error) {
	if months < 1 {
		return money.Amount{}, fmt.Errorf("plan: months must be positive")
	}
	cache := stepCache{}
	cur := cash.Monthly.Currency()
	lo, err := requiredNow(loans, cur, cache)
	if err != nil {
		return money.Amount{}, err
	}
	// Upper bound: twice everything owed, paid in month one, clears anything.
	hi := money.Zero(cur)
	for _, l := range loans {
		if hi, err = hi.Add(l.Balance); err != nil {
			return money.Amount{}, err
		}
	}
	hi = money.FromMinor(hi.Minor()*2, cur)
	unit := money.DefaultPolicy(cur).Unit
	clears := func(b money.Amount) bool {
		r, err := run(loans, Cash{Monthly: b, Day: cash.Day, Lumps: cash.Lumps}, pol, cache)
		// A budget below the minimums fails the run: that is "does not clear".
		return err == nil && r.Months <= months
	}
	if !clears(hi) {
		return money.Amount{}, fmt.Errorf("plan: no budget clears the loans in %d months", months)
	}
	l, h := lo.Minor()/unit, hi.Minor()/unit+1
	for l < h {
		mid := l + (h-l)/2
		if clears(money.FromMinor(mid*unit, cur)) {
			h = mid
		} else {
			l = mid + 1
		}
	}
	return money.FromMinor(h*unit, cur), nil
}

// ties names the reasons candidates coincide.
func ties(loans []Position, cash Cash, rep Report) []string {
	var out []string
	live := 0
	for _, l := range loans {
		if l.Balance.Sign() > 0 {
			live++
		}
	}
	if live == 1 {
		out = append(out, "one loan: the order cannot matter")
	}
	if rep.Goal != FreeUpMonthly && rep.Best.TotalPaid.Cmp(rep.Minimum.TotalPaid) == 0 {
		out = append(out, "no surplus: every plan pays only what is required")
	}
	if live > 1 {
		os := namedOrders(loans)
		if fmt.Sprint(os[0].idx) == fmt.Sprint(os[1].idx) {
			out = append(out, "the highest rate is also the smallest balance: avalanche and snowball are the same order")
		}
	}
	switch {
	case !anyReducesOnPayment(loans):
		out = append(out, "no lender credits early payment: timing cannot matter")
	case cash.Day == 0:
		out = append(out, "no payday given: early payment was not simulated")
	}
	return out
}

// better orders two results for a goal. Ties fall through to the next
// criterion, and finally to the simplest policy, so the ranking is total.
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
		// Earliest relief, then the lowest outflow once it lands, then cost.
		am, bm := a.ReliefMonth, b.ReliefMonth
		if am == 0 {
			am = 1 << 30
		}
		if bm == 0 {
			bm = 1 << 30
		}
		if am != bm {
			return am < bm
		}
		if fc := a.FinalMonthly.Cmp(b.FinalMonthly); fc != 0 {
			return fc < 0
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
	if (a.Policy.Name == namePermuted) != (b.Policy.Name == namePermuted) {
		return a.Policy.Name != namePermuted
	}
	if a.Policy.Timing != b.Policy.Timing {
		return a.Policy.Timing < b.Policy.Timing
	}
	return a.Policy.Effect < b.Policy.Effect
}

// stepKey identifies one loan's state at the start of a cycle. Loans that are
// not receiving surplus follow the contractual path, which is the same under
// every policy, so a search memoises the projection by state and only the
// target loan's diverging states cost a fresh solve.
type stepKey struct {
	idx        int
	balance    int64
	from       date.Date
	instalment int64
}

type stepVal struct {
	due      date.Date
	interest money.Amount
	required money.Amount
	// instalment is the level payment the loan carries forward under
	// shorten_term.
	instalment money.Amount
}

type stepCache map[stepKey]stepVal

// next projects one loan's next instalment.
//
// Under reduce_instalment the schedule is rebuilt from the current balance
// to maturity every cycle, which is what a lender does when it re-issues the
// schedule after a prepayment. Under shorten_term the instalment fixed at the
// anchor is carried, interest accrues on the actual balance, and the loan
// ends when the balance does. Declining-principal loans are always rebuilt:
// their principal part is a term of the contract, not a solved figure.
func (m stepCache) next(idx int, p Position, effect model.PrepaymentEffect, carried money.Amount) (stepVal, error) {
	fixed := effect == model.PrepayShortenTerm && p.Contract.Type == model.Annuity && carried.Sign() > 0
	k := stepKey{idx: idx, balance: p.Balance.Minor(), from: p.From}
	if fixed {
		k.instalment = carried.Minor()
	}
	if v, ok := m[k]; ok {
		return v, nil
	}
	var v stepVal
	if fixed {
		var due date.Date
		if dates, err := amortisation.RemainingDates(p.Contract, p.From); err == nil {
			due = dates[0]
		} else {
			// Past the last contractual date with a balance left: settle on
			// the next monthly occurrence rather than pretend it vanished.
			due = date.Occurrence(p.From, p.Contract.PaymentDay, 1)
		}
		c := p.Contract
		interest, err := money.Accrue(p.Balance, c.NominalRate, int64(date.DaysBetween(p.From, due)), c.DayCount, c.Rounding)
		if err != nil {
			return v, fmt.Errorf("plan: projecting %s: %w", p.ID, err)
		}
		owed, err := p.Balance.Add(interest)
		if err != nil {
			return v, err
		}
		req := carried
		if req.Cmp(owed) > 0 {
			req = owed
		}
		v = stepVal{due: due, interest: interest, required: req, instalment: carried}
	} else {
		s, err := amortisation.Build(p.Contract, p.Balance, p.From)
		if err != nil || len(s.Rows) == 0 {
			return v, fmt.Errorf("plan: projecting %s: %w", p.ID, err)
		}
		r := s.Rows[0]
		v = stepVal{due: r.Due, interest: r.Interest, required: r.Payment, instalment: s.Instalment}
	}
	m[k] = v
	return v, nil
}

// Run follows one policy month by month until every loan is clear.
func Run(loans []Position, cash Cash, pol Policy) (Result, error) {
	return run(loans, cash, pol, stepCache{})
}

// run is Run with a shared cache.
//
// Each cycle the required instalment of every live loan is reserved first;
// the surplus then goes to the loans in the policy's order, each taking as
// much as it can absorb before the remainder moves to the next. Under
// KeepBudget a cleared loan's instalment joins the surplus; under DropBudget
// it leaves the plan and the outflow falls.
func run(loans []Position, cash Cash, pol Policy, cache stepCache) (Result, error) {
	if len(pol.Order) != len(loans) {
		return Result{}, fmt.Errorf("plan: policy covers %d of %d loans", len(pol.Order), len(loans))
	}
	cur := cash.Monthly.Currency()
	res := Result{
		Policy:        pol,
		TotalInterest: money.Zero(cur),
		TotalPaid:     money.Zero(cur),
		NextMonthOwed: money.Zero(cur),
		MonthlyFreed:  money.Zero(cur),
		PeakMonthly:   money.Zero(cur),
		FinalMonthly:  money.Zero(cur),
	}

	live := make([]Position, len(loans))
	copy(live, loans)
	carried := make([]money.Amount, len(loans)) // instalment under shorten_term
	effects := make([]model.PrepaymentEffect, len(loans))
	for i := range loans {
		effects[i] = loans[i].Contract.Prepayment.Effect
		if effects[i] == model.PrepayBorrowerChooses {
			effects[i] = pol.Effect
		}
		if effects[i] == model.PrepayBorrowerChooses {
			effects[i] = model.PrepayShortenTerm
		}
	}
	lumps := map[int]money.Amount{}
	for _, l := range cash.Lumps {
		lumps[l.Month] = l.Amount
	}
	budget := cash.Monthly
	var err error

	for month := 1; month <= maxMonths; month++ {
		type cycle struct {
			idx      int
			due      date.Date
			payday   date.Date
			interest money.Amount
			required money.Amount
		}
		var cycles []cycle
		required := money.Zero(cur)

		for i := range live {
			if live[i].Balance.Sign() <= 0 {
				continue
			}
			st, err := cache.next(i, live[i], effects[i], carried[i])
			if err != nil {
				return res, err
			}
			if carried[i].Sign() == 0 {
				carried[i] = st.instalment
			}
			c := cycle{idx: i, due: st.due, interest: st.interest, required: st.required}
			if pol.Timing != OnDue && live[i].Excess == allocation.ExcessReducePrincipal {
				c.payday = paydayIn(live[i].From, st.due, cash.Day)
			}
			cycles = append(cycles, c)
			if required, err = required.Add(st.required); err != nil {
				return res, err
			}
		}
		if len(cycles) == 0 {
			res.Months = month - 1
			return res, nil
		}

		available := budget
		if l, ok := lumps[month]; ok {
			if available, err = available.Add(l); err != nil {
				return res, err
			}
		}
		surplus, err := available.Sub(required)
		if err != nil {
			return res, err
		}
		if surplus.Sign() < 0 {
			return res, fmt.Errorf("plan: month %d requires %s, budget is %s", month, required, available)
		}

		// Cascade the surplus down the priority order.
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

		paid := money.Zero(cur)
		monthInterest := money.Zero(cur)
		var cleared string
		for _, c := range cycles {
			l := &live[c.idx]
			e, hasExtra := extra[c.idx]
			interest := c.interest
			earlyPart, earlySaves := money.Zero(cur), money.Zero(cur)

			if hasExtra && !c.payday.IsZero() {
				earlyPart = e
				if pol.Timing == SplitHalf {
					earlyPart = money.FromMinor(e.Minor()/2, cur)
				}
				if earlyPart.Sign() > 0 {
					res.TimingCredited = true
					days := int64(date.DaysBetween(c.payday, c.due))
					ct := l.Contract
					if earlySaves, err = money.Accrue(earlyPart, ct.NominalRate, days, ct.DayCount, ct.Rounding); err != nil {
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
			if paid, err = paid.Add(pay); err != nil {
				return res, err
			}
			if monthInterest, err = monthInterest.Add(interest); err != nil {
				return res, err
			}
			l.From = c.due

			if month == 1 {
				res.Actions = appendActions(res.Actions, loans, c.idx, c.due, c.payday, c.required, e, earlyPart, earlySaves)
			}
			if l.Balance.Sign() <= 0 {
				cleared = loans[c.idx].Name
				freed := carried[c.idx]
				if freed.Sign() == 0 {
					freed = c.required
				}
				if res.ClearedFirst == "" {
					res.ClearedFirst, res.ClearedMonth, res.MonthlyFreed = cleared, month, freed
				}
				if pol.AfterClear == DropBudget {
					if budget, err = budget.Sub(freed); err != nil {
						return res, err
					}
					if res.ReliefMonth == 0 {
						res.ReliefMonth = month + 1
					}
				}
			}
		}

		owedTotal := money.Zero(cur)
		for i := range live {
			if live[i].Balance.Sign() > 0 {
				if owedTotal, err = owedTotal.Add(live[i].Balance); err != nil {
					return res, err
				}
			}
		}
		if res.TotalPaid, err = res.TotalPaid.Add(paid); err != nil {
			return res, err
		}
		res.Timeline = append(res.Timeline, MonthState{
			Month: month, Required: required, Paid: paid, Interest: monthInterest, Owed: owedTotal, Cleared: cleared,
		})
		if paid.Cmp(res.PeakMonthly) > 0 {
			res.PeakMonthly = paid
		}
		res.FinalMonthly = paid
		if month == 1 {
			res.NextMonthOwed = owedTotal
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

func anyBorrowerChooses(loans []Position) bool {
	for _, l := range loans {
		if l.Contract.Prepayment.Effect == model.PrepayBorrowerChooses && l.Contract.Type == model.Annuity {
			return true
		}
	}
	return false
}

func identity(n int) []int {
	o := make([]int, n)
	for i := range o {
		o[i] = i
	}
	return o
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
	// also lists every name this order answers to. The avalanche and the
	// snowball coincide whenever the highest rate sits on the smallest
	// balance, and the report still needs both baselines.
	also []string
}

// namedOrders are the strategies people have names for.
func namedOrders(loans []Position) []order {
	av := identity(len(loans))
	sort.SliceStable(av, func(a, b int) bool {
		ra, rb := loans[av[a]].Contract.NominalRate, loans[av[b]].Contract.NominalRate
		if ra != rb {
			return ra > rb
		}
		return loans[av[a]].Balance.Cmp(loans[av[b]].Balance) < 0
	})
	sn := identity(len(loans))
	sort.SliceStable(sn, func(a, b int) bool {
		c := loans[sn[a]].Balance.Cmp(loans[sn[b]].Balance)
		if c != 0 {
			return c < 0
		}
		return loans[sn[a]].Contract.NominalRate > loans[sn[b]].Contract.NominalRate
	})
	return []order{{name: nameAvalanche, idx: av}, {name: nameSnowball, idx: sn}}
}

// permutations lists every priority order of n loans.
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

// dedupeOrders merges orders that coincide, keeping the first name for the
// ranking and every name for the baselines.
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
