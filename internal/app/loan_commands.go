package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// LoanCommandStore starts the transaction owned by the borrower use case.
type LoanCommandStore interface {
	LoanCommandCurrency(context.Context, string, string) (money.Currency, error)
	BeginLoanCommand(context.Context) (LoanCommandTransaction, error)
}

// LoanCommandTransaction serializes commands before reading their receipts or facts.
type LoanCommandTransaction interface {
	LockUser(context.Context, string) error
	LockLoan(context.Context, string, string) (int64, error)
	Receipt(context.Context, string, string) (LoanCommandReceipt, string, error)
	RecordReceipt(context.Context, string, string, string, LoanCommandReceipt) error
	CreateLoan(context.Context, LoanDraft) (string, error)
	LoanForUser(context.Context, string, string) (UserLoan, error)
	ApplyLoanRevision(context.Context, string, string, LoanRevision) error
	ArchiveLoan(context.Context, string, string) error
	Version(context.Context, string, string) (int64, error)
	Commit(context.Context) error
	Rollback(context.Context) error
}

// LoanCommandReceipt survives a lost response, even after the loan is archived.
type LoanCommandReceipt struct {
	ID      string `json:"id"`
	Version int64  `json:"version"`
}

type LoanCommands struct {
	Store LoanCommandStore
	Clock Clock
	Users UserStore
}

func (s LoanCommands) execute(ctx context.Context, user, key, kind string, payload any, apply func(LoanCommandTransaction) (LoanCommandReceipt, error)) (LoanCommandReceipt, error) {
	if len(key) < 16 || len(key) > 128 || s.Store == nil {
		return LoanCommandReceipt{}, ErrPaymentInvalid
	}
	raw, err := json.Marshal(struct {
		Kind    string
		Payload any
	}{kind, payload})
	if err != nil {
		return LoanCommandReceipt{}, err
	}
	digest := sha256.Sum256(raw)
	hash := hex.EncodeToString(digest[:])
	tx, err := s.Store.BeginLoanCommand(ctx)
	if err != nil {
		return LoanCommandReceipt{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = tx.LockUser(ctx, user); err != nil {
		return LoanCommandReceipt{}, err
	}
	receipt, old, err := tx.Receipt(ctx, user, key)
	if err == nil {
		if old != hash {
			return LoanCommandReceipt{}, ErrConflict
		}
		return receipt, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return LoanCommandReceipt{}, err
	}
	receipt, err = apply(tx)
	if err != nil {
		return LoanCommandReceipt{}, err
	}
	if err = tx.RecordReceipt(ctx, user, key, hash, receipt); err != nil {
		return LoanCommandReceipt{}, err
	}
	return receipt, tx.Commit(ctx)
}

func (s LoanCommands) Create(ctx context.Context, key string, d LoanDraft) (LoanCommandReceipt, error) {
	identity := d
	// Filing date is assigned by the server; a retry tomorrow is the same command.
	identity.AsOf = date.Date{}
	return s.execute(ctx, d.UserID, key, "create", identity, func(tx LoanCommandTransaction) (LoanCommandReceipt, error) {
		id, err := tx.CreateLoan(ctx, d)
		if err != nil {
			return LoanCommandReceipt{}, err
		}
		version, err := tx.Version(ctx, id, d.UserID)
		return LoanCommandReceipt{ID: id, Version: version}, err
	})
}

func (s LoanCommands) Revise(ctx context.Context, user, id, key string, expected int64, edit LoanEdit) (LoanCommandReceipt, error) {
	today, err := (PaymentService{Clock: s.Clock, Users: s.Users}).BusinessDate(ctx, user)
	if err != nil {
		return LoanCommandReceipt{}, err
	}

	return s.execute(ctx, user, key, "revise", struct {
		ID       string
		Expected int64
		Edit     LoanEdit
	}{id, expected, edit}, func(tx LoanCommandTransaction) (LoanCommandReceipt, error) {
		if err := lockLoanCommand(ctx, tx, id, user, expected); err != nil {
			return LoanCommandReceipt{}, err
		}
		ln, err := tx.LoanForUser(ctx, id, user)
		if err != nil {
			return LoanCommandReceipt{}, err
		}
		revision, err := prepareLoanRevision(ln, edit, today)
		if err != nil {
			return LoanCommandReceipt{}, err
		}
		if err = tx.ApplyLoanRevision(ctx, id, user, revision); err != nil {
			return LoanCommandReceipt{}, err
		}
		version, err := tx.Version(ctx, id, user)
		return LoanCommandReceipt{ID: id, Version: version}, err
	})
}

func (s LoanCommands) Rename(ctx context.Context, user, id, key string, expected int64, name, description string) (LoanCommandReceipt, error) {
	today, err := (PaymentService{Clock: s.Clock, Users: s.Users}).BusinessDate(ctx, user)
	if err != nil {
		return LoanCommandReceipt{}, err
	}

	return s.execute(ctx, user, key, "rename", struct {
		ID                string
		Expected          int64
		Name, Description string
	}{id, expected, name, description}, func(tx LoanCommandTransaction) (LoanCommandReceipt, error) {
		if err := lockLoanCommand(ctx, tx, id, user, expected); err != nil {
			return LoanCommandReceipt{}, err
		}
		if err := tx.ApplyLoanRevision(ctx, id, user, LoanRevision{Name: name, Description: description, Rename: true, EffectiveFrom: today}); err != nil {
			return LoanCommandReceipt{}, err
		}
		version, err := tx.Version(ctx, id, user)
		return LoanCommandReceipt{ID: id, Version: version}, err
	})
}

func (s LoanCommands) Archive(ctx context.Context, user, id, key string, expected int64) (LoanCommandReceipt, error) {
	return s.execute(ctx, user, key, "archive", struct {
		ID       string
		Expected int64
	}{id, expected}, func(tx LoanCommandTransaction) (LoanCommandReceipt, error) {
		if err := lockLoanCommand(ctx, tx, id, user, expected); err != nil {
			return LoanCommandReceipt{}, err
		}
		if err := tx.ArchiveLoan(ctx, id, user); err != nil {
			return LoanCommandReceipt{}, err
		}
		version, err := tx.Version(ctx, id, user)
		return LoanCommandReceipt{ID: id, Version: version}, err
	})
}

func lockLoanCommand(ctx context.Context, tx LoanCommandTransaction, id, user string, expected int64) error {
	version, err := tx.LockLoan(ctx, id, user)
	if err != nil {
		return err
	}
	if expected < 1 || expected != version {
		return ErrConflict
	}
	return nil
}

// Currency is only a decoding aid. Ownership remains mandatory, but archive
// must not hide the immutable currency needed to retry a committed command.
func (s LoanCommands) Currency(ctx context.Context, id, user string) (money.Currency, error) {
	return s.Store.LoanCommandCurrency(ctx, id, user)
}
