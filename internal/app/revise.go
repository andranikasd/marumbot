package app

import (
	"context"
	"fmt"
	"time"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// Editing a loan.
//
// A loan is three different kinds of fact, and each changes differently:
// the borrower's own words (name, note) are overwritten; the contract terms
// are VERSIONED -- a change is a new version with its own effective date, so
// every past balance keeps meaning what it meant; and the balance is a new
// snapshot, never an edit of an old one. One form, three write disciplines.

// LoanEdit is everything the borrower may change about a loan. The currency
// is deliberately absent: the ledger behind a loan is in one currency, and
// "changing" it would re-denominate history. That loan is archive-and-refile.
type LoanEdit struct {
	Name         string
	Description  string
	NominalRate  money.Rate
	Type         model.RepaymentType
	StartDate    date.Date
	MaturityDate date.Date
	PaymentDay   int
	PrepayEffect model.PrepaymentEffect
	// BalanceMinor restates what is owed today; nil leaves the anchor alone.
	BalanceMinor *int64
}

// ContractReviser writes a new contract version. Ownership is enforced in the
// query's predicate, like every loan write.
type ContractReviser interface {
	ReviseContract(ctx context.Context, loanID, userID string, c model.Contract, effectiveFrom date.Date) error
}

// ReviseLoan applies a full edit: words overwritten, terms versioned when
// they actually changed, balance re-anchored when restated. Terms that the
// form does not carry -- day count, rounding, the fee side of the prepayment
// policy -- ride over from the current version untouched.
func (w *Worker) ReviseLoan(ctx context.Context, loanID, userID string, e LoanEdit) error {
	if w.Editor == nil || w.Contracts == nil {
		return fmt.Errorf("loan revision is not wired")
	}
	ln, err := w.Editor.LoanForUser(ctx, loanID, userID)
	if err != nil {
		return err
	}

	if e.Name != ln.Name || e.Description != ln.Description {
		if err := w.Editor.UpdateLoan(ctx, loanID, userID, e.Name, e.Description); err != nil {
			return fmt.Errorf("renaming: %w", err)
		}
	}

	next := ln.Contract
	next.NominalRate = e.NominalRate
	next.Type = e.Type
	next.StartDate = e.StartDate
	next.MaturityDate = e.MaturityDate
	next.PaymentDay = e.PaymentDay
	next.Prepayment.Effect = e.PrepayEffect

	today := date.From(w.Clock.Now(), time.UTC)
	termsChanged := next.NominalRate != ln.Contract.NominalRate ||
		next.Type != ln.Contract.Type ||
		next.StartDate != ln.Contract.StartDate ||
		next.MaturityDate != ln.Contract.MaturityDate ||
		next.PaymentDay != ln.Contract.PaymentDay ||
		next.Prepayment.Effect != ln.Contract.Prepayment.Effect
	if termsChanged {
		if err := w.Contracts.ReviseContract(ctx, loanID, userID, next, today); err != nil {
			return fmt.Errorf("revising the contract: %w", err)
		}
	}

	if e.BalanceMinor != nil && w.Balances != nil {
		if err := w.Balances.RecordBalance(ctx, loanID, userID, *e.BalanceMinor, today.String()); err != nil {
			return fmt.Errorf("recording the balance: %w", err)
		}
	}

	// A changed payment day or maturity moves every occurrence, and a stale
	// reminder about a date that no longer exists is worse than none. Cancel
	// and regenerate from the revised schedule; failures are logged, because
	// the edit itself has already happened.
	if termsChanged && w.Reminders != nil {
		if err := w.Reminders.CancelRemindersForLoan(ctx, loanID); err != nil {
			w.Log.WarnContext(ctx, "cancelling stale reminders failed", "loan", loanID, "error", err)
		}
		if err := w.Reminders.EnsureDefaultReminders(ctx, loanID); err != nil {
			w.Log.WarnContext(ctx, "restoring reminder rules failed", "loan", loanID, "error", err)
		}
		if err := w.ScheduleForUser(ctx, userID); err != nil {
			w.Log.WarnContext(ctx, "rescheduling reminders failed", "loan", loanID, "error", err)
		}
	}
	return nil
}
