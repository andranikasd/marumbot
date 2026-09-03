package app

import (
	"context"
	"errors"
	"fmt"

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

// ErrSnapshotContractDate refuses a balance that predates its attached terms.
var ErrSnapshotContractDate = errors.New("balance date precedes the current contract")

// LoanEdit is everything the borrower may change about a loan. The currency
// is deliberately absent: the ledger behind a loan is in one currency, and
// "changing" it would re-denominate history. That loan is archive-and-refile.
type LoanEdit struct {
	Key              string
	ExpectedVersion  int64
	BalanceAsOf      date.Date
	Icon             *string
	OptionalExcluded *bool
	Name             string
	Description      string
	NominalRate      money.Rate
	Type             model.RepaymentType
	StartDate        date.Date
	MaturityDate     date.Date
	PaymentDay       int
	PrepayEffect     model.PrepaymentEffect
	// BalanceMinor restates what is owed today; nil leaves the anchor alone.
	BalanceMinor *int64
}

// ContractReviser writes a new contract version. Ownership is enforced in the
// query's predicate, like every loan write.
type ContractReviser interface {
	ApplyLoanRevision(ctx context.Context, loanID, userID string, revision LoanRevision) error
}

// LoanRevision is one atomic borrower edit. Nil fields mean that part did not
// change; persistence must commit every supplied part together or none of it.
type LoanRevision struct {
	BalanceAsOf       date.Date
	Icon              *string
	OptionalExcluded  *bool
	Name, Description string
	Rename            bool
	Contract          *model.Contract
	BalanceMinor      *int64
	EffectiveFrom     date.Date
}

// ReviseLoan applies a full edit: words overwritten, terms versioned when
// they actually changed, balance re-anchored when restated. Terms that the
// form does not carry -- day count, rounding, the fee side of the prepayment
// policy -- ride over from the current version untouched.
func (w *Worker) ReviseLoan(ctx context.Context, loanID, userID string, e LoanEdit) error {
	if len(e.Key) < 16 || e.ExpectedVersion < 1 {
		return ErrPaymentInvalid
	}
	store, ok := w.Contracts.(LoanCommandStore)
	if !ok {
		return fmt.Errorf("loan commands are not wired")
	}
	// This read only decides whether to refresh reminders. All financial
	// comparisons and writes run against the locked transaction's loan.
	var termsChanged bool
	if w.Editor != nil {
		if before, err := w.Editor.LoanForUser(ctx, loanID, userID); err == nil {
			c := before.Contract
			termsChanged = c.NominalRate != e.NominalRate || c.Type != e.Type || c.StartDate != e.StartDate || c.MaturityDate != e.MaturityDate || c.PaymentDay != e.PaymentDay || c.Prepayment.Effect != e.PrepayEffect
		}
	}
	_, err := (LoanCommands{Store: store, Clock: w.Clock, Users: w.Users}).Revise(ctx, userID, loanID, e.Key, e.ExpectedVersion, e)
	if err != nil {
		return err
	}
	if termsChanged {
		return w.loanRevisionReminders(ctx, loanID, userID)
	}
	return nil
}

func (w *Worker) loanRevisionReminders(ctx context.Context, loanID, userID string) error {
	if w.Reminders != nil {
		if err := w.Reminders.CancelRemindersForLoan(ctx, loanID); err != nil {
			w.Log.WarnContext(ctx, "cancelling stale reminders failed", "error", err)
		}
		if err := w.Reminders.EnsureDefaultReminders(ctx, loanID); err != nil {
			w.Log.WarnContext(ctx, "restoring reminder rules failed", "error", err)
		}
		if err := w.ScheduleForUser(ctx, userID); err != nil {
			w.Log.WarnContext(ctx, "rescheduling reminders failed", "error", err)
		}
	}
	return nil
}

func prepareLoanRevision(ln UserLoan, e LoanEdit, today date.Date) (LoanRevision, error) {
	next := ln.Contract
	next.NominalRate = e.NominalRate
	next.Type = e.Type
	next.StartDate = e.StartDate
	next.MaturityDate = e.MaturityDate
	next.PaymentDay = e.PaymentDay
	next.Prepayment.Effect = e.PrepayEffect

	balanceAsOf := today
	if !e.BalanceAsOf.IsZero() {
		if e.BalanceAsOf.After(today) || e.BalanceAsOf.Before(ln.Contract.StartDate) {
			return LoanRevision{}, fmt.Errorf("invalid balance date")
		}
		balanceAsOf = e.BalanceAsOf
	}
	termsChanged := next.NominalRate != ln.Contract.NominalRate ||
		next.Type != ln.Contract.Type ||
		next.StartDate != ln.Contract.StartDate ||
		next.MaturityDate != ln.Contract.MaturityDate ||
		next.PaymentDay != ln.Contract.PaymentDay ||
		next.Prepayment.Effect != ln.Contract.Prepayment.Effect
	if e.BalanceMinor != nil && (balanceAsOf.Before(ln.Contract.EffectiveFrom) || (termsChanged && balanceAsOf.Before(today))) {
		return LoanRevision{}, ErrSnapshotContractDate
	}
	if e.Icon != nil {
		icon, err := LoanIcon(*e.Icon)
		if err != nil {
			return LoanRevision{}, err
		}
		e.Icon = &icon
	}
	revision := LoanRevision{
		Icon: e.Icon, OptionalExcluded: e.OptionalExcluded,
		Name: e.Name, Description: e.Description,
		Rename:       e.Name != ln.Name || e.Description != ln.Description,
		BalanceMinor: e.BalanceMinor, BalanceAsOf: balanceAsOf, EffectiveFrom: today,
	}
	if termsChanged {
		revision.Contract = &next
	}
	return revision, nil
}
