package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/andranikasd/marumbot/internal/app"
)

type budgetQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}
type budgetCommandTx struct{ tx pgx.Tx }

func (s *Store) BeginBudgetCommand(ctx context.Context) (app.BudgetCommandTransaction, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return &budgetCommandTx{tx}, nil
}
func (t *budgetCommandTx) Commit(ctx context.Context) error   { return t.tx.Commit(ctx) }
func (t *budgetCommandTx) Rollback(ctx context.Context) error { return t.tx.Rollback(ctx) }
func (t *budgetCommandTx) LockBudgetUser(ctx context.Context, user string) error {
	var id string
	return paymentError(t.tx.QueryRow(ctx, q("LockPlanUser"), user).Scan(&id))
}

func (t *budgetCommandTx) BudgetReceipt(ctx context.Context, user, key string) (string, int64, error) {
	var hash string
	var version int64
	err := t.tx.QueryRow(ctx, q("BudgetCommandReceipt"), user, key).Scan(&hash, &version)
	return hash, version, paymentError(err)
}

func (t *budgetCommandTx) RecordBudgetReceipt(ctx context.Context, user, key, hash string, version int64) error {
	_, err := t.tx.Exec(ctx, q("InsertBudgetCommandReceipt"), user, key, hash, version)
	return err
}

func (t *budgetCommandTx) Budget(ctx context.Context, user string) (app.Budget, error) {
	return budget(ctx, t.tx, user)
}

func (t *budgetCommandTx) SetBudget(ctx context.Context, user, currency string, minor int64, day int) error {
	var n int64
	err := t.tx.QueryRow(ctx, q("SetBudget"), user, currency, minor, day).Scan(&n)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrConflict
	}
	return err
}

func (t *budgetCommandTx) SetBudgetConfiguration(ctx context.Context, c app.BudgetConfiguration) error {
	return setBudgetConfiguration(ctx, t.tx, c)
}

func (t *budgetCommandTx) AppendBudgetPolicy(ctx context.Context, user, currency string, version int64, p app.BudgetPolicy) (int64, error) {
	return appendBudgetPolicy(ctx, t.tx, user, currency, version, p)
}

func (t *budgetCommandTx) UpdateBudgetFunding(ctx context.Context, user string, in app.BudgetFundingUpdate) error {
	raw, err := json.Marshal(in.Events)
	if err != nil {
		return err
	}
	var version int64
	err = t.tx.QueryRow(ctx, q("UpdateBudgetFunding"), user, in.Currency, in.ExpectedVersion, in.PayDay, in.MonthlyMinor, string(raw)).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrConflict
	}
	return err
}

func (t *budgetCommandTx) LoansForUser(ctx context.Context, user string, limit int32) ([]app.UserLoan, error) {
	return loansForUser(ctx, t.tx, user, limit)
}
