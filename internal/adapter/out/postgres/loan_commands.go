package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

type loanQuery interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}
type loanCommandTx struct{ tx pgx.Tx }

func (s *Store) BeginLoanCommand(ctx context.Context) (app.LoanCommandTransaction, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return &loanCommandTx{tx: tx}, nil
}
func (t *loanCommandTx) Commit(ctx context.Context) error   { return t.tx.Commit(ctx) }
func (t *loanCommandTx) Rollback(ctx context.Context) error { return t.tx.Rollback(ctx) }
func (t *loanCommandTx) LockUser(ctx context.Context, user string) error {
	var id string
	return paymentError(t.tx.QueryRow(ctx, q("LockLoanCommandUser"), user).Scan(&id))
}

func (t *loanCommandTx) LockLoan(ctx context.Context, id, user string) (int64, error) {
	var version int64
	err := t.tx.QueryRow(ctx, q("LockLoanCommand"), id, user).Scan(&version)
	return version, paymentError(err)
}

func (t *loanCommandTx) Version(ctx context.Context, id, user string) (int64, error) {
	var version int64
	err := t.tx.QueryRow(ctx, q("LoanCommandVersion"), id, user).Scan(&version)
	return version, paymentError(err)
}

func (t *loanCommandTx) Receipt(ctx context.Context, user, key string) (app.LoanCommandReceipt, string, error) {
	var r app.LoanCommandReceipt
	var hash string
	err := t.tx.QueryRow(ctx, q("LoanCommandReceipt"), user, key).Scan(&r.ID, &r.Version, &hash)
	return r, hash, paymentError(err)
}

func (t *loanCommandTx) RecordReceipt(ctx context.Context, user, key, hash string, r app.LoanCommandReceipt) error {
	_, err := t.tx.Exec(ctx, q("RecordLoanCommandReceipt"), user, key, hash, r.ID, r.Version)
	return err
}

func (t *loanCommandTx) CreateLoan(ctx context.Context, d app.LoanDraft) (string, error) {
	return createLoan(ctx, t.tx, d)
}

func (t *loanCommandTx) LoanForUser(ctx context.Context, id, user string) (app.UserLoan, error) {
	return loanForUser(ctx, t.tx, id, user)
}

func (t *loanCommandTx) ApplyLoanRevision(ctx context.Context, id, user string, r app.LoanRevision) error {
	return applyLoanRevision(ctx, t.tx, id, user, r)
}

func (t *loanCommandTx) ArchiveLoan(ctx context.Context, id, user string) error {
	var got string
	return paymentError(t.tx.QueryRow(ctx, q("ArchiveLoanForUser"), id, user).Scan(&got))
}

func (s *Store) LoanCommandCurrency(ctx context.Context, id, user string) (money.Currency, error) {
	var code string
	if err := s.pool.QueryRow(ctx, q("LoanCommandCurrency"), id, user).Scan(&code); err != nil {
		return money.Currency{}, paymentError(err)
	}
	return money.Lookup(code)
}
