package app

import (
	"context"
	"errors"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

type InverseBudget struct {
	Supported    bool   `json:"supported"`
	Reason       string `json:"reason,omitempty"`
	Currency     string `json:"currency"`
	Exponent     uint8  `json:"currency_exponent"`
	MinimumMinor int64  `json:"minimum_minor,omitempty"`
	Target       string `json:"target"`
	InputHash    string `json:"input_hash"`
}

func (w *Worker) BudgetByDate(ctx context.Context, user, proposal, target string) (InverseBudget, error) {
	m, ok := w.proposals.get(user, proposal)
	if !ok || w.History == nil {
		return InverseBudget{}, ErrConflict
	}
	sources, err := w.History.PlanSources(ctx, user)
	if err != nil {
		return InverseBudget{}, err
	}
	if sources != m.Sources {
		return InverseBudget{}, ErrConflict
	}
	today, err := (PaymentService{Clock: w.Clock, Users: w.Users}).BusinessDate(ctx, user)
	if err != nil {
		return InverseBudget{}, err
	}
	if today != m.Input.ValuationDate {
		return InverseBudget{}, ErrConflict
	}
	by, err := date.Parse(target)
	if err != nil || by.Before(today) {
		return InverseBudget{}, ErrPaymentInvalid
	}
	normalized, _, err := plan.Normalize(m.Input)
	if err != nil {
		return InverseBudget{}, err
	}
	cur := m.Input.Cash.Monthly.Currency()
	out := InverseBudget{Currency: cur.Code, Exponent: cur.Exponent, Target: target, InputHash: searchFingerprint(normalized, plan.Goal{})}
	minimum, err := plan.BudgetFor(m.Input, m.Policy, by)
	var unsupported *plan.NonMonotoneError
	if errors.As(err, &unsupported) {
		out.Reason = "unproven_domain"
		return out, nil
	}
	if err != nil {
		return out, err
	}
	out.Supported = true
	out.MinimumMinor = minimum.Minor()
	return out, nil
}
