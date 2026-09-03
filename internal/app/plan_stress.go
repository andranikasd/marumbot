package app

import (
	"context"

	"github.com/andranikasd/marumbot/pkg/core/plan"
)

// PlanStressReader replays an issued proposal without modifying active plans.
type PlanStressReader interface {
	PlanStress(context.Context, string, string, int64) (PlanStressSheet, error)
}

// PlanStressSheet identifies the original proposal and its coverage evidence.
type PlanStressSheet struct {
	Proposal           string            `json:"proposal"`
	InputHash          string            `json:"input_hash"`
	EngineVersion      string            `json:"engine_version"`
	Currency           string            `json:"currency"`
	CurrencyExponent   uint8             `json:"currency_exponent"`
	AsOf               string            `json:"as_of"`
	Health             plan.StressHealth `json:"health"`
	RequiredIncreaseBP int64             `json:"required_increase_bp"`
	Base               PlanStressCase    `json:"base"`
	Cases              []PlanStressCase  `json:"cases"`
}

// PlanStressCase carries typed coverage and optional exact failure amounts.
type PlanStressCase struct {
	ID         string             `json:"id"`
	Status     plan.StressStatus  `json:"status"`
	Reason     plan.StressReason  `json:"reason"`
	PayoffDate string             `json:"payoff_date,omitempty"`
	Failure    *PlanStressFailure `json:"failure,omitempty"`
}

// PlanStressFailure contains minor units, never fabricated zero estimates.
type PlanStressFailure struct {
	On             string `json:"on"`
	LoanID         string `json:"loan_id"`
	Constraint     string `json:"constraint"`
	RequiredMinor  int64  `json:"required_minor"`
	AvailableMinor int64  `json:"available_minor"`
	ShortfallMinor int64  `json:"shortfall_minor"`
}

// PlanStress uses the original manifest and selected policy; no new proposal,
// optimized result, source update or activation is created.
func (w *Worker) PlanStress(ctx context.Context, user, proposal string, increaseBP int64) (PlanStressSheet, error) {
	var out PlanStressSheet
	if increaseBP < 0 || increaseBP > 10000 {
		return out, ErrPaymentInvalid
	}
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
	if _, err = ReplayManifest(original); err != nil {
		return out, err
	}
	normalized, _, err := plan.Normalize(original.Input)
	if err != nil {
		return out, err
	}
	report, err := plan.StressCases(normalized, original.Policy, plan.StressOptions{RequiredIncreaseBP: increaseBP})
	if err != nil {
		return out, err
	}
	latest, err := w.History.PlanSources(ctx, user)
	if err != nil {
		return out, err
	}
	if latest != original.Sources {
		return out, ErrConflict
	}
	cur := original.Input.Cash.Monthly.Currency()
	out = PlanStressSheet{Proposal: proposal, InputHash: searchFingerprint(normalized, plan.Goal{}), EngineVersion: original.Engine, Currency: cur.Code, CurrencyExponent: cur.Exponent, AsOf: today.String(), Health: report.Health, RequiredIncreaseBP: increaseBP, Base: stressCaseView(report.Base), Cases: []PlanStressCase{}}
	for _, c := range report.Cases {
		out.Cases = append(out.Cases, stressCaseView(c))
	}
	return out, nil
}

func stressCaseView(c plan.StressCase) PlanStressCase {
	out := PlanStressCase{ID: c.ID, Status: c.Status, Reason: c.Reason}
	if !c.PayoffDate.IsZero() {
		out.PayoffDate = c.PayoffDate.String()
	}
	if f := c.Failure; f != nil {
		out.Failure = &PlanStressFailure{On: f.On.String(), LoanID: f.LoanID, Constraint: f.Constraint, RequiredMinor: f.Required.Minor(), AvailableMinor: f.Available.Minor(), ShortfallMinor: f.Shortfall.Minor()}
	}
	return out
}
