package app

import (
	"context"

	"github.com/andranikasd/marumbot/pkg/core/date"
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

// UserLoan is one of a borrower's loans, with enough of its contract to project
// a schedule.
//
// It carries the contract rather than a stored payment figure because a
// schedule is a projection: computing it on demand from the terms and the
// anchor is what keeps one source of truth. An earlier version kept only a
// balance and a rate, which made every number the engine can produce
// unreachable from the bot.
type UserLoan struct {
	ID       string
	Name     string
	Lender   string
	Contract model.Contract
	// Balance is the latest anchor: what was owed on AsOf, by whoever said so.
	Balance money.Amount
	AsOf    date.Date
	// Trust is how the balance was established: user_entered, bank_confirmed or
	// imported_verified. Only a confirmed figure resets drift, so this decides
	// whether a number is shown as reliable or as indicative.
	Trust string
}

// Confirmed reports whether the balance came from the lender rather than the
// borrower's recollection.
func (l UserLoan) Confirmed() bool {
	return l.Trust == "bank_confirmed" || l.Trust == "imported_verified"
}

// LoanReader lists a borrower's loans.
type LoanReader interface {
	LoansForUser(ctx context.Context, userID string, limit int32) ([]UserLoan, error)
}

// Budget is how much a borrower can put towards loans each month.
type Budget struct {
	Currency string
	Monthly  money.Amount
	Set      bool
}

// BudgetStore records and reads a monthly budget.
type BudgetStore interface {
	SetBudget(ctx context.Context, userID, currency string, minor int64) error
	Budget(ctx context.Context, userID string) (Budget, error)
}
