// Package date holds a date without a time or a zone.
//
// A due date is not an instant. "5 September" is the same business fact
// whether the borrower reads it at midnight or at noon, and attaching a clock
// to it invites an off-by-one every time a value crosses a zone boundary.
// Time zones are applied in exactly one place — converting a local reminder
// slot into an instant to send at — and never inside the engine.
package date

import (
	"errors"
	"fmt"
	"time"
)

// Date is a calendar date in the proleptic Gregorian calendar.
// The zero Date is invalid; construct with New, Parse or From.
type Date struct {
	y int
	m time.Month
	d int
}

// ErrInvalid is returned for a date that cannot exist.
var ErrInvalid = errors.New("invalid date")

// New validates and builds a Date. A day beyond the month's length is an
// error, not a silent roll-over into the next month.
func New(year int, month time.Month, day int) (Date, error) {
	if month < time.January || month > time.December {
		return Date{}, fmt.Errorf("%w: month %d", ErrInvalid, int(month))
	}
	if day < 1 || day > DaysInMonth(year, month) {
		return Date{}, fmt.Errorf("%w: %04d-%02d-%02d", ErrInvalid, year, int(month), day)
	}
	return Date{y: year, m: month, d: day}, nil
}

// MustNew is New for constants and tests.
func MustNew(year int, month time.Month, day int) Date {
	d, err := New(year, month, day)
	if err != nil {
		panic(err)
	}
	return d
}

// Parse reads an ISO 8601 calendar date, YYYY-MM-DD.
func Parse(s string) (Date, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return Date{}, fmt.Errorf("%w: %q", ErrInvalid, s)
	}
	return Date{y: t.Year(), m: t.Month(), d: t.Day()}, nil
}

// From takes the calendar date of an instant as observed in loc.
func From(t time.Time, loc *time.Location) Date {
	t = t.In(loc)
	return Date{y: t.Year(), m: t.Month(), d: t.Day()}
}

// Year returns the calendar year.
func (d Date) Year() int { return d.y }

// Month returns the calendar month.
func (d Date) Month() time.Month { return d.m }

// Day returns the day of the month.
func (d Date) Day() int { return d.d }

// IsZero reports whether the date is unset.
func (d Date) IsZero() bool   { return d == Date{} }
func (d Date) String() string { return fmt.Sprintf("%04d-%02d-%02d", d.y, int(d.m), d.d) }

// time renders the date as midnight UTC. Used only for internal arithmetic;
// it is deliberately unexported so no caller can mistake it for an instant.
func (d Date) time() time.Time {
	return time.Date(d.y, d.m, d.d, 0, 0, 0, 0, time.UTC)
}

// AtLocal converts the date and a local wall-clock time into an instant. This
// is the single sanctioned crossing from business date to timestamp.
func (d Date) AtLocal(hour, minute int, loc *time.Location) time.Time {
	return time.Date(d.y, d.m, d.d, hour, minute, 0, 0, loc)
}

// Compare orders two dates: -1, 0 or 1.
func (d Date) Compare(o Date) int {
	switch {
	case d.y != o.y:
		if d.y < o.y {
			return -1
		}
		return 1
	case d.m != o.m:
		if d.m < o.m {
			return -1
		}
		return 1
	case d.d != o.d:
		if d.d < o.d {
			return -1
		}
		return 1
	}
	return 0
}

// Before reports whether d precedes o.
func (d Date) Before(o Date) bool { return d.Compare(o) < 0 }

// After reports whether d follows o.
func (d Date) After(o Date) bool { return d.Compare(o) > 0 }

// Equal reports whether the two dates are the same day.
func (d Date) Equal(o Date) bool { return d == o }

// AddDays returns the date n days later; n may be negative.
func AddDays(d Date, n int) Date {
	t := d.time().AddDate(0, 0, n)
	return Date{y: t.Year(), m: t.Month(), d: t.Day()}
}

// DaysBetween returns the number of days from a to b, negative if b precedes
// a. This is the actual day count that Actual/365 and Actual/360 accrue over.
func DaysBetween(a, b Date) int {
	return int(b.time().Sub(a.time()).Hours() / 24)
}

// Days30360 counts days under the 30/360 US convention, where every month is
// thirty days long. It is the convention a naive rate/12 calculator assumes
// without saying so, and it is why such calculators disagree with statements.
func Days30360(a, b Date) int {
	d1, d2 := a.d, b.d
	if d1 == 31 {
		d1 = 30
	}
	if d2 == 31 && d1 == 30 {
		d2 = 30
	}
	return (b.y-a.y)*360 + (int(b.m)-int(a.m))*30 + (d2 - d1)
}

// DaysInMonth returns the length of the given month.
func DaysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// IsLeapYear reports whether the year has a 29 February.
func IsLeapYear(year int) bool {
	return (year%4 == 0 && year%100 != 0) || year%400 == 0
}

// EndOfMonth returns the last day of the date's month.
func (d Date) EndOfMonth() Date {
	return Date{y: d.y, m: d.m, d: DaysInMonth(d.y, d.m)}
}
