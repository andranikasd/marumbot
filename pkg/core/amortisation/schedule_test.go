package amortisation_test

import (
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/amortisation"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// amd5m is 5,000,000 dram at 14% over three years, paid on the 15th, ACT/365.
//
// The expected figures below were produced by an independent implementation in
// exact rational arithmetic — Python's Fraction, dividing once and rounding
// half-up to the dram — rather than by running this package and recording what
// it said. A test written the second way passes by construction and proves
// nothing.
func amd5m() (model.Contract, money.Amount) {
	c := model.Contract{
		LoanID:       "test",
		Version:      1,
		Currency:     money.MustLookup("AMD"),
		NominalRate:  money.RateFromPercent(14, 0),
		DayCount:     money.Actual365,
		Type:         model.Annuity,
		StartDate:    date.MustNew(2026, 1, 15),
		MaturityDate: date.MustNew(2029, 1, 15),
		PaymentDay:   15,
		// The quantum is stated rather than taken from the registry. These
		// expectations came from an independent implementation at one whole
		// dram, so the test must fix that unit -- otherwise it measures the
		// registry's current default instead of the engine, and changes
		// meaning silently when that default is corrected.
		Rounding: money.Policy{Mode: money.HalfUp, Unit: 100},
	}
	return c, money.FromMinor(500_000_000, money.MustLookup("AMD"))
}

func TestSolveMatchesIndependentReference(t *testing.T) {
	c, principal := amd5m()
	got, err := amortisation.Solve(c, principal, c.StartDate)
	if err != nil {
		t.Fatalf("Solve: %v", err)
	}
	const want = 17_085_400 // 170,854.00 AMD
	if got.Minor() != want {
		t.Errorf("instalment = %d minor, want %d", got.Minor(), want)
	}
}

// The constant-period annuity formula is wrong here by 34.15 AMD an instalment,
// because February is not January. This test exists so that nobody "simplifies"
// the solver into the closed form and finds out from a borrower.
func TestDatedScheduleDiffersFromTextbookAnnuity(t *testing.T) {
	c, principal := amd5m()
	got, err := amortisation.Solve(c, principal, c.StartDate)
	if err != nil {
		t.Fatalf("Solve: %v", err)
	}
	const textbook = 17_088_815
	if diff := textbook - got.Minor(); diff < 3_000 || diff > 4_000 {
		t.Errorf("dated instalment %d differs from textbook %d by %d minor; "+
			"expected roughly 3,415 — has the day-count handling changed?",
			got.Minor(), textbook, diff)
	}
}

func TestProjectRowsMatchReference(t *testing.T) {
	c, principal := amd5m()
	s, err := amortisation.SolveAndProject(c, principal, c.StartDate)
	if err != nil {
		t.Fatalf("SolveAndProject: %v", err)
	}
	if len(s.Rows) != 36 {
		t.Fatalf("rows = %d, want 36", len(s.Rows))
	}

	want := []struct {
		n                                              int
		days                                           int
		opening, interest, principal, payment, closing int64
	}{
		{1, 31, 500_000_000, 5_945_200, 11_140_200, 17_085_400, 488_859_800},
		{2, 28, 488_859_800, 5_250_200, 11_835_200, 17_085_400, 477_024_600},
		{3, 31, 477_024_600, 5_672_000, 11_413_400, 17_085_400, 465_611_200},
	}
	for _, w := range want {
		r := s.Rows[w.n-1]
		if r.Days != w.days || r.Opening.Minor() != w.opening ||
			r.Interest.Minor() != w.interest || r.Principal.Minor() != w.principal ||
			r.Payment.Minor() != w.payment || r.Closing.Minor() != w.closing {
			t.Errorf("row %d = %+v", w.n, r)
		}
	}

	// February really is shorter, and the interest really is lower for it.
	if s.Rows[1].Interest.Cmp(s.Rows[0].Interest) >= 0 {
		t.Error("February accrued at least as much interest as January; the schedule is not dated")
	}
}

// The last payment is not the level instalment. Rounding to the dram each period
// leaves a residue, and a schedule that ends on a level payment plus a stray
// balance is wrong in the way a borrower notices immediately.
func TestFinalPaymentSettlesExactly(t *testing.T) {
	c, principal := amd5m()
	s, err := amortisation.SolveAndProject(c, principal, c.StartDate)
	if err != nil {
		t.Fatalf("SolveAndProject: %v", err)
	}
	last := s.Rows[len(s.Rows)-1]
	if last.Closing.Sign() != 0 {
		t.Errorf("closing balance = %s, want zero", last.Closing)
	}
	if last.Payment.Cmp(s.Instalment) >= 0 {
		t.Errorf("final payment %s is not below the level instalment %s", last.Payment, s.Instalment)
	}
	const wantFinal = 17_081_500
	if last.Payment.Minor() != wantFinal {
		t.Errorf("final payment = %d minor, want %d", last.Payment.Minor(), wantFinal)
	}
}

func TestTotalsMatchReference(t *testing.T) {
	c, principal := amd5m()
	s, err := amortisation.SolveAndProject(c, principal, c.StartDate)
	if err != nil {
		t.Fatalf("SolveAndProject: %v", err)
	}
	if got, want := s.TotalPaid.Minor(), int64(615_070_500); got != want {
		t.Errorf("total paid = %d, want %d", got, want)
	}
	if got, want := s.TotalInterest.Minor(), int64(115_070_500); got != want {
		t.Errorf("total interest = %d, want %d", got, want)
	}
	// Money is conserved: everything paid is principal returned plus interest.
	sum, err := principal.Add(s.TotalInterest)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Cmp(s.TotalPaid) != 0 {
		t.Errorf("principal + interest = %s but total paid = %s", sum, s.TotalPaid)
	}
}

// Solve returns the SMALLEST instalment that clears the loan, so one unit less
// must fail to. This is the property that makes the answer canonical rather
// than merely sufficient.
func TestSolveIsMinimal(t *testing.T) {
	c, principal := amd5m()
	got, err := amortisation.Solve(c, principal, c.StartDate)
	if err != nil {
		t.Fatalf("Solve: %v", err)
	}
	unit := c.Rounding.Unit
	lower := money.FromMinor(got.Minor()-unit, principal.Currency())
	s, err := amortisation.Project(c, principal, lower, c.StartDate)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if s.Rows[len(s.Rows)-1].Closing.Sign() <= 0 {
		t.Errorf("one dram less (%s) also clears the loan; Solve is not minimal", lower)
	}
}

// A loan taken on the 31st falls due on the 28th in February and returns to the
// 31st in March. Stepping from the clamped date instead would drift the anchor
// permanently after one short month.
func TestPaymentDatesCarryTheContractualDay(t *testing.T) {
	c, _ := amd5m()
	c.StartDate = date.MustNew(2026, 1, 31)
	c.MaturityDate = date.MustNew(2026, 5, 31)
	c.PaymentDay = 31

	dates, err := amortisation.PaymentDates(c)
	if err != nil {
		t.Fatalf("PaymentDates: %v", err)
	}
	want := []string{"2026-02-28", "2026-03-31", "2026-04-30", "2026-05-31"}
	if len(dates) != len(want) {
		t.Fatalf("got %d dates %v, want %d", len(dates), dates, len(want))
	}
	for i, w := range want {
		if dates[i].String() != w {
			t.Errorf("date %d = %s, want %s", i, dates[i], w)
		}
	}
}

func TestRejectsImpossibleContracts(t *testing.T) {
	base, principal := amd5m()
	for name, mutate := range map[string]func(*model.Contract){
		"maturity before start": func(c *model.Contract) { c.MaturityDate = date.MustNew(2025, 1, 1) },
		"no maturity":           func(c *model.Contract) { c.MaturityDate = date.Date{} },
		"no start":              func(c *model.Contract) { c.StartDate = date.Date{} },
	} {
		t.Run(name, func(t *testing.T) {
			c := base
			mutate(&c)
			if _, err := amortisation.Solve(c, principal, c.StartDate); err == nil {
				t.Error("expected an error, got none")
			}
		})
	}
	if _, err := amortisation.Solve(base, money.FromMinor(0, base.Currency), base.StartDate); err == nil {
		t.Error("zero principal: expected an error, got none")
	}
}

// A zero-rate loan is the one case with an exact expected answer that owes
// nothing to rounding: the instalment is the principal divided by the number of
// instalments, rounded up to the settlement unit.
func TestZeroRateDividesEvenly(t *testing.T) {
	c, _ := amd5m()
	c.NominalRate = 0
	principal := money.FromMinor(3_600_000, c.Currency) // 36,000.00 AMD
	s, err := amortisation.SolveAndProject(c, principal, c.StartDate)
	if err != nil {
		t.Fatalf("SolveAndProject: %v", err)
	}
	if s.TotalInterest.Sign() != 0 {
		t.Errorf("interest = %s on a zero-rate loan", s.TotalInterest)
	}
	if got, want := s.Instalment.Minor(), int64(100_000); got != want {
		t.Errorf("instalment = %d, want %d (36,000.00 over 36 instalments)", got, want)
	}
	if s.TotalPaid.Cmp(principal) != 0 {
		t.Errorf("paid %s for a %s zero-rate loan", s.TotalPaid, principal)
	}
}

// A projection anchored on a payment date must start at the next one. The
// date itself is the instalment just paid; a row for it would accrue zero
// days and then treat a whole instalment as principal.
func TestRemainingDatesSkipTheAnchor(t *testing.T) {
	c, _ := amd5m()
	all, err := amortisation.PaymentDates(c)
	if err != nil {
		t.Fatal(err)
	}
	rest, err := amortisation.RemainingDates(c, all[2])
	if err != nil {
		t.Fatal(err)
	}
	if len(rest) != len(all)-3 || !rest[0].Equal(all[3]) {
		t.Fatalf("from %s got %d dates starting %s, want %d starting %s", all[2], len(rest), rest[0], len(all)-3, all[3])
	}
	if _, err := amortisation.RemainingDates(c, c.MaturityDate); err == nil {
		t.Fatal("no dates after maturity, yet no error")
	}
}
