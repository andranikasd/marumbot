package app

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

// The metadata port returns versions AND their activation revision on each read.
// Keep the fallback store separate so both optional-port paths are exercised.
type countedSheetHistory struct {
	PlanHistoryStore
	calls                      []string
	sources                    string
	sourceReads, metadataReads int
	changeAt                   int
	sourceErrorAt              int
	metadataErrorAt            int
	manifest                   PlanManifest
	finalOutdated              bool
	proposalPublishedEarly     bool
	worker                     *Worker
}

func (h *countedSheetHistory) PlanSources(context.Context, string) (string, error) {
	h.calls = append(h.calls, "sources")
	h.sourceReads++
	if len(h.worker.proposals.rows) != 0 {
		h.proposalPublishedEarly = true
	}
	if h.sourceReads == h.sourceErrorAt {
		return "", errors.New("source read failed")
	}
	return h.sources, nil
}

func (h *countedSheetHistory) PlanHistory(context.Context, string) ([]PlanVersion, int64, error) {
	return h.readMetadata("history")
}

func (h *countedSheetHistory) readMetadata(name string) ([]PlanVersion, int64, error) {
	h.calls = append(h.calls, name)
	h.metadataReads++
	if len(h.worker.proposals.rows) != 0 {
		h.proposalPublishedEarly = true
	}
	if h.metadataReads == h.changeAt {
		h.sources = "changed-sources"
	}
	if h.metadataReads == h.metadataErrorAt {
		return nil, 0, errors.New("metadata read failed")
	}
	if h.metadataReads == 1 {
		return nil, 7, nil
	}
	m := h.manifest
	if h.finalOutdated {
		m.Sources = "older-sources"
	}
	return []PlanVersion{{ID: "newly-active", Currency: "AMD", Active: true, Manifest: m}}, 8, nil
}

type countedActiveSheetHistory struct{ *countedSheetHistory }

func (h countedActiveSheetHistory) ActivePlanVersions(context.Context, string) ([]PlanVersion, int64, error) {
	return h.readMetadata("active")
}

func sheetSourcesFixture(t *testing.T, activeReader bool) (*Worker, *countedSheetHistory) {
	t.Helper()
	w, in := chartWorker(t)
	g := plan.Goal{Kind: plan.LeastInterest}
	report, err := plan.Search(in, g)
	if err != nil {
		t.Fatal(err)
	}
	m, err := manifestFor(in, g, report, 0)
	if err != nil {
		t.Fatal(err)
	}
	m.Sources = "original-sources"
	h := &countedSheetHistory{sources: m.Sources, manifest: m, worker: w}
	w.History = h
	if activeReader {
		w.History = countedActiveSheetHistory{h}
	}
	return w, h
}

func TestPlanSheetSourcesReadTwiceAfterFinalMetadata(t *testing.T) {
	for _, activeReader := range []bool{false, true} {
		name := "history"
		if activeReader {
			name = "active"
		}
		t.Run(name, func(t *testing.T) {
			w, h := sheetSourcesFixture(t, activeReader)
			sh, err := w.PlanSheet(t.Context(), "chart-user", nil)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(h.calls, []string{"sources", name, name, "sources"}) {
				t.Fatalf("read order/count: %v", h.calls)
			}
			if h.proposalPublishedEarly {
				t.Fatal("proposal published before final metadata/source check")
			}
			if sh.ActiveRevision != 8 || !sh.AnyPlan || !sh.Approved || sh.Outdated {
				t.Fatalf("final activation metadata lost: revision=%d any=%t approved=%t outdated=%t", sh.ActiveRevision, sh.AnyPlan, sh.Approved, sh.Outdated)
			}
			m, ok := w.proposals.get("chart-user", sh.Proposal)
			if !ok || m.Sources != h.sources {
				t.Fatal("guarded proposal missing or uses different sources")
			}
			// A pre-existing outdated approval must still be marked against the guarded sources.
			h.finalOutdated = true
			sh, err = w.PlanSheet(t.Context(), "chart-user", nil)
			if err != nil || !sh.Outdated || sh.Approved {
				t.Fatalf("outdated classification changed: %v", err)
			}
		})
	}
}

