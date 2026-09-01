package amortisation

import (
	"errors"
	"strings"
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// A lender-stated instalment is the authority -- until the arithmetic
// contradicts it. Then it is a typo or a misread contract, and Build must
// refuse by name rather than project a schedule that never clears.
func TestStatedInstalmentIsValidated(t *testing.T) {
	amd := money.MustLookup("AMD")
	start := date.MustNew(2026, 1, 15)
	base := model.Contract{
		LoanID: "l", Version: 1, Currency: amd, EffectiveFrom: start,
		NominalRate: money.RateFromPercent(18, 0), DayCount: money.Actual365,
		Type: model.Annuity, StartDate: start, MaturityDate: date.MustNew(2028, 1, 15),
		PaymentDay: 15, Rounding: money.DefaultPolicy(amd),
	}
	principal := money.FromMinor(200_000_000, amd)

	solved, err := Solve(base, principal, date.Date{})
	if err != nil {
		t.Fatalf("solve: %v", err)
	}

	// The solved figure, stated: fine.
	ok := base
	ok.HasScheduled, ok.ScheduledPayment = true, solved
	if _, err := Build(ok, principal, date.Date{}); err != nil {
		t.Fatalf("the solved instalment refused: %v", err)
	}

	// Below the first month's interest: the balance would grow.
	growing := base
	growing.HasScheduled, growing.ScheduledPayment = true, money.FromMinor(100_000, amd)
	_, err = Build(growing, principal, date.Date{})
	if !errors.Is(err, ErrUnsolvable) || !strings.Contains(err.Error(), "does not cover the first interest") {
		t.Fatalf("growing balance not refused by name: %v", err)
	}

	// Covers interest but cannot finish: a balance stays at maturity.
	short := base
	short.HasScheduled, short.ScheduledPayment = true, money.FromMinor(solved.Minor()*8/10, amd)
	_, err = Build(short, principal, date.Date{})
	if !errors.Is(err, ErrUnsolvable) || !strings.Contains(err.Error(), "owed at maturity") {
		t.Fatalf("non-clearing schedule not refused by name: %v", err)
	}
}
