package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/andranikasd/marumbot/internal/app"
)

// Overview is the dashboard's snapshot of the whole system.

// Overview returns the dashboard counters in one round trip.
func (s *Store) Overview(ctx context.Context) (app.Overview, error) {
	var o app.Overview
	err := s.pool.QueryRow(ctx, q("CountsOverview")).Scan(
		&o.Users, &o.Loans, &o.Events, &o.Snapshots, &o.Policies,
		&o.CommandsPending, &o.CommandsDead,
		&o.DeliveriesPending, &o.DeliveriesDead,
		&o.OccurrencesScheduled, &o.OldestCommandAgeS, &o.OldestDeliveryAgeS)
	return o, err
}

// ListUsers returns the most recently created accounts.
func (s *Store) ListUsers(ctx context.Context, limit int32) ([]app.UserRow, error) {
	rows, err := s.pool.Query(ctx, q("ListUsers"), limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[app.UserRow])
}

// ListLoans returns loans with their cached reliability, newest first.
func (s *Store) ListLoans(ctx context.Context, limit int32) ([]app.LoanRow, error) {
	rows, err := s.pool.Query(ctx, q("ListLoans"), limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[app.LoanRow])
}

// GetLoan returns one loan, or an error when it does not exist.
func (s *Store) GetLoan(ctx context.Context, id string) (app.LoanDetail, error) {
	rows, err := s.pool.Query(ctx, q("GetLoan"), id)
	if err != nil {
		return app.LoanDetail{}, err
	}
	return pgx.CollectExactlyOneRow(rows, pgx.RowToStructByPos[app.LoanDetail])
}

// ContractsForLoan returns every contract version, oldest first.
func (s *Store) ContractsForLoan(ctx context.Context, loanID string) ([]app.ContractRow, error) {
	rows, err := s.pool.Query(ctx, q("ListContractsForLoan"), loanID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[app.ContractRow])
}

// SnapshotsForLoan returns every bank snapshot, newest first.
func (s *Store) SnapshotsForLoan(ctx context.Context, loanID string) ([]app.SnapshotRow, error) {
	rows, err := s.pool.Query(ctx, q("ListSnapshotsForLoan"), loanID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[app.SnapshotRow])
}

// EventsForLoan returns the ledger in replay order.
func (s *Store) EventsForLoan(ctx context.Context, loanID string) ([]app.EventRow, error) {
	rows, err := s.pool.Query(ctx, q("ListEventsForLoan"), loanID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[app.EventRow])
}

// ListPolicies returns every recorded allocation policy.
func (s *Store) ListPolicies(ctx context.Context) ([]app.PolicyRow, error) {
	rows, err := s.pool.Query(ctx, q("ListPolicies"))
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[app.PolicyRow])
}

// InsertPolicy records a lender's payment behaviour. This is the one write the
// admin interface exists for: allocation policies are read off real contracts
// by a person, and there is no other surface that can capture them.
func (s *Store) InsertPolicy(ctx context.Context, id, key string, version int32, definition []byte, excess, source string) error {
	_, err := s.pool.Exec(ctx, q("InsertPolicy"), id, key, version, definition, excess, source)
	return err
}

// ListCommands returns the most recent inbox entries.
func (s *Store) ListCommands(ctx context.Context, limit int32) ([]app.CommandRow, error) {
	rows, err := s.pool.Query(ctx, q("ListCommands"), limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[app.CommandRow])
}

// ListDeliveries returns the most recent outbound messages.
func (s *Store) ListDeliveries(ctx context.Context, limit int32) ([]app.DeliveryRow, error) {
	rows, err := s.pool.Query(ctx, q("ListDeliveries"), limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[app.DeliveryRow])
}

// ListReconciliationRuns returns recent drift measurements.
func (s *Store) ListReconciliationRuns(ctx context.Context, limit int32) ([]app.ReconRow, error) {
	rows, err := s.pool.Query(ctx, q("ListReconciliationRuns"), limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[app.ReconRow])
}

// SetUserAccess pauses or restores an account.
func (s *Store) SetUserAccess(ctx context.Context, userID, state string) error {
	var id, got string
	err := s.pool.QueryRow(ctx, q("SetUserAccess"), userID, state).Scan(&id, &got)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrNotFound
	}
	return err
}

// RequestUserDeletion marks an account for erasure without erasing it.
func (s *Store) RequestUserDeletion(ctx context.Context, userID string) error {
	var id string
	err := s.pool.QueryRow(ctx, q("RequestUserDeletion"), userID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrNotFound
	}
	return err
}

// DeleteUser honours a deletion request, leaving a tombstone so a restored
// backup cannot resurrect the account.
func (s *Store) DeleteUser(ctx context.Context, userID string) error {
	var id string
	err := s.pool.QueryRow(ctx, q("DeleteUser"), userID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrNotFound
	}
	return err
}

// ArchiveLoanAdmin hides a loan regardless of who owns it.
func (s *Store) ArchiveLoanAdmin(ctx context.Context, loanID string) error {
	var id string
	err := s.pool.QueryRow(ctx, q("ArchiveLoan"), loanID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrNotFound
	}
	return err
}

// RestoreLoan un-archives a loan.
func (s *Store) RestoreLoan(ctx context.Context, loanID string) error {
	var id string
	err := s.pool.QueryRow(ctx, q("RestoreLoan"), loanID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrNotFound
	}
	return err
}

// RenameLoanAdmin renames a loan regardless of who owns it.
func (s *Store) RenameLoanAdmin(ctx context.Context, loanID, name, description string) error {
	var id string
	err := s.pool.QueryRow(ctx, q("RenameLoan"), loanID, name, description).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrNotFound
	}
	return err
}

// CommandsDetailed returns the queue, optionally filtered by status.
func (s *Store) CommandsDetailed(ctx context.Context, status string, limit int32) ([]app.CommandDetail, error) {
	rows, err := s.pool.Query(ctx, q("ListCommandsDetailed"), status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByPos[app.CommandDetail])
}

// RetryCommand puts a command back in the queue with a fresh attempt budget.
func (s *Store) RetryCommand(ctx context.Context, id string) error {
	var got string
	err := s.pool.QueryRow(ctx, q("RetryCommand"), id).Scan(&got)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrNotFound
	}
	return err
}

// PurgeDeadCommands removes every dead command and reports the count.
func (s *Store) PurgeDeadCommands(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, q("PurgeDeadCommands"))
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
