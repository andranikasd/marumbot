package app

import (
	"errors"
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

func TestUnknownInterestReminderUsesOnlyBankObligation(t *testing.T) {
	loan := paidLoan(t)
	loan.InterestUnknown = true
	loan.Contract.NotBeforeDue, _ = date.Parse("2026-10-15")
	loan.Contract.HasScheduled = true
	loan.Contract.ScheduledPayment = money.FromMinor(10000, loan.Contract.Currency)
	reminder, err := requiredReminderSchedule(loan)
	if err != nil || len(reminder.Rows) != 1 || reminder.Rows[0].Due != loan.Contract.NotBeforeDue || reminder.Rows[0].Payment != loan.Contract.ScheduledPayment {
		t.Fatal("missing bank obligation", err)
	}
	if _, err = loan.Schedule(); !errors.Is(err, ErrLoanInterestUnknown) {
		t.Fatal("bank obligation leaked into forecast", err)
	}
	loan.UnreconciledPayments = true
	if _, err = requiredReminderSchedule(loan); !errors.Is(err, ErrPaymentReconciliation) {
		t.Fatal("stale bank obligation used after payment", err)
	}
	loan.UnreconciledPayments = false
	loan.Contract.HasScheduled = false
	if _, err = requiredReminderSchedule(loan); !errors.Is(err, ErrLoanInterestUnknown) {
		t.Fatal("missing bank amount guessed", err)
	}
}

func TestBalanceOnlyPreservesUnknownTerms(t *testing.T) {
	loan := paidLoan(t)
	loan.InterestUnknown = true
	today, _ := date.Parse("2026-09-14")
	balance := loan.Balance.Minor()
	revision, err := prepareLoanRevision(loan, LoanEdit{BalanceOnly: true, BalanceMinor: &balance, BalanceAsOf: today}, today)
	if err != nil || revision.Contract != nil || revision.Rename || revision.BalanceMinor == nil {
		t.Fatal("balance-only update changed unknown terms", err)
	}
}
