package app

import (
	"context"

	"github.com/andranikasd/marumbot/pkg/core/plan"
)

type PlanPayment struct {
	On          string `json:"on"`
	LoanID      string `json:"loan_id"`
	Loan        string `json:"loan"`
	Kind        string `json:"kind"`
	AmountMinor int64  `json:"amount_minor"`
	FeeMinor    int64  `json:"fee_minor"`
}
type PlanTimeline struct {
	Currency  string        `json:"currency"`
	Exponent  uint8         `json:"currency_exponent"`
	Quantum   int64         `json:"settlement_quantum"`
	InputHash string        `json:"input_hash"`
	Engine    string        `json:"engine"`
	Payments  []PlanPayment `json:"payments"`
}

func (w *Worker) PaymentTimeline(ctx context.Context, user, proposal, id string) (PlanTimeline, error) {
	var m PlanManifest
	if id != "" {
		if w.History == nil {
			return PlanTimeline{}, ErrNotFound
		}
		v, err := w.History.PlanVersion(ctx, user, id)
		if err != nil {
			return PlanTimeline{}, err
		}
		m = v.Manifest
	} else {
		var ok bool
		m, ok = w.proposals.get(user, proposal)
		if !ok {
			return PlanTimeline{}, ErrConflict
		}
	}
	// Proposal exports describe current instructions. Historical exports remain
	// replayable after their sources change or their valuation date passes.
	checkCurrent := func() error {
		if id != "" {
			return nil
		}
		if w.History == nil {
			return ErrConflict
		}
		sources, err := w.History.PlanSources(ctx, user)
		if err != nil {
			return err
		}
		today, err := (PaymentService{Clock: w.Clock, Users: w.Users}).BusinessDate(ctx, user)
		if err != nil {
			return err
		}
		if sources != m.Sources || today != m.Input.ValuationDate {
			return ErrConflict
		}
		return nil
	}
	if err := checkCurrent(); err != nil {
		return PlanTimeline{}, err
	}
	if _, err := ReplayManifest(m); err != nil {
		return PlanTimeline{}, err
	}
	normalized, _, err := plan.Normalize(m.Input)
	if err != nil {
		return PlanTimeline{}, err
	}
	_, actions, err := plan.PaymentTimeline(normalized, m.Policy)
	if err != nil {
		return PlanTimeline{}, err
	}
	if err := checkCurrent(); err != nil {
		return PlanTimeline{}, err
	}
	cur := m.Input.Cash.Monthly.Currency()
	out := PlanTimeline{Currency: cur.Code, Exponent: cur.Exponent, Quantum: cur.SettlementUnit, InputHash: searchFingerprint(normalized, plan.Goal{}), Engine: m.Engine, Payments: []PlanPayment{}}
	for _, a := range actions {
		out.Payments = append(out.Payments, PlanPayment{a.On.String(), a.LoanID, a.Loan, a.Kind.String(), a.Amount.Minor(), a.Fee.Minor()})
	}
	return out, nil
}
