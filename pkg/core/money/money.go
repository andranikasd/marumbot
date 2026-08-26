// Package money holds the only arithmetic Marum performs on amounts.
//
// An Amount is an integer count of a currency's minor units. Nothing outside
// this package may do arithmetic on the underlying int64, and no operation
// anywhere in Marum may route an amount through a floating-point value.
package money

import (
	"errors"
	"fmt"
	"math"
)

// Errors returned by arithmetic. Currency mismatch is a programmer error and
// panics; overflow is a runtime condition and is returned.
var (
	ErrOverflow = errors.New("amount overflow")
	ErrNegative = errors.New("amount must not be negative")
)

// Amount is a signed quantity of money. The zero Amount is unusable until it
// is given a currency; use Zero.
type Amount struct {
	minor int64
	cur   Currency
}

// Zero returns the zero amount in cur.
func Zero(cur Currency) Amount { return Amount{cur: cur} }

// FromMinor builds an Amount from a count of minor units.
func FromMinor(minor int64, cur Currency) Amount { return Amount{minor: minor, cur: cur} }

// FromMajor builds an Amount from whole currency units — 1500 AMD becomes
// 150000 minor units.
func FromMajor(major int64, cur Currency) (Amount, error) {
	scale := pow10(cur.Exponent)
	if major != 0 && (major > math.MaxInt64/scale || major < math.MinInt64/scale) {
		return Amount{}, fmt.Errorf("%w: %d %s", ErrOverflow, major, cur.Code)
	}
	return Amount{minor: major * scale, cur: cur}, nil
}

// Minor returns the raw count of minor units. Intended for persistence and
// tests; callers must not compute with it.
func (a Amount) Minor() int64       { return a.minor }
func (a Amount) Currency() Currency { return a.cur }
func (a Amount) IsZero() bool       { return a.minor == 0 }
func (a Amount) Sign() int {
	switch {
	case a.minor > 0:
		return 1
	case a.minor < 0:
		return -1
	}
	return 0
}

// String renders the amount for a human.
//
// Sub-units are shown only when the currency actually circulates them: AMD has
// two decimal places on paper but the luma is obsolete and lenders settle in
// whole drams, so "1,740,927 AMD" is what a borrower recognises and
// "1740927.00 AMD" is noise. A value that is not a whole settlement unit keeps
// its digits, because hiding them would hide a rounding bug.
func (a Amount) String() string {
	scale := pow10(a.cur.Exponent)
	minor := a.minor
	sign := ""
	if minor < 0 {
		sign, minor = "-", -minor
	}
	whole, frac := minor/scale, minor%scale

	unit := a.cur.SettlementUnit
	if unit < 1 {
		unit = 1
	}
	if a.cur.Exponent == 0 || (unit > 1 && frac == 0) {
		return fmt.Sprintf("%s%s %s", sign, group(whole), a.cur.Code)
	}
	return fmt.Sprintf("%s%s.%0*d %s", sign, group(whole), int(a.cur.Exponent), frac, a.cur.Code)
}

// group inserts thin thousands separators. A seven-digit balance is unreadable
// without them, and a misread balance is the failure this product exists to
// prevent.
func group(n int64) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	return string(out)
}

// mustMatch panics on a currency mismatch. Mixing currencies is a bug in the
// caller, not a condition a user can cause, and a panic surfaces it in tests
// long before it reaches anyone.
func (a Amount) mustMatch(b Amount) {
	if a.cur.Code != b.cur.Code {
		panic(fmt.Sprintf("money: currency mismatch %s vs %s", a.cur.Code, b.cur.Code))
	}
}

// Add returns a+b, or ErrOverflow.
func (a Amount) Add(b Amount) (Amount, error) {
	a.mustMatch(b)
	sum := a.minor + b.minor
	// Overflow happened iff the operands share a sign that the result does not.
	if (a.minor > 0 && b.minor > 0 && sum < 0) || (a.minor < 0 && b.minor < 0 && sum >= 0) {
		return Amount{}, fmt.Errorf("%w: %d + %d", ErrOverflow, a.minor, b.minor)
	}
	return Amount{minor: sum, cur: a.cur}, nil
}

// Sub returns a-b, or ErrOverflow.
func (a Amount) Sub(b Amount) (Amount, error) {
	a.mustMatch(b)
	diff := a.minor - b.minor
	if (b.minor < 0 && diff < a.minor) || (b.minor > 0 && diff > a.minor) {
		return Amount{}, fmt.Errorf("%w: %d - %d", ErrOverflow, a.minor, b.minor)
	}
	return Amount{minor: diff, cur: a.cur}, nil
}

// Neg returns -a.
func (a Amount) Neg() Amount { return Amount{minor: -a.minor, cur: a.cur} }

// Cmp reports whether a is less than, equal to, or greater than b.
func (a Amount) Cmp(b Amount) int {
	a.mustMatch(b)
	switch {
	case a.minor < b.minor:
		return -1
	case a.minor > b.minor:
		return 1
	}
	return 0
}

// Min returns the smaller of a and b.
func Min(a, b Amount) Amount {
	if a.Cmp(b) <= 0 {
		return a
	}
	return b
}

func pow10(e uint8) int64 {
	p := int64(1)
	for i := uint8(0); i < e; i++ {
		p *= 10
	}
	return p
}
