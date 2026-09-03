package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

type paymentTx struct{ tx pgx.Tx }

func (s *Store) BeginPayment(ctx context.Context) (app.PaymentTransaction, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return &paymentTx{tx: tx}, nil
}
func (p *paymentTx) Commit(ctx context.Context) error   { return p.tx.Commit(ctx) }
func (p *paymentTx) Rollback(ctx context.Context) error { return p.tx.Rollback(ctx) }
func (p *paymentTx) LockLoan(ctx context.Context, loanID, userID string) (app.PaymentLoan, error) {
	var loan app.PaymentLoan
	err := p.tx.QueryRow(ctx, q("LockPaymentLoan"), loanID, userID).Scan(&loan.Version, &loan.Currency)
	return loan, paymentError(err)
}

func (p *paymentTx) Receipt(ctx context.Context, loanID, key string) (app.PaymentReceipt, string, error) {
	var r app.PaymentReceipt
	var hash string
	err := p.tx.QueryRow(ctx, q("PaymentReceipt"), loanID, loanID+":"+key).Scan(&r.ID, &r.Version, &r.Status, &hash)
	return r, hash, paymentError(err)
}

func (p *paymentTx) ActiveEvent(ctx context.Context, loanID, eventID string) (bool, error) {
	var active bool
	err := p.tx.QueryRow(ctx, q("ActivePaymentEvent"), loanID, eventID).Scan(&active)
	return active, err
}

func (p *paymentTx) Duplicate(ctx context.Context, c app.PaymentCommand) (bool, error) {
	kind := "payment_reported"
	if c.Extra {
		kind = "prepayment_reported"
	}
	var duplicate bool
	err := p.tx.QueryRow(ctx, q("DuplicatePayment"), c.LoanID, c.AmountMinor, c.TransactionDate, nullableText(c.ValueDate), kind, c.Replaces).Scan(&duplicate)
	return duplicate, err
}

func (p *paymentTx) Append(ctx context.Context, loanID, key string, e app.PaymentEntry) (app.PaymentReceipt, error) {
	payload, err := paymentPayload(e)
	if err != nil {
		return app.PaymentReceipt{}, err
	}
	var r app.PaymentReceipt
	contractDate := e.ValueDate
	if contractDate == "" {
		contractDate = e.TransactionDate
	}
	err = p.tx.QueryRow(ctx, q("AppendPayment"), loanID, uuid.NewString(), e.Kind, nullableText(e.ValueDate), contractDate, e.AmountMinor, nullableText(e.Voids), loanID+":"+key, payload).Scan(&r.ID, &r.Version)
	r.Status = "needs_reconciliation"
	if e.ValueDate == "" {
		r.Status = "pending_bank_posting"
	}
	if e.Kind == "entry_voided" {
		r.Status = "voided"
	}
	return r, paymentError(err)
}

func nullableText(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func paymentError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrNotFound
	}
	return err
}

func (s *Store) PaymentContext(ctx context.Context, loanID, userID string) (app.PaymentContext, error) {
	var out app.PaymentContext
	err := s.pool.QueryRow(ctx, q("PaymentContext"), loanID, userID).Scan(&out.LoanID, &out.Loan, &out.Currency, &out.Version)
	if err != nil {
		return out, paymentError(err)
	}
	cur, err := money.Lookup(out.Currency)
	if err != nil {
		return out, err
	}
	out.CurrencyExponent = cur.Exponent
	return out, nil
}

func paymentPayload(e app.PaymentEntry) ([]byte, error) {
	return json.Marshal(struct {
		TransactionDate string                 `json:"transaction_date"`
		Trust           string                 `json:"trust"`
		RequestHash     string                 `json:"request_hash"`
		Allocation      *app.PaymentAllocation `json:"allocation,omitempty"`
	}{e.TransactionDate, "user_entered", e.Hash, e.Allocation})
}
