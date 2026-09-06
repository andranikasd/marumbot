package plan

// Ranking and the certificate: how the simulated candidates become one
// answer, and how far that answer may be trusted. Nothing here simulates;
// everything here reads Results the search already produced.

import (
	"fmt"
	"sort"

	"github.com/andranikasd/marumbot/pkg/core/allocation"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// Certificate says how the answer was found and how far to trust it. The
// borrower sees one sentence; the admin sees all of it.
type Certificate struct {
	Strength    Strength
	Eligibility string // the rule a proof relied on, when Strength is ProvenOptimal
	// Policies counts candidate simulation attempts, including infeasible
	// policies, across all rollovers explored so far (at most 4096 each).
	// FeasiblePolicies counts their successful results. Both exclude the
	// report's named baselines, minimum and budget ladder runs.
	Policies         int
	FeasiblePolicies int
	// DynamicStates and DynamicExpansions are only populated by SearchDynamic;
	// dynamic state/branch work is not reported as simulated static policies.
	DynamicStates     int
	DynamicExpansions int
	// Axis sizes describe the candidate set after vector/order fallback;
	// the attempt cap may stop before every axis entry is visited.
	Orders          int
	EffectVectors   int
	TimingVectors   int
	Truncation      string
	BestCost        money.Amount
	LowerBound      *money.Amount // nil unless an admissible bound was established
	Gap             *money.Amount
	Quantum         int64
	CandidateDates  []date.Date // distinct dates on which optional payments were considered
	EngineVersion   string
	Fingerprints    []string
	AssumedPayments map[string]int
	// Positions records each loan's identity and how far its balance can be
	// trusted, so a report can carry its own caveat.
	Positions []CertifiedPosition
}

// CertifiedPosition is one loan as the certificate records it.
type CertifiedPosition struct {
	ID    string
	Trust string
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
	Goal                 Goal
	Best                 Result
	Ranked               []Result
	Avalanche            Result // verified fee-free marginal-rate baseline, on due
	HighestRate          Result // nominal-rate baseline, including fee-bearing domains
	AvalancheUnsupported string // nonempty when Avalanche is unavailable
	Snowball             Result // smallest balance first, on due, budget kept
	Minimum              Result // only what the contracts require
	Ladder               []Rung
	Ties                 []string
	Certificate          Certificate
	// TimingSaving is the cost of the best policy paid on due dates minus
	// its cost as ranked; what the payday is worth.
	TimingSaving money.Amount
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
	rep.AvalancheUnsupported = avalancheDomain(in)

	base := rep.Best.Policy
	for _, o := range u.orders {
		for _, name := range o.also {
			pol := Policy{Name: name, Order: o.idx, Timing: uniform(len(in.Loans), OnDue), Effect: base.Effect, Rollover: RollFreed}
			switch name {
			case nameAvalanche, string(StrategyHighestRate):
				if rep.HighestRate, err = run(in, pol, u.cache); err != nil && !isInfeasible(err) {
					return Report{}, err
				}
				if name == nameAvalanche {
					rep.Avalanche = rep.HighestRate
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

	// The payday's worth is best-against-best: the winner among plans that
	// pay only on due dates, minus the overall winner. Re-timing just the
	// winning policy understates it -- a different order might be the best
	// way to live without the payday, and the ranked list already holds it.
	rep.TimingSaving = money.Zero(cur)
	if t, ok := uniformTiming(base.Timing); !ok || t != OnDue {
		for _, r := range ranked {
			if t, ok := uniformTiming(r.Policy.Timing); ok && t == OnDue {
				if rep.TimingSaving, err = r.Cost().Sub(rep.Best.Cost()); err != nil {
					return Report{}, err
				}
				break
			}
		}
		if rep.TimingSaving.Sign() < 0 {
			rep.TimingSaving = money.Zero(cur)
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

// certificate states what the search covered and what it may claim.
func (u *Universe) certificate(goal Goal, rep Report) Certificate {
	in := u.Input
	c := Certificate{
		Policies: u.attempted, FeasiblePolicies: len(u.Results), Orders: u.nOrders, EffectVectors: u.nEffects, TimingVectors: u.nTimings,
		Truncation: u.trunc, BestCost: rep.Best.Cost(), Quantum: money.DefaultPolicy(in.Cash.Monthly.Currency()).Unit,
		EngineVersion: EngineVersion, AssumedPayments: u.assumed,
	}
	for _, l := range in.Loans {
		c.Fingerprints = append(c.Fingerprints, fingerprint(l.Contract))
		c.Positions = append(c.Positions, CertifiedPosition{ID: l.ID, Trust: l.Trust})
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
	// Winner interest is not an admissible lower bound across other policies.
	// Leave both bound and gap unknown until a relaxation has been solved.
	if rule, ok := u.provable(goal, rep); ok {
		c.Strength, c.Eligibility = ProvenOptimal, rule
	}
	return c
}

// provable only claims a global least-cost proof when a feasible policy has
// reached the nonnegative zero-cost floor. Continuous exchange arguments do
// not establish optimality under per-event discrete rounding, even for one loan.
func (u *Universe) provable(goal Goal, rep Report) (string, bool) {
	if goal.Kind != LeastInterest || u.feeBearer || u.trunc != "" || u.Input.Cash.SeparateSpending() || rep.Best.Cost().Sign() != 0 {
		return "", false
	}
	for _, l := range u.Input.Loans {
		if l.Contract.NominalRate < 0 || l.OptionalExcluded {
			return "", false
		}
	}
	return "feasible zero interest and fees reaches the nonnegative cost floor; payoff/tie-break optimality is not claimed", true
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
			out = append(out, "the highest rate is also the smallest balance: highest rate and snowball are the same order")
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
