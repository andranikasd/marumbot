package amortisation

import (
	"fmt"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// Solve returns the smallest level instalment that clears the loan by maturity.
//
// The instalment is a multiple of the settlement unit, because that is what a
// lender can actually collect: AMD carries two decimal places in ISO 4217 but
// banks settle in whole drams, and an instalment of 45,231.37 dram is not a
// thing that can be paid.
//
// Bisection rather than a formula. The closed-form annuity payment assumes
// equal periods; these periods are actual days apart, so it gives the wrong
// answer by a few dram per row. It cannot be inverted either — the projection
// is a loop with a rounding step in it, not an expression. What the projection
// does have is monotonicity: raise the instalment and the closing balance
// falls, never rises. That is the only property bisection needs, and it holds
// exactly in integer arithmetic where a derivative would not.
//
// Complexity is log2(range) projections, about 25 for a loan of a few million
// dram, each of which is a few hundred rows of integer arithmetic.
func Solve(c model.Contract, principal money.Amount, from date.Date) (money.Amount, error) {
	cal, err := NewCalendar(c)
	if err != nil {
		if principal.Sign() <= 0 {
			return money.Amount{}, fmt.Errorf("%w: principal must be positive", ErrUnsolvable)
		}
		return money.Amount{}, err
	}
	return cal.Solve(principal, from)
}

// Solve bisects on an already-resolved calendar. The whole bisection reuses
// one calendar and materialises no schedule; see project.
func (cal Calendar) Solve(principal money.Amount, from date.Date) (money.Amount, error) {
	c := cal.contract
	if principal.Sign() <= 0 {
		return money.Amount{}, fmt.Errorf("%w: principal must be positive", ErrUnsolvable)
	}
	cur := principal.Currency()
	unit := c.Rounding.Unit
	if unit <= 0 {
		unit = 1
	}

	// Upper bound: settle the whole loan on the first instalment. Nothing larger
	// can be required, because that clears the balance outright.
	dates, err := cal.Dates(from)
	if err != nil {
		return money.Amount{}, err
	}
	prev := from
	if prev.IsZero() {
		prev = c.StartDate
	}
	firstInterest, err := money.Accrue(principal, c.NominalRate,
		int64(date.DaysBetween(prev, dates[0])), c.DayCount, c.Rounding)
	if err != nil {
		return money.Amount{}, fmt.Errorf("amortisation: upper bound: %w", err)
	}
	hiAmount, err := principal.Add(firstInterest)
	if err != nil {
		return money.Amount{}, fmt.Errorf("amortisation: upper bound: %w", err)
	}

	// Work in whole settlement units so every candidate is payable.
	lo := int64(1)
	hi := ceilDiv(hiAmount.Minor(), unit)

	// Every candidate walks the same calendar, so the dates resolved above are
	// reused rather than rebuilt per probe, and no schedule is materialised:
	// bisection reads the closing balance and nothing else.
	clears := func(units int64) (bool, error) {
		out, err := project(c, principal, money.FromMinor(units*unit, cur), from, dates, nil)
		if err != nil {
			return false, err
		}
		if out.Periods == 0 {
			return false, nil
		}
		return out.FinalClosing.Sign() <= 0, nil
	}

	// The upper bound clears by construction; assert it rather than trust it,
	// because a contract whose maturity precedes its first instalment would
	// otherwise bisect toward a silently wrong answer.
	ok, err := clears(hi)
	if err != nil {
		return money.Amount{}, err
	}
	if !ok {
		return money.Amount{}, fmt.Errorf(
			"%w: paying the full balance on the first instalment still leaves a balance at maturity", ErrUnsolvable)
	}

	// Invariant: lo never clears, hi always clears. Each step halves the gap,
	// and the loop ends holding the smallest instalment that clears.
	for lo < hi {
		mid := lo + (hi-lo)/2
		ok, err := clears(mid)
		if err != nil {
			return money.Amount{}, err
		}
		if ok {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	return money.FromMinor(hi*unit, cur), nil
}

// SolveAndProject returns the instalment and the schedule it produces, which is
// what every caller actually wants and saves projecting the loan twice.
func SolveAndProject(c model.Contract, principal money.Amount, from date.Date) (Schedule, error) {
	cal, err := NewCalendar(c)
	if err != nil {
		return Schedule{}, err
	}
	return cal.SolveAndProject(principal, from)
}

// SolveAndProject solves and projects on one already-resolved calendar, so the
// contract's dates are walked twice but built once.
func (cal Calendar) SolveAndProject(principal money.Amount, from date.Date) (Schedule, error) {
	instalment, err := cal.Solve(principal, from)
	if err != nil {
		return Schedule{}, err
	}
	dates, err := cal.Dates(from)
	if err != nil {
		return Schedule{}, err
	}
	s := Schedule{Rows: make([]Row, 0, len(dates)), Instalment: instalment}
	out, err := project(cal.contract, principal, instalment, from, dates, &s.Rows)
	if err != nil {
		return Schedule{}, err
	}
	s.TotalPaid, s.TotalInterest, s.FinalPayment = out.TotalPaid, out.TotalInterest, out.FinalPayment
	return s, nil
}

func ceilDiv(a, b int64) int64 {
	if a <= 0 {
		return 1
	}
	return (a + b - 1) / b
}
