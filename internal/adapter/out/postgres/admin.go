package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/andranikasd/marumbot/internal/app"
)

// Overview is the dashboard's snapshot of the whole system.

func (s *Store) Overview(ctx context.Context) (app.Overview, error) {
	var o app.Overview
	err := s.pool.QueryRow(ctx, q("CountsOverview")).Scan(
		&o.Users, &o.Loans, &o.Events, &o.Snapshots, &o.Policies,
		&o.CommandsPending, &o.CommandsDead,
		&o.DeliveriesPending, &o.DeliveriesDead,
		&o.OccurrencesScheduled, &o.OldestCommandAgeS, &o.OldestDeliveryAgeS)
	return o, err
}

func (s *Store) ListUsers(ctx context.Context, limit int32) ([]app.UserRow, error) {
	rows, err := s.pool.Query(ctx, q("ListUsers"), limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[app.UserRow])
}

func (s *Store) ListLoans(ctx context.Context, limit int32) ([]app.LoanRow, error) {
	rows, err := s.pool.Query(ctx, q("ListLoans"), limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[app.LoanRow])
}

func (s *Store) GetLoan(ctx context.Context, id string) (app.LoanDetail, error) {
	rows, err := s.pool.Query(ctx, q("GetLoan"), id)
	if err != nil {
		return app.LoanDetail{}, err
	}
	return pgx.CollectExactlyOneRow(rows, pgx.RowToStructByPos[app.LoanDetail])
}

func (s *Store) ContractsForLoan(ctx context.Context, loanID string) ([]app.ContractRow, error) {
	rows, err := s.pool.Query(ctx, q("ListContractsForLoan"), loanID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[app.ContractRow])
}

func (s *Store) SnapshotsForLoan(ctx context.Context, loanID string) ([]app.SnapshotRow, error) {
	rows, err := s.pool.Query(ctx, q("ListSnapshotsForLoan"), loanID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[app.SnapshotRow])
}

func (s *Store) EventsForLoan(ctx context.Context, loanID string) ([]app.EventRow, error) {
	rows, err := s.pool.Query(ctx, q("ListEventsForLoan"), loanID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[app.EventRow])
}

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

func (s *Store) ListCommands(ctx context.Context, limit int32) ([]app.CommandRow, error) {
	rows, err := s.pool.Query(ctx, q("ListCommands"), limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[app.CommandRow])
}

func (s *Store) ListDeliveries(ctx context.Context, limit int32) ([]app.DeliveryRow, error) {
	rows, err := s.pool.Query(ctx, q("ListDeliveries"), limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[app.DeliveryRow])
}

func (s *Store) ListReconciliationRuns(ctx context.Context, limit int32) ([]app.ReconRow, error) {
	rows, err := s.pool.Query(ctx, q("ListReconciliationRuns"), limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[app.ReconRow])
}
