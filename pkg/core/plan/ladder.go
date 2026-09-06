package plan

// The budget ladder and its inverse: what a given budget buys, and what
// budget buys a given date. Both are questions about a fixed policy, asked by
// re-simulating it rather than by inverting any formula.

import (
	"errors"
	"fmt"

	"github.com/andranikasd/marumbot/pkg/core/allocation"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// minimum pays only what the contracts require, on their dates, with no
// optional payment. Explicit spending preserves the declared cash and
// permissions; legacy inputs retain the required-instalment budget baseline.
func minimum(in Input, c *cache) (Result, error) {
	n := len(in.Loans)
	pol := Policy{Name: "minimum", RequiredOnly: true, Order: identity(n), Timing: uniform(n, OnDue), Effect: uniform(n, model.PrepayReduceInstalment), Rollover: KeepFreed}
	if in.Cash.SeparateSpending() {
		// Funding and permission are independent in this domain. Preserve
		// all dated cash, reserves and spending declarations; suppress extras
		// explicitly rather than manufacturing a required-payment budget.
		return run(in, pol, c)
	}
	req, err := requiredNow(in, c)
	if err != nil {
		return Result{}, err
	}
	// Under KeepFreed the budget falls as loans close; between closures the
	// required total of an annuity is level, so the first cycle's figure
	// carries. Declining-principal loans require less each month; the
	// surplus that creates is not spent because RequiredOnly forbids extras,
	// including closing payments that would bypass MinPrepay.
	// Overrides ride along: a month the borrower stated as tight can be too
	// tight even for the required instalments, and the minimum run is where
	// that becomes a typed refusal with the date instead of a generic "no
	// feasible policy". In a feasible plan they change nothing here -- the
	// minimum spends only what is required, and spare income idles.
	inMin := in
	inMin.Cash = CashPlan{
		Monthly: req, PayDay: in.Cash.PayDay, OpeningCash: in.Cash.OpeningCash,
		MonthlyOverrides: in.Cash.MonthlyOverrides,
	}
	pol.MinPrepay = money.FromMinor(1<<62, req.Currency())
	return run(inMin, pol, c)
}

// requiredNow totals the next instalment of every live loan.
func requiredNow(in Input, c *cache) (money.Amount, error) {
	total := money.Zero(in.Cash.Monthly.Currency())
	for _, l := range in.Loans {
		if l.Balance.Sign() <= 0 {
			continue
		}
		ls := &loanState{pos: l, fp: fingerprint(l.Contract), effect: model.PrepayReduceInstalment, balance: l.Balance, from: l.From}
		o, err := c.next(ls)
		if err != nil {
			return money.Amount{}, err
		}
		if total, err = total.Add(o.required); err != nil {
			return money.Amount{}, err
		}
	}
	return total, nil
}

// ladder runs the best policy at a few larger budgets.
func ladder(in Input, pol Policy, c *cache) ([]Rung, error) {
	cur := in.Cash.Monthly.Currency()
	var out []Rung
	for _, pct := range []int64{100, 110, 125, 150, 200} {
		b := money.Quantise(money.FromMinor(in.Cash.Monthly.Minor()*pct/100, cur), money.DefaultPolicy(cur))
		more := in
		more.Cash.Monthly = b
		r, err := run(more, pol, c)
		if err != nil {
			if isInfeasible(err) {
				continue
			}
			return nil, err
		}
		out = append(out, Rung{Budget: b, Months: r.Months, Payoff: r.PayoffDate, Interest: r.TotalInterest})
	}
	return out, nil
}

// NonMonotoneError refuses an inverse search whose monotonicity is unproven.
type NonMonotoneError struct {
	Reason string
}

func (e *NonMonotoneError) Error() string {
	return "plan: inverse budget monotonicity not established: " + e.Reason
}

// BudgetFor finds the smallest monthly settlement quantum meeting by for a
// single zero-interest, fee-free annuity paid on its due dates. With aligned
// principal and instalments, no payment is rounded and the post-due balance is
// max(0, balance-budget) whenever the required payment can be met. Increasing
// budget cannot increase that balance or turn a met obligation into a shortfall.
// Positive interest is excluded: allocation-dependent accrual splits invalidate
// the unrounded exchange argument, even with descending rates and shared units.
// This proof covers only the current immediate-credit model, which has no
// notice periods, allowed payment windows or delayed-credit fields.
func BudgetFor(in Input, pol Policy, by date.Date) (money.Amount, error) {
	norm, _, err := Normalize(in)
	if err != nil {
		return money.Amount{}, err
	}
	if by.IsZero() || by.Before(norm.ValuationDate) {
		return money.Amount{}, fmt.Errorf("plan: invalid inverse target date")
	}
	if err := inverseDomain(norm, pol); err != nil {
		return money.Amount{}, err
	}
	c := newCache()
	cur := norm.Cash.Monthly.Currency()
	hi, err := requiredNow(norm, c)
	if err != nil {
		return money.Amount{}, err
	}
	for _, l := range norm.Loans {
		if hi, err = hi.Add(l.Balance); err != nil {
			return money.Amount{}, err
		}
	}
	unit := money.DefaultPolicy(cur).Unit
	clears := func(b money.Amount) (bool, error) {
		trial := norm
		trial.Cash.Monthly = b
		r, runErr := run(trial, pol, c)
		return runErr == nil && !r.PayoffDate.After(by), runErr
	}
	h := hi.Minor() / unit
	if hi.Minor()%unit != 0 {
		h++
	}
	if h > (1<<63-1)/unit {
		return money.Amount{}, fmt.Errorf("plan: inverse budget bound overflow")
	}
	if ok, runErr := clears(money.FromMinor(h*unit, cur)); runErr != nil {
		return money.Amount{}, runErr
	} else if !ok {
		return money.Amount{}, fmt.Errorf("plan: inverse budget bound misses target")
	}
	miss := func(ok bool, runErr error) (bool, error) {
		if runErr != nil && !isInfeasible(runErr) && !errors.Is(runErr, ErrHorizon) {
			return false, runErr
		}
		return !ok, nil
	}
	l := int64(0)
	for l < h {
		mid := l + (h-l)/2
		failed, runErr := miss(clears(money.FromMinor(mid*unit, cur)))
		if runErr != nil {
			return money.Amount{}, runErr
		}
		if failed {
			l = mid + 1
		} else {
			h = mid
		}
	}
	budget := money.FromMinor(h*unit, cur)
	if ok, runErr := clears(budget); runErr != nil {
		return money.Amount{}, runErr
	} else if !ok {
		return money.Amount{}, &NonMonotoneError{Reason: "result budget misses target"}
	}
	if h > 0 {
		failed, runErr := miss(clears(money.FromMinor((h-1)*unit, cur)))
		if runErr != nil {
			return money.Amount{}, runErr
		}
		if !failed {
			return money.Amount{}, &NonMonotoneError{Reason: "one quantum less also succeeds"}
		}
	}
	return budget, nil
}

func inverseDomain(in Input, pol Policy) error {
	refuse := func(reason string) error { return &NonMonotoneError{Reason: reason} }
	cash := in.Cash
	if !cash.CashThrough.IsZero() || cash.SeparateSpending() || len(cash.MonthlyOverrides) != 0 || len(cash.Lumps) != 0 ||
		cash.OpeningCash.Sign() != 0 || cash.ReserveFloor.Sign() != 0 {
		return refuse("funding or spending varies independently of the budget")
	}
	if pol.Rollover != RollFreed || pol.RequiredOnly || pol.MinPrepay.Sign() != 0 {
		return refuse("rollover, required-only policy or batching")
	}
	n := len(in.Loans)
	if len(pol.Order) != n || len(pol.Timing) != n || len(pol.Effect) != n {
		return refuse("policy dimensions")
	}
	if n != 1 {
		return refuse("multiple-loan allocation is unproven")
	}
	if pol.Order[0] != 0 {
		return refuse("priority must be a permutation")
	}
	p := in.Loans[0]
	ct := p.Contract
	if ct.NominalRate != 0 {
		return refuse("rounded interest is outside the proven domain")
	}
	if ct.Prepayment.FeeBP != 0 || len(ct.Prepayment.Charges) != 0 || ct.Prepayment.MinAmount.Sign() != 0 {
		return refuse("prepayment fees or thresholds")
	}
	effect := ct.Prepayment.Effect
	if effect == model.PrepayBorrowerChooses {
		effect = pol.Effect[0]
	}
	if ct.Type != model.Annuity || (effect != model.PrepayShortenTerm && effect != model.PrepayBorrowerChooses) ||
		p.OptionalExcluded || p.Excess != allocation.ExcessReducePrincipal {
		return refuse("requires fixed instalments and unrestricted principal reduction")
	}
	rounding := money.DefaultPolicy(cash.Monthly.Currency())
	if ct.Rounding != rounding || p.Balance.Sign() < 0 || p.Balance.Minor()%rounding.Unit != 0 ||
		!ct.HasScheduled || ct.ScheduledPayment.Sign() <= 0 || ct.ScheduledPayment.Minor()%rounding.Unit != 0 {
		return refuse("requires quantum-aligned principal and supplied instalment")
	}
	if pol.Timing[0] != OnDue || cash.PayDay != 0 || !p.From.Equal(in.ValuationDate) {
		return refuse("requires funding and payment on the due calendar without anchor advancement")
	}
	return nil
}
