package app

import (
	"context"

	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// LoanDraft is a loan as filed by a borrower, before anything is derived from
// it. It carries the contract terms and nothing computed: a schedule is a
// projection, and storing one would create a second source of truth that can
// disagree with replay.
type LoanDraft struct {
	UserID   string
	Lender   string
	Contract model.Contract
	// Principal is the amount advanced. It becomes the opening balance of the
	// ledger rather than a field on the loan, so every later figure is derived
	// from events instead of from a number somebody typed once.
	Principal money.Amount
}

// LoanWriter records loans. Declared by the consumer.
type LoanWriter interface {
	CreateLoan(ctx context.Context, d LoanDraft) (id string, err error)
}

// UserLoan is one of a borrower's loans, as the bot lists it.
//
// The figures are what was filed or last confirmed, not a projection. A
// schedule is computed on demand from the contract and the anchor, because
// storing one would create a second source of truth that can disagree with
// replay.
type UserLoan struct {
	ID           string
	Name         string
	Lender       string
	Currency     string
	Balance      money.Amount
	Rate         string
	Method       string
	MaturityDate string
	PaymentDay   int
	// Trust is how the balance was established: user_entered, bank_confirmed or
	// imported_verified. Only a confirmed figure resets drift, so this is what
	// decides whether a number is shown as reliable or as indicative.
	Trust string
}

// LoanReader lists a borrower's loans.
type LoanReader interface {
	LoansForUser(ctx context.Context, userID string, limit int32) ([]UserLoan, error)
}
