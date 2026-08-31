package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/andranikasd/marumbot/internal/app"
)

// The reminder store. Idempotency lives in the schema — unique keys on
// (loan_id, offset_days) and on the occurrence idempotency key — so a tick
// that runs twice produces one reminder, and that survives a restart.

func (s *Store) EnsureDefaultReminders(ctx context.Context, loanID string) error {
	_, err := s.pool.Exec(ctx, q("EnsureDefaultReminders"), loanID)
	return err
}

func (s *Store) ScheduleReminders(ctx context.Context, due time.Time, loanID string) error {
	_, err := s.pool.Exec(ctx, q("ScheduleReminders"), due.Format("2006-01-02"), loanID)
	return err
}

func (s *Store) DueReminders(ctx context.Context, limit int32) ([]app.DueReminder, error) {
	rows, err := s.pool.Query(ctx, q("DueReminders"), limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[app.DueReminder])
}

func (s *Store) MarkReminderSatisfied(ctx context.Context, id string) error {
	var got string
	err := s.pool.QueryRow(ctx, q("MarkReminderSatisfied"), id).Scan(&got)
	return err
}

func (s *Store) CancelRemindersForLoan(ctx context.Context, loanID string) error {
	rows, err := s.pool.Query(ctx, q("CancelRemindersForLoan"), loanID)
	if err != nil {
		return err
	}
	rows.Close()
	return rows.Err()
}

// ActiveLoanUsers lists accounts with live loans, for the reminder tick.
func (s *Store) ActiveLoanUsers(ctx context.Context, limit int32) ([]string, error) {
	rows, err := s.pool.Query(ctx, q("ActiveLoanUsers"), limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}
