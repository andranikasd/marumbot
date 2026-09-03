package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/andranikasd/marumbot/pkg/core/date"
)

// ReconciliationCommand contains new source statements, never inferred allocation.
// Cash and spending are whole-account figures AFTER all payments, not deltas.
type ReconciliationCommand struct {
	SpentPeriodStart string `json:"spent_period_start,omitempty"`
	LoanID           string `json:"loan_id"`
	Key              string `json:"idempotency_key"`
	ExpectedVersion  int64  `json:"expected_version"`
	BudgetVersion    int64  `json:"budget_version"`
	AsOf             string `json:"as_of"`
	PrincipalMinor   int64  `json:"principal_minor"`
	NextDue          string `json:"next_due"`
	NextPaymentMinor int64  `json:"next_payment_minor"`
	CashMinor        int64  `json:"cash_minor"`
	SpentMinor       int64  `json:"spent_minor"`
	IncludePosted    bool   `json:"include_posted"`
}

type ReconciliationState struct {
	PeriodStart      string
	Posted, Contract bool
	ReportedMinor    int64
}

type ReconciliationTransaction interface {
	ReconciliationState(context.Context, string, ReconciliationCommand) (ReconciliationState, error)
	PaymentTransaction
	ReconciliationReceipt(context.Context, string, string) (PaymentReceipt, string, error)
	Reconcile(context.Context, string, ReconciliationCommand, string) (PaymentReceipt, error)
}

func (p PaymentService) Reconcile(ctx context.Context, userID string, c ReconciliationCommand) (PaymentReceipt, error) {
	empty := PaymentReceipt{}
	today, err := p.BusinessDate(ctx, userID)
	if err != nil {
		return empty, err
	}
	if err = c.validate(today); err != nil {
		return empty, err
	}
	base, err := p.Store.BeginPayment(ctx)
	if err != nil {
		return empty, err
	}
	defer func() { _ = base.Rollback(ctx) }()
	tx, ok := base.(ReconciliationTransaction)
	if !ok {
		return empty, ErrPaymentReconciliation
	}
	loan, err := tx.LockLoan(ctx, c.LoanID, userID)
	if err != nil {
		return empty, err
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return empty, err
	}
	sum := sha256.Sum256(raw)
	hash := hex.EncodeToString(sum[:])
	old, oldHash, err := tx.ReconciliationReceipt(ctx, c.LoanID, c.Key)
	if err == nil {
		if oldHash != hash {
			return empty, ErrConflict
		}
		return old, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return empty, err
	}
	if c.AsOf != today.String() {
		return empty, ErrPaymentInvalid
	}
	if loan.Version != c.ExpectedVersion {
		return empty, ErrConflict
	}
	state, err := tx.ReconciliationState(ctx, userID, c)
	if err != nil {
		return empty, err
	}
	if !state.Posted || !state.Contract {
		return empty, ErrPaymentReconciliation
	}
	if state.PeriodStart != "" {
		calendar := date.OnDayOfMonth(today, 1).String()
		if (c.SpentPeriodStart != "" && c.SpentPeriodStart != state.PeriodStart) || (c.SpentPeriodStart == "" && state.PeriodStart != calendar) {
			return empty, ErrPaymentInvalid
		}
	}
	if c.SpentMinor < state.ReportedMinor {
		return empty, ErrPaymentInvalid
	}
	receipt, err := tx.Reconcile(ctx, userID, c, hash)
	if err != nil {
		return empty, err
	}
	return receipt, tx.Commit(ctx)
}

func (c ReconciliationCommand) validate(today date.Date) error {
	invalid := func() error {
		return fmt.Errorf("%w: reconciliation requires today's balance, next obligation and post-payment cash totals", ErrPaymentInvalid)
	}
	if c.LoanID == "" || len(c.Key) < 16 || len(c.Key) > 100 || c.ExpectedVersion < 0 || c.BudgetVersion < 1 || !c.IncludePosted {
		return invalid()
	}
	asOf, err := date.Parse(c.AsOf)
	if err != nil || asOf.After(today) {
		return invalid()
	}
	if c.SpentPeriodStart != "" {
		start, err := date.Parse(c.SpentPeriodStart)
		if err != nil || start.After(asOf) {
			return invalid()
		}
	}
	for _, n := range []int64{c.PrincipalMinor, c.NextPaymentMinor, c.CashMinor, c.SpentMinor} {
		if n < 0 || n > 9007199254740991 {
			return invalid()
		}
	}
	if c.PrincipalMinor == 0 {
		if c.NextPaymentMinor != 0 || c.NextDue != "" {
			return invalid()
		}
		return nil
	}
	next, err := date.Parse(c.NextDue)
	if err != nil || !next.After(asOf) || c.NextPaymentMinor == 0 {
		return invalid()
	}
	return nil
}
