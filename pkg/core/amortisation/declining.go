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
	cal, err := NewCalendar(c)
	if err != nil {
		if principal.Sign() <= 0 {
			return Schedule{}, fmt.Errorf("%w: principal must be positive", ErrUnsolvable)
		}
		return Schedule{}, err
	}
	return cal.ProjectDeclining(principal, from)
}

// ProjectDeclining builds the equal-principal schedule on an already-resolved
// calendar.
func (cal Calendar) ProjectDeclining(principal money.Amount, from date.Date) (Schedule, error) {
	dates, err := cal.decliningDates(principal, from)
	if err != nil {
		return Schedule{}, err
	}
	s := Schedule{Rows: make([]Row, 0, len(dates)), TotalPaid: money.Zero(principal.Currency()), TotalInterest: money.Zero(principal.Currency())}
	out, err := projectDeclining(cal.contract, principal, from, dates, &s.Rows)
	if err != nil {
		return Schedule{}, err
	}
	s.TotalPaid, s.TotalInterest = out.TotalPaid, out.TotalInterest
	if out.Periods > 0 {
		// There is no level instalment here. Instalment reports the first and
		// largest payment, which is the figure a borrower has to be able to
		// afford, and FinalPayment the smallest.
		s.Instalment, s.FinalPayment = out.FirstPayment, out.FinalPayment
	}
	return s, nil
}

func (cal Calendar) decliningDates(principal money.Amount, from date.Date) ([]date.Date, error) {
	if principal.Sign() <= 0 {
		return nil, fmt.Errorf("%w: principal must be positive", ErrUnsolvable)
	}
	return cal.Dates(from)
}

// projectDeclining walks the equal-principal schedule once. rows is nil for
// callers that need only the outcome, so asking for the next obligation does
// not allocate a schedule that is then thrown away.
func projectDeclining(c model.Contract, principal money.Amount, from date.Date, dates []date.Date, rows *[]Row) (outcome, error) {
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

	out := outcome{TotalPaid: money.Zero(cur), TotalInterest: money.Zero(cur)}
	balance := principal
	prev := from
	if prev.IsZero() {
		prev = c.StartDate
	}

	for i, due := range dates {
		days := date.DaysBetween(prev, due)
		if days < 0 {
			return outcome{}, fmt.Errorf("%w: instalment %s precedes %s", ErrUnsolvable, due, prev)
		}
		interest, err := money.Accrue(balance, c.NominalRate, int64(days), c.DayCount, c.Rounding)
		if err != nil {
			return outcome{}, fmt.Errorf("amortisation: row %d: %w", i+1, err)
		}

		// The final row clears whatever is left, which is the rounded-down
		// remainder plus any residue.
		principalPart := per
		if i == len(dates)-1 || balance.Cmp(per) < 0 {
			principalPart = balance
		}
		payment, err := principalPart.Add(interest)
		if err != nil {
			return outcome{}, fmt.Errorf("amortisation: row %d: %w", i+1, err)
		}
		if c.HasScheduled && !c.NotBeforeDue.IsZero() && due.Equal(c.NotBeforeDue) && payment.Cmp(c.ScheduledPayment) != 0 {
			return outcome{}, fmt.Errorf("%w: bank instalment differs from the declining schedule", ErrUnsolvable)
		}
		closing, err := balance.Sub(principalPart)
		if err != nil {
			return outcome{}, fmt.Errorf("amortisation: row %d: %w", i+1, err)
		}

		if rows != nil {
			*rows = append(*rows, Row{
				N: i + 1, Due: due, Days: days,
				Opening: balance, Interest: interest,
				Principal: principalPart, Payment: payment, Closing: closing,
			})
		}
		if out.TotalPaid, err = out.TotalPaid.Add(payment); err != nil {
			return outcome{}, fmt.Errorf("amortisation: row %d: %w", i+1, err)
		}
		if out.TotalInterest, err = out.TotalInterest.Add(interest); err != nil {
			return outcome{}, fmt.Errorf("amortisation: row %d: %w", i+1, err)
		}

		out.Periods++
		if out.Periods == 1 {
			out.FirstDue, out.FirstInterest = due, interest
			out.FirstPrincipal, out.FirstPayment = principalPart, payment
		}
		out.FinalPayment, out.FinalClosing = payment, closing

		balance = closing
		prev = due
		if balance.Sign() == 0 {
			break
		}
	}
	return out, nil
}

// Build projects a contract using whichever repayment structure it declares,
// solving for the instalment when the structure requires one.
//
// This is the entry point callers should use: which method applies is a
// contract term, and a caller that picks one is a caller that can pick wrong.
func Build(c model.Contract, principal money.Amount, from date.Date) (Schedule, error) {
	cal, err := NewCalendar(c)
	if err != nil {
		return Schedule{}, err
	}
	return cal.Build(principal, from)
}
