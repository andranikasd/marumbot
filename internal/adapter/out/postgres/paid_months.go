package postgres

import (
	"context"

	"github.com/google/uuid"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/pkg/core/date"
)

func (t *loanCommandTx) RecordPaidMonths(ctx context.Context, id, user string, statement app.PaidMonthStatement, next date.Date) error {
	var nextDue any
	if !next.IsZero() {
		nextDue = next.String()
	}
	var snapshot string
	err := t.tx.QueryRow(ctx, q("RecordPaidMonthStatement"), id, user, uuid.NewString(), statement.AsOf, statement.PrincipalMinor, nextDue, statement.NextPaymentMinor, statement.Month).Scan(&snapshot)
	if err != nil {
		return paymentError(err)
	}
	if statement.AccruedInterestMinor != nil {
		asof, parseErr := date.Parse(statement.AsOf)
		if parseErr != nil {
			return parseErr
		}
		if err = t.RecordLoanOpeningInterest(ctx, id, user, *statement.AccruedInterestMinor, asof); err != nil {
			return err
		}
	}
	_, err = t.tx.Exec(ctx, q("CancelRemindersBeforeNextDue"), id, nextDue)
	return err
}
