// Package amortisation projects a repayment schedule and solves for the level
// instalment that clears a loan by its maturity date.
//
// Everything here is dated. The textbook annuity formula assumes every period
// is the same length, which is false: February is 28 days and January is 31, so
// the interest charged in each differs even at a constant rate. A schedule built
// on the closed form disagrees with the lender's own paperwork by a few dram per
// row, and those few dram are exactly what makes a borrower distrust the number.
//
// Because the periods differ, there is no closed form to invert. The instalment
// is found by bisection over the projection itself: the closing balance falls
// monotonically as the instalment rises, so the smallest instalment that clears
// the balance can be found by halving the interval. Bisection is also what keeps
// the arithmetic in integers — a Newton step would need a derivative, and a
// derivative would need floats.
package amortisation

import (
	"errors"
	"fmt"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// ErrUnsolvable is returned when no instalment clears the loan by maturity, or
// when the contract does not describe a schedule at all.
var ErrUnsolvable = errors.New("amortisation: unsolvable")

// maxPeriods bounds a projection. A 40-year monthly loan is 480 rows; anything
// past this is a malformed contract rather than a long one, and an unbounded
// loop over borrower-supplied dates is a denial of service.
const maxPeriods = 600

// Row is one line of a repayment schedule.
type Row struct {
	N         int       // 1-based instalment number
	Due       date.Date // when the instalment falls due
	Days      int       // days accrued since the previous row, or since drawdown
	Opening   money.Amount
	Interest  money.Amount
	Principal money.Amount // the part of Payment that reduces the balance
	Payment   money.Amount
	Closing   money.Amount
}

// Schedule is a full projection. It is computed on demand and never stored: it
// is a function of the contract and the balance, and storing it would create a
// second source of truth that can disagree with replay.
type Schedule struct {
	Rows          []Row
	Instalment    money.Amount // the level instalment, before the final adjustment
	FinalPayment  money.Amount // the last row, which settles the remainder exactly
	TotalPaid     money.Amount
	TotalInterest money.Amount
}

// PaymentDates returns the instalment dates implied by the contract, from the
// first after start through maturity.
//
// The dates come from date.Occurrence, which carries the contractual day rather
// than walking month to month: a loan taken on the 31st falls due on the 28th in
// February and returns to the 31st in March. Stepping from the clamped date
// instead would drift the anchor permanently after one short month.
func PaymentDates(c model.Contract) ([]date.Date, error) {
	if c.StartDate.IsZero() || c.MaturityDate.IsZero() {
		return nil, fmt.Errorf("%w: contract has no start or maturity date", ErrUnsolvable)
	}
	if !c.MaturityDate.After(c.StartDate) {
		return nil, fmt.Errorf("%w: maturity %s is not after start %s",
			ErrUnsolvable, c.MaturityDate, c.StartDate)
	}
	day := c.PaymentDay
	if day == 0 {
		day = c.StartDate.Day()
	}

	out := make([]date.Date, 0, 64)
	for n := 1; n <= maxPeriods; n++ {
		d := date.Occurrence(c.StartDate, day, n)
		if d.After(c.MaturityDate) {
			break
		}
		out = append(out, d)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: no instalment falls between %s and %s",
			ErrUnsolvable, c.StartDate, c.MaturityDate)
	}
	if len(out) == maxPeriods {
		return nil, fmt.Errorf("%w: more than %d instalments", ErrUnsolvable, maxPeriods)
	}
	return out, nil
}

// RemainingDates returns the instalment dates still ahead of from: strictly
// after it, because a date equal to from is the instalment just paid, and a
// row for it would accrue zero days and then swallow a whole instalment as
// principal. A zero from means the drawdown. Dates before NotBeforeDue are
// excluded when it is set, including when from is zero.
//
// Every projection goes through this, so a loan anchored on a mid-life
// balance — the normal case for a loan filed months after it was taken —
// projects from its next instalment rather than failing on its first.
func RemainingDates(c model.Contract, from date.Date) ([]date.Date, error) {
	dates, err := PaymentDates(c)
	if err != nil {
		return nil, err
	}
	return remainingFrom(c, dates, from)
}

// remainingFrom applies the rule above to a calendar already resolved, so a
// caller holding a Calendar never rebuilds the contract's dates.
func remainingFrom(c model.Contract, dates []date.Date, from date.Date) ([]date.Date, error) {
	if from.IsZero() && c.NotBeforeDue.IsZero() {
		return dates, nil
	}
	i := 0
	for i < len(dates) && ((!from.IsZero() && !dates[i].After(from)) ||
		(!c.NotBeforeDue.IsZero() && dates[i].Before(c.NotBeforeDue))) {
		i++
	}
	if i == len(dates) {
		return nil, fmt.Errorf("%w: no instalment falls after %s", ErrUnsolvable, from)
	}
	return dates[i:], nil
}