func TestPlanSheetSourceChangeDuringMetadataPreventsPublication(t *testing.T) {
	for _, activeReader := range []bool{false, true} {
		for _, changeAt := range []int{2, 1} {
			w, h := sheetSourcesFixture(t, activeReader)
			h.changeAt = changeAt
			sh, err := w.PlanSheet(t.Context(), "chart-user", nil)
			if !errors.Is(err, ErrConflict) {
				t.Fatalf("active=%t changeAt=%d: want conflict, got %v", activeReader, changeAt, err)
			}
			if h.sourceReads != 2 || h.metadataReads != 2 {
				t.Fatalf("guard did not enclose both metadata reads: %v", h.calls)
			}
			if sh.Proposal != "" || len(w.proposals.rows) != 0 || h.proposalPublishedEarly {
				t.Fatal("conflicting request published a proposal")
			}
		}
	}
}

func TestPlanSheetMetadataAndSourceErrorsPreventPublication(t *testing.T) {
	for _, tc := range []struct{ sourceAt, metadataAt int }{{1, 0}, {2, 0}, {0, 1}, {0, 2}} {
		w, h := sheetSourcesFixture(t, true)
		h.sourceErrorAt, h.metadataErrorAt = tc.sourceAt, tc.metadataAt
		_, err := w.PlanSheet(t.Context(), "chart-user", nil)
		if err == nil || len(w.proposals.rows) != 0 || h.proposalPublishedEarly {
			t.Fatalf("read error did not stop publication: %+v err=%v", tc, err)
		}
	}
}

func TestPlanSheetPolicyPermissionAndNoGrowthPreserved(t *testing.T) {
	w, _ := chartWorker(t)
	f := w.Budgets.(*shadowFakes)
	fixed, capMinor := int64(1_000_000), int64(30_000_000)
	f.budget.Policies = []BudgetPolicy{{Version: 1, EffectiveFrom: "2026-01-01", MonthlyMinor: 20_000_000, CarryRule: "carry_cash", ReleasedPaymentRule: "roll_all", Growth: &BudgetPolicyGrowth{EveryMonths: 12, StartsOn: "2026-01-01", FixedMinor: &fixed, MaximumMinor: &capMinor}}}
	before := f.budget.Monthly
	sh, err := w.PlanSheet(t.Context(), "chart-user", nil)
	if err != nil {
		t.Fatal(err)
	}
	// Permission is 200,000 + 10,000 AMD, not legacy 250,000 or funding 300,000.
	if sh.Summary.BudgetMinor != 21_000_000 || sh.NoGrowth == nil || sh.NoGrowth.BudgetMinor != 21_000_000 || f.budget.Monthly != before {
		t.Fatal("permission, fallback summary or stored budget changed")
	}
	// The already-computed pair must still be independently validated.
	delta := int64(-20_500_000)
	f.budget.Policies[0].Adjustments = []BudgetPolicyAdjustment{{Month: "2026-01", DeltaMinor: &delta}}
	if _, err := w.PlanSheet(t.Context(), "chart-user", nil); err == nil {
		t.Fatal("invalid no-growth timeline accepted")
	}
	// A legacy zero override remains zero permission, regardless of funding.
	f.budget.Policies = nil
	f.budget.Overrides = map[string]int64{plan.MonthKey(date.MustNew(2026, 1, 15)): 0}
	// January has no due payment; the February funding still supports the schedule.
	sh, err = w.PlanSheet(t.Context(), "chart-user", nil)
	if err != nil || sh.Summary.BudgetMinor != 0 {
		t.Fatalf("legacy zero override changed: %v", err)
	}
}
