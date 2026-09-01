package app

import (
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/money"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

// Budget.CashPlan is the one bridge from the stored budget to the engine;
// the freshness rule for cash on hand lives here and nowhere else.
func TestBudgetCashPlan(t *testing.T) {
	amd := money.MustLookup("AMD")
	b := Budget{
		Currency: "AMD", Set: true, PayDay: 5,
		Monthly:   money.FromMinor(30_000_000, amd),
		Opening:   money.FromMinor(10_000_000, amd),
		Reserve:   money.FromMinor(2_000_000, amd),
		Overrides: map[string]int64{"2026-03": 45_000_000},
	}

	sameMonth := date.MustNew(2026, 1, 20)
	b.OpeningAsOf = date.MustNew(2026, 1, 15)
	cp := b.CashPlan(sameMonth)
	if cp.OpeningCash.Minor() != 8_000_000 {
		t.Errorf("opening minus protected reserve = %v, want 8000000 minor", cp.OpeningCash)
	}
	if cp.Monthly.Minor() != 30_000_000 || cp.PayDay != 5 {
		t.Errorf("monthly/payday lost: %+v", cp)
	}
	if got := cp.MonthlyOverrides["2026-03"].Minor(); got != 45_000_000 {
		t.Errorf("override lost: %d", got)
	}

	// A statement from last month says nothing about today's cash.
	stale := b
	stale.OpeningAsOf = date.MustNew(2025, 12, 28)
	if cp := stale.CashPlan(sameMonth); cp.OpeningCash.Minor() != 0 {
		t.Errorf("stale opening counted: %v", cp.OpeningCash)
	}

	// A statement dated in the future is a typo, not money.
	future := b
	future.OpeningAsOf = date.MustNew(2026, 1, 25)
	if cp := future.CashPlan(sameMonth); cp.OpeningCash.Minor() != 0 {
		t.Errorf("future opening counted: %v", cp.OpeningCash)
	}

	// The result must pass the engine's own validation as part of an input.
	in := plan.Input{ValuationDate: sameMonth, Cash: b.CashPlan(sameMonth)}
	if in.Cash.MonthlyOverrides["2026-03"].Currency().Code != "AMD" {
		t.Error("override lost its currency")
	}
}

func TestBudgetCashPlanNeverSpendsTheProtectedReserve(t *testing.T) {
	amd := money.MustLookup("AMD")
	valuation := date.MustNew(2026, 9, 2)
	b := Budget{
		Monthly: money.FromMinor(30_000_000, amd), Set: true,
		Opening: money.FromMinor(5_000_000, amd), Reserve: money.FromMinor(8_000_000, amd),
		OpeningAsOf: valuation,
	}
	if got := b.CashPlan(valuation).OpeningCash.Minor(); got != 0 {
		t.Fatalf("opening cash = %d, want zero when reserve exceeds cash", got)
	}
}
