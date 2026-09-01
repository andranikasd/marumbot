package plan

import (
	"fmt"

	"github.com/andranikasd/marumbot/pkg/core/allocation"
	"github.com/andranikasd/marumbot/pkg/core/amortisation"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// MaxLoans is the most loans one plan covers. It is a product limit enforced
// where loans are filed, and the planner refuses a portfolio that exceeds it
// rather than silently planning a subset.
const MaxLoans = 12

// DefaultHorizon bounds a run in months. Forty years of monthly instalments
// is 480; a loan still open after fifty is a finding, not a reason to loop.
const DefaultHorizon = 600

// Position is one loan as the planner sees it: the contract, the trusted
// anchor, and the lender's rule for excess money.
type Position struct {
	ID       string
	Name     string
	Contract model.Contract
	Balance  money.Amount
	From     date.Date // the anchor: Balance is what was owed on this date
	// Excess is what the lender does with money paid beyond the instalment.
	// Only ExcessReducePrincipal lets an early payment save interest.
	Excess allocation.ExcessRule
	// Trust is how the balance was established: user_entered,
	// bank_confirmed or imported_verified. The engine plans either way; the
	// report says when the figures rest on what the borrower typed.
	Trust string
}

// CashEvent is money that becomes available on a date.
type CashEvent struct {
	On     date.Date
	Amount money.Amount
}

// CashPlan is what the borrower has to spend and when.
type CashPlan struct {
	// Monthly is the debt budget: the most the borrower puts towards loans
	// per cycle, arriving on PayDay. It must cover the required instalments.
	Monthly money.Amount
	// PayDay is the day of the month the money arrives, 1..31. Zero means
	// unknown: income is then credited on the first instalment date of each
	// month, so nothing can be paid early.
	PayDay int
	// OpeningCash is money on hand at the valuation date, before any income.
	OpeningCash money.Amount
	// ReserveFloor is cash that must never be spent on optional payments.
	ReserveFloor money.Amount
	// Lumps are one-off sums on given dates: a bonus, a sale.
	Lumps []CashEvent
	// MonthlyOverrides replaces Monthly for the named months, keyed
	// "2006-01". An override is the whole month's figure, not a delta, and
	// it may be lower than Monthly -- a tight month is exactly what a
	// borrower needs to state. Months without an entry use Monthly.
	MonthlyOverrides map[string]money.Amount
}

// MonthKey renders a date as the MonthlyOverrides key for its month.
func MonthKey(d date.Date) string {
	return fmt.Sprintf("%04d-%02d", d.Year(), int(d.Month()))
}

// Input is everything a search needs.
type Input struct {
	// ValuationDate is the day the plan starts. Every position is advanced
	// to it before search; a position anchored after it is refused.
	ValuationDate date.Date
	Cash          CashPlan
	Loans         []Position
	// Horizon in months; zero means DefaultHorizon.
	Horizon int
}

func (in Input) horizon() int {
	if in.Horizon > 0 {
		return in.Horizon
	}
	return DefaultHorizon
}

// Validate refuses what the engine cannot plan, by name.
func (in Input) Validate() error {
	if len(in.Loans) == 0 {
		return ErrNoLoans
	}
	if len(in.Loans) > MaxLoans {
		return &TruncatedError{Max: MaxLoans}
	}
	if in.ValuationDate.IsZero() {
		return fmt.Errorf("plan: valuation date is required")
	}
	if in.Cash.PayDay < 0 || in.Cash.PayDay > 31 {
		return fmt.Errorf("plan: pay day %d out of range", in.Cash.PayDay)
	}
	cur := in.Cash.Monthly.Currency()
	for _, l := range in.Loans {
		if l.Contract.Currency.Code != cur.Code {
			return &MixedCurrencyError{Have: l.Contract.Currency.Code, Want: cur.Code}
		}
		if l.Balance.Currency().Code != cur.Code {
			return &MixedCurrencyError{Have: l.Balance.Currency().Code, Want: cur.Code}
		}
		if l.From.After(in.ValuationDate) {
			return &UnsupportedError{LoanID: l.ID, Feature: "balance anchored after the valuation date"}
		}
		if l.Contract.Type != model.Annuity && l.Contract.Type != model.DecliningPrincipal {
			return &UnsupportedError{LoanID: l.ID, Feature: "repayment type " + l.Contract.Type.String()}
		}
		if l.Contract.NominalRate < 0 {
			return &UnsupportedError{LoanID: l.ID, Feature: "negative rate"}
		}
		if l.Excess == allocation.ExcessUnknown {
			// Planned, but no early-payment credit: the search handles it.
			continue
		}
	}
	for k, v := range in.Cash.MonthlyOverrides {
		var y, m int
		if n, err := fmt.Sscanf(k, "%d-%d", &y, &m); n != 2 || err != nil || m < 1 || m > 12 || y < 1900 || y > 9999 {
			return fmt.Errorf("plan: budget override key %q is not a month", k)
		}
		if v.Currency().Code != cur.Code {
			return &MixedCurrencyError{Have: v.Currency().Code, Want: cur.Code}
		}
		if v.Sign() < 0 {
			return fmt.Errorf("plan: budget override %q is negative", k)
		}
	}
	return nil
}

// Normalize advances every position to the valuation date, assuming the
// instalments that fell due in between were paid as the contract required.
// The number of instalments assumed is returned per loan so the report can
// say so; a plan that silently backdates a balance is a plan that is wrong
// by exactly the interest it forgot.
func Normalize(in Input) (Input, map[string]int, error) {
	if err := in.Validate(); err != nil {
		return in, nil, err
	}
	assumed := map[string]int{}
	out := in
	out.Loans = make([]Position, len(in.Loans))
	copy(out.Loans, in.Loans)
	for i := range out.Loans {
		p := &out.Loans[i]
		n := 0
		for p.Balance.Sign() > 0 {
			dates, err := amortisation.RemainingDates(p.Contract, p.From)
			if err != nil || !dates[0].Before(in.ValuationDate) {
				break
			}
			s, err := amortisation.Build(p.Contract, p.Balance, p.From)
			if err != nil || len(s.Rows) == 0 {
				return in, nil, fmt.Errorf("plan: advancing %s to %s: %w", p.ID, in.ValuationDate, err)
			}
			p.Balance, p.From = s.Rows[0].Closing, s.Rows[0].Due
			n++
		}
		if n > 0 {
			assumed[p.ID] = n
		}
	}
	return out, assumed, nil
}
