package amortisation

import (
	"fmt"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// ProjectDeclining builds an equal-principal schedule: the same amount comes off
// the balance every period, so the interest and therefore the instalment fall
// over the life of the loan.
//
// Armenian lenders offer this alongside the annuity and call it դիֆերենցված.
// It is not a variation on the annuity, it is the other way round: here the
// principal is fixed and the payment is derived, where an annuity fixes the
// payment and derives the principal. Nothing needs solving, so there is no
// bisection — the whole schedule follows from the arithmetic.
//
// It costs less in total interest than an annuity over the same term, because
// the balance falls faster, and it demands more in the early months. Which of
// those matters is the borrower's decision, not the engine's.
func ProjectDeclining(c model.Contract, principal money.Amount, from date.Date) (Schedule, error) {
	if principal.Sign() <= 0 {
		return Schedule{}, fmt.Errorf("%w: principal must be positive", ErrUnsolvable)
	}
	dates, err := PaymentDates(c)
	if err != nil {
		return Schedule{}, err
	}

	cur := principal.Currency()
	unit := c.Rounding.Unit
	if unit <= 0 {
		unit = 1
	}
	n := int64(len(dates))

	// Round the per-period principal DOWN to the settlement unit. Rounding up
	// would retire the loan early and leave the last row negative; rounding down
	// leaves a remainder that the final row absorbs, which is what lenders do.
	per := money.FromMinor(principal.Minor()/n/unit*unit, cur)

	s := Schedule{
		Rows:          make([]Row, 0, len(dates)),
		TotalPaid:     money.Zero(cur),
		TotalInterest: money.Zero(cur),
	}
	balance := principal
	prev := from
	if prev.IsZero() {
		prev = c.StartDate
	}

	for i, due := range dates {
		days := date.DaysBetween(prev, due)
		if days < 0 {
			return Schedule{}, fmt.Errorf("%w: instalment %s precedes %s", ErrUnsolvable, due, prev)
		}
		interest, err := money.Accrue(balance, c.NominalRate, int64(days), c.DayCount, c.Rounding)
		if err != nil {
			return Schedule{}, fmt.Errorf("amortisation: row %d: %w", i+1, err)
		}

		// The final row clears whatever is left, which is the rounded-down
		// remainder plus any residue.
		principalPart := per
		if i == len(dates)-1 || balance.Cmp(per) < 0 {
			principalPart = balance
		}
		payment, err := principalPart.Add(interest)
		if err != nil {
			return Schedule{}, fmt.Errorf("amortisation: row %d: %w", i+1, err)
		}
		closing, err := balance.Sub(principalPart)
		if err != nil {
			return Schedule{}, fmt.Errorf("amortisation: row %d: %w", i+1, err)
		}

		s.Rows = append(s.Rows, Row{
			N: i + 1, Due: due, Days: days,
			Opening: balance, Interest: interest,
			Principal: principalPart, Payment: payment, Closing: closing,
		})
		if s.TotalPaid, err = s.TotalPaid.Add(payment); err != nil {
			return Schedule{}, fmt.Errorf("amortisation: row %d: %w", i+1, err)
		}
		if s.TotalInterest, err = s.TotalInterest.Add(interest); err != nil {
			return Schedule{}, fmt.Errorf("amortisation: row %d: %w", i+1, err)
		}

		balance = closing
		prev = due
		if balance.Sign() == 0 {
			break
		}
	}

	if n := len(s.Rows); n > 0 {
		// There is no level instalment here. Instalment reports the first and
		// largest payment, which is the figure a borrower has to be able to
		// afford, and FinalPayment the smallest.
		s.Instalment = s.Rows[0].Payment
		s.FinalPayment = s.Rows[n-1].Payment
	}
	return s, nil
}

// Build projects a contract using whichever repayment structure it declares,
// solving for the instalment when the structure requires one.
//
// This is the entry point callers should use: which method applies is a
// contract term, and a caller that picks one is a caller that can pick wrong.
func Build(c model.Contract, principal money.Amount, from date.Date) (Schedule, error) {
	switch c.Type {
	case model.DecliningPrincipal:
		return ProjectDeclining(c, principal, from)
	case model.Annuity:
		if c.HasScheduled && c.ScheduledPayment.Sign() > 0 {
			// The lender stated the instalment. Use it rather than solving:
			// the contract is the authority on what is owed, and a solved
			// figure that disagrees by a dram is the engine being wrong.
			return Project(c, principal, c.ScheduledPayment, from)
		}
		return SolveAndProject(c, principal, from)
	default:
		return Schedule{}, fmt.Errorf("%w: unsupported repayment type %s", ErrUnsolvable, c.Type)
	}
}
