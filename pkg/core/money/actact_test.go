package money

import "testing"

// A period that straddles New Year into a leap year is divided by 365 for its
// December days and 366 for its January days. This is the whole reason
// ActualActual cannot use a single denominator, and the test pins the split.
func TestActualActualSplitsAtTheYearBoundary(t *testing.T) {
	p := FromMinor(10_000_000, AMD) // 100,000.00 AMD
	r := RateFromPercent(12, 0)
	luma := Policy{Mode: HalfUp, Unit: 1}

	// 20 Dec 2027 → 20 Jan 2028: 11 days in 2027 (365), 20 in 2028 (leap, 366).
	got, err := AccrueBetween(p, r, []YearSpan{{2027, 11}, {2028, 20}}, ActualActual, luma)
	if err != nil {
		t.Fatal(err)
	}
	// 100,000 × 12% × (11/365 + 20/366) = 361.64 + 655.74 = 1,017.38
	if want := int64(101_738); got.Minor() != want {
		t.Errorf("act/act across the boundary = %d, want %d", got.Minor(), want)
	}

	// The same 31 days entirely inside 2027 divide by 365 throughout.
	flat, err := AccrueBetween(p, r, []YearSpan{{2027, 31}}, ActualActual, luma)
	if err != nil {
		t.Fatal(err)
	}
	if want := int64(101_918); flat.Minor() != want { // 100,000 × 12% × 31/365
		t.Errorf("act/act within one year = %d, want %d", flat.Minor(), want)
	}
	if got.Minor() >= flat.Minor() {
		t.Error("days in a leap year should accrue LESS than the same count in a common year")
	}
}

// For every convention with one divisor, AccrueBetween is exactly Accrue over
// the total: the split must not change a figure it does not apply to.
func TestAccrueBetweenIsAccrueForFixedDenominators(t *testing.T) {
	p := FromMinor(501_300_200, AMD)
	r := RateFromPercent(17, 150000)
	pol := Policy{Mode: HalfUp, Unit: 10}
	for _, dc := range []DayCount{Actual365, Actual360, Thirty360} {
		want, err := Accrue(p, r, 31, dc, pol)
		if err != nil {
			t.Fatal(err)
		}
		got, err := AccrueBetween(p, r, []YearSpan{{2027, 11}, {2028, 20}}, dc, pol)
		if err != nil {
			t.Fatal(err)
		}
		if got.Cmp(want) != 0 {
			t.Errorf("%s: AccrueBetween %s != Accrue %s", dc, got, want)
		}
	}
}

// Rounding happens once, after the parts are summed. Rounding each part and
// then adding would let the year boundary itself move the figure by a unit.
func TestActualActualRoundsOnce(t *testing.T) {
	p := FromMinor(10_000_000, AMD)
	r := RateFromPercent(12, 0)
	coarse := Policy{Mode: HalfUp, Unit: 100} // whole dram
	got, err := AccrueBetween(p, r, []YearSpan{{2027, 11}, {2028, 20}}, ActualActual, coarse)
	if err != nil {
		t.Fatal(err)
	}
	// exact sum 1,017.38 → whole dram 1,017; per-part rounding would give
	// 362 + 656 = 1,018.
	if got.Minor() != 101_700 {
		t.Errorf("got %d, want 101700 (one rounding of the summed exact figure)", got.Minor())
	}
}

func TestYearDenominator(t *testing.T) {
	for _, c := range []struct {
		dc   DayCount
		year int
		want int64
	}{
		{ActualActual, 2028, 366},
		{ActualActual, 2027, 365},
		{ActualActual, 2000, 366},
		{ActualActual, 1900, 365},
		{ActualActual, 2100, 365},
		{Actual365, 2028, 365},
		{Actual360, 2028, 360},
	} {
		if got := c.dc.YearDenominator(c.year); got != c.want {
			t.Errorf("%s in %d = %d, want %d", c.dc, c.year, got, c.want)
		}
	}
}
