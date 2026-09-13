package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/andranikasd/marumbot/pkg/core/amortisation"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// PaidMonthStatement is a borrower statement, not a synthetic money transfer.
// Balance and next instalment come from the lender after the completed months.
type PaidMonthStatement struct {
	AccruedInterestMinor *int64 `json:"accrued_interest_minor,omitempty"`
	Month                string `json:"month"`
	AsOf                 string `json:"as_of"`
	PrincipalMinor       int64  `json:"principal_minor"`
	NextPaymentMinor     int64  `json:"next_payment_minor"`
	Confirmed            bool   `json:"confirmed"`
}

type PaidMonthRecorder interface {
	RecordPaidMonths(context.Context, string, string, PaidMonthStatement, date.Date) error
}

// PaidMonthNextDue resolves the next contractual date after a completed month.
// It never moves the balance anchor into the future or infers a principal payment.
func PaidMonthNextDue(loan UserLoan, month string, today date.Date) (date.Date, error) {
	invalid := func() (date.Date, error) {
		return date.Date{}, fmt.Errorf("%w: choose a completed month with a future contractual payment", ErrPaymentInvalid)
	}
	if len(month) != 7 {
		return invalid()
	}
	first, err := date.Parse(month + "-01")
	if err != nil || first.String()[:7] != month || first.After(date.OnDayOfMonth(today, 1)) {
		return invalid()
	}
	dates, err := amortisation.PaymentDates(loan.Contract)
	if err != nil {
		return date.Date{}, err
	}
	paidMonthExists := false
	for _, due := range dates {
		if due.String()[:7] == month {
			paidMonthExists = true
		}
		if due.String()[:7] > month {
			if !paidMonthExists || !due.After(today) {
				return invalid()
			}
			return due, nil
		}
	}
	return invalid()
}

func (s LoanCommands) MarkMonthsPaid(ctx context.Context, user, id, key string, expected int64, statement PaidMonthStatement) (LoanCommandReceipt, error) {
	today, err := (PaymentService{Clock: s.Clock, Users: s.Users}).BusinessDate(ctx, user)
	if err != nil {
		return LoanCommandReceipt{}, err
	}
	return s.execute(ctx, user, key, "paid_months", struct {
		ID        string
		Expected  int64
		Statement PaidMonthStatement
	}{id, expected, statement}, func(tx LoanCommandTransaction) (LoanCommandReceipt, error) {
		if err := lockLoanCommand(ctx, tx, id, user, expected); err != nil {
			return LoanCommandReceipt{}, err
		}
		loan, err := tx.LoanForUser(ctx, id, user)
		if err != nil {
			return LoanCommandReceipt{}, err
		}
		if loan.UnreconciledPayments {
			return LoanCommandReceipt{}, ErrPaymentReconciliation
		}
		if !statement.Confirmed || statement.AsOf != today.String() || today.Before(loan.AsOf) || statement.PrincipalMinor < 0 || statement.PrincipalMinor > 9007199254740991 || statement.NextPaymentMinor < 0 || statement.NextPaymentMinor > 9007199254740991 {
			return LoanCommandReceipt{}, ErrPaymentInvalid
		}
		if statement.AccruedInterestMinor != nil && (*statement.AccruedInterestMinor < 0 || *statement.AccruedInterestMinor > 9007199254740991 || (statement.PrincipalMinor == 0 && *statement.AccruedInterestMinor > 0)) {
			return LoanCommandReceipt{}, ErrPaymentInvalid
		}
		var next date.Date
		if statement.PrincipalMinor == 0 {
			first, err := date.Parse(statement.Month + "-01")
			if err != nil || len(statement.Month) != 7 || first.After(date.OnDayOfMonth(today, 1)) || first.Before(date.OnDayOfMonth(loan.Contract.StartDate, 1)) || statement.NextPaymentMinor != 0 {
				return LoanCommandReceipt{}, ErrPaymentInvalid
			}
		} else {
			if statement.NextPaymentMinor == 0 {
				return LoanCommandReceipt{}, ErrPaymentInvalid
			}
			next, err = PaidMonthNextDue(loan, statement.Month, today)
			if err != nil {
				return LoanCommandReceipt{}, err
			}
			projection := loan
			projection.Balance = money.FromMinor(statement.PrincipalMinor, loan.Contract.Currency)
			projection.AsOf = today
			projection.Contract.NotBeforeDue = next
			projection.Contract.HasScheduled = true
			projection.Contract.ScheduledPayment = money.FromMinor(statement.NextPaymentMinor, loan.Contract.Currency)
			if _, err := projection.Schedule(); err != nil && !errors.Is(err, ErrLoanInterestUnknown) {
				return LoanCommandReceipt{}, ErrPaymentInvalid
			}
		}
		recorder, ok := tx.(PaidMonthRecorder)
		if !ok {
			return LoanCommandReceipt{}, fmt.Errorf("paid month statements are unavailable")
		}
		if err := recorder.RecordPaidMonths(ctx, id, user, statement, next); err != nil {
			return LoanCommandReceipt{}, err
		}
		version, err := tx.Version(ctx, id, user)
		return LoanCommandReceipt{ID: id, Version: version}, err
	})
}
