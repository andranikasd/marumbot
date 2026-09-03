package postgres

import (
	"context"
	"time"

	"github.com/andranikasd/marumbot/internal/app"
)

func (s *Store) ScheduleOptionalReminder(ctx context.Context, user, planID, loan string, index int, on string) error {
	_, err := s.pool.Exec(ctx, q("ScheduleOptionalReminder"), user, planID, loan, index, on)
	return err
}

func (s *Store) CancelObsoleteOptionalReminders(ctx context.Context, user string, active []string) error {
	_, err := s.pool.Exec(ctx, q("CancelObsoleteOptionalReminders"), user, active)
	return err
}

func (s *Store) CancelOptionalReminder(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, q("CancelOptionalReminder"), id)
	return err
}

func (s *Store) DueOptionalReminders(ctx context.Context, now time.Time, limit int32) ([]app.OptionalReminder, error) {
	rows, err := s.pool.Query(ctx, q("DueOptionalReminders"), now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []app.OptionalReminder{}
	for rows.Next() {
		var d app.OptionalReminder
		if err := rows.Scan(&d.ID, &d.UserID, &d.LoanID, &d.DueDate, &d.OffsetDays, &d.LoanName, &d.Currency, &d.PlanID, &d.ActionIndex); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