// Project runs the schedule for a given instalment and returns every row.
//
// The final row is not the level instalment. Rounding each period to the
// settlement unit leaves a residue of a few minor units, so the last payment is
// whatever settles the balance exactly. Lenders do the same thing, and a
// schedule that ends on a level payment and a stray balance of 3 dram is wrong
// in the way a borrower notices.
func Project(c model.Contract, principal money.Amount, instalment money.Amount, from date.Date) (Schedule, error) {
	cal, err := NewCalendar(c)
	if err != nil {
		if checked := checkProjection(principal, instalment); checked != nil {
			return Schedule{}, checked
		}
		return Schedule{}, err
	}
	return cal.Project(principal, instalment, from)
}

// Project runs the schedule on an already-resolved calendar.
func (cal Calendar) Project(principal money.Amount, instalment money.Amount, from date.Date) (Schedule, error) {
	if err := checkProjection(principal, instalment); err != nil {
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

// checkProjection rejects money arguments a projection cannot mean, before any
// calendar work. The order of these checks is part of the package's behaviour:
// callers match on the sentinel and the message.
func checkProjection(principal, instalment money.Amount) error {
	if principal.Sign() < 0 {
		return fmt.Errorf("%w: negative principal", ErrUnsolvable)
	}
	if instalment.Sign() <= 0 {
		return fmt.Errorf("%w: instalment must be positive", ErrUnsolvable)
	}
	if principal.Currency() != instalment.Currency() {
		return fmt.Errorf("%w: principal in %s, instalment in %s",
			ErrUnsolvable, principal.Currency(), instalment.Currency())
	}
	return nil
}

// outcome is what a projection produces once its rows are dropped: enough to
// decide whether an instalment clears the loan, and nothing that has to be
// allocated per candidate.
type outcome struct {
	Periods int
	// The first row, which is the next obligation: every planner question
	// about "what is owed next" is answered from these three fields.
	FirstDue       date.Date
	FirstInterest  money.Amount
	FirstPrincipal money.Amount
	FirstPayment   money.Amount
	// The last row, which is what a bisection and a maturity check read.
	FinalPayment  money.Amount
	FinalClosing  money.Amount
	TotalPaid     money.Amount
	TotalInterest money.Amount
}

// project walks the dates once and is the only place the arithmetic lives.
//
// rows is nil for callers that want the outcome alone. Solve bisects over this
// about twenty-five times per loan and reads only the last closing balance;
// building a schedule for each probe made the discarded rows the largest
// single source of allocation in the engine.
func project(c model.Contract, principal, instalment money.Amount, from date.Date, dates []date.Date, rows *[]Row) (outcome, error) {
	cur := principal.Currency()
	out := outcome{TotalPaid: money.Zero(cur), TotalInterest: money.Zero(cur)}

	balance := principal
	prev := from
	if prev.IsZero() {
		prev = c.StartDate
	}

	for i, due := range dates {
		if balance.Sign() <= 0 {
			break
		}
		days := date.DaysBetween(prev, due)
		if days < 0 {
			return outcome{}, fmt.Errorf("%w: instalment %s precedes %s", ErrUnsolvable, due, prev)
		}
		interest, err := money.Accrue(balance, c.NominalRate, int64(days), c.DayCount, c.Rounding)
		if err != nil {
			return outcome{}, fmt.Errorf("amortisation: row %d: %w", i+1, err)
		}

		// Settling the loan on this date costs the balance plus the interest
		// that has accrued to it.
		owed, err := balance.Add(interest)
		if err != nil {
			return outcome{}, fmt.Errorf("amortisation: row %d: %w", i+1, err)
		}
		pay := instalment
		if pay.Cmp(owed) >= 0 {
			// Either the last row, or an instalment large enough to close the
			// loan early: pay exactly what is owed, never more.
			pay = owed
		}

		principalPart, err := pay.Sub(interest)
		if err != nil {
			return outcome{}, fmt.Errorf("amortisation: row %d: %w", i+1, err)
		}
		closing, err := balance.Sub(principalPart)
		if err != nil {
			return outcome{}, fmt.Errorf("amortisation: row %d: %w", i+1, err)
		}

		if rows != nil {
			*rows = append(*rows, Row{
				N: i + 1, Due: due, Days: days,
				Opening: balance, Interest: interest,
				Principal: principalPart, Payment: pay, Closing: closing,
			})
		}
		if out.TotalPaid, err = out.TotalPaid.Add(pay); err != nil {
			return outcome{}, fmt.Errorf("amortisation: row %d: %w", i+1, err)
		}
		if out.TotalInterest, err = out.TotalInterest.Add(interest); err != nil {
			return outcome{}, fmt.Errorf("amortisation: row %d: %w", i+1, err)
		}

		out.Periods++
		if out.Periods == 1 {
			out.FirstDue, out.FirstInterest = due, interest
			out.FirstPrincipal, out.FirstPayment = principalPart, pay
		}
		out.FinalPayment, out.FinalClosing = pay, closing
		balance = closing
		prev = due
	}
	return out, nil
}
