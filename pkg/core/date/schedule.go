package date

import "time"

// AddMonths advances by n calendar months, clamping the day to the length of
// the target month: 31 January plus one month is 28 or 29 February.
//
// Repeated application DRIFTS, and that is correct for a general date shift but
// wrong for a payment schedule. Use OnDayOfMonth or Occurrence to generate due
// dates, so a loan due on the 31st returns to the 31st in the next long month
// instead of staying on the 28th forever.
func AddMonths(d Date, n int) Date {
	y, m := shift(d.y, d.m, n)
	day := d.d
	if max := DaysInMonth(y, m); day > max {
		day = max
	}
	return Date{y: y, m: m, d: day}
}

// OnDayOfMonth returns the given day of the date's month, clamped to the
// month's length. Day 31 in a 30-day month is the 30th; in February it is the
// 28th or 29th.
func OnDayOfMonth(d Date, day int) Date {
	if day < 1 {
		day = 1
	}
	if max := DaysInMonth(d.y, d.m); day > max {
		day = max
	}
	return Date{y: d.y, m: d.m, d: day}
}

// Occurrence returns the nth payment date of a schedule anchored on
// contractualDay, counting from the month of start.
//
// The anchor day is carried, not the previously clamped date, so a schedule
// due on the 31st reads 31 Jan, 28 Feb, 31 Mar — which is what lenders do, and
// what naive month arithmetic gets wrong by drifting to 28 Mar.
func Occurrence(start Date, contractualDay, n int) Date {
	y, m := shift(start.y, start.m, n)
	day := contractualDay
	if day < 1 {
		day = 1
	}
	if max := DaysInMonth(y, m); day > max {
		day = max
	}
	return Date{y: y, m: m, d: day}
}

func shift(y int, m time.Month, n int) (int, time.Month) {
	total := (y*12 + int(m) - 1) + n
	return total / 12, time.Month(total%12 + 1)
}
