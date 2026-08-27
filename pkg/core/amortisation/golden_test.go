package amortisation_test

import (
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/amortisation"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// The Central Bank of Armenia publishes worked examples in Regulation 8/01,
// including full amortisation tables. They are the closest thing to an
// authoritative Armenian fixture that exists in public, so they are the first
// entries in the golden corpus.
//
// Example 1: 500,000 AMD at 10% over 12 monthly payments.
// Published: level payment 43,955.44, total interest 27,465.31, APR 10.47%.
// Source: https://www.arlis.am/hy/acts/140917
//
// The day offsets are the regulator's own, and they are not a tidy sequence --
// 31, 62, 90, 121 -- which is precisely the point. A constant-period annuity on
// this loan gives 43,957.94, and the CBA's figure is the DATED one. The
// regulator's arithmetic is the same arithmetic this package does.
func cbaExample1() (model.Contract, money.Amount, []date.Date) {
	amd := money.MustLookup("AMD")
	// The regulation prints day offsets rather than dates. Their gaps are
	// 31, 31, 28, 31, 30, ... -- two 31-day months and then February -- which
	// fixes the disbursement to 15 December of a year whose following February
	// has 28 days. All twelve offsets then reproduce exactly.
	start := date.MustNew(2025, 12, 15)
	c := model.Contract{
		LoanID:       "cba-8-01-ex-1",
		Version:      1,
		Currency:     amd,
		NominalRate:  money.RateFromPercent(10, 0),
		DayCount:     money.Actual365,
		Type:         model.Annuity,
		StartDate:    start,
		MaturityDate: date.MustNew(2026, 12, 15),
		PaymentDay:   15,
		// One luma, not one dram. The CBA's own tables carry two decimals, and
		// nothing in Armenian law prescribes rounding to the whole dram --
		// verified across the Civil Code, the Consumer Lending Law, the Mortgage
		// Law and Regulations 8/01 and 8/05. Whole-dram settlement is common
		// practice, and practice belongs in a product policy rather than in the
		// engine.
		Rounding: money.Policy{Mode: money.HalfUp, Unit: 1},
	}
	return c, money.FromMinor(50_000_000, amd), nil
}

// The interest column is what pins the day-count handling: it is dated ACT/365
// on the actual outstanding balance, and every row is checkable against the
// published table.
func TestCBAExample1InterestMatchesThePublishedTable(t *testing.T) {
	c, principal, _ := cbaExample1()

	// Project at the published payment rather than solving, so this test
	// measures the accrual and nothing else.
	published := money.FromMinor(4_395_544, c.Currency) // 43,955.44
	s, err := amortisation.Project(c, principal, published, c.StartDate)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if len(s.Rows) != 12 {
		t.Fatalf("rows = %d, want 12", len(s.Rows))
	}

	// The published offsets, differenced.
	wantDays := []int{31, 31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30}
	for i, w := range wantDays {
		if s.Rows[i].Days != w {
			t.Errorf("row %d spans %d days, want %d", i+1, s.Rows[i].Days, w)
		}
	}
	// The first row is the one the regulator's own worked note derives by hand.
	if got, want := s.Rows[0].Interest.Minor(), int64(424_658); got != want {
		t.Errorf("row 1 interest = %d, want %d (4,246.58)", got, want)
	}
}

// The distinction this fixture exists to defend. A constant-period annuity is
// wrong on the regulator's own example, and by enough to see.
func TestCBAExample1RejectsTheConstantPeriodFormula(t *testing.T) {
	c, principal, _ := cbaExample1()
	got, err := amortisation.Solve(c, principal, c.StartDate)
	if err != nil {
		t.Fatalf("Solve: %v", err)
	}
	const (
		published = 4_395_544 // 43,955.44 dated
		textbook  = 4_395_794 // 43,957.94 constant-period
	)
	if got.Minor() >= textbook {
		t.Errorf("solved %d is at or above the constant-period figure %d; "+
			"the schedule is not dated", got.Minor(), textbook)
	}
	// Within one luma of the regulator's published figure. Not exact, and the
	// difference is a documented convention rather than an error: the CBA
	// accrues at full precision and rounds for display, so its payment is the
	// exact annuity rounded down; this engine rounds each period to the
	// settlement unit and returns the smallest payment that clears, which lands
	// one unit higher. Both agree to the dram, which is the unit anyone pays in.
	if d := got.Minor() - published; d < -1 || d > 1 {
		t.Errorf("solved %d differs from published %d by %d luma, want within 1",
			got.Minor(), published, d)
	}
}
