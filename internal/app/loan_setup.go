package app

import (
	"context"
	"errors"

	"github.com/andranikasd/marumbot/pkg/core/date"
)

var ErrLoanOverdueNeedsReview = errors.New("app: overdue bank payment needs review")

var ErrLoanInterestUnknown = errors.New("app: loan interest rate is not known")

type LoanSetupStatement struct {
	AccruedInterestMinor     *int64 `json:",omitempty"`
	Draft                    LoanDraft
	NextDue                  date.Date
	PaymentMinor             int64
	ProjectionTermsConfirmed bool
	InterestKnown            bool
	Confirmed                bool
}

type LoanSetupRecorder interface {
	RecordLoanSetup(context.Context, string, LoanSetupStatement) error
}

// CreateSetup files the source figures atomically; completed months are not
// payments and never subtract from the bank-reported remaining balance again.
func (s LoanCommands) CreateSetup(ctx context.Context, key string, statement LoanSetupStatement) (LoanCommandReceipt, error) {
	identity := statement
	identity.Draft.AsOf = date.Date{}
	return s.execute(ctx, statement.Draft.UserID, key, "setup", identity, func(tx LoanCommandTransaction) (LoanCommandReceipt, error) {
		d := statement.Draft
		if statement.AccruedInterestMinor != nil && (*statement.AccruedInterestMinor < 0 || *statement.AccruedInterestMinor > 9007199254740991 || (d.Balance.Minor() == 0 && *statement.AccruedInterestMinor > 0)) {
			return LoanCommandReceipt{}, ErrPaymentInvalid
		}
		if !statement.Confirmed || d.Principal.Minor() <= 0 || d.Balance.Minor() < 0 || d.Balance.Minor() > d.Principal.Minor() || d.AsOf.Before(d.Contract.StartDate) {
			return LoanCommandReceipt{}, ErrPaymentInvalid
		}
		if d.Balance.Minor() > 0 && (statement.PaymentMinor <= 0 || statement.NextDue.After(d.Contract.MaturityDate) || !statement.NextDue.After(d.Contract.StartDate)) {
			return LoanCommandReceipt{}, ErrPaymentInvalid
		}
		if d.Balance.Minor() == 0 && (statement.PaymentMinor != 0 || !statement.NextDue.IsZero()) {
			return LoanCommandReceipt{}, ErrPaymentInvalid
		}
		recorder, ok := tx.(LoanSetupRecorder)
		if !ok {
			return LoanCommandReceipt{}, ErrPaymentInvalid
		}
		id, err := tx.CreateLoan(ctx, d)
		if err != nil {
			return LoanCommandReceipt{}, err
		}
		if err = recorder.RecordLoanSetup(ctx, id, statement); err != nil {
			return LoanCommandReceipt{}, err
		}
		if outbox, ok := tx.(interface {
			EnqueueLoanFiled(context.Context, string) error
		}); ok {
			if err := outbox.EnqueueLoanFiled(ctx, id); err != nil {
				return LoanCommandReceipt{}, err
			}
		}
		version, err := tx.Version(ctx, id, d.UserID)
		return LoanCommandReceipt{ID: id, Version: version}, err
	})
}
