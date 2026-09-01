package plan

import (
	"errors"
	"fmt"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// Typed refusals. A caller maps each to a user message and an operational
// signal; none of them may be reported as a plan.

// ErrNoLoans means there is nothing to plan.
var ErrNoLoans = errors.New("plan: no loans")

// ErrHorizon means the loans were still open when the planning horizon
// ended. It is a finding about the inputs, not a partial answer.
var ErrHorizon = errors.New("plan: horizon exceeded with loans still open")

// ErrInvariant is an arithmetic identity failing inside the simulator. It
// must never be shown as anything but a calculation failure, and it must page.
var ErrInvariant = errors.New("plan: conservation invariant violated")

// UnsupportedError is a contract feature the engine will not guess at.
type UnsupportedError struct {
	LoanID  string
	Feature string
}

func (e *UnsupportedError) Error() string {
	return fmt.Sprintf("plan: loan %s: unsupported: %s", e.LoanID, e.Feature)
}

// InfeasibleError is a required payment the cash could not meet. It carries
// the first failing date and the exact gap so the borrower can be told what
// would have to change.
type InfeasibleError struct {
	On        date.Date
	LoanID    string
	Required  money.Amount
	Available money.Amount
	Shortfall money.Amount
}

func (e *InfeasibleError) Error() string {
	return fmt.Sprintf("plan: %s: %s requires %s, only %s available (short %s)",
		e.On, e.LoanID, e.Required, e.Available, e.Shortfall)
}

// StaleBalanceError refuses to plan on a balance so old that the plan would
// rest mostly on assumed payments. A plan is only as true as its anchor, and
// past a few assumed instalments the honest answer is "confirm the balance",
// not a recommendation built on payments nobody has verified.
type StaleBalanceError struct {
	LoanID  string
	AsOf    date.Date
	Assumed int
}

func (e *StaleBalanceError) Error() string {
	return fmt.Sprintf("plan: loan %s: balance anchored %s needs %d assumed instalments; confirm the balance",
		e.LoanID, e.AsOf, e.Assumed)
}

// MixedCurrencyError refuses a portfolio the engine cannot value in one unit.
type MixedCurrencyError struct{ Have, Want string }

func (e *MixedCurrencyError) Error() string {
	return fmt.Sprintf("plan: loan in %s cannot be planned with a %s budget", e.Have, e.Want)
}

// TruncatedError refuses a portfolio the caller could not fully load.
type TruncatedError struct{ Max int }

func (e *TruncatedError) Error() string {
	return fmt.Sprintf("plan: more than %d active loans; refusing to plan an incomplete portfolio", e.Max)
}
