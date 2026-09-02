package app

import (
	"context"
	"errors"

	"github.com/andranikasd/marumbot/pkg/core/allocation"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

// LoanDraft is a loan as filed by a borrower, before anything is derived from
// it. It carries the contract terms and nothing computed: a schedule is a
// projection, and storing one would create a second source of truth that can
// disagree with replay.
type LoanDraft struct {
	Icon             string
	OptionalExcluded bool
	UserID           string
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
	Icon             string
	OptionalExcluded bool
	ID               string
	Name             string
	Description      string
	Contract         model.Contract
	// Balance is the latest anchor: what was owed on AsOf, by whoever said so.
	Balance money.Amount
	// OriginalPrincipal is the earliest recorded balance: what the loan
	// started at, as far as Marum has seen. Zero when only one snapshot
	// exists. It exists so a card can show how much is already behind.
	OriginalPrincipal money.Amount
	AsOf              date.Date
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

// BudgetFunding is an explicitly declared source of funds. Amounts are minor
// units; omission preserves the pre-v2 funded-budget interpretation.
type BudgetFunding struct {
	MonthlyMinor int64             `json:"monthly_minor"`
	SpentMinor   int64             `json:"spent_minor"`
	Events       []BudgetCashEvent `json:"events"`
}

// BudgetCashEvent records a dated source declaration, not a payment.
type BudgetCashEvent struct {
	On       string `json:"on"`
	Minor    int64  `json:"minor"`
	Expected bool   `json:"expected"`
}

// Budget is how much a borrower can put towards loans each month.
type Budget struct {
	Version  int64
	Funding  *BudgetFunding
	Currency string
	Monthly  money.Amount
	// PayDay is the day of the month the money arrives, 1..31, or 0 when the
	// borrower has not said.
	PayDay int
	Set    bool
	// Opening is money on hand for loans, as stated on OpeningAsOf. Cash on
	// hand decays -- it gets spent -- so the figure only feeds a plan within
	// the month it was stated.
	Opening     money.Amount
	OpeningAsOf date.Date
	// Reserve is the part of Opening the planner must leave untouched.
	Reserve money.Amount
	// Overrides are whole-month budget figures keyed "2006-01", in minor
	// units, replacing Monthly for exactly those months.
	Overrides map[string]int64
}

// CashPlan renders the stated budget as the engine's cash plan for a
// valuation date. Opening cash counts only within the month it was stated
// and never from the future: a January figure says nothing about March.
func (b Budget) CashPlan(valuation date.Date) plan.CashPlan {
	cp := plan.CashPlan{Monthly: b.Monthly, PayDay: b.PayDay}
	if b.Opening.Sign() > 0 && !b.OpeningAsOf.IsZero() &&
		plan.MonthKey(b.OpeningAsOf) == plan.MonthKey(valuation) &&
		!b.OpeningAsOf.After(valuation) {
		cp.OpeningCash = b.Opening
		if b.Reserve.Sign() > 0 {
			if usable, err := b.Opening.Sub(b.Reserve); err == nil && usable.Sign() > 0 {
				cp.OpeningCash = usable
			} else {
				cp.OpeningCash = money.Zero(b.Opening.Currency())
			}
		}
	}
	if len(b.Overrides) > 0 {
		cur := b.Monthly.Currency()
		cp.MonthlyOverrides = make(map[string]money.Amount, len(b.Overrides))
		for k, v := range b.Overrides {
			cp.MonthlyOverrides[k] = money.FromMinor(v, cur)
		}
	}
	if b.Funding != nil {
		cp.Spending = &plan.SpendingPlan{Monthly: b.Monthly, Overrides: cp.MonthlyOverrides}
		cp.MonthlyOverrides = nil
		cp.Monthly = money.FromMinor(b.Funding.MonthlyMinor, b.Monthly.Currency())
		cp.ReserveFloor = b.Reserve
		cp.OpeningCash = money.Zero(b.Monthly.Currency())
		if !b.OpeningAsOf.IsZero() && plan.MonthKey(b.OpeningAsOf) == plan.MonthKey(valuation) && !b.OpeningAsOf.After(valuation) {
			cp.OpeningCash = b.Opening
			cp.Spending.Spent = money.FromMinor(b.Funding.SpentMinor, b.Monthly.Currency())
		}
		for _, e := range b.Funding.Events {
			on, err := date.Parse(e.On)
			if err == nil && !on.Before(valuation) {
				cp.Lumps = append(cp.Lumps, plan.CashEvent{On: on, Amount: money.FromMinor(e.Minor, b.Monthly.Currency()), Expected: e.Expected})
			}
		}
	}
	return cp
}

// BudgetStore records and reads a monthly budget.
type BudgetStore interface {
	// SetBudget records the amount; a payDay of zero keeps the stored one.
	SetBudget(ctx context.Context, userID, currency string, minor int64, payDay int) error
	Budget(ctx context.Context, userID string) (Budget, error)
}

// BudgetConfiguration is the complete budget form after boundary validation.
// Keeping it whole lets persistence commit one user action atomically.
type BudgetConfiguration struct {
	Funding         *BudgetFunding
	ExpectedVersion *int64
	UserID          string
	Currency        string
	MonthlyMinor    int64
	PayDay          int
	OpeningMinor    int64
	ReserveMinor    int64
	OpeningAsOf     date.Date
	Overrides       map[string]int64
}

// BudgetConfigurator records a complete budget form in one operation.
type BudgetConfigurator interface {
	SetBudgetConfiguration(ctx context.Context, configuration BudgetConfiguration) error
}

// Conversation states. Stored, so they are part of the schema: renaming one
// strands every row that used it.
const (
	StateAwaitingBudget = "awaiting_budget"
	// StateAwaitingBalance is set when the borrower taps "paid" on a
	// reminder; the loan id rides in the state after a colon, because the
	// answer is meaningless without knowing which loan it settles.
	StateAwaitingBalance = "awaiting_balance"
	// StateAwaitingReliefCap is set after the borrower picks "pay less per
	// month": the engine needs a target, so the bot asks for one.
	StateAwaitingReliefCap = "awaiting_relief_cap"
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

// BalanceRecorder stores the borrower's statement of what is owed, as a
// snapshot the schedule re-anchors on. Ownership is enforced in the query's
// predicate, like every loan write.
type BalanceRecorder interface {
	RecordBalance(ctx context.Context, loanID, userID string, minor int64, asOf string) error
}

// LoanFiledHook is told when a loan is created, so dependent state — the
// default reminders and their first occurrences — exists immediately.
type LoanFiledHook interface {
	OnLoanFiled(ctx context.Context, userID, loanID string) error
}

// ErrNotFound is returned when a loan does not exist, or does not belong to the
// caller. Deliberately the same error for both: telling someone a loan exists
// but is not theirs is telling them something about another account.
var ErrNotFound = errors.New("app: not found")

// ErrTooManyLoans is returned when filing a loan would exceed plan.MaxLoans.
// The planner refuses a portfolio it cannot hold in full, so the limit is
// enforced where loans enter rather than discovered when advice is asked.
var ErrTooManyLoans = errors.New("app: too many active loans")

// RequiredReader reports what a borrower's loans contractually require this
// month. The Mini App shows it beside the budget so the figure is chosen
// against a fact.
type RequiredReader interface {
	RequiredThisMonth(ctx context.Context, userID string) (money.Amount, money.Currency, error)
}

// ErrConflict means the form was based on an older aggregate version.
var ErrConflict = errors.New("app: stale version")

// LoanIcon validates an explicit choice. Names never determine an icon.
func LoanIcon(value string) (string, error) {
	switch value {
	case "":
		return "bank", nil
	case "bank", "car", "home", "phone", "document", "wallet":
		return value, nil
	default:
		return "", errors.New("app: unknown loan icon")
	}
}
