package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/andranikasd/marumbot/internal/app"
)

func (p *paymentTx) ReconciliationReceipt(ctx context.Context, loanID, key string) (app.PaymentReceipt, string, error) {
	var r app.PaymentReceipt
	var hash string
	err := p.tx.QueryRow(ctx, q("ReconciliationReceipt"), loanID, "reconcile:"+loanID+":"+key).Scan(&r.ID, &r.Version, &hash)
	r.Status = "reconciled"
	return r, hash, paymentError(err)
}

func (p *paymentTx) ReconciliationState(ctx context.Context, userID string, c app.ReconciliationCommand) (app.ReconciliationState, error) {
	var out app.ReconciliationState
	err := p.tx.QueryRow(ctx, q("ReconciliationEligibility"), c.LoanID, c.AsOf, nullableText(c.NextDue)).Scan(&out.Posted, &out.Contract)
	if err != nil {
		return out, err
	}
	loan, err := p.LockLoan(ctx, c.LoanID, userID)
	if err != nil {
		return out, err
	}
	err = p.tx.QueryRow(ctx, q("PeriodReportedSpending"), userID, loan.Currency, c.AsOf).Scan(&out.ReportedMinor)
	return out, err
}

func (p *paymentTx) Reconcile(ctx context.Context, userID string, c app.ReconciliationCommand, hash string) (app.PaymentReceipt, error) {
	var r app.PaymentReceipt
	loan, err := p.LockLoan(ctx, c.LoanID, userID)
	if err != nil {
		return r, err
	}
	var budgetVersion int64
	// Loan is locked first for every payment command; the budget update also
	// serializes different loans sharing the same account/currency declaration.
	err = p.tx.QueryRow(ctx, q("ReconciliationBudget"), userID, loan.Currency, c.AsOf, c.CashMinor, c.SpentMinor, c.BudgetVersion).Scan(&budgetVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, app.ErrConflict
	}
	if err != nil {
		return r, err
	}
	err = p.tx.QueryRow(ctx, q("ReconcilePaymentSnapshot"), c.LoanID, uuid.NewString(), c.AsOf, c.PrincipalMinor, nullableText(c.NextDue), c.NextPaymentMinor, "reconcile:"+c.LoanID+":"+c.Key, hash).Scan(&r.ID, &r.Version)
	r.Status = "reconciled"
	return r, paymentError(err)
}
