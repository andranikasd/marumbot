package app

import (
	"context"
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/money"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

type replayHistoryFake struct {
	comparisonHistoryFake
	manifest PlanManifest
}

func (h *replayHistoryFake) PlanVersion(_ context.Context, user, id string) (PlanVersion, error) {
	if user != "owner" || id != "historical" {
		return PlanVersion{}, ErrNotFound
	}
	return PlanVersion{Manifest: h.manifest}, nil
}

func TestHistoricalPlanEffectivePermission(t *testing.T) {
	for _, kind := range []string{"monthly override", "spending override", "cycle override", "policy change"} {
		t.Run(kind, func(t *testing.T) {
			w, _, _, m := comparisonWorker(t)
			cur := m.Input.Cash.Monthly.Currency()
			want := money.FromMinor(30_000_000, cur)
			switch kind {
			case "monthly override":
				m.Input.Cash.MonthlyOverrides = map[string]money.Amount{"2026-02": want}
			case "spending override":
				m.Input.Cash.Spending = &plan.SpendingPlan{Monthly: m.Input.Cash.Monthly, Overrides: map[string]money.Amount{"2026-02": want}}
			case "cycle override":
				m.Input.Cash.Spending = &plan.SpendingPlan{Monthly: m.Input.Cash.Monthly, CycleDay: 20, Overrides: map[string]money.Amount{"2026-01": want}}
			case "policy change":
				m.Input.Cash.Spending = &plan.SpendingPlan{Monthly: m.Input.Cash.Monthly, Overrides: map[string]money.Amount{"2026-02": money.FromMinor(29_000_000, cur)}, Changes: []plan.SpendingChange{{On: m.Input.ValuationDate, Limit: want}, {On: date.MustNew(2026, 3, 1), Limit: money.FromMinor(31_000_000, cur)}}}
			}
			report, err := plan.Search(m.Input, m.Goal)
			if err != nil {
				t.Fatal(err)
			}
			m, err = manifestFor(m.Input, m.Goal, report, 11)
			if err != nil {
				t.Fatal(err)
			}
			h := &replayHistoryFake{manifest: m}
			h.sources = m.Sources
			w.History = h
			original, err := sheetFromReport(m.Input, m.Goal, report, m.Input.Loans[0].Balance, want, nil)
			if err != nil {
				t.Fatal(err)
			}
			historical, err := w.HistoricalPlan(t.Context(), "owner", "historical")
			if err != nil {
				t.Fatal(err)
			}
			if historical.Summary.BudgetMinor != original.Summary.BudgetMinor {
				t.Fatalf("historical budget=%d original=%d", historical.Summary.BudgetMinor, original.Summary.BudgetMinor)
			}
			if historical.Summary.InterestMinor != original.Summary.InterestMinor || historical.Summary.PayoffDate != original.Summary.PayoffDate {
				t.Fatal("historical arithmetic changed")
			}
		})
	}
}
