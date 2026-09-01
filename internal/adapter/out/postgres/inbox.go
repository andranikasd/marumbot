package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/andranikasd/marumbot/internal/app"
)

// Enqueue records an inbound update, ignoring one that has already been seen.
//
// The returned bool distinguishes a new command from a repeat. Telegram retries
// until it is acknowledged, so repeats are normal traffic rather than an error,
// and the caller answers 200 either way -- refusing a duplicate would only make
// Telegram send it again.
func (s *Store) Enqueue(ctx context.Context, c app.InboundCommand) (bool, error) {
	var userID any
	if c.UserID != "" {
		userID = c.UserID
	}
	var id string
	err := s.pool.QueryRow(ctx, q("EnqueueCommand"),
		c.ID, c.UpdateID, userID, c.Kind, c.Payload, payloadSchemaVersion, nullable(c.TraceContext),
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil // already recorded; nothing to do and nothing wrong
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// payloadSchemaVersion is stamped on every stored command so a future reader
// knows which shape it is looking at. Raising it is a migration, not an edit.
const payloadSchemaVersion = 1

// Lease claims due commands for a worker.
func (s *Store) Lease(ctx context.Context, owner string, n int, until time.Time) ([]app.Lease, error) {
	rows, err := s.pool.Query(ctx, q("LeaseCommands"), owner, n, until)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []app.Lease
	for rows.Next() {
		var l app.Lease
		if err := rows.Scan(
			&l.Command.ID, &l.Command.UpdateID, &l.Command.UserID, &l.Command.Kind,
			&l.Command.Payload, &l.Command.TraceContext, &l.Command.Attempts,
			&l.Command.ReceivedAt, &l.Token, &l.Until,
		); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// LeaseByID claims one named command, if it is still available.
//
// Returns false rather than an error when the row is already leased or done:
// that is an ordinary race with the tick, not a fault.
func (s *Store) LeaseByID(ctx context.Context, id, owner string, until time.Time) (app.Lease, bool, error) {
	var l app.Lease
	err := s.pool.QueryRow(ctx, q("LeaseCommandByID"), id, owner, until).Scan(
		&l.Command.ID, &l.Command.UpdateID, &l.Command.UserID, &l.Command.Kind,
		&l.Command.Payload, &l.Command.TraceContext, &l.Command.Attempts,
		&l.Command.ReceivedAt, &l.Token, &l.Until,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.Lease{}, false, nil
	}
	if err != nil {
		return app.Lease{}, false, err
	}
	return l, true, nil
}

// Complete marks a command done, but only if this worker still holds the lease.
//
// A stalled worker whose lease expired will find no row to update, and gets
// ErrNotLeased rather than silently overwriting the work of whoever replaced it.
func (s *Store) Complete(ctx context.Context, id, token string) error {
	var got string
	err := s.pool.QueryRow(ctx, q("CompleteCommand"), id, token).Scan(&got)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrNotLeased
	}
	return err
}

// Fail schedules a retry, or sets the command aside once it has been tried
// enough times. Same fencing rule as Complete.
func (s *Store) Fail(ctx context.Context, id, token, code string, retryAt time.Time, dead bool) error {
	var got string
	err := s.pool.QueryRow(ctx, q("FailCommand"), id, token, code, dead, retryAt).Scan(&got)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrNotLeased
	}
	return err
}

// PurgeCompletedBefore removes completed commands older than the cutoff. They
// exist only for update-id dedup, which needs recent rows, not history.
func (s *Store) PurgeCompletedBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx, q("PurgeCompletedCommands"), cutoff)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
