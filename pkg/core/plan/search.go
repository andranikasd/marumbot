package plan

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/andranikasd/marumbot/pkg/core/allocation"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// EngineVersion is printed in every certificate so a stored report can be
// traced to the arithmetic that produced it.
const EngineVersion = "plan/5"

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
	cache     *cache
	feeBearer bool
	attempted int
	runs      map[string]cachedPolicyRun
}

func (u *Universe) truncate(reason string) {
	if strings.Contains(u.trunc, reason) {
		return
	}
	if u.trunc != "" {
		u.trunc += "; "
	}
	u.trunc += reason
}

// explore attempts at most maxPolicies candidates for one rollover.
// Freed-cash behaviour is a policy dimension explored on demand: the
// least-interest family never needs the kept-cash runs and vice versa.
//
// The candidate set is enumerated first and simulated second. Enumeration is
// the part that has to be deterministic — which candidates are attempted, in
// which order, and where the cap falls — and separating it means the
// simulations, which are pure and independent, can run on every core while
// the results are still merged in candidate order.
func (u *Universe) explore(r Rollover) error {
	if u.explored == nil {
		u.explored = map[Rollover]bool{}
	}
	if u.explored[r] {
		return nil
	}
	u.explored[r] = true

	policies, capped := u.candidates(r)
	if capped {
		u.truncate(fmt.Sprintf("policies: attempted simulation cap of %d per rollover; candidate prefix only", maxPolicies))
	}
	u.attempted += len(policies)

	results, err := u.simulateAll(policies)
	if err != nil {
		return err
	}
	for _, res := range results {
		res.Assumed = u.assumed
		u.Results = append(u.Results, res)
	}
	return nil
}

// candidates enumerates the deterministic prefix of the candidate set for one
// rollover, stopping at the attempt cap. The second return says whether the
// cap truncated it. Infeasible policies are counted here, not discovered here:
// the cap charges attempts, not successes.
func (u *Universe) candidates(r Rollover) ([]Policy, bool) {
	out := make([]Policy, 0, len(u.orders)*len(u.effects)*len(u.timings)*len(u.batches))
	for _, o := range u.orders {
		for _, e := range u.effects {
			for _, t := range u.timings {
				for _, b := range u.batches {
					// Bound actual attempts even when dropping permutations was
					// insufficient. Keep the deterministic order/effect/timing/batch
					// prefix, and charge infeasible runs against the same cap.
					if len(out) == maxPolicies {
						return out, true
					}
					out = append(out, Policy{Name: o.name, Order: o.idx, Timing: t, Effect: e, Rollover: r, MinPrepay: b})
				}
			}
		}
	}
	return out, false
}

// simulateAll runs every candidate and returns the feasible results in
// candidate order.
//
// A policy that cannot be followed is dropped -- others may still work -- but
// any other failure is arithmetic, and the first one in candidate order is
// returned so a fault does not depend on which worker reached it first.
func (u *Universe) simulateAll(policies []Policy) ([]Result, error) {
	if u.runs != nil {
		// Compare memoises identical canonical policies across goals, which is
		// a sequential dependency. That path simulates far fewer candidates,
		// so it keeps the memo rather than the cores.
		out := make([]Result, 0, len(policies))
		for _, pol := range policies {
			res, err := u.simulate(pol)
			if err != nil {
				if isInfeasible(err) {
					continue
				}
				return nil, err
			}
			out = append(out, res)
		}
		return out, nil
	}

	workers := runtime.GOMAXPROCS(0)
	if workers > len(policies) {
		workers = len(policies)
	}
	if workers < 2 {
		return u.simulateSerially(policies)
	}

	type slot struct {
		res Result
		ok  bool
		err error
	}
	slots := make([]slot, len(policies))
	var next atomic.Int64
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// One memo per worker. The obligation and calendar caches are
			// plain maps, and sharing them would need a lock on the hottest
			// path in the engine; a worker simulates hundreds of policies
			// over the same loans, so it warms its own within a few of them.
			c := newCache()
			for {
				i := int(next.Add(1)) - 1
				if i >= len(policies) {
					return
				}
				res, err := run(u.Input, policies[i], c)
				if err != nil {
					slots[i].err = err
					continue
				}
				slots[i].res, slots[i].ok = res, true
			}
		}()
	}
	wg.Wait()

	out := make([]Result, 0, len(policies))
	for i := range slots {
		if slots[i].err != nil {
			if isInfeasible(slots[i].err) {
				continue // this policy cannot be followed; others may
			}
			return nil, slots[i].err
		}
		if slots[i].ok {
			out = append(out, slots[i].res)
		}
	}
	return out, nil
}

func (u *Universe) simulateSerially(policies []Policy) ([]Result, error) {
	out := make([]Result, 0, len(policies))
	c := newCache()
	for _, pol := range policies {
		res, err := run(u.Input, pol, c)
		if err != nil {
			if isInfeasible(err) {
				continue
			}
			return nil, err
		}
		out = append(out, res)
	}
	return out, nil
}

// Explore simulates the bounded candidate set for an input with RollFreed.
func Explore(in Input) (*Universe, error) {
	norm, assumed, err := Normalize(in)
	if err != nil {
		return nil, err
	}
	return exploreNormalized(norm, assumed, false)
}

func exploreNormalized(norm Input, assumed map[string]int, memoize bool) (*Universe, error) {
	u := &Universe{Input: norm, assumed: assumed, cache: newCache()}
	if memoize {
		u.runs = map[string]cachedPolicyRun{}
	}
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
	freeEffects, freeTimings := 0, 0
	for _, l := range norm.Loans {
		if l.Balance.Sign() <= 0 {
			continue
		}
		if l.Contract.Prepayment.Effect == model.PrepayBorrowerChooses && l.Contract.Type == model.Annuity {
			freeEffects++
		}
		if norm.Cash.PayDay > 0 && l.Excess == allocation.ExcessReducePrincipal {
			freeTimings++
		}
	}
	if freeEffects > maxVectorLoans {
		u.truncate(fmt.Sprintf("effects: %d free loans exceed the vector limit of %d; uniform vectors only", freeEffects, maxVectorLoans))
	}
	if freeTimings > maxVectorLoans {
		u.truncate(fmt.Sprintf("timings: %d free loans exceed the vector limit of %d; uniform vectors only", freeTimings, maxVectorLoans))
	}
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
		u.truncate(fmt.Sprintf("policies: %d candidates exceed the cap of %d; permutations dropped", total, maxPolicies))
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
