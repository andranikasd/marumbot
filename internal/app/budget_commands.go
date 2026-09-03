package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

// BudgetCommandStore opens a transaction for one borrower declaration.
type BudgetCommandStore interface {
	BeginBudgetCommand(context.Context) (BudgetCommandTransaction, error)
}

// BudgetCommandTransaction keeps the declaration and its retry receipt atomic.
type BudgetCommandTransaction interface {
	BudgetPolicyStore
	LoanReader
	BudgetConfigurator
	UpdateBudgetFunding(context.Context, string, BudgetFundingUpdate) error
	LockBudgetUser(context.Context, string) error
	BudgetReceipt(context.Context, string, string) (string, int64, error)
	RecordBudgetReceipt(context.Context, string, string, string, int64) error
	Commit(context.Context) error
	Rollback(context.Context) error
}

// BudgetCommands preserves command identity across lost responses and restarts.
type BudgetCommands struct {
	Store BudgetCommandStore
	Clock Clock
	Users UserStore
}

func (s BudgetCommands) execute(ctx context.Context, user, key, kind string, payload any, apply func(BudgetCommandTransaction) (int64, error)) (int64, error) {
	if len(key) < 16 || len(key) > 128 || s.Store == nil {
		return 0, ErrPaymentInvalid
	}
	raw, err := json.Marshal(struct {
		Kind    string
		Payload any
	}{kind, payload})
	if err != nil {
		return 0, err
	}
	digest := sha256.Sum256(raw)
	hash := hex.EncodeToString(digest[:])
	tx, err := s.Store.BeginBudgetCommand(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = tx.LockBudgetUser(ctx, user); err != nil {
		return 0, err
	}
	old, version, err := tx.BudgetReceipt(ctx, user, key)
	if err == nil {
		if old != hash {
			return 0, ErrConflict
		}
		return version, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return 0, err
	}
	version, err = apply(tx)
	if err != nil {
		return 0, err
	}
	if err = tx.RecordBudgetReceipt(ctx, user, key, hash, version); err != nil {
		return 0, err
	}
	return version, tx.Commit(ctx)
}

func (s BudgetCommands) SetBudgetConfiguration(ctx context.Context, c BudgetConfiguration) error {
	today, err := (PaymentService{Clock: s.Clock, Users: s.Users}).BusinessDate(ctx, c.UserID)
	if err != nil {
		return err
	}
	identity := c
	_, err = s.execute(ctx, c.UserID, c.Key, "configuration", identity, func(tx BudgetCommandTransaction) (int64, error) {
		if c.ExpectedVersion == nil || c.OpeningAsOf != today {
			return 0, ErrConflict
		}
		if c.Funding != nil {
			if !retainedCashFits(c.Funding.Events, c.OpeningMinor) {
				return 0, ErrPaymentInvalid
			}
			if err := validateCommandCash(ctx, tx, c.UserID, c.Currency, c.OpeningAsOf, c.Funding.Events); err != nil {
				return 0, err
			}
		}
		if err := tx.SetBudgetConfiguration(ctx, c); err != nil {
			return 0, err
		}
		b, err := tx.Budget(ctx, c.UserID)
		return b.Version, err
	})
	return err
}

func (s BudgetCommands) SavePolicy(ctx context.Context, user, currency string, expected int64, key string, p BudgetPolicy) (int64, error) {
	today, err := (PaymentService{Clock: s.Clock, Users: s.Users}).BusinessDate(ctx, user)
	if err != nil {
		return 0, err
	}
	return s.execute(ctx, user, key, "policy", struct {
		Currency string
		Expected int64
		Policy   BudgetPolicy
	}{currency, expected, p}, func(tx BudgetCommandTransaction) (int64, error) {
		return SaveBudgetPolicy(ctx, tx, user, currency, expected, today, p)
	})
}

func (s BudgetCommands) UpdateBudgetFunding(ctx context.Context, user string, in BudgetFundingUpdate) error {
	today, err := (PaymentService{Clock: s.Clock, Users: s.Users}).BusinessDate(ctx, user)
	if err != nil {
		return err
	}
	_, err = s.execute(ctx, user, in.Key, "funding", in, func(tx BudgetCommandTransaction) (int64, error) {
		if err := in.Validate(today); err != nil {
			return 0, err
		}
		b, err := tx.Budget(ctx, user)
		if err != nil {
			return 0, err
		}
		if !b.Set || b.Currency != in.Currency || b.Version != in.ExpectedVersion {
			return 0, ErrConflict
		}
		cash, _, err := b.CashPlans(today)
		if err != nil {
			return 0, err
		}
		if !retainedCashFits(in.Events, cash.OpeningCash.Minor()) {
			return 0, ErrPaymentInvalid
		}
		if err := validateCommandCash(ctx, tx, user, in.Currency, today, in.Events); err != nil {
			return 0, err
		}
		if err := tx.UpdateBudgetFunding(ctx, user, in); err != nil {
			return 0, err
		}
		updated, err := tx.Budget(ctx, user)
		return updated.Version, err
	})
	return err
}

func validateCommandCash(ctx context.Context, tx BudgetCommandTransaction, user, currency string, today date.Date, events []BudgetCashEvent) error {
	if err := ValidateBudgetCashEvents(events, currency, today); err != nil {
		return err
	}
	loans, err := tx.LoansForUser(ctx, user, plan.MaxLoans)
	if err != nil {
		return err
	}
	return ValidateBudgetCashTargets(events, currency, loans)
}

func retainedCashFits(events []BudgetCashEvent, available int64) bool {
	for _, event := range events {
		if event.FromOpening {
			if event.Minor < 0 || event.Minor > available {
				return false
			}
			available -= event.Minor
		}
	}
	return true
}
