package app

import (
	"context"
	"errors"
	"time"
)

type (
	LoanFiledNotification      struct{ ID, UserID, LoanID, Token string }
	LoanFiledNotificationStore interface {
		LeaseLoanFiled(context.Context, time.Time, int32) ([]LoanFiledNotification, error)
		CompleteLoanFiled(context.Context, LoanFiledNotification, time.Time) error
		RetryLoanFiled(context.Context, LoanFiledNotification, time.Time) error
	}
)

func (w *Worker) sendLoanFiled(ctx context.Context) error {
	store, ok := w.Reminders.(LoanFiledNotificationStore)
	if !ok {
		return nil
	}
	rows, err := store.LeaseLoanFiled(ctx, w.Clock.Now(), 5)
	if err != nil {
		return err
	}
	var failures error
	for _, row := range rows {
		err := w.OnLoanFiled(ctx, row.UserID, row.LoanID)
		markCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		if err == nil {
			err = store.CompleteLoanFiled(markCtx, row, w.Clock.Now())
		} else {
			err = errors.Join(err, store.RetryLoanFiled(markCtx, row, w.Clock.Now()))
		}
		cancel()
		failures = errors.Join(failures, err)
	}
	return failures
}
