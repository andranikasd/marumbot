package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/andranikasd/marumbot/pkg/core/plan"
)

func TestDatedTimelineReconcilesToApprovedResult(t *testing.T) {
	in := cacheInput(t)
	goal := plan.Goal{Kind: plan.LeastInterest}
	report, err := plan.Search(in, goal)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := manifestFor(in, goal, report, 1)
	if err != nil {
		t.Fatal(err)
	}
	w := Worker{Clock: &fixedClock{at: in.ValuationDate.AtLocal(12, 0, time.UTC)}, History: &comparisonHistoryFake{sources: manifest.Sources}}
	key, err := w.proposals.put("owner", manifest)
	if err != nil {
		t.Fatal(err)
	}
	timeline, err := w.PaymentTimeline(context.Background(), "owner", key, "")
	if err != nil {
		t.Fatal(err)
	}
	var paid, fees int64
	for i, a := range timeline.Payments {
		paid += a.AmountMinor
		fees += a.FeeMinor
		if i > 0 && a.On < timeline.Payments[i-1].On {
			t.Fatal("payment dates moved backwards")
		}
	}
	if paid != report.Best.TotalPaid.Minor() || fees != report.Best.TotalFees.Minor() {
		t.Fatal("dated actions do not reconcile")
	}
	if len(timeline.Payments) <= len(report.Best.Actions) {
		t.Fatal("only first cycle exported")
	}
	if _, err = w.PaymentTimeline(context.Background(), "stranger", key, ""); err == nil {
		t.Fatal("another user read the proposal")
	}
}

func TestTimelineProposalFreshnessAndHistoricalExport(t *testing.T) {
	w, h, key, m := comparisonWorker(t)
	if _, err := w.PaymentTimeline(t.Context(), "owner", key, ""); err != nil {
		t.Fatal(err)
	}
	h.sources = "changed"
	if _, err := w.PaymentTimeline(t.Context(), "owner", key, ""); !errors.Is(err, ErrConflict) {
		t.Fatal("stale source exported", err)
	}
	h.sources = m.Sources
	w.Clock.(*fixedClock).at = w.Clock.Now().Add(24 * time.Hour)
	if _, err := w.PaymentTimeline(t.Context(), "owner", key, ""); !errors.Is(err, ErrConflict) {
		t.Fatal("previous-day proposal exported", err)
	}
	historical := &replayHistoryFake{manifest: m}
	historical.sources = "changed"
	w.History = historical
	if out, err := w.PaymentTimeline(t.Context(), "owner", "", "historical"); err != nil || len(out.Payments) == 0 {
		t.Fatal("historical export lost", err)
	}
	if _, err := w.PaymentTimeline(t.Context(), "stranger", "", "historical"); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign history exported", err)
	}
	w.History = nil
	if _, err := w.PaymentTimeline(t.Context(), "owner", key, ""); !errors.Is(err, ErrConflict) {
		t.Fatal("proposal exported without source verification", err)
	}
}

func TestPlanOutputIdentityPreservesRawManifest(t *testing.T) {
	w, _, key, m := comparisonWorker(t)
	report, err := plan.Search(m.Input, m.Goal)
	if err != nil {
		t.Fatal(err)
	}
	sheet, err := sheetFromReport(m.Input, m.Goal, report, m.Input.Loans[0].Balance, m.Input.Cash.Monthly, nil)
	if err != nil {
		t.Fatal(err)
	}
	if sheet.InputHash == m.InputHash {
		t.Fatal("fixture must advance a balance during normalization")
	}
	timeline, err := w.PaymentTimeline(t.Context(), "owner", key, "")
	if err != nil {
		t.Fatal(err)
	}
	inverse, err := w.BudgetByDate(t.Context(), "owner", key, "2027-01-01")
	if err != nil {
		t.Fatal(err)
	}
	if timeline.InputHash != sheet.InputHash || inverse.InputHash != sheet.InputHash {
		t.Fatal("output hashes must match the normalized sheet identity")
	}
	stored, ok := w.proposals.get("owner", key)
	if !ok || stored.InputHash != m.InputHash {
		t.Fatal("raw manifest hash changed")
	}
	if _, err = ReplayManifest(stored); err != nil {
		t.Fatal("raw integrity replay failed", err)
	}
	h := &replayHistoryFake{manifest: stored}
	w.History = h
	historical, err := w.PaymentTimeline(t.Context(), "owner", "", "historical")
	if err != nil || historical.InputHash != sheet.InputHash {
		t.Fatal("historical output identity mismatch", err)
	}
}

type changingTimelineSources struct {
	comparisonHistoryFake
	reads int
}

func (h *changingTimelineSources) PlanSources(context.Context, string) (string, error) {
	h.reads++
	if h.reads > 1 {
		return "changed during export", nil
	}
	return h.sources, nil
}

func TestTimelineRejectsSourcesChangedDuringCalculation(t *testing.T) {
	w, _, key, m := comparisonWorker(t)
	h := &changingTimelineSources{}
	h.sources = m.Sources
	w.History = h
	if _, err := w.PaymentTimeline(t.Context(), "owner", key, ""); !errors.Is(err, ErrConflict) {
		t.Fatal("export returned after source change", err)
	}
	if h.reads != 2 {
		t.Fatal("source check must bracket calculation")
	}
}
