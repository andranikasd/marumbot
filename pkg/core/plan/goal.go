package plan

import (
	"fmt"

	"github.com/andranikasd/marumbot/pkg/core/money"
)

// GoalKind is what the borrower is optimising for. Each kind has a written
// lexicographic comparator in better(); there is no universal "best".
type GoalKind uint8

const (
	// LeastInterest minimises interest plus fees with the budget kept, then
	// pays off earliest, then makes fewer payments.
	LeastInterest GoalKind = iota
	// Fastest pays off on the earliest exact date, then cheapest, then with
	// the lowest peak required outflow.
	Fastest
	// Relief reaches a required-payment target soonest: either the required
	// total falls to at most Cap, or at least Free per month is released
	// against today's required total. Freed instalments are kept, not
	// redeployed. Then cheapest.
	Relief
	// FirstWin closes any one loan on the earliest exact date, then cheapest.
	FirstWin
)

func (k GoalKind) String() string {
	switch k {
	case Fastest:
		return "fastest"
	case Relief:
		return "relief"
	case FirstWin:
		return "first_win"
	default:
		return "least_interest"
	}
}

// Goal is a kind with its parameters.
type Goal struct {
	Kind GoalKind
	// Cap is the required monthly total the borrower wants to get under.
	Cap money.Amount
	// Free is how much of the required monthly total they want released.
	// Exactly one of Cap and Free must be set for Relief.
	Free money.Amount
}

// String names a goal for logs and certificates.
func (g Goal) String() string {
	switch {
	case g.Kind == Relief && g.Cap.Sign() > 0:
		return fmt.Sprintf("relief(cap=%s)", g.Cap)
	case g.Kind == Relief && g.Free.Sign() > 0:
		return fmt.Sprintf("relief(free=%s)", g.Free)
	default:
		return g.Kind.String()
	}
}

// Validate refuses an underspecified goal.
func (g Goal) Validate() error {
	if g.Kind == Relief && g.Cap.Sign() <= 0 && g.Free.Sign() <= 0 {
		return fmt.Errorf("plan: relief needs a cap or an amount to free")
	}
	return nil
}
