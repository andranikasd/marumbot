package amortisation

import (
	"fmt"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// Calendar is the instalment dates one contract implies, resolved once.
//
// The dates are a pure function of the contract: they do not depend on the
// balance, on the anchor the projection starts from, or on the instalment.
// Every projection needs them, and the planner projects the same loan
// thousands of times while it searches, so resolving them inside each
// projection made the calendar the single largest allocation in the engine.
//
// A Calendar is immutable once built and safe to share between projections
// and goroutines. The package-level Build, Project, Solve and ProjectDeclining
// resolve one for a single call; a caller that projects the same contract more
// than once should hold the Calendar instead.
type Calendar struct {
	contract model.Contract
	dates    []date.Date
}

// NewCalendar resolves the contract's instalment dates.
func NewCalendar(c model.Contract) (Calendar, error) {
	dates, err := PaymentDates(c)
	if err != nil {
		return Calendar{}, err
	}
	return Calendar{contract: c, dates: dates}, nil
}

// Contract is the contract this calendar was resolved for. Projecting a
// different contract through it would silently use the wrong dates, so every
// method takes only the figures that vary.
func (cal Calendar) Contract() model.Contract { return cal.contract }

// Dates returns the instalment dates still ahead of from, under the rule
// documented on RemainingDates. The result aliases the calendar and must not
// be modified.
func (cal Calendar) Dates(from date.Date) ([]date.Date, error) {
	if len(cal.dates) == 0 {
		return nil, fmt.Errorf("%w: calendar was never resolved", ErrUnsolvable)
	}
	return remainingFrom(cal.contract, cal.dates, from)
}

// Obligation is the next payment a contract implies from a balance: what falls
// due, when, and the instalment behind it.
type Obligation struct {
	Due        date.Date
	Payment    money.Amount
	Instalment money.Amount
}

// Build projects the contract using whichever repayment structure it declares.
// See the package-level Build for why this is the entry point callers want.
func (cal Calendar) Build(principal money.Amount, from date.Date) (Schedule, error) {
	s := Schedule{}
	out, instalment, err := cal.build(principal, from, &s.Rows)
	if err != nil {
		return Schedule{}, err
	}
	s.Instalment = instalment
	s.TotalPaid, s.TotalInterest = out.TotalPaid, out.TotalInterest
	if out.Periods > 0 {
		s.FinalPayment = out.FinalPayment
	}
	return s, nil
}

// Next is Build without the schedule: the next obligation alone.
//
// The planner asks this question once per loan per event and reads only the
// first row of the answer. Building a full schedule for it was the second
// largest allocation in a search, behind the calendar itself.
func (cal Calendar) Next(principal money.Amount, from date.Date) (Obligation, error) {
	out, instalment, err := cal.build(principal, from, nil)
	if err != nil {
		return Obligation{}, err
	}
	if out.Periods == 0 {
		return Obligation{}, fmt.Errorf("%w: no instalment falls after %s", ErrUnsolvable, from)
	}
	return Obligation{Due: out.FirstDue, Payment: out.FirstPayment, Instalment: instalment}, nil
}

// build is the one place the repayment structures are dispatched. rows is nil
// for callers that want the outcome without a schedule; both entry points
// otherwise run identical arithmetic and return identical refusals.
func (cal Calendar) build(principal money.Amount, from date.Date, rows *[]Row) (outcome, money.Amount, error) {
	c := cal.contract
	switch c.Type {
	case model.DecliningPrincipal:
		dates, err := cal.decliningDates(principal, from)
		if err != nil {
			return outcome{}, money.Amount{}, err
		}
		if rows != nil {
			*rows = make([]Row, 0, len(dates))
		}
		out, err := projectDeclining(c, principal, from, dates, rows)
		if err != nil {
			return outcome{}, money.Amount{}, err
		}
		// There is no level instalment here. Instalment reports the first and
		// largest payment, which is the figure a borrower has to be able to
		// afford.
		var instalment money.Amount
		if out.Periods > 0 {
			instalment = out.FirstPayment
		}
		return out, instalment, nil

	case model.Annuity:
		instalment := c.ScheduledPayment
		stated := c.HasScheduled && c.ScheduledPayment.Sign() > 0
		if !stated {
			// The lender stated nothing, so the instalment is solved for.
			solved, err := cal.Solve(principal, from)
			if err != nil {
				return outcome{}, money.Amount{}, err
			}
			instalment = solved
		}
		if err := checkProjection(principal, instalment); err != nil {
			return outcome{}, money.Amount{}, err
		}
		dates, err := cal.Dates(from)
		if err != nil {
			return outcome{}, money.Amount{}, err
		}
		if rows != nil {
			*rows = make([]Row, 0, len(dates))
		}
		out, err := project(c, principal, instalment, from, dates, rows)
		if err != nil {
			return outcome{}, money.Amount{}, err
		}
		if stated && out.Periods > 0 {
			// The lender stated the instalment, so it is used rather than
			// solved: the contract is the authority on what is owed, and a
			// solved figure that disagrees by a dram is the engine being
			// wrong. But a stated figure the arithmetic contradicts is a typo
			// or a misread contract, and projecting it silently produces a
			// schedule that never clears -- or a balance that grows, when the
			// payment does not even cover the first interest. Both are
			// findings about the input, so they refuse by name, the sharper
			// one first.
			if out.FirstPrincipal.Sign() < 0 {
				return outcome{}, money.Amount{}, fmt.Errorf(
					"%w: the stated instalment %s does not cover the first interest of %s; the balance would grow",
					ErrUnsolvable, c.ScheduledPayment, out.FirstInterest)
			}
			if out.FinalClosing.Sign() > 0 {
				return outcome{}, money.Amount{}, fmt.Errorf(
					"%w: the stated instalment %s leaves %s owed at maturity; check the figure or the dates",
					ErrUnsolvable, c.ScheduledPayment, out.FinalClosing)
			}
		}
		return out, instalment, nil

	default:
		return outcome{}, money.Amount{}, fmt.Errorf("%w: unsupported repayment type %s", ErrUnsolvable, c.Type)
	}
}
