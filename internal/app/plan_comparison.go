package app

import (
	"context"
	"errors"

	"github.com/andranikasd/marumbot/pkg/core/plan"
)

// PlanComparer previews named methods against one already issued proposal.
type PlanComparer interface {
	PlanComparisons(context.Context, string, string) (PlanComparisonSheet, error)
}

// PlanComparisonSheet keeps the original manifest identity separate from the
// normalized comparison hash. Every available row carries its own exact proposal.
type PlanComparisonSheet struct {
	Proposal         string              `json:"proposal"`
	InputHash        string              `json:"input_hash"`
	SharedInputHash  string              `json:"shared_input_hash"`
	EngineVersion    string              `json:"engine_version"`
	Currency         string              `json:"currency"`
	CurrencyExponent uint8               `json:"currency_exponent"`
	AsOf             string              `json:"as_of"`
	Goal             string              `json:"goal"`
	ActiveRevision   int64               `json:"active_revision"`
	Rows             []PlanComparisonRow `json:"rows"`
}

// PlanComparisonRow uses pointers so an unavailable method never renders zero
// cost. Deltas are this method minus the originally proposed result, not savings.
type PlanComparisonRow struct {
	Strategy string                 `json:"strategy"`
	Proposal string                 `json:"proposal,omitempty"`
	Refusal  string                 `json:"refusal,omitempty"`
	Summary  *PlanComparisonSummary `json:"summary"`
}

type PlanComparisonSummary struct {
	PayoffDate        string `json:"payoff_date"`
	Months            int    `json:"months"`
	InterestMinor     int64  `json:"interest_minor"`
	FeesMinor         int64  `json:"fees_minor"`
	CostMinor         int64  `json:"cost_minor"`
	PeakRequiredMinor int64  `json:"peak_required_minor"`
	FirstClear        string `json:"first_clear"`
	FirstClearOn      string `json:"first_clear_on"`
	CostDeltaMinor    int64  `json:"cost_delta_minor"`
	MonthsDelta       int    `json:"months_delta"`
}

// PlanComparisons reads only the exact user-scoped proposal's source manifest.
// It never refetches/reconstructs loans or budgets and never activates a plan.
// Candidate manifests preserve original Input, InputHash, Sources and budget
// version; only Policy and the hash of the selected complete Result change.
func (w *Worker) PlanComparisons(ctx context.Context, user, proposal string) (PlanComparisonSheet, error) {
	var out PlanComparisonSheet
	if w.History == nil {
		return out, ErrNotFound
	}
	original, ok := w.proposals.get(user, proposal)
	if !ok {
		return out, ErrConflict
	}
	sources, err := w.History.PlanSources(ctx, user)
	if err != nil {
		return out, err
	}
	today, err := (PaymentService{Clock: w.Clock, Users: w.Users}).BusinessDate(ctx, user)
	if err != nil {
		return out, err
	}
	if sources != original.Sources || today != original.Input.ValuationDate {
		return out, ErrConflict
	}
	approvedResult, err := ReplayManifest(original)
	if err != nil {
		return out, err
	}
	ids := []plan.StrategyID{plan.StrategyHighestRate, plan.StrategySnowball, plan.StrategyHybrid, plan.StrategyHighestRequired, plan.StrategyHighestInterest, plan.StrategyCashflowIndex, plan.StrategyAvalanche, plan.StrategyUtilisation}
	request := plan.ComparisonRequest{Input: original.Input, StrategyIDs: ids}
	// Relief methods must retain freed payments, just like the original goal.
	if original.Policy.Rollover == plan.KeepFreed {
		request.OptimizedGoals = []plan.Goal{original.Goal}
	}
	compared, err := plan.Compare(request)
	if err != nil {
		return out, err
	}
	cur := original.Input.Cash.Monthly.Currency()
	out = PlanComparisonSheet{Proposal: proposal, InputHash: original.InputHash, SharedInputHash: compared.SharedInputHash, EngineVersion: original.Engine, Currency: cur.Code, CurrencyExponent: cur.Exponent, AsOf: original.Input.ValuationDate.String(), Goal: original.Goal.Kind.String(), Rows: []PlanComparisonRow{}}
	summary, err := comparisonSummary(approvedResult, approvedResult)
	if err != nil {
		return out, err
	}
	out.Rows = append(out.Rows, PlanComparisonRow{Strategy: "proposed", Proposal: proposal, Summary: &summary})
	for _, row := range compared.Results {
		if row.Rollover != original.Policy.Rollover {
			continue
		}
		item := PlanComparisonRow{Strategy: string(row.StrategyID)}
		switch {
		case row.Refusal != nil:
			var unsupported *plan.UnsupportedError
			if errors.As(row.Refusal, &unsupported) {
				item.Refusal = "unsupported"
			} else {
				item.Refusal = "infeasible"
			}
		case row.Result != nil:
			// Compare restores each alias's policy name and Normalize assumptions;
			// hashing that exact result must agree with ReplayManifest on activation.
			candidate := original
			candidate.Policy = row.Result.Policy
			candidate.ResultHash, err = resultHash(*row.Result)
			if err != nil {
				return out, err
			}
			item.Proposal, err = w.proposals.put(user, candidate)
			if err != nil {
				return out, err
			}
			value, e := comparisonSummary(*row.Result, approvedResult)
			if e != nil {
				return out, e
			}
			item.Summary = &value
		default:
			item.Refusal = "unavailable"
		}
		out.Rows = append(out.Rows, item)
	}
	latest, err := w.History.PlanSources(ctx, user)
	if err != nil {
		return out, err
	}
	if latest != original.Sources {
		return PlanComparisonSheet{}, ErrConflict
	}
	_, out.ActiveRevision, err = w.History.PlanHistory(ctx, user)
	if err != nil {
		return PlanComparisonSheet{}, err
	}
	return out, nil
}

func comparisonSummary(result, baseline plan.Result) (PlanComparisonSummary, error) {
	delta, err := result.Cost().Sub(baseline.Cost())
	if err != nil {
		return PlanComparisonSummary{}, err
	}
	first := ""
	if !result.FirstClearOn.IsZero() {
		first = result.FirstClearOn.String()
	}
	return PlanComparisonSummary{PayoffDate: result.PayoffDate.String(), Months: result.Months, InterestMinor: result.TotalInterest.Minor(), FeesMinor: result.TotalFees.Minor(), CostMinor: result.Cost().Minor(), PeakRequiredMinor: result.PeakRequired.Minor(), FirstClear: result.FirstClear, FirstClearOn: first, CostDeltaMinor: delta.Minor(), MonthsDelta: result.Months - baseline.Months}, nil
}
