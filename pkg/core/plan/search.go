package plan

import (
	"fmt"
	"sort"

	"github.com/andranikasd/marumbot/pkg/core/allocation"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// EngineVersion is printed in every certificate so a stored report can be
// traced to the arithmetic that produced it.
const EngineVersion = "plan/2"

// Strength is how much a result may claim.
type Strength string

const (
	// ProvenOptimal is the best policy under printed mathematical
	// assumptions, all of which the search checked before claiming it.
	ProvenOptimal Strength = "proven_optimal"
	// ExhaustiveStaticOrder is the best of every static priority order, per
	// loan timing and per loan effect. Dynamic switching between loans from
	// month to month was not explored.
	ExhaustiveStaticOrder Strength = "exhaustive_static_order"
	// BoundedHeuristic is the best found in a capped candidate set; the
	// certificate carries the truncation reason.
	BoundedHeuristic Strength = "bounded_heuristic"
	// NamedStrategiesOnly compares the named strategies; no optimality
	// claim is made.
	NamedStrategiesOnly Strength = "named_strategies_only"
)

// Certificate says how the answer was found and how far to trust it. The
// borrower sees one sentence; the admin sees all of it.
type Certificate struct {
	Strength        Strength
	Eligibility     string // the rule a proof relied on, when Strength is ProvenOptimal
	Policies        int    // simulated
	Orders          int
	EffectVectors   int
	TimingVectors   int
	Truncation      string
	BestCost        money.Amount
	LowerBound      *money.Amount // interest with no fees, when fees exist
	Gap             *money.Amount
	Quantum         int64
	CandidateDates  []date.Date // distinct dates on which optional payments were considered
	EngineVersion   string
	Fingerprints    []string
	AssumedPayments map[string]int
}

// Rung is one step of the budget ladder.
type Rung struct {
	Budget   money.Amount
	Months   int
	Payoff   date.Date
	Interest money.Amount
}

// Report is the answer to "how should I pay".
type Report struct {
	Goal        Goal
	Best        Result
	Ranked      []Result
	Avalanche   Result // highest rate first, on due, budget kept
	Snowball    Result // smallest balance first, on due, budget kept
	Minimum     Result // only what the contracts require
	Ladder      []Rung
	Ties        []string
	Certificate Certificate
	// TimingSaving is the cost of the best policy paid on due dates minus
	// its cost as ranked; what the payday is worth.
	TimingSaving money.Amount
}

// Caps on the candidate set. Orders are exhaustive up to five loans (120);
// per-loan vectors up to four free loans each (16). Beyond those the search
// labels itself bounded and says which axis was capped.
const (
	maxExhaustiveOrders = 5
	maxVectorLoans      = 4
	maxPolicies         = 4096
)

// Universe is every policy simulated once, so several goals can be ranked
// over the same runs with the same feasibility assumptions.
type Universe struct {
	Input     Input
	Results   []Result
	orders    []order
	effects   [][]model.PrepaymentEffect
	timings   [][]Timing
	batches   []money.Amount
	explored  map[Rollover]bool
	nOrders   int
	nEffects  int
	nTimings  int
	trunc     string
	assumed   map[string]int
	cache     cache
	feeBearer bool
}

// explore simulates every policy for one rollover. Freed-cash behaviour is
// a policy dimension the goals select on, so it is explored on demand: the
// least-interest family never needs the kept-cash runs and vice versa.
func (u *Universe) explore(r Rollover) error {
	if u.explored == nil {
		u.explored = map[Rollover]bool{}
	}
	if u.explored[r] {
		return nil
	}
	u.explored[r] = true
	for _, o := range u.orders {
		for _, e := range u.effects {
			for _, t := range u.timings {
				for _, b := range u.batches {
					pol := Policy{Name: o.name, Order: o.idx, Timing: t, Effect: e, Rollover: r, MinPrepay: b}
					res, err := run(u.Input, pol, u.cache)
					if err != nil {
						if isInfeasible(err) {
							continue // this policy cannot be followed; others may
						}
						return err
					}
					res.Assumed = u.assumed
					u.Results = append(u.Results, res)
				}
			}
		}
	}
	return nil
}

// Explore simulates the whole candidate set for an input.
func Explore(in Input) (*Universe, error) {
	norm, assumed, err := Normalize(in)
	if err != nil {
		return nil, err
	}
	u := &Universe{Input: norm, assumed: assumed, cache: cache{}}
	n := len(norm.Loans)
	for _, l := range norm.Loans {
		if len(l.Contract.Prepayment.Charges) > 0 || l.Contract.Prepayment.FeeBP > 0 {
			u.feeBearer = true
		}
	}

	u.orders = namedOrders(norm.Loans)
	if n <= maxExhaustiveOrders {
		u.orders = append(u.orders, permutations(n)...)
	} else {
		u.trunc = fmt.Sprintf("orders: %d loans exceed the exhaustive limit of %d", n, maxExhaustiveOrders)
	}
	u.orders = dedupeOrders(u.orders)
	u.nOrders = len(u.orders)

	effects := effectVectors(norm.Loans)
	timings := timingVectors(norm, norm.Loans)
	u.nEffects, u.nTimings = len(effects), len(timings)
	if len(effects) == 0 || len(timings) == 0 {
		return nil, fmt.Errorf("plan: empty candidate axis")
	}
	u.effects, u.timings = effects, timings
	u.batches = []money.Amount{{}}
	if u.feeBearer {
		u.batches = append(u.batches, batchThresholds(norm)...)
	}

	total := u.nOrders * len(effects) * len(timings) * len(u.batches)
	if total > maxPolicies {
		// Drop the permutations first: named orders with full vectors say
		// more than every order with one vector.
		u.orders = namedOrders(norm.Loans)
		u.orders = dedupeOrders(u.orders)
		u.nOrders = len(u.orders)
		u.trunc = fmt.Sprintf("policies: %d candidates exceed the cap of %d; permutations dropped", total, maxPolicies)
	}

	if err := u.explore(RollFreed); err != nil {
		return nil, err
	}
	if len(u.Results) == 0 {
		// Every policy failed feasibility; report the minimum's failure,
		// which is the earliest date the budget cannot be met.
		_, err := minimum(norm, u.cache)
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("plan: no feasible policy")
	}
	return u, nil
}

func asInfeasible(err error, target **InfeasibleError) bool {
	e, ok := err.(*InfeasibleError) //nolint:errorlint // run returns it unwrapped
	if ok {
		*target = e
	}
	return ok
}

// Rank orders a universe for a goal and assembles the report.
func (u *Universe) Rank(goal Goal) (Report, error) {
	if err := goal.Validate(); err != nil {
		return Report{}, err
	}
	in := u.Input
	cur := in.Cash.Monthly.Currency()
	baseline, err := requiredNow(in, u.cache)
	if err != nil {
		return Report{}, err
	}
	// Relief goals are only meaningful when freed money is kept; the other
	// goals only when it is redeployed. Filter rather than mix, so a
	// comparator never has to guess what a policy meant.
	want := RollFreed
	if goal.Kind == Relief {
		want = KeepFreed
	}
	if err := u.explore(want); err != nil {
		return Report{}, err
	}
	var ranked []Result
	for _, r := range u.Results {
		if r.Policy.Rollover == want {
			ranked = append(ranked, r)
		}
	}
	if len(ranked) == 0 {
		return Report{}, fmt.Errorf("plan: no feasible policy for %s", goal)
	}
	sort.SliceStable(ranked, func(i, j int) bool { return better(goal, baseline, ranked[i], ranked[j]) })
	rep := Report{Goal: goal, Best: ranked[0], Ranked: ranked, Ties: nil}

	base := rep.Best.Policy
	for _, o := range u.orders {
		for _, name := range o.also {
			pol := Policy{Name: name, Order: o.idx, Timing: uniform(len(in.Loans), OnDue), Effect: base.Effect, Rollover: RollFreed}
			switch name {
			case nameAvalanche:
				if rep.Avalanche, err = run(in, pol, u.cache); err != nil && !isInfeasible(err) {
					return Report{}, err
				}
			case nameSnowball:
				if rep.Snowball, err = run(in, pol, u.cache); err != nil && !isInfeasible(err) {
					return Report{}, err
				}
			}
		}
	}
	if rep.Minimum, err = minimum(in, u.cache); err != nil && !isInfeasible(err) {
		return Report{}, err
	}

	rep.TimingSaving = money.Zero(cur)
	if t, ok := uniformTiming(base.Timing); !ok || t != OnDue {
		p := base
		p.Timing = uniform(len(in.Loans), OnDue)
		if due, err := run(in, p, u.cache); err == nil {
			if rep.TimingSaving, err = due.Cost().Sub(rep.Best.Cost()); err != nil {
				return Report{}, err
			}
		}
	}
	if goal.Kind != Relief {
		if rep.Ladder, err = ladder(in, base, u.cache); err != nil {
			return Report{}, err
		}
	}
	rep.Ties = ties(in, rep)
	rep.Certificate = u.certificate(goal, rep)
	return rep, nil
}

func isInfeasible(err error) bool {
	var inf *InfeasibleError
	return asInfeasible(err, &inf)
}

// Search explores and ranks in one call.
func Search(in Input, goal Goal) (Report, error) {
	u, err := Explore(in)
	if err != nil {
		return Report{}, err
	}
	return u.Rank(goal)
}

// certificate states what the search covered and what it may claim.
func (u *Universe) certificate(goal Goal, rep Report) Certificate {
	in := u.Input
	c := Certificate{
		Policies: len(u.Results), Orders: u.nOrders, EffectVectors: u.nEffects, TimingVectors: u.nTimings,
		Truncation: u.trunc, BestCost: rep.Best.Cost(), Quantum: money.DefaultPolicy(in.Cash.Monthly.Currency()).Unit,
		EngineVersion: EngineVersion, AssumedPayments: u.assumed,
	}
	for _, l := range in.Loans {
		c.Fingerprints = append(c.Fingerprints, fingerprint(l.Contract))
	}
	c.CandidateDates = candidateDates(rep.Ranked)

	switch {
	case u.trunc != "":
		c.Strength = BoundedHeuristic
		if len(in.Loans) > maxExhaustiveOrders && u.nEffects == 1 && u.nTimings == 1 {
			c.Strength = NamedStrategiesOnly
		}
	case u.feeBearer:
		c.Strength = BoundedHeuristic
		c.Truncation = "fees: batching thresholds are sampled at contract breakpoints, not solved"
	default:
		c.Strength = ExhaustiveStaticOrder
	}
	if u.feeBearer {
		lb := rep.Best.TotalInterest
		c.LowerBound = &lb
		if gap, err := rep.Best.Cost().Sub(lb); err == nil {
			c.Gap = &gap
		}
	}
	if rule, ok := u.provable(goal, rep); ok {
		c.Strength, c.Eligibility = ProvenOptimal, rule
	}
	return c
}

// provable checks the exchange-argument preconditions under which the
// avalanche paid on receipt is optimal for least interest, and only claims
// a proof when the ranked winner is that policy.
func (u *Universe) provable(goal Goal, rep Report) (string, bool) {
	in := u.Input
	if goal.Kind != LeastInterest || u.feeBearer || u.trunc != "" {
		return "", false
	}
	live := 0
	for _, l := range in.Loans {
		if l.Balance.Sign() > 0 {
			live++
		}
		if l.Contract.Prepayment.MinAmount.Sign() > 0 || l.Contract.NominalRate < 0 {
			return "", false
		}
	}
	if live == 1 {
		return "one loan: every order is the same order; earliest payment dominates under daily accrual on a declining balance", true
	}
	best := rep.Best.Policy
	if best.Name != nameAvalanche {
		return "", false
	}
	for i, l := range in.Loans {
		if l.Excess == allocation.ExcessReducePrincipal && best.Timing[i] != OnReceipt && in.Cash.PayDay > 0 {
			return "", false
		}
	}
	return "fixed non-negative rates, one day-count basis, no fees or thresholds, required instalments always met, " +
		"surplus credited to principal on payment: by exchange, money moved from a lower to a higher daily rate " +
		"cannot increase interest, and paying earlier cannot increase it", true
}

// better is the written comparator for each goal.
func better(goal Goal, baseline money.Amount, a, b Result) bool {
	cc := a.Cost().Cmp(b.Cost())
	pd := a.PayoffDate.Compare(b.PayoffDate)
	switch goal.Kind {
	case Fastest:
		if pd != 0 {
			return pd < 0
		}
		if cc != 0 {
			return cc < 0
		}
		if pr := a.PeakRequired.Cmp(b.PeakRequired); pr != 0 {
			return pr < 0
		}
	case Relief:
		ra, rb := reliefMonth(goal, baseline, a), reliefMonth(goal, baseline, b)
		if ra != rb {
			return ra < rb
		}
		if cc != 0 {
			return cc < 0
		}
	case FirstWin:
		fa, fb := a.FirstClearOn, b.FirstClearOn
		if fa.IsZero() != fb.IsZero() {
			return !fa.IsZero()
		}
		if c := fa.Compare(fb); c != 0 {
			return c < 0
		}
		if cc != 0 {
			return cc < 0
		}
	default:
		if cc != 0 {
			return cc < 0
		}
		if pd != 0 {
			return pd < 0
		}
		if a.Prepayments != b.Prepayments {
			return a.Prepayments < b.Prepayments
		}
	}
	// Total order: a named strategy before a bare permutation, then the
	// canonical policy identifier.
	if (a.Policy.Name == namePermuted) != (b.Policy.Name == namePermuted) {
		return a.Policy.Name != namePermuted
	}
	return a.Policy.ID() < b.Policy.ID()
}

// ReliefMonth is the first cycle from which the contractual required total
// stays at or under the goal's target for the rest of the run; a very large
// number when it never does. It reads required amounts only: voluntary
// extras and the small final instalment do not count as relief.
func ReliefMonth(goal Goal, baseline money.Amount, r Result) int {
	return reliefMonth(goal, baseline, r)
}

func reliefMonth(goal Goal, baseline money.Amount, r Result) int {
	const never = 1 << 30
	target := goal.Cap
	if goal.Free.Sign() > 0 {
		t, err := baseline.Sub(goal.Free)
		if err != nil {
			return never
		}
		target = t
	}
	if target.Sign() < 0 {
		return never
	}
	// Walk from the end: the relief month is the first index of the final
	// run of cycles whose required total is within the target.
	n := len(r.Timeline)
	if n == 0 {
		return never
	}
	i := n
	for i > 0 && r.Timeline[i-1].Required.Cmp(target) <= 0 {
		i--
	}
	if i == n {
		return never
	}
	return r.Timeline[i].Month
}

// minimum pays only what the contracts require, on their dates, with no
// optional payment: the budget each cycle is exactly that cycle's
// instalments, so a changing required total is followed rather than fixed
// at today's figure.
func minimum(in Input, c cache) (Result, error) {
	n := len(in.Loans)
	pol := Policy{Name: "minimum", Order: identity(n), Timing: uniform(n, OnDue), Effect: uniform(n, model.PrepayReduceInstalment), Rollover: KeepFreed}
	req, err := requiredNow(in, c)
	if err != nil {
		return Result{}, err
	}
	// Under KeepFreed the budget falls as loans close; between closures the
	// required total of an annuity is level, so the first cycle's figure
	// carries. Declining-principal loans require less each month; the
	// surplus that creates is not spent because MinPrepay is set above any
	// balance.
	inMin := in
	inMin.Cash = CashPlan{Monthly: req, PayDay: in.Cash.PayDay, OpeningCash: in.Cash.OpeningCash}
	pol.MinPrepay = money.FromMinor(1<<62, req.Currency())
	return run(inMin, pol, c)
}

// requiredNow totals the next instalment of every live loan.
func requiredNow(in Input, c cache) (money.Amount, error) {
	total := money.Zero(in.Cash.Monthly.Currency())
	for _, l := range in.Loans {
		if l.Balance.Sign() <= 0 {
			continue
		}
		ls := &loanState{pos: l, fp: fingerprint(l.Contract), effect: model.PrepayReduceInstalment, balance: l.Balance, from: l.From}
		o, err := c.next(ls)
		if err != nil {
			return money.Amount{}, err
		}
		if total, err = total.Add(o.required); err != nil {
			return money.Amount{}, err
		}
	}
	return total, nil
}

// ladder runs the best policy at a few larger budgets.
func ladder(in Input, pol Policy, c cache) ([]Rung, error) {
	cur := in.Cash.Monthly.Currency()
	var out []Rung
	for _, pct := range []int64{100, 110, 125, 150, 200} {
		b := money.Quantise(money.FromMinor(in.Cash.Monthly.Minor()*pct/100, cur), money.DefaultPolicy(cur))
		more := in
		more.Cash.Monthly = b
		r, err := run(more, pol, c)
		if err != nil {
			if isInfeasible(err) {
				continue
			}
			return nil, err
		}
		out = append(out, Rung{Budget: b, Months: r.Months, Payoff: r.PayoffDate, Interest: r.TotalInterest})
	}
	return out, nil
}

// BudgetFor finds the smallest monthly budget, in settlement units, that
// pays everything off by `by` under the policy. Bisection is valid because
// feasibility is monotone in the budget when unused cash may be carried.
func BudgetFor(in Input, pol Policy, by date.Date) (money.Amount, error) {
	norm, _, err := Normalize(in)
	if err != nil {
		return money.Amount{}, err
	}
	c := cache{}
	cur := norm.Cash.Monthly.Currency()
	lo, err := requiredNow(norm, c)
	if err != nil {
		return money.Amount{}, err
	}
	hi := money.Zero(cur)
	for _, l := range norm.Loans {
		if hi, err = hi.Add(l.Balance); err != nil {
			return money.Amount{}, err
		}
	}
	hi = money.FromMinor(hi.Minor()*2, cur)
	unit := money.DefaultPolicy(cur).Unit
	clears := func(b money.Amount) bool {
		trial := norm
		trial.Cash.Monthly = b
		r, err := run(trial, pol, c)
		return err == nil && !r.PayoffDate.After(by)
	}
	if !clears(hi) {
		return money.Amount{}, fmt.Errorf("plan: no budget clears the loans by %s", by)
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
func ties(in Input, rep Report) []string {
	var out []string
	live := 0
	credits := false
	for _, l := range in.Loans {
		if l.Balance.Sign() > 0 {
			live++
		}
		if l.Excess == allocation.ExcessReducePrincipal {
			credits = true
		}
	}
	if live == 1 {
		out = append(out, "one loan: the order cannot matter")
	}
	if rep.Goal.Kind != Relief && rep.Minimum.PayoffDate.Equal(rep.Best.PayoffDate) && rep.Best.Cost().Cmp(rep.Minimum.Cost()) == 0 {
		out = append(out, "no surplus: every plan pays only what is required")
	}
	if live > 1 {
		os := namedOrders(in.Loans)
		if fmt.Sprint(os[0].idx) == fmt.Sprint(os[1].idx) {
			out = append(out, "the highest rate is also the smallest balance: avalanche and snowball are the same order")
		}
	}
	switch {
	case !credits:
		out = append(out, "no lender credits early payment: timing cannot matter")
	case in.Cash.PayDay == 0:
		out = append(out, "no payday given: early payment was not simulated")
	}
	return out
}

// candidateDates lists the distinct dates optional payments were made on
// across the ranked results' first cycles.
func candidateDates(rs []Result) []date.Date {
	seen := map[date.Date]bool{}
	var out []date.Date
	for _, r := range rs {
		for _, a := range r.Actions {
			if a.Kind == Extra && !seen[a.On] {
				seen[a.On] = true
				out = append(out, a.On)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return out
}

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
