package model

import (
	"errors"
	"fmt"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// ID is an opaque identifier. The engine never interprets it beyond equality
// and ordering, which is what makes it usable as a deterministic tie-break.
type ID string

var ErrInvalid = errors.New("invalid model value")

// RepaymentType is how principal is scheduled to come down.
type RepaymentType uint8

const (
	Annuity            RepaymentType = iota // level instalment; the MVP's only type
	DecliningPrincipal                      // equal principal, falling instalment; not yet exposed
)

func (r RepaymentType) String() string {
	if r == DecliningPrincipal {
		return "declining"
	}
	return "annuity"
}

// Contract is what the loan agreement says. It is immutable: a restructuring,
// a rate change or a maturity change creates a new version, and every event
// records the version under which it was interpreted.
type Contract struct {
	LoanID   ID
	Version  int
	Currency money.Currency

	EffectiveFrom date.Date
	EffectiveThru date.Date // zero means open-ended

	NominalRate  money.Rate
	DayCount     money.DayCount
	Type         RepaymentType
	StartDate    date.Date
	MaturityDate date.Date
	PaymentDay   int // contractual day of month, 1..31, clamped per month

	// ScheduledPayment is the contractual instalment. The zero value means the
	// borrower did not supply it and the engine must solve for it — which is a
	// different statement from an instalment of zero.
	ScheduledPayment money.Amount
	HasScheduled     bool

	Rounding money.Policy

	// AllocationPolicy identifies how this lender applies a payment across
	// buckets. The zero value is the unknown policy, which makes the engine
	// ask for a bank balance rather than derive one.
	AllocationPolicy PolicyRef
}

// PolicyRef names a versioned allocation policy.
type PolicyRef struct {
	Key     string
	Version int
}

// IsZero reports whether the reference is unset, meaning the lender's
// allocation behaviour has not been established.
func (p PolicyRef) IsZero() bool { return p.Key == "" }

func (p PolicyRef) String() string {
	if p.IsZero() {
		return "unknown/v0"
	}
	return fmt.Sprintf("%s/v%d", p.Key, p.Version)
}

// CoversDate reports whether the contract version applies on d.
func (c Contract) CoversDate(d date.Date) bool {
	if d.Before(c.EffectiveFrom) {
		return false
	}
	return c.EffectiveThru.IsZero() || !d.After(c.EffectiveThru)
}

func (c Contract) Validate() error {
	switch {
	case c.LoanID == "":
		return fmt.Errorf("%w: contract has no loan", ErrInvalid)
	case c.Version < 1:
		return fmt.Errorf("%w: contract version must be positive", ErrInvalid)
	case c.NominalRate < 0:
		return fmt.Errorf("%w: negative nominal rate", ErrInvalid)
	case c.PaymentDay < 1 || c.PaymentDay > 31:
		return fmt.Errorf("%w: payment day %d out of range", ErrInvalid, c.PaymentDay)
	case c.StartDate.IsZero() || c.MaturityDate.IsZero():
		return fmt.Errorf("%w: contract needs a start and a maturity date", ErrInvalid)
	case !c.MaturityDate.After(c.StartDate):
		return fmt.Errorf("%w: maturity %s does not follow start %s", ErrInvalid, c.MaturityDate, c.StartDate)
	case c.EffectiveFrom.IsZero():
		return fmt.Errorf("%w: contract version needs an effective date", ErrInvalid)
	case !c.EffectiveThru.IsZero() && c.EffectiveThru.Before(c.EffectiveFrom):
		return fmt.Errorf("%w: effective range ends before it starts", ErrInvalid)
	case c.Type != Annuity:
		return fmt.Errorf("%w: only annuity loans are supported", ErrInvalid)
	case c.HasScheduled && c.ScheduledPayment.Sign() <= 0:
		return fmt.Errorf("%w: scheduled payment must be positive when supplied", ErrInvalid)
	case c.Rounding.Unit < 1:
		return fmt.Errorf("%w: rounding unit must be at least 1", ErrInvalid)
	}
	return nil
}

// Trust records how much weight a snapshot carries. Only BankConfirmed resets
// drift and makes a loan fully eligible for planning.
type Trust uint8

const (
	UserEntered Trust = iota
	BankConfirmed
	ImportedVerified
)

func (t Trust) String() string {
	switch t {
	case BankConfirmed:
		return "bank_confirmed"
	case ImportedVerified:
		return "imported_verified"
	}
	return "user_entered"
}

// Snapshot is an observation of the lender's own state at the end of a stated
// business date. Marum never infers one; it is entered, confirmed or imported.
type Snapshot struct {
	ID         ID
	LoanID     ID
	ContractID int // contract version in force
	AsOf       date.Date
	Trust      Trust
	Position   Buckets

	NextInstalment       money.Amount
	NextDueDate          date.Date
	RemainingInstalments int
}

func (s Snapshot) Validate() error {
	switch {
	case s.LoanID == "":
		return fmt.Errorf("%w: snapshot has no loan", ErrInvalid)
	case s.AsOf.IsZero():
		return fmt.Errorf("%w: snapshot needs an as-of date", ErrInvalid)
	case s.RemainingInstalments < 0:
		return fmt.Errorf("%w: negative remaining instalments", ErrInvalid)
	}
	return s.Position.Validate()
}
