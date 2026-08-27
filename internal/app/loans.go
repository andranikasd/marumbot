package app

import (
	"context"
	"errors"

	"github.com/andranikasd/marumbot/pkg/core/allocation"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// LoanDraft is a loan as filed by a borrower, before anything is derived from
// it. It carries the contract terms and nothing computed: a schedule is a
// projection, and storing one would create a second source of truth that can
// disagree with replay.
type LoanDraft struct {
	UserID string
	// Title is what the borrower calls this loan. Marum does not ask which bank
	// issued it: the arithmetic does not need a lender's name, and a name it
	// does not need is a name it should not hold beside a balance.
	Title       string
	Description string
	Contract    model.Contract
	// Principal is the amount advanced.
	Principal money.Amount
	// Balance is what is owed on the day the loan is filed. It is the anchor the
	// schedule projects from. For a fresh loan it equals Principal; for one that
	// has been running it is lower, and the difference is what has already been
	// paid down -- which the ledger never has to reconstruct.
	Balance money.Amount
	// AsOf is the day Balance was true: today, for a loan filed by its owner.
	AsOf date.Date
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
	ID          string
	Name        string
	Description string
	Contract    model.Contract
	// Balance is the latest anchor: what was owed on AsOf, by whoever said so.
	Balance money.Amount
	AsOf    date.Date
	// Trust is how the balance was established: user_entered, bank_confirmed or
	// imported_verified. Only a confirmed figure resets drift, so this decides
	// whether a number is shown as reliable or as indicative.
	Trust string
	// Excess is the lender's rule for money paid beyond the instalment, from
	// the allocation policy the contract names. The planner credits early
	// payment only under ExcessReducePrincipal.
	Excess allocation.ExcessRule
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
	// PayDay is the day of the month the money arrives, 1..31, or 0 when the
	// borrower has not said.
	PayDay int
	Set    bool
}

// BudgetStore records and reads a monthly budget.
type BudgetStore interface {
	// SetBudget records the amount; a payDay of zero keeps the stored one.
	SetBudget(ctx context.Context, userID, currency string, minor int64, payDay int) error
	Budget(ctx context.Context, userID string) (Budget, error)
}

// Conversation states. Stored, so they are part of the schema: renaming one
// strands every row that used it.
const (
	StateAwaitingBudget = "awaiting_budget"
)

// ConversationStore remembers what the bot is waiting for.
//
// A bot that asks a question and cannot receive the answer is worse than one
// that never asks: the user replies, nothing happens, and they conclude it is
// broken. That is exactly what /budget did.
type ConversationStore interface {
	SetState(ctx context.Context, userID, state string) error
	State(ctx context.Context, userID string) (string, error)
	ClearState(ctx context.Context, userID string) error
}

// LoanEditor lets a borrower manage their own loans.
//
// Every method takes the user id and every query scopes on it, so ownership is
// enforced in the predicate rather than by a caller remembering to check. A
// mismatched id then updates nothing, instead of updating somebody else's loan.
type LoanEditor interface {
	UpdateLoan(ctx context.Context, loanID, userID, name, description string) error
	ArchiveLoan(ctx context.Context, loanID, userID string) error
	LoanForUser(ctx context.Context, loanID, userID string) (UserLoan, error)
}

// ErrNotFound is returned when a loan does not exist, or does not belong to the
// caller. Deliberately the same error for both: telling someone a loan exists
// but is not theirs is telling them something about another account.
var ErrNotFound = errors.New("app: not found")

// RequiredReader reports what a borrower's loans contractually require this
// month. The Mini App shows it beside the budget so the figure is chosen
// against a fact.
type RequiredReader interface {
	RequiredThisMonth(ctx context.Context, userID string) (money.Amount, money.Currency, error)
}
