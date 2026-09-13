package postgres

import (
	"context"

	"github.com/google/uuid"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/pkg/core/date"
)

func (t *loanCommandTx) RecordLoanSetup(ctx context.Context, id string, s app.LoanSetupStatement) error {
	var due any
	if !s.NextDue.IsZero() {
		due = s.NextDue.String()
	}
	var snapshot string
	if err := t.tx.QueryRow(ctx, q("RecordLoanSetupStatement"), id, s.Draft.UserID, uuid.NewString(), s.Draft.AsOf.String(), s.Draft.Balance.Minor(), due, s.PaymentMinor).Scan(&snapshot); err != nil {
		return err
	}
	_, err := t.tx.Exec(ctx, q("RecordLoanSetupSource"), id, s.InterestKnown, s.Draft.Principal.Minor(), s.Draft.AsOf.String(), s.ProjectionTermsConfirmed, s.AccruedInterestMinor, snapshot)
	return err
}

func (t *loanCommandTx) ConfirmLoanProjectionTerms(ctx context.Context, id, user string, original int64, today date.Date, confirmed bool) error {
	_, err := t.tx.Exec(ctx, q("ConfirmLoanProjectionTerms"), id, user, original, today.String(), confirmed)
	return err
}

func (t *loanCommandTx) RecordLoanOpeningInterest(ctx context.Context, id, user string, minor int64, asof date.Date) error {
	_, err := t.tx.Exec(ctx, q("RecordLoanOpeningInterest"), id, user, minor, asof.String())
	return err
}
