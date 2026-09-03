package plan

import (
	"fmt"
	"math/big"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// BudgetGrowth increases spending permission, never income or funding.
// StartsOn is the first increase and must be the first day of a month.
// A zero EndsOn means no end. Fixed and
// Maximum are absent only when they are zero Amounts without a currency.
// PercentPPB is parts per billion (30_000_000 means 3%); zero means no growth.
// Fixed and a nonzero PercentPPB are mutually exclusive.
type BudgetGrowth struct {
	EveryMonths int
	StartsOn    date.Date
	EndsOn      date.Date
	Fixed       money.Amount
	PercentPPB  int64
	Maximum     money.Amount
}

// BudgetAdjustment changes one calendar month's grown limit. Exactly one
// pointer must be set; a replacement may be zero and a delta may be negative.
type BudgetAdjustment struct {
	Month       string
	Replacement *money.Amount
	Delta       *money.Amount
}

// ExpandBudgetRules returns replacement limits for 1..600 calendar months
// beginning with from's month. Growth is limited to calendar-month boundaries;
// callers must normalize other spending cycles separately. Each recurrence
// affects its entire month;
// earlier recurrences are compounded from base, including those before from.
// Growth stops after EndsOn but its last limit persists. Month adjustments do
// not compound and may exceed a growth cap. Inputs are never mutated.
func ExpandBudgetRules(base money.Amount, from date.Date, months int, growth *BudgetGrowth, adjustments []BudgetAdjustment) (map[string]money.Amount, error) {
	if months < 1 || months > 600 || !budgetRuleDateValid(from) {
		return nil, fmt.Errorf("plan: invalid budget horizon")
	}
	last := date.AddMonths(from, months-1).EndOfMonth()
	if !budgetRuleDateValid(last) {
		return nil, fmt.Errorf("plan: invalid budget horizon")
	}
	cur, err := money.Lookup(base.Currency().Code)
	if err != nil {
		return nil, money.ErrUnknownCurrency
	}
	if base.Currency() != cur {
		return nil, fmt.Errorf("plan: invalid budget currency metadata")
	}
	if base.Sign() < 0 {
		return nil, money.ErrNegative
	}
	checkAmount := func(a money.Amount) error {
		if a.Currency() != cur {
			return fmt.Errorf("plan: invalid or mismatched budget currency")
		}
		return nil
	}
	if growth != nil {
		if growth.EveryMonths < 1 || !budgetRuleDateValid(growth.StartsOn) || growth.StartsOn.Day() != 1 ||
			(!growth.EndsOn.IsZero() && (!budgetRuleDateValid(growth.EndsOn) || growth.EndsOn.Before(growth.StartsOn))) {
			return nil, fmt.Errorf("plan: invalid budget growth dates or frequency")
		}
		if growth.PercentPPB < 0 || (growth.Fixed != (money.Amount{}) && growth.PercentPPB != 0) {
			return nil, fmt.Errorf("plan: invalid budget growth increase")
		}
		for _, a := range []money.Amount{growth.Fixed, growth.Maximum} {
			if a == (money.Amount{}) {
				continue
			}
			if err := checkAmount(a); err != nil {
				return nil, err
			}
			if a.Sign() < 0 {
				return nil, money.ErrNegative
			}
		}
	}
	byMonth := make(map[string]BudgetAdjustment, len(adjustments))
	for _, a := range adjustments {
		d, err := date.Parse(a.Month + "-01")
		if err != nil || !budgetRuleDateValid(d) || MonthKey(d) != a.Month {
			return nil, fmt.Errorf("plan: invalid budget adjustment month")
		}
		if _, exists := byMonth[a.Month]; exists {
			return nil, fmt.Errorf("plan: duplicate budget adjustment month")
		}
		if (a.Replacement == nil) == (a.Delta == nil) {
			return nil, fmt.Errorf("plan: budget adjustment requires exactly one operation")
		}
		value := a.Delta
		if a.Replacement != nil {
			value = a.Replacement
		}
		if err := checkAmount(*value); err != nil {
			return nil, err
		}
		if a.Replacement != nil && value.Sign() < 0 {
			return nil, money.ErrNegative
		}
		byMonth[a.Month] = a
	}
	limits := make(map[string]money.Amount, months)
	grown := base
	next := date.Date{}
	if growth != nil {
		next = growth.StartsOn
	}
	for i := 0; i < months; i++ {
		month := date.AddMonths(from, i)
		for !next.IsZero() && !next.After(month.EndOfMonth()) {
			if !growth.EndsOn.IsZero() && next.After(growth.EndsOn) {
				next = date.Date{}
				break
			}
			grown, err = growBudgetLimit(grown, *growth)
			if err != nil {
				return nil, err
			}
			// Subtract before advancing: even an int-sized frequency cannot overflow.
			remaining := (last.Year()-next.Year())*12 + int(last.Month()-next.Month())
			if growth.EveryMonths > remaining {
				next = date.Date{}
				break
			}
			offset := (next.Year()-growth.StartsOn.Year())*12 + int(next.Month()-growth.StartsOn.Month())
			next = date.AddMonths(growth.StartsOn, offset+growth.EveryMonths)
		}
		key := MonthKey(month)
		limit := grown
		if a, ok := byMonth[key]; ok {
			if a.Replacement != nil {
				limit = *a.Replacement
			} else {
				limit, err = grown.Add(*a.Delta)
				if err != nil {
					return nil, money.ErrOverflow
				}
			}
		}
		if limit.Sign() < 0 {
			return nil, money.ErrNegative
		}
		limits[key] = limit
	}
	return limits, nil
}

func budgetRuleDateValid(d date.Date) bool {
	// Date.New accepts arbitrary years; month keys intentionally use ISO years.
	return !d.IsZero() && d.Year() >= 1 && d.Year() <= 9999
}

func growBudgetLimit(current money.Amount, growth BudgetGrowth) (money.Amount, error) {
	// Exact integer intermediates allow a representable capped result even when
	// the uncapped product exceeds int64. Round the total once per recurrence,
	// using the currency's settled half-up default, before applying the cap.
	numerator := big.NewInt(current.Minor())
	denominator := big.NewInt(1)
	switch {
	case growth.Fixed != (money.Amount{}):
		numerator.Add(numerator, big.NewInt(growth.Fixed.Minor()))
	case growth.PercentPPB != 0:
		factor := new(big.Int).Add(big.NewInt(1_000_000_000), big.NewInt(growth.PercentPPB))
		numerator.Mul(numerator, factor)
		denominator.SetInt64(1_000_000_000)
	}
	if growth.Fixed.Sign() != 0 || growth.PercentPPB != 0 {
		policy := money.DefaultPolicy(current.Currency())
		denominator.Mul(denominator, big.NewInt(policy.Unit))
		units, remainder := new(big.Int), new(big.Int)
		units.QuoRem(numerator, denominator, remainder)
		if remainder.Lsh(remainder, 1).Cmp(denominator) >= 0 {
			units.Add(units, big.NewInt(1))
		}
		numerator.Mul(units, big.NewInt(policy.Unit))
	}
	if growth.Maximum != (money.Amount{}) && numerator.Cmp(big.NewInt(growth.Maximum.Minor())) > 0 {
		return growth.Maximum, nil
	}
	if !numerator.IsInt64() {
		return money.Amount{}, money.ErrOverflow
	}
	return money.FromMinor(numerator.Int64(), current.Currency()), nil
}
