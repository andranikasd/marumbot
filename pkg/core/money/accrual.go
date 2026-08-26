package money

import (
	"errors"
	"fmt"
	"math"
	"math/bits"
)

// Rate is an annual interest rate held as parts per billion, matching the
// numeric(12,9) column it is persisted in. 18% is 180_000_000.
//
// This is the only representation of a rate in the engine. There is no
// float64 anywhere on the path from a stored rate to an accrued amount.
type Rate int64

const ratePPB = 1_000_000_000

// RateFromPercent builds a Rate from a whole-percent figure and up to six
// further decimal places, e.g. RateFromPercent(18, 500000) is 18.5%.
func RateFromPercent(percent int64, microFraction int64) Rate {
	return Rate(percent*ratePPB/100 + microFraction*1000/100)
}

func (r Rate) String() string {
	return fmt.Sprintf("%d.%09d", int64(r)/ratePPB, abs64(int64(r))%ratePPB)
}

// DayCount is the convention that turns a date range into a fraction of a
// year. It is a contract term, never an assumption.
type DayCount uint8

// The day-count conventions the engine supports. Which one applies is a
// contract term, never an assumption.
const (
	Actual365 DayCount = iota // Armenian consumer default
	Actual360
	Thirty360
)

// Denominator returns the days-in-year divisor for the convention.
func (d DayCount) Denominator() int64 {
	switch d {
	case Actual360:
		return 360
	case Thirty360:
		return 360
	default:
		return 365
	}
}

func (d DayCount) String() string {
	switch d {
	case Actual360:
		return "act360"
	case Thirty360:
		return "30_360"
	}
	return "act365"
}

// ErrAccrualRange is returned when an accrual cannot be computed exactly.
// It is never a silent wrap: the caller must decide what to tell the user.
var ErrAccrualRange = errors.New("accrual out of representable range")

// Accrue returns the interest on principal over days at the annual rate,
// rounded once under the policy.
//
//	interest = principal x rate x days / (denominator x 10^9)
//
// The intermediate product does NOT fit in 64 bits for ordinary Armenian loan
// sizes. At 18%, principal x rate x 31 exceeds int64 above roughly 16.5M AMD;
// at 26% the ceiling is about 11.4M. Mortgages here routinely exceed both, and
// the failure mode of a naive implementation is a silent wrap.
//
// So the numerator is accumulated in 128 bits and divided once. The operands
// are grouped as (principal x days) x rate rather than principal x rate x days
// purely to keep the first product inside int64; there is still exactly one
// division and exactly one rounding step, so the result is identical to the
// mathematical definition.
func Accrue(principal Amount, rate Rate, days int64, dc DayCount, p Policy) (Amount, error) {
	if days < 0 {
		return Amount{}, fmt.Errorf("%w: negative day count %d", ErrAccrualRange, days)
	}
	if principal.minor < 0 {
		return Amount{}, fmt.Errorf("%w: negative principal", ErrNegative)
	}
	if rate < 0 {
		return Amount{}, fmt.Errorf("%w: negative rate", ErrAccrualRange)
	}
	if days == 0 || rate == 0 || principal.minor == 0 {
		return Amount{cur: principal.cur}, nil
	}

	// Step 1: principal x days. Both are modest; guard anyway.
	if days != 0 && principal.minor > math.MaxInt64/days {
		return Amount{}, fmt.Errorf("%w: principal %d x days %d", ErrAccrualRange, principal.minor, days)
	}
	pd := principal.minor * days

	// Step 2: x rate, in 128 bits.
	hi, lo := bits.Mul64(uint64(pd), uint64(rate))

	// Step 3: divide by denominator x 10^9 x rounding-unit, so the value is
	// quantised to the settlement unit in the same operation it is scaled by.
	unit := p.unit()
	div, err := mulNoOverflow(dc.Denominator(), ratePPB)
	if err != nil {
		return Amount{}, err
	}
	div, err = mulNoOverflow(div, unit)
	if err != nil {
		return Amount{}, err
	}
	if hi >= uint64(div) {
		// bits.Div64 would panic; this only happens for absurd inputs.
		return Amount{}, fmt.Errorf("%w: quotient exceeds 64 bits", ErrAccrualRange)
	}
	quo, rem := bits.Div64(hi, lo, uint64(div))
	if quo > math.MaxInt64/uint64(unit) {
		return Amount{}, fmt.Errorf("%w: result exceeds int64", ErrAccrualRange)
	}

	units := roundQuotient(int64(quo), int64(rem), div, p.Mode)
	if units > math.MaxInt64/unit {
		return Amount{}, fmt.Errorf("%w: result exceeds int64", ErrAccrualRange)
	}
	return Amount{minor: units * unit, cur: principal.cur}, nil
}

// Quantise rounds an amount to the policy's settlement unit.
func Quantise(a Amount, p Policy) Amount {
	unit := p.unit()
	if unit == 1 {
		return a
	}
	neg := a.minor < 0
	v := abs64(a.minor)
	quo, rem := v/unit, v%unit
	quo = roundQuotient(quo, rem, unit, p.Mode)
	out := quo * unit
	if neg {
		out = -out
	}
	return Amount{minor: out, cur: a.cur}
}

func mulNoOverflow(a, b int64) (int64, error) {
	if a == 0 || b == 0 {
		return 0, nil
	}
	if a > math.MaxInt64/b {
		return 0, fmt.Errorf("%w: %d x %d", ErrAccrualRange, a, b)
	}
	return a * b, nil
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
