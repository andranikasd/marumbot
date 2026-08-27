package amortisation_test

import (
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/amortisation"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// Same loan as the annuity tests, so the two structures can be compared
// directly. Expected figures come from the same independent rational-arithmetic
// reference, not from this package.
func TestDecliningMatchesReference(t *testing.T) {
	c, principal := amd5m()
	c.Type = model.DecliningPrincipal

	s, err := amortisation.Build(c, principal, c.StartDate)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(s.Rows) != 36 {
		t.Fatalf("rows = %d, want 36", len(s.Rows))
	}

	want := []struct {
		n                                              int
		days                                           int
		opening, interest, principal, payment, closing int64
	}{
		{1, 31, 500_000_000, 5_945_200, 13_888_800, 19_834_000, 486_111_200},
		{2, 28, 486_111_200, 5_220_700, 13_888_800, 19_109_500, 472_222_400},
		{3, 31, 472_222_400, 5_614_900, 13_888_800, 19_503_700, 458_333_600},
	}
	for _, w := range want {
		r := s.Rows[w.n-1]
		if r.Days != w.days || r.Opening.Minor() != w.opening ||
			r.Interest.Minor() != w.interest || r.Principal.Minor() != w.principal ||
			r.Payment.Minor() != w.payment || r.Closing.Minor() != w.closing {
			t.Errorf("row %d = %+v", w.n, r)
		}
	}

	last := s.Rows[len(s.Rows)-1]
	if last.Closing.Sign() != 0 {
		t.Errorf("closing = %s, want zero", last.Closing)
	}
	if got, want := last.Payment.Minor(), int64(14_057_200); got != want {
		t.Errorf("final payment = %d, want %d", got, want)
	}
	if got, want := s.TotalInterest.Minor(), int64(107_824_300); got != want {
		t.Errorf("total interest = %d, want %d", got, want)
	}
}

// Every principal payment is identical except the last, which absorbs the
// rounding remainder. That is the defining property of the structure.
func TestDecliningPrincipalIsLevel(t *testing.T) {
	c, principal := amd5m()
	c.Type = model.DecliningPrincipal
	s, err := amortisation.Build(c, principal, c.StartDate)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	first := s.Rows[0].Principal
	for _, r := range s.Rows[:len(s.Rows)-1] {
		if r.Principal.Cmp(first) != 0 {
			t.Fatalf("row %d principal %s differs from %s", r.N, r.Principal, first)
		}
	}
	// And the payment falls, monotonically, because the balance does.
	for i := 1; i < len(s.Rows); i++ {
		if s.Rows[i].Opening.Cmp(s.Rows[i-1].Opening) >= 0 {
			t.Fatalf("balance did not fall at row %d", i+1)
		}
	}
}

// Declining principal costs less interest than an annuity over the same term,
// and demands more in the first month. Both halves matter: the cheaper option
// is the one the borrower may not be able to afford.
func TestDecliningCostsLessButDemandsMoreEarly(t *testing.T) {
	c, principal := amd5m()

	ann, err := amortisation.Build(c, principal, c.StartDate)
	if err != nil {
		t.Fatalf("annuity: %v", err)
	}
	c.Type = model.DecliningPrincipal
	dec, err := amortisation.Build(c, principal, c.StartDate)
	if err != nil {
		t.Fatalf("declining: %v", err)
	}

	if dec.TotalInterest.Cmp(ann.TotalInterest) >= 0 {
		t.Errorf("declining interest %s is not below annuity %s",
			dec.TotalInterest, ann.TotalInterest)
	}
	if dec.Rows[0].Payment.Cmp(ann.Rows[0].Payment) <= 0 {
		t.Errorf("declining first payment %s is not above annuity %s",
			dec.Rows[0].Payment, ann.Rows[0].Payment)
	}
	// Money is conserved in both.
	for name, s := range map[string]amortisation.Schedule{"annuity": ann, "declining": dec} {
		sum, err := principal.Add(s.TotalInterest)
		if err != nil {
			t.Fatal(err)
		}
		if sum.Cmp(s.TotalPaid) != 0 {
			t.Errorf("%s: principal + interest = %s but paid %s", name, sum, s.TotalPaid)
		}
	}
}

// When the lender states the instalment, that figure is authoritative. Solving
// for one and using it instead would make the engine disagree with the paperwork
// by a dram, which is the engine being wrong.
func TestBuildPrefersTheContractualInstalment(t *testing.T) {
	c, principal := amd5m()
	stated := money.FromMinor(18_000_000, c.Currency) // above the solved figure
	c.ScheduledPayment = stated
	c.HasScheduled = true

	s, err := amortisation.Build(c, principal, c.StartDate)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if s.Instalment.Cmp(stated) != 0 {
		t.Errorf("instalment = %s, want the contractual %s", s.Instalment, stated)
	}
	// A larger instalment retires the loan sooner.
	if len(s.Rows) >= 36 {
		t.Errorf("rows = %d; a larger instalment should finish early", len(s.Rows))
	}
}
