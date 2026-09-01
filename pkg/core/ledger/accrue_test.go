package ledger

import (
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// An accrual interval that straddles a contract revision must charge each
// day under the terms in force on it. Accruing the whole span under one
// version was wrong by exactly the difference between the versions -- and
// revisions are now an ordinary edit, not a rarity.
func TestAccrueAcrossSplitsAtVersionBoundaries(t *testing.T) {
	amd := money.MustLookup("AMD")
	mk := func(version int, ratePct int64, from, thru date.Date) model.Contract {
		return model.Contract{
			LoanID: "l", Version: version, Currency: amd,
			NominalRate: money.RateFromPercent(ratePct, 0), DayCount: money.Actual365,
			Type: model.Annuity, StartDate: date.MustNew(2025, 1, 15),
			MaturityDate: date.MustNew(2028, 1, 15), PaymentDay: 15,
			Rounding: money.DefaultPolicy(amd), EffectiveFrom: from, EffectiveThru: thru,
		}
	}
	boundary := date.MustNew(2026, 2, 10)
	v1 := mk(1, 12, date.MustNew(2025, 1, 15), boundary)
	v2 := mk(2, 24, boundary, date.Date{})
	versions := []model.Contract{v1, v2}

	pos := model.NewBuckets(amd)
	pos.Principal = money.FromMinor(100_000_000, amd)

	from, to := date.MustNew(2026, 1, 20), date.MustNew(2026, 3, 1)
	got, err := accrueAcross(pos, versions, from, to)
	if err != nil {
		t.Fatalf("accrueAcross: %v", err)
	}

	// By hand: 21 days at 12% up to the boundary, then 19 days at 24%.
	seg1, err := accrue(pos, v1, from, boundary)
	if err != nil {
		t.Fatal(err)
	}
	seg2, err := accrue(pos, v2, boundary, to)
	if err != nil {
		t.Fatal(err)
	}
	want, err := seg1.Add(seg2)
	if err != nil {
		t.Fatal(err)
	}
	if got.Cmp(want) != 0 {
		t.Errorf("split accrual = %s, want %s (segments %s + %s)", got, want, seg1, seg2)
	}

	// And it must differ from the old single-version behaviour, or the split
	// proves nothing.
	whole, err := accrue(pos, v2, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if got.Cmp(whole) == 0 {
		t.Error("split accrual equals accruing the whole span at the later rate; the fixture is too weak")
	}

	// A single-version loan is exactly the old arithmetic.
	single, err := accrueAcross(pos, []model.Contract{mk(1, 12, date.MustNew(2025, 1, 15), date.Date{})}, from, to)
	if err != nil {
		t.Fatalf("single version: %v", err)
	}
	old, err := accrue(pos, mk(1, 12, date.MustNew(2025, 1, 15), date.Date{}), from, to)
	if err != nil {
		t.Fatal(err)
	}
	if single.Cmp(old) != 0 {
		t.Errorf("single-version accrual changed: %s vs %s", single, old)
	}
}

// On the shared boundary date both versions cover; the later effective_from
// must win deterministically regardless of slice order.
func TestContractForPrefersTheLatestEffectiveFrom(t *testing.T) {
	amd := money.MustLookup("AMD")
	boundary := date.MustNew(2026, 2, 10)
	old := model.Contract{LoanID: "l", Version: 1, Currency: amd, EffectiveFrom: date.MustNew(2025, 1, 1), EffectiveThru: boundary}
	newer := model.Contract{LoanID: "l", Version: 2, Currency: amd, EffectiveFrom: boundary}

	for _, order := range [][]model.Contract{{old, newer}, {newer, old}} {
		got, err := contractFor(order, boundary)
		if err != nil {
			t.Fatalf("contractFor: %v", err)
		}
		if got.Version != 2 {
			t.Errorf("boundary date resolved to version %d; want the newer one", got.Version)
		}
	}
}
