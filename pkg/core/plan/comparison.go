package plan

import (
	"crypto/sha256"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// ComparisonRequest is the MA-09 engine operation. Empty StrategyIDs uses the
// six supported named baselines. OptimizedGoals adds the existing bounded
// static universe, sharing simulations with baselines. CustomOrder uses loan IDs.
type ComparisonRequest struct {
	Input          Input
	StrategyIDs    []StrategyID
	CustomOrder    []string
	OptimizedGoals []Goal
}

// ComparisonResult preserves each requested strategy, including typed refusals.
// Aliased methods share CanonicalID and a single simulation, not a zero result.
type ComparisonResult struct {
	StrategyID  StrategyID
	CanonicalID string
	Rollover    Rollover
	Result      *Result
	Refusal     error
}

// ResultDelta is A minus B, only for feasible results with the same rollover.
// Negative cost/months means A costs less/finishes sooner. Indices address Results.
type ResultDelta struct {
	A, B                               int
	Cost, Interest, Fees, PeakRequired money.Amount
	Months                             int
}

// GoalComparison ranks the shared universe under one compatible goal.
type GoalComparison struct {
	Goal        Goal
	Ranked      []Result
	Certificate Certificate
}

// ComparisonReport carries reproducible normalized-input identity and explicit
// missing methods. Simulations counts unique policy attempts, including failures.
// Hashes include every input field (including amounts and spending permissions).
type ComparisonReport struct {
	SharedInputHash string
	Results         []ComparisonResult
	PairwiseDeltas  []ResultDelta
	Optimized       []GoalComparison
	Simulations     int
	AssumedPayments map[string]int
}

type cachedPolicyRun struct {
	result Result
	err    error
}

func canonicalPolicy(in Input, p Policy) Policy {
	p.Effect = append([]model.PrepaymentEffect(nil), p.Effect...)
	for i, l := range in.Loans {
		if l.Contract.Prepayment.Effect != model.PrepayBorrowerChooses {
			p.Effect[i] = l.Contract.Prepayment.Effect
		}
		if p.Effect[i] == model.PrepayBorrowerChooses {
			p.Effect[i] = model.PrepayShortenTerm
		}
	}
	return p
}
func policyKey(p Policy) string { p.Name = ""; return p.ID() }
func (u *Universe) simulate(p Policy) (Result, error) {
	if u.runs == nil {
		return run(u.Input, p, u.cache)
	}
	key := policyKey(canonicalPolicy(u.Input, p))
	saved, ok := u.runs[key]
	if !ok {
		saved.result, saved.err = run(u.Input, p, u.cache)
		u.runs[key] = saved
	}
	result := saved.result
	result.Policy = p
	result.Assumed = u.assumed
	return result, saved.err
}

// Compare normalizes exactly once, simulates each canonical policy once, and
// computes pairwise deltas only within a common released-payment convention.
// Strategy refusals are per-row; invalid inputs/goals and arithmetic faults fail
// the whole operation. This API performs no persistence and no budget ladder.
func Compare(req ComparisonRequest) (ComparisonReport, error) {
	var out ComparisonReport
	for _, g := range req.OptimizedGoals {
		if g.Kind > FirstWin {
			return out, &UnsupportedError{Feature: "unknown comparison goal"}
		}
		if err := g.Validate(); err != nil {
			return out, err
		}
		if g.Kind == Relief && g.Cap.Sign() > 0 && g.Free.Sign() > 0 {
			return out, &UnsupportedError{Feature: "relief needs exactly one target"}
		}
		if g.Kind == Relief {
			for _, a := range []money.Amount{g.Cap, g.Free} {
				if a.Sign() > 0 && a.Currency() != req.Input.Cash.Monthly.Currency() {
					return out, &MixedCurrencyError{Have: a.Currency().Code, Want: req.Input.Cash.Monthly.Currency().Code}
				}
			}
		}
	}
	norm, assumed, err := Normalize(req.Input)
	if err != nil {
		return out, err
	}
	if err = validateStrategyIDs(norm); err != nil {
		return out, err
	}
	out.SharedInputHash = inputHash(norm)
	out.AssumedPayments = assumed
	u := &Universe{Input: norm, assumed: assumed, cache: cache{}, runs: map[string]cachedPolicyRun{}}
	if len(req.OptimizedGoals) > 0 {
		u, err = exploreNormalized(norm, assumed, true)
		if err != nil {
			return out, err
		}
	}
	ids := req.StrategyIDs
	if len(ids) == 0 {
		ids = []StrategyID{StrategyHighestRate, StrategySnowball, StrategyHybrid, StrategyHighestRequired, StrategyHighestInterest, StrategyCashflowIndex}
	}
	rollovers := []Rollover{RollFreed}
	for _, g := range req.OptimizedGoals {
		if g.Kind == Relief {
			rollovers = append(rollovers, KeepFreed)
			break
		}
	}
	for _, roll := range rollovers {
		seen := map[StrategyID]bool{}
		for _, id := range ids {
			if seen[id] {
				continue
			}
			seen[id] = true
			row := ComparisonResult{StrategyID: id, Rollover: roll}
			p, e := strategyPolicy(norm, id, req.CustomOrder, roll)
			if e != nil {
				row.Refusal = strategyError(id, e)
			} else {
				row.CanonicalID = policyKey(canonicalPolicy(norm, p))
				r, e := u.simulate(p)
				if e != nil {
					if !isInfeasible(e) {
						return out, e
					}
					row.Refusal = e
				} else {
					row.Result = &r
				}
			}
			out.Results = append(out.Results, row)
		}
	}
	for _, g := range req.OptimizedGoals {
		want := RollFreed
		if g.Kind == Relief {
			want = KeepFreed
		}
		if err = u.explore(want); err != nil {
			return out, err
		}
		baseline, e := requiredNow(norm, u.cache)
		if e != nil {
			return out, e
		}
		ranked := []Result{}
		seen := map[string]bool{}
		add := func(r Result) {
			key := policyKey(canonicalPolicy(norm, r.Policy))
			if r.Policy.Rollover == want && !seen[key] {
				ranked = append(ranked, r)
				seen[key] = true
			}
		}
		for _, r := range u.Results {
			add(r)
		}
		for _, r := range out.Results {
			if r.Result != nil {
				add(*r.Result)
			}
		}
		if len(ranked) == 0 {
			return out, fmt.Errorf("plan: no feasible policy for %s", g)
		}
		sort.SliceStable(ranked, func(i, j int) bool { return better(g, baseline, ranked[i], ranked[j]) })
		cert := u.certificate(g, Report{Goal: g, Best: ranked[0], Ranked: ranked})
		out.Optimized = append(out.Optimized, GoalComparison{Goal: g, Ranked: ranked, Certificate: cert})
	}
	for i, a := range out.Results {
		for j := i + 1; j < len(out.Results); j++ {
			b := out.Results[j]
			if a.Result == nil || b.Result == nil || a.Rollover != b.Rollover {
				continue
			}
			d := ResultDelta{A: i, B: j, Months: a.Result.Months - b.Result.Months}
			pairs := [][3]*money.Amount{{&d.Interest, &a.Result.TotalInterest, &b.Result.TotalInterest}, {&d.Fees, &a.Result.TotalFees, &b.Result.TotalFees}, {&d.PeakRequired, &a.Result.PeakRequired, &b.Result.PeakRequired}}
			for _, p := range pairs {
				v, e := p[1].Sub(*p[2])
				if e != nil {
					return out, e
				}
				*p[0] = v
			}
			d.Cost, err = a.Result.Cost().Sub(b.Result.Cost())
			if err != nil {
				return out, err
			}
			out.PairwiseDeltas = append(out.PairwiseDeltas, d)
		}
	}
	out.Simulations = len(u.runs)
	feasible := 0
	for _, saved := range u.runs {
		if saved.err == nil {
			feasible++
		}
	}
	for i := range out.Optimized {
		// Comparison certificates count unique attempts across all requested
		// methods and rollovers; aliases and repeated goal ranks add no attempts.
		out.Optimized[i].Certificate.Policies = out.Simulations
		out.Optimized[i].Certificate.FeasiblePolicies = feasible
	}
	return out, nil
}

// Reflection deliberately descends into Amount and Date independently of their
// wire serialization, including all monetary and calendar metadata.
// Input contains only deterministic value fields; maps are sorted, pointers
// dereferenced, and both types and lengths are included to avoid ambiguity.
func inputHash(in Input) string {
	var b strings.Builder
	var write func(reflect.Value)
	write = func(v reflect.Value) {
		fmt.Fprintf(&b, "%s:", v.Type())
		switch v.Kind() {
		case reflect.Pointer:
			if v.IsNil() {
				b.WriteString("nil;")
			} else {
				write(v.Elem())
			}
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				write(v.Field(i))
			}
		case reflect.Slice, reflect.Array:
			fmt.Fprintf(&b, "%d[", v.Len())
			for i := 0; i < v.Len(); i++ {
				write(v.Index(i))
			}
			b.WriteString("]")
		case reflect.Map:
			keys := v.MapKeys()
			sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
			fmt.Fprintf(&b, "%d{", len(keys))
			for _, k := range keys {
				write(k)
				write(v.MapIndex(k))
			}
			b.WriteString("}")
		case reflect.String:
			fmt.Fprintf(&b, "%q;", v.String())
		case reflect.Bool:
			fmt.Fprintf(&b, "%t;", v.Bool())
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			fmt.Fprintf(&b, "%d;", v.Int())
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			fmt.Fprintf(&b, "%d;", v.Uint())
		default:
			panic("plan: unsupported input hash field type " + v.Type().String())
		}
	}
	write(reflect.ValueOf(in))
	return fmt.Sprintf("%x", sha256.Sum256([]byte(b.String())))
}
