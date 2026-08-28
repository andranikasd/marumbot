package plan

import (
	"fmt"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// Quote is what an optional payment on a date would do: the cash it takes,
// the principal it credits, the interest it settles, and the fee it costs.
// The simulator never applies a payment it did not quote, so every early
// payment in a plan is one the lender's rules allow.
type Quote struct {
	Principal money.Amount // credited to the balance
	Interest  money.Amount // settled now; only when the payment closes the loan
	Fee       money.Amount
	Outflow   money.Amount // Principal + Interest + Fee
	Closes    bool
}

// charge computes the fee on a principal credit under the contract's dated
// rules. allowanceUsed is the free allowance already consumed this contract
// year, so a second payment in the year is charged on the remainder.
func charge(c model.Contract, on date.Date, principal, allowanceUsed money.Amount) (money.Amount, error) {
	cur := principal.Currency()
	zero := money.Zero(cur)
	pp := c.Prepayment
	if len(pp.Charges) == 0 {
		if pp.FeeBP <= 0 {
			return zero, nil
		}
		return bp(principal, int64(pp.FeeBP), c.Rounding)
	}
	year := contractYear(c, on)
	total := zero
	for _, r := range pp.Charges {
		if (r.FromYear > 0 && year < r.FromYear) || (r.ThroughYear > 0 && year > r.ThroughYear) {
			continue
		}
		chargeable := principal
		if r.FreeAllowance.Sign() > 0 {
			left, err := r.FreeAllowance.Sub(allowanceUsed)
			if err != nil {
				return zero, err
			}
			if left.Sign() > 0 {
				if chargeable, err = chargeable.Sub(left); err != nil {
					return zero, err
				}
				if chargeable.Sign() < 0 {
					chargeable = zero
				}
			}
		}
		fee := zero
		var err error
		if r.PercentBP > 0 && chargeable.Sign() > 0 {
			if fee, err = bp(chargeable, r.PercentBP, c.Rounding); err != nil {
				return zero, err
			}
		}
		if r.Fixed.Sign() > 0 {
			if fee, err = fee.Add(r.Fixed); err != nil {
				return zero, err
			}
		}
		if r.MinCharge.Sign() > 0 && fee.Cmp(r.MinCharge) < 0 {
			fee = r.MinCharge
		}
		if r.MaxCharge.Sign() > 0 && fee.Cmp(r.MaxCharge) > 0 {
			fee = r.MaxCharge
		}
		if total, err = total.Add(fee); err != nil {
			return zero, err
		}
	}
	return total, nil
}

// bp is amount × basis points, rounded half-up to the settlement unit. The
// product fits int64 for any amount below 9e14 minor units, which is every
// loan this engine will see; the check below refuses the rest.
func bp(a money.Amount, points int64, p money.Policy) (money.Amount, error) {
	m := a.Minor()
	if m > 0 && points > (1<<62)/m {
		return money.Amount{}, fmt.Errorf("plan: fee overflow on %s", a)
	}
	raw := m * points
	minor := (raw + 5_000) / 10_000
	return money.Quantise(money.FromMinor(minor, a.Currency()), p), nil
}

// contractYear is the 1-based year of the contract a date falls in.
func contractYear(c model.Contract, on date.Date) int {
	y := on.Year() - c.StartDate.Year()
	if on.Month() < c.StartDate.Month() || (on.Month() == c.StartDate.Month() && on.Day() < c.StartDate.Day()) {
		y--
	}
	return y + 1
}
