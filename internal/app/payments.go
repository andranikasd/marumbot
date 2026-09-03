package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/andranikasd/marumbot/pkg/core/date"
)

var (
	ErrPaymentReconciliation = errors.New("payment_reconciliation_required")
	ErrPaymentInvalid        = errors.New("invalid_payment")
	ErrPaymentDuplicate      = errors.New("possible_duplicate_payment")
)

// PaymentCommand records what happened, never an instruction to send money.
// An absent value date means bank posting is unknown. Trust is always user_entered.
type PaymentCommand struct {
	LoanID          string `json:"loan_id"`
	Key             string `json:"idempotency_key"`
	ExpectedVersion int64  `json:"expected_version"`
	AmountMinor     int64  `json:"amount_minor"`
	TransactionDate string `json:"transaction_date"`
	ValueDate       string `json:"value_date"`
	Extra           bool   `json:"extra"`
	Replaces        string `json:"replaces,omitempty"`
	VoidOnly        bool   `json:"void_only,omitempty"`
	AllowDuplicate  bool   `json:"allow_duplicate,omitempty"`
}

type PaymentReceipt struct {
	ID      string `json:"id"`
	Version int64  `json:"version"`
	Status  string `json:"status"`
}

// PaymentLoan is read under a row lock; Version is the ledger sequence.
type (
	PaymentLoan struct {
		Version  int64
		Currency string
	}
	// PaymentEntry carries one immutable source fact.
	PaymentEntry struct {
		Kind, ValueDate, TransactionDate, Voids, Hash string
		AmountMinor                                   int64
	}
)

type PaymentTransaction interface {
	LockLoan(context.Context, string, string) (PaymentLoan, error)
	Receipt(context.Context, string, string) (PaymentReceipt, string, error)
	ActiveEvent(context.Context, string, string) (bool, error)
	Duplicate(context.Context, PaymentCommand) (bool, error)
	Append(context.Context, string, string, PaymentEntry) (PaymentReceipt, error)
	Commit(context.Context) error
	Rollback(context.Context) error
}

type PaymentStore interface {
	BeginPayment(context.Context) (PaymentTransaction, error)
}

// PaymentService owns the transaction so replacement and void are inseparable.
type PaymentService struct {
	Store PaymentStore
	Clock Clock
	Users UserStore
}

func (p PaymentService) Record(ctx context.Context, userID string, c PaymentCommand) (PaymentReceipt, error) {
	today, err := p.BusinessDate(ctx, userID)
	if err != nil {
		return PaymentReceipt{}, err
	}
	if err := c.validate(today); err != nil {
		return PaymentReceipt{}, err
	}
	tx, err := p.Store.BeginPayment(ctx)
	if err != nil {
		return PaymentReceipt{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	return p.record(ctx, tx, userID, c)
}

func (p PaymentService) record(ctx context.Context, tx PaymentTransaction, userID string, c PaymentCommand) (PaymentReceipt, error) {
	empty := PaymentReceipt{}
	loan, err := tx.LockLoan(ctx, c.LoanID, userID)
	if err != nil {
		return empty, err
	}
	// A retry must succeed even after other payments changed the version.
	body, err := json.Marshal(c)
	if err != nil {
		return empty, err
	}
	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])
	old, oldHash, err := tx.Receipt(ctx, c.LoanID, c.Key)
	if err == nil {
		if oldHash != hash {
			return empty, ErrConflict
		}
		return old, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return empty, err
	}
	if loan.Version != c.ExpectedVersion {
		return empty, ErrConflict
	}
	if c.Replaces != "" {
		active, err := tx.ActiveEvent(ctx, c.LoanID, c.Replaces)
		if err != nil {
			return empty, err
		}
		if !active {
			return empty, ErrConflict
		}
	}
	if !c.VoidOnly && !c.AllowDuplicate {
		duplicate, err := tx.Duplicate(ctx, c)
		if err != nil {
			return empty, err
		}
		if duplicate {
			return empty, ErrPaymentDuplicate
		}
	}
	entry := PaymentEntry{Kind: "payment_reported", AmountMinor: c.AmountMinor, TransactionDate: c.TransactionDate, ValueDate: c.ValueDate, Hash: hash}
	if c.Extra {
		entry.Kind = "prepayment_reported"
	}
	if c.Replaces != "" {
		key := c.Key + ":void"
		if c.VoidOnly {
			key = c.Key
		}
		receipt, err := tx.Append(ctx, c.LoanID, key, PaymentEntry{Kind: "entry_voided", Voids: c.Replaces, TransactionDate: c.TransactionDate, ValueDate: c.TransactionDate, Hash: hash})
		if err != nil {
			return empty, err
		}
		if c.VoidOnly {
			return receipt, tx.Commit(ctx)
		}
	}
	receipt, err := tx.Append(ctx, c.LoanID, c.Key, entry)
	if err != nil {
		return empty, err
	}
	return receipt, tx.Commit(ctx)
}

func (c PaymentCommand) validate(today date.Date) error {
	invalid := func(s string) error { return fmt.Errorf("%w: %s", ErrPaymentInvalid, s) }
	if c.LoanID == "" || len(c.Key) < 16 || len(c.Key) > 100 || c.ExpectedVersion < 0 {
		return invalid("identity or version")
	}
	transaction, err := date.Parse(c.TransactionDate)
	if err != nil || transaction.After(today) {
		return invalid("transaction date must not be in the future")
	}
	if c.VoidOnly {
		if c.Replaces == "" || c.AmountMinor != 0 {
			return invalid("void must name a payment and carry no amount")
		}
		return nil
	}
	if c.AmountMinor <= 0 || c.AmountMinor > 9007199254740991 {
		return invalid("positive exact amount required")
	}
	if c.ValueDate != "" {
		value, err := date.Parse(c.ValueDate)
		if err != nil || value.Before(transaction) || value.After(today) {
			return invalid("value date must be between transaction date and today")
		}
	}
	return nil
}

// PaymentContext contains fresh form metadata, scoped to the borrower.
type PaymentContext struct {
	LoanID           string `json:"loan_id"`
	Loan             string `json:"loan"`
	Currency         string `json:"currency"`
	CurrencyExponent uint8  `json:"currency_exponent"`
	Today            string `json:"today"`
	Version          int64  `json:"version"`
}
type PaymentReader interface {
	PaymentContext(context.Context, string, string) (PaymentContext, error)
}

// BusinessDate uses the same account timezone as the payment form.
func (p PaymentService) BusinessDate(ctx context.Context, userID string) (date.Date, error) {
	location := time.UTC
	if p.Users != nil {
		_, zone, err := p.Users.Locale(ctx, userID)
		if err != nil {
			return date.Date{}, err
		}
		location, err = time.LoadLocation(zone)
		if err != nil {
			return date.Date{}, err
		}
	}
	return date.From(p.Clock.Now(), location), nil
}
