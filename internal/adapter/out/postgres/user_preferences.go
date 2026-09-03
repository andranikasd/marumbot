package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/andranikasd/marumbot/internal/app"
)

type preferenceTx struct{ tx pgx.Tx }

func (s *Store) BeginPreferences(ctx context.Context) (app.PreferenceTransaction, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return &preferenceTx{tx}, nil
}
func (t *preferenceTx) Commit(ctx context.Context) error   { return t.tx.Commit(ctx) }
func (t *preferenceTx) Rollback(ctx context.Context) error { return t.tx.Rollback(ctx) }
func (t *preferenceTx) LockUser(ctx context.Context, user string) error {
	var id string
	return preferenceError(t.tx.QueryRow(ctx, q("LockPreferenceUser"), user).Scan(&id))
}

func preferenceError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrNotFound
	}
	return err
}

func scanPreferences(row pgx.Row) (app.UserPreferences, error) {
	var p app.UserPreferences
	err := row.Scan(&p.Timezone, &p.QuietEnabled, &p.QuietStart, &p.QuietEnd, &p.Version)
	return p, preferenceError(err)
}

func (s *Store) UserPreferences(ctx context.Context, user string) (app.UserPreferences, error) {
	return scanPreferences(s.pool.QueryRow(ctx, q("GetUserPreferences"), user))
}

func (t *preferenceTx) Receipt(ctx context.Context, user, key string) (string, []byte, error) {
	var payload string
	var result []byte
	err := t.tx.QueryRow(ctx, q("PreferenceReceipt"), user, key).Scan(&payload, &result)
	return payload, result, preferenceError(err)
}

func (t *preferenceTx) SaveReceipt(ctx context.Context, user, key, payload string, result []byte) error {
	_, err := t.tx.Exec(ctx, q("SavePreferenceReceipt"), user, key, payload, result)
	return err
}

func (t *preferenceTx) SetPreferences(ctx context.Context, user string, p app.UserPreferences) (app.UserPreferences, error) {
	old, err := scanPreferences(t.tx.QueryRow(ctx, q("GetUserPreferences"), user))
	if err != nil {
		return p, err
	}
	out, err := scanPreferences(t.tx.QueryRow(ctx, q("UpdateUserPreferences"), user, p.Timezone, p.QuietEnabled, p.QuietStart, p.QuietEnd, p.Version))
	if errors.Is(err, app.ErrNotFound) {
		return out, app.ErrConflict
	}
	if err != nil {
		return out, err
	}
	if old.Timezone != p.Timezone {
		_, err = t.tx.Exec(ctx, q("RetimeUserReminders"), user, p.Timezone)
	}
	return out, err
}

func scanOccurrence(row pgx.Row) (app.ReminderOccurrence, error) {
	var r app.ReminderOccurrence
	err := row.Scan(&r.ID, &r.LoanID, &r.DueDate, &r.TargetSendAt, &r.Status, &r.Version, &r.Required)
	return r, preferenceError(err)
}

func (s *Store) ReminderOccurrence(ctx context.Context, user, id string) (app.ReminderOccurrence, error) {
	return scanOccurrence(s.pool.QueryRow(ctx, q("PreferenceOccurrence"), user, id))
}

func (t *preferenceTx) Snooze(ctx context.Context, user string, c app.SnoozeCommand) (app.ReminderOccurrence, error) {
	// Ownership is checked separately: an inaccessible occurrence is never a conflict.
	if _, err := scanOccurrence(t.tx.QueryRow(ctx, q("PreferenceOccurrence"), user, c.OccurrenceID)); err != nil {
		return app.ReminderOccurrence{}, err
	}
	r, err := scanOccurrence(t.tx.QueryRow(ctx, q("SnoozePreferenceOccurrence"), user, c.OccurrenceID, c.Until, c.ExpectedVersion))
	if errors.Is(err, app.ErrNotFound) {
		err = app.ErrConflict
	}
	return r, err
}

func (s *Store) ReadyReminders(ctx context.Context, now time.Time, limit int32) ([]app.DueReminder, error) {
	rows, err := s.pool.Query(ctx, q("ReadyPreferenceReminders"), now, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[app.DueReminder])
}

func (s *Store) MarkReminderDelivered(ctx context.Context, id string, now time.Time) error {
	_, err := s.pool.Exec(ctx, q("MarkPreferenceReminderDelivered"), id, now)
	return err
}

var _ app.UserPreferenceStore = (*Store)(nil)
