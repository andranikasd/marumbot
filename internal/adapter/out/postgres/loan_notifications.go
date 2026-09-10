package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/andranikasd/marumbot/internal/app"
)

func (t *loanCommandTx) EnqueueLoanFiled(ctx context.Context, id string) error {
	_, err := t.tx.Exec(ctx, q("EnqueueLoanFiled"), id)
	return err
}

func (s *Store) LeaseLoanFiled(ctx context.Context, now time.Time, limit int32) ([]app.LoanFiledNotification, error) {
	rows, err := s.pool.Query(ctx, q("LeaseLoanFiled"), now, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[app.LoanFiledNotification])
}

func (s *Store) CompleteLoanFiled(ctx context.Context, n app.LoanFiledNotification, now time.Time) error {
	_, err := s.pool.Exec(ctx, q("CompleteLoanFiled"), n.ID, n.Token, now)
	return err
}

func (s *Store) RetryLoanFiled(ctx context.Context, n app.LoanFiledNotification, now time.Time) error {
	_, err := s.pool.Exec(ctx, q("RetryLoanFiled"), n.ID, n.Token, now)
	return err
}
