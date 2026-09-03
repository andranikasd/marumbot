package app

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/andranikasd/marumbot/pkg/core/allocation"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/money"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

// ErrFundingRequired blocks new calculations without a cash declaration.
// Historical manifests are replayed from their original inputs independently.
var ErrFundingRequired = errors.New("budget: declare available funding before creating a plan")

var ErrBudgetFundingInvalid = errors.New("budget: invalid cash declaration")

var ErrCashRoutingStale = errors.New("budget: refresh the retained-cash declaration")

// BudgetFundingUpdate changes future cash declarations, never reconciled facts
// or spending permission. Nil funding must first be declared through onboarding.
type BudgetFundingUpdate struct {
	Key             string            `json:"idempotency_key"`
	Currency        string            `json:"currency"`
	ExpectedVersion int64             `json:"expected_version"`
	PayDay          int               `json:"pay_day"`
	MonthlyMinor    int64             `json:"monthly_minor"`
	Events          []BudgetCashEvent `json:"events"`
}

type BudgetFundingStore interface {
	UpdateBudgetFunding(context.Context, string, BudgetFundingUpdate) error
}

// BudgetCashRouting preserves the user's cash restrictions as declarations.
// Split amounts include all of the event; a hold without targets is pooled.
type BudgetCashRouting struct {
	LoanID           string            `json:"loan_id,omitempty"`
	Splits           []BudgetCashSplit `json:"splits,omitempty"`
	HoldUntil        string            `json:"hold_until,omitempty"`
	HoldMinimumMinor *int64            `json:"hold_minimum_minor,omitempty"`
}

type BudgetCashSplit struct {
	LoanID string `json:"loan_id"`
	Minor  int64  `json:"minor"`
}

// Validate checks a declaration's exact amounts and dates, before storage.
func (u BudgetFundingUpdate) Validate(today date.Date) error {
	if u.ExpectedVersion < 1 || u.PayDay < 1 || u.PayDay > 31 || u.MonthlyMinor < 0 || u.MonthlyMinor > math.MaxInt64/1000 {
		return fmt.Errorf("%w: funding", ErrBudgetFundingInvalid)
	}
	return ValidateBudgetCashEvents(u.Events, u.Currency, today)
}

func ValidateBudgetCashEvents(events []BudgetCashEvent, currency string, today date.Date) error {
	if err := validateBudgetCashEvents(events, currency, today); err != nil {
		return fmt.Errorf("%w: %w", ErrBudgetFundingInvalid, err)
	}
	return nil
}

func validateBudgetCashEvents(events []BudgetCashEvent, currency string, today date.Date) error {
	cur, err := money.Lookup(currency)
	if err != nil || len(events) > 36 {
		return fmt.Errorf("budget: invalid cash events")
	}
	unit := money.DefaultPolicy(cur).Unit
	ids := map[string]bool{}
	for _, e := range events {
		on, err := date.Parse(e.On)
		if err != nil || on.Before(today) || e.Minor <= 0 || e.Minor > math.MaxInt64/1000 {
			return fmt.Errorf("budget: invalid cash event")
		}
		if len(e.ID) > 128 || (e.ID != "" && ids[e.ID]) {
			return fmt.Errorf("budget: duplicate or invalid cash event identity")
		}
		if e.ID != "" {
			ids[e.ID] = true
		}
		if e.FromOpening && (e.Expected || e.Routing == nil || !on.Equal(today)) {
			return fmt.Errorf("budget: retained cash requires today's confirmed routing declaration")
		}
		r := e.Routing
		if r == nil {
			continue
		}
		if e.ID == "" || e.Minor%unit != 0 || len(r.Splits) > plan.MaxLoans || (r.LoanID != "" && len(r.Splits) > 0) {
			return fmt.Errorf("budget: invalid cash routing")
		}
		if r.LoanID == "" && len(r.Splits) == 0 && r.HoldUntil == "" && r.HoldMinimumMinor == nil {
			return fmt.Errorf("budget: empty cash routing")
		}
		if r.HoldUntil != "" {
			held, err := date.Parse(r.HoldUntil)
			if err != nil || held.Before(on) {
				return fmt.Errorf("budget: invalid hold date")
			}
		}
		if r.HoldMinimumMinor != nil && (*r.HoldMinimumMinor <= 0 || *r.HoldMinimumMinor > math.MaxInt64/1000 || *r.HoldMinimumMinor%unit != 0) {
			return fmt.Errorf("budget: invalid hold threshold")
		}
		seen := map[string]bool{}
		left := e.Minor
		for _, split := range r.Splits {
			if split.LoanID == "" || seen[split.LoanID] || split.Minor <= 0 || split.Minor > left || split.Minor%unit != 0 {
				return fmt.Errorf("budget: invalid cash split")
			}
			seen[split.LoanID] = true
			left -= split.Minor
		}
		if len(r.Splits) > 0 && left != 0 {
			return fmt.Errorf("budget: splits must use the entire cash event")
		}
	}
	return nil
}

// ValidateBudgetCashTargets checks ownership and known optional-payment support.
func ValidateBudgetCashTargets(events []BudgetCashEvent, currency string, loans []UserLoan) error {
	eligible := map[string]bool{}
	for _, l := range loans {
		eligible[l.ID] = l.Contract.Currency.Code == currency && l.Balance.Sign() > 0 && !l.OptionalExcluded && l.Excess == allocation.ExcessReducePrincipal
	}
	for _, e := range events {
		if e.Routing == nil {
			continue
		}
		if e.Routing.LoanID != "" && !eligible[e.Routing.LoanID] {
			return &plan.UnsupportedError{Feature: "cash routing target is not an eligible owned loan"}
		}
		for _, split := range e.Routing.Splits {
			if !eligible[split.LoanID] {
				return &plan.UnsupportedError{Feature: "cash routing target is not an eligible owned loan"}
			}
		}
	}
	return nil
}

func (b Budget) validateCurrentCashRouting(today date.Date) error {
	if b.Funding == nil {
		return ErrFundingRequired
	}
	for _, e := range b.Funding.Events {
		if e.Routing != nil && e.On < today.String() {
			return fmt.Errorf("%w: %w", ErrCashRoutingStale, &plan.UnsupportedError{Feature: "cash routing requires a current retained-cash declaration"})
		}
	}
	return nil
}

func (e BudgetCashEvent) cashEvent(cur money.Currency) plan.CashEvent {
	on, _ := date.Parse(e.On)
	out := plan.CashEvent{ID: e.ID, On: on, Amount: money.FromMinor(e.Minor, cur), Expected: e.Expected, FromOpening: e.FromOpening}
	if e.Routing != nil {
		r := e.Routing
		routing := &plan.CashRouting{LoanID: r.LoanID}
		if r.HoldUntil != "" {
			routing.HoldUntil, _ = date.Parse(r.HoldUntil)
		}
		if r.HoldMinimumMinor != nil {
			routing.HoldMinimum = money.FromMinor(*r.HoldMinimumMinor, cur)
		}
		for _, split := range r.Splits {
			routing.Splits = append(routing.Splits, plan.CashSplit{LoanID: split.LoanID, Amount: money.FromMinor(split.Minor, cur)})
		}
		out.Routing = routing
	}
	return out
}
