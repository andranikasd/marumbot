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

// ErrInvalid is returned by Validate for a value the engine must not accept.
var ErrInvalid = errors.New("invalid model value")

// RepaymentType is how principal is scheduled to come down.
type RepaymentType uint8

// The repayment types the engine understands.
const (
	Annuity            RepaymentType = iota // level instalment
	DecliningPrincipal                      // equal principal, falling instalment; not yet exposed
)

func (r RepaymentType) String() string {
	if r == DecliningPrincipal {
		return "declining"
	}
	return "annuity"
}

// PrepaymentEffect is what a partial early repayment does to the schedule.
//
// Armenian lenders offer both readings and usually let the borrower choose
// at the counter. The difference matters to a plan: keeping the instalment
// shortens the loan, lowering the instalment keeps the term and frees cash.
type PrepaymentEffect uint8

const (
	// PrepayBorrowerChooses is the default: the contract does not fix the
	// effect, so the planner considers both and says which it assumed.
	PrepayBorrowerChooses PrepaymentEffect = iota
	// PrepayShortenTerm keeps the instalment; the loan ends sooner.
	PrepayShortenTerm
	// PrepayReduceInstalment keeps the maturity; the instalment is re-solved
	// on the lower balance.
	PrepayReduceInstalment
)

func (e PrepaymentEffect) String() string {
	switch e {
	case PrepayShortenTerm:
		return "shorten_term"
	case PrepayReduceInstalment:
		return "reduce_instalment"
	default:
		return "borrower_chooses"
	}
}

// ParsePrepaymentEffect reads a persisted effect. An empty string is the
// default, not an error: contracts filed before the field existed have it.
func ParsePrepaymentEffect(s string) (PrepaymentEffect, error) {
	switch s {
	case "", "borrower_chooses":
		return PrepayBorrowerChooses, nil
	case "shorten_term":
		return PrepayShortenTerm, nil
	case "reduce_instalment":
		return PrepayReduceInstalment, nil
	}
	return PrepayBorrowerChooses, fmt.Errorf("%w: unknown prepayment effect %q", ErrInvalid, s)
}

// PrepaymentCharge is one dated rule for what an early payment costs.
//
// Consumer credit in Armenia carries no charge by law. Residential mortgage
// credit may carry a capped percentage in the first contract years, and
// some products add a fixed transfer commission. A rule applies from
// FromYear through ThroughYear of the contract (1-based; zero means
// unbounded), charges PercentBP of the principal credited beyond the year's
// FreeAllowance plus Fixed, and is clamped to [MinCharge, MaxCharge] when
// those are set.
type PrepaymentCharge struct {
	FromYear      int
	ThroughYear   int
	PercentBP     int64
	Fixed         money.Amount
	FreeAllowance money.Amount // per contract year, zero means none
	MinCharge     money.Amount
	MaxCharge     money.Amount
}

// Prepayment is the contract's terms for paying early.
type Prepayment struct {
	Effect PrepaymentEffect
	// FeeBP is a flat fee on the prepaid amount in basis points, kept for
	// contracts recorded before dated charge rules existed. When Charges is
	// non-empty it is ignored.
	FeeBP int
	// Charges are the dated rules; empty means free.
	Charges []PrepaymentCharge
	// MinAmount is the smallest optional payment the lender accepts; zero
	// means any.
	MinAmount money.Amount
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

	// NotBeforeDue is the bank-reported earliest outstanding instalment date.
	// Zero retains the original schedule behaviour.
	NotBeforeDue date.Date

	// ScheduledPayment is the contractual instalment. The zero value means the
	// borrower did not supply it and the engine must solve for it — which is a
	// different statement from an instalment of zero.
	ScheduledPayment money.Amount
	HasScheduled     bool

	Rounding money.Policy

	// Prepayment is what an early payment does to the schedule and costs.
	Prepayment Prepayment

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

// Validate rejects a contract the engine must not compute with.
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
	case c.Type != Annuity && c.Type != DecliningPrincipal:
		return fmt.Errorf("%w: unsupported repayment type", ErrInvalid)
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

// How much weight a snapshot carries.
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

// Validate rejects a snapshot that cannot be an anchor.
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
