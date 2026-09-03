package plan

import (
	"fmt"
	"math/big"
	"sort"

	"github.com/andranikasd/marumbot/pkg/core/allocation"
	"github.com/andranikasd/marumbot/pkg/core/amortisation"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// StrategyID identifies a deterministic initial priority, never an optimality claim.
type StrategyID string

// Named strategy IDs are stable comparison API identifiers. Eligibility is
// checked by StrategyOrder; an identifier does not imply domain support.
const (
	StrategyHighestRate     StrategyID = "highest_rate"
	StrategySnowball        StrategyID = "snowball"
	StrategyAvalanche       StrategyID = "avalanche"
	StrategyHybrid          StrategyID = "hybrid"
	StrategyHighestRequired StrategyID = "highest_required"
	StrategyHighestInterest StrategyID = "highest_interest"
	StrategyCashflowIndex   StrategyID = "cashflow_index"
	StrategyUtilisation     StrategyID = "utilisation"
	StrategyCustom          StrategyID = "custom"
)

// StrategyOrder returns indices into the supplied NORMALIZED input. Priorities
// are fixed at valuation; ties use stable IDs. CustomIDs must contain each live,
// optional-eligible loan exactly once. Ineligible loans are appended by ID for
// the simulator's required-payment guards. It never changes the caller's input.
func StrategyOrder(in Input, id StrategyID, customIDs []string) ([]int, error) {
	if err := validateStrategyIDs(in); err != nil {
		return nil, err
	}
	eligible, rest := []int{}, []int{}
	for i, l := range in.Loans {
		if l.Balance.Sign() > 0 && !l.OptionalExcluded {
			eligible = append(eligible, i)
		} else {
			rest = append(rest, i)
		}
	}
	lessID := func(a, b int) bool { return in.Loans[a].ID < in.Loans[b].ID }
	sort.Slice(rest, func(i, j int) bool { return lessID(rest[i], rest[j]) })
	if id == StrategyCustom {
		if len(customIDs) != len(eligible) {
			return nil, &UnsupportedError{Feature: "custom order must name every eligible loan exactly once"}
		}
		lookup := map[string]int{}
		for _, i := range eligible {
			lookup[in.Loans[i].ID] = i
		}
		ordered := make([]int, 0, len(in.Loans))
		for _, name := range customIDs {
			i, ok := lookup[name]
			if !ok {
				return nil, &UnsupportedError{Feature: "custom order contains duplicate, unknown or excluded ID"}
			}
			ordered = append(ordered, i)
			delete(lookup, name)
		}
		return append(ordered, rest...), nil
	}
	switch id {
	case StrategyHighestRate, StrategySnowball, StrategyAvalanche, StrategyHybrid, StrategyHighestRequired, StrategyHighestInterest, StrategyCashflowIndex:
	default:
		return nil, &UnsupportedError{Feature: "strategy " + string(id) + ": unsupported; utilisation requires a verified revolving limit"}
	}
	if id == StrategyAvalanche {
		if reason := avalancheDomain(in); reason != "" {
			return nil, &UnsupportedError{Feature: "avalanche: " + reason + "; use highest_rate baseline"}
		}
	}
	payoff := make([]int64, len(in.Loans))
	required := make([]int64, len(in.Loans))
	interest := make([]int64, len(in.Loans))
	for _, i := range eligible {
		l := in.Loans[i]
		if id == StrategyHybrid && l.Contract.NominalRate <= 0 {
			return nil, &UnsupportedError{LoanID: l.ID, Feature: "hybrid requires positive rates"}
		}
		if id == StrategyHighestRate || id == StrategyAvalanche || id == StrategyHybrid {
			continue
		}
		schedule, err := amortisation.Build(l.Contract, l.Balance, l.From)
		if err != nil {
			return nil, err
		}
		if len(schedule.Rows) == 0 {
			return nil, &UnsupportedError{LoanID: l.ID, Feature: "strategy needs next contractual period"}
		}
		required[i] = schedule.Rows[0].Payment.Minor()
		interest[i] = schedule.Rows[0].Interest.Minor()
		// At the trusted anchor no interest is outstanding. Off-anchor settlement
		// needs an accrued-interest snapshot that Position does not yet provide.
		if id == StrategySnowball || id == StrategyCashflowIndex {
			if !l.From.Equal(in.ValuationDate) {
				return nil, &UnsupportedError{LoanID: l.ID, Feature: "payoff priority requires valuation-date anchor"}
			}
			fee, err := charge(l.Contract, in.ValuationDate, l.Balance, money.Zero(l.Balance.Currency()))
			if err != nil {
				return nil, err
			}
			total, err := l.Balance.Add(fee)
			if err != nil {
				return nil, err
			}
			payoff[i] = total.Minor()
		}
		if id == StrategyCashflowIndex && required[i] <= 0 {
			return nil, &UnsupportedError{LoanID: l.ID, Feature: "cashflow index requires known released payment"}
		}
	}
	sort.Slice(eligible, func(a, b int) bool {
		i, j := eligible[a], eligible[b]
		x, y := in.Loans[i], in.Loans[j]
		cmp := 0
		switch id {
		case StrategyHighestRate, StrategyAvalanche:
			cmp = compareInt(int64(y.Contract.NominalRate), int64(x.Contract.NominalRate))
		case StrategySnowball:
			cmp = compareInt(payoff[i], payoff[j])
		case StrategyHybrid:
			cmp = compareRatio(x.Balance.Minor(), int64(x.Contract.NominalRate), y.Balance.Minor(), int64(y.Contract.NominalRate))
		case StrategyHighestRequired:
			cmp = compareInt(required[j], required[i])
		case StrategyHighestInterest:
			cmp = compareInt(interest[j], interest[i])
		case StrategyCashflowIndex:
			cmp = compareRatio(payoff[i], required[i], payoff[j], required[j])
		}
		if cmp != 0 {
			return cmp < 0
		}
		return lessID(i, j)
	})
	return append(eligible, rest...), nil
}

func validateStrategyIDs(in Input) error {
	seen := map[string]bool{}
	for _, l := range in.Loans {
		if l.ID == "" || seen[l.ID] {
			return &UnsupportedError{Feature: "strategy requires unique nonempty stable loan IDs"}
		}
		seen[l.ID] = true
	}
	return nil
}

func compareInt(a, b int64) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

// Cross-products are exact even at int64 amount/rate limits.
func compareRatio(a, b, c, d int64) int {
	var left, right big.Int
	left.Mul(big.NewInt(a), big.NewInt(d))
	right.Mul(big.NewInt(c), big.NewInt(b))
	return left.Cmp(&right)
}

// This permits a recognisable fee-free marginal-rate baseline. Rounding means
// its priority is not itself a proof of a globally optimal discrete plan.
func avalancheDomain(in Input) string {
	var basis money.DayCount
	var rounding money.Policy
	for i, l := range in.Loans {
		c := l.Contract
		if len(c.Prepayment.Charges) > 0 || c.Prepayment.FeeBP != 0 || c.Prepayment.MinAmount.Sign() != 0 {
			return "fees or thresholds prevent a verified marginal-rate ordering"
		}
		if l.Excess != allocation.ExcessReducePrincipal || l.OptionalExcluded {
			return "immediate principal credit required"
		}
		if !c.EffectiveThru.IsZero() || c.NominalRate < 0 {
			return "fixed nonnegative rates required"
		}
		if c.DayCount != money.Actual365 && c.DayCount != money.Actual360 {
			return "unsupported marginal accrual convention"
		}
		if i == 0 {
			basis, rounding = c.DayCount, c.Rounding
		} else if basis != c.DayCount || rounding != c.Rounding {
			return "common day count and rounding required"
		}
	}
	return ""
}

// strategyPolicy fixes common on-due timing and shorten-term choice, so method
// comparison changes priority only. Contract-fixed effects still take precedence.
func strategyPolicy(in Input, id StrategyID, ids []string, r Rollover) (Policy, error) {
	order, err := StrategyOrder(in, id, ids)
	if err != nil {
		return Policy{}, err
	}
	effects := uniform(len(in.Loans), model.PrepayShortenTerm)
	for i, l := range in.Loans {
		if l.Contract.Prepayment.Effect != model.PrepayBorrowerChooses {
			effects[i] = l.Contract.Prepayment.Effect
		}
	}
	return Policy{Name: string(id), Order: order, Timing: uniform(len(in.Loans), OnDue), Effect: effects, Rollover: r}, nil
}

func strategyError(id StrategyID, err error) error {
	return fmt.Errorf("plan: strategy %s: %w", id, err)
}
