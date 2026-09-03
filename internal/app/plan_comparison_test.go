package app

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

type comparisonHistoryFake struct {
	sources   string
	begun     int
	activated PlanManifest
	command   PlanActivationCommand
}

func (h *comparisonHistoryFake) PlanSources(context.Context, string) (string, error) {
	return h.sources, nil
}

func (h *comparisonHistoryFake) PlanHistory(context.Context, string) ([]PlanVersion, int64, error) {
	if h.activated.Engine != "" {
		return []PlanVersion{{ID: "approved", Currency: h.activated.Input.Cash.Monthly.Currency().Code, Manifest: h.activated, Active: true}}, 8, nil
	}
	return nil, 7, nil
}

func (h *comparisonHistoryFake) PlanVersion(context.Context, string, string) (PlanVersion, error) {
	return PlanVersion{}, ErrNotFound
}

func (h *comparisonHistoryFake) BeginPlanActivation(context.Context) (PlanActivationTransaction, error) {
	h.begun++
	return h, nil
}

func (h *comparisonHistoryFake) LockPlanSources(context.Context, string) (string, error) {
	return h.sources, nil
}

func (h *comparisonHistoryFake) Receipt(context.Context, string, string) (PlanActivation, string, error) {
	return PlanActivation{}, "", ErrNotFound
}

func (h *comparisonHistoryFake) Activate(_ context.Context, _ string, c PlanActivationCommand, m PlanManifest) (PlanActivation, error) {
	h.activated = m
	h.command = c
	return PlanActivation{ID: "approved", Revision: 8}, nil
}
func (h *comparisonHistoryFake) Commit(context.Context) error   { return nil }
func (h *comparisonHistoryFake) Rollback(context.Context) error { return nil }

func comparisonWorker(t *testing.T) (*Worker, *comparisonHistoryFake, string, PlanManifest) {
	t.Helper()
	in := cacheInput(t)
	// One instalment is assumed during normalization; candidate hashes must
	// retain it even though each named simulation uses the normalized input.
	in.ValuationDate = date.MustNew(2026, 2, 16)
	report, err := plan.Search(in, plan.Goal{Kind: plan.LeastInterest})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := manifestFor(in, plan.Goal{Kind: plan.LeastInterest}, report, 11)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Sources = "original-sources"
	history := &comparisonHistoryFake{sources: manifest.Sources}
	worker := &Worker{Clock: &fixedClock{at: time.Date(2026, 2, 16, 12, 0, 0, 0, time.UTC)}, History: history}
	proposal, err := worker.proposals.put("owner", manifest)
	if err != nil {
		t.Fatal(err)
	}
	return worker, history, proposal, manifest
}

func TestPlanComparisonsPreserveManifestAndActivateSelectedResult(t *testing.T) {
	w, h, proposal, original := comparisonWorker(t)
	sheet, err := w.PlanComparisons(t.Context(), "owner", proposal)
	if err != nil {
		t.Fatal(err)
	}
	if h.begun != 0 || sheet.ActiveRevision != 7 || sheet.Proposal != proposal || sheet.InputHash != original.InputHash {
		t.Fatal("preview changed active state or source identity")
	}
	var selected string
	for _, row := range sheet.Rows {
		if row.Refusal != "" {
			if row.Proposal != "" || row.Summary != nil {
				t.Fatal("refused method has zero-valued selectable result")
			}
			continue
		}
		candidate, ok := w.proposals.get("owner", row.Proposal)
		if !ok {
			t.Fatal("candidate proposal missing")
		}
		if !reflect.DeepEqual(candidate.Input, original.Input) || candidate.Sources != original.Sources || candidate.BudgetVersion != 11 || candidate.InputHash != original.InputHash {
			t.Fatal("candidate reconstructed original sources")
		}
		replay, err := ReplayManifest(candidate)
		if err != nil {
			t.Fatalf("%s replay: %v", row.Strategy, err)
		}
		if replay.Assumed["a"] != 1 {
			t.Fatal("normalization assumption missing from approved result")
		}
		if replay.Cost().Minor() != row.Summary.CostMinor || replay.Months != row.Summary.Months {
			t.Fatal("row doesn't describe exact replay")
		}
		if row.Strategy == "highest_rate" {
			selected = row.Proposal
		}
	}
	if selected == "" || selected == proposal {
		t.Fatal("named method did not issue distinct policy proposal")
	}
	_, err = w.ActivateProposal(t.Context(), "owner", PlanActivationCommand{Proposal: selected, ExpectedRevision: sheet.ActiveRevision, Key: "comparison-activation-key"})
	if err != nil {
		t.Fatal(err)
	}
	if h.begun != 1 || h.activated.Policy.Name != "highest_rate" || h.command.Proposal != selected || h.command.ExpectedRevision != 7 {
		t.Fatal("activation replaced selected policy with optimized winner")
	}
}

func TestPlanComparisonsRejectOtherOwnerAndChangedSources(t *testing.T) {
	w, h, proposal, _ := comparisonWorker(t)
	if _, err := w.PlanComparisons(t.Context(), "other", proposal); !errors.Is(err, ErrConflict) {
		t.Fatal("cross-owner proposal accepted", err)
	}
	h.sources = "changed"
	if _, err := w.PlanComparisons(t.Context(), "owner", proposal); !errors.Is(err, ErrConflict) {
		t.Fatal("stale sources accepted", err)
	}
	if h.begun != 0 {
		t.Fatal("preview opened activation transaction")
	}
}

func TestPlanComparisonsSelectedBaselineSurvivesDefaultReopen(t *testing.T) {
	w, _ := chartWorker(t)
	history := &comparisonHistoryFake{sources: "unchanged-sources"}
	w.History = history
	original, err := w.PlanSheet(t.Context(), "chart-user", nil)
	if err != nil {
		t.Fatal(err)
	}
	methods, err := w.PlanComparisons(t.Context(), "chart-user", original.Proposal)
	if err != nil {
		t.Fatal(err)
	}
	var chosen PlanComparisonRow
	for _, row := range methods.Rows {
		if row.Strategy == "highest_required" {
			chosen = row
			break
		}
	}
	if chosen.Proposal == "" || chosen.Proposal == original.Proposal || chosen.Summary == nil {
		t.Fatal("named baseline unavailable")
	}
	_, err = w.ActivateProposal(t.Context(), "chart-user", PlanActivationCommand{Proposal: chosen.Proposal, ExpectedRevision: methods.ActiveRevision, Key: "reopen-selected-baseline"})
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := w.PlanSheet(t.Context(), "chart-user", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reopened.Approved || reopened.Proposal != chosen.Proposal {
		t.Fatal("reopening replaced the active selected proposal")
	}
	manifest, ok := w.proposals.get("chart-user", reopened.Proposal)
	if !ok || manifest.Policy.Name != "highest_required" {
		t.Fatal("named policy lost on reopen")
	}
	if reopened.Summary.PayoffDate != chosen.Summary.PayoffDate || reopened.Summary.InterestMinor != chosen.Summary.InterestMinor || reopened.Summary.FeesMinor != chosen.Summary.FeesMinor {
		t.Fatal("reopened figures differ from selected comparison")
	}
	if reopened.Certificate.Strength != plan.NamedStrategiesOnly {
		t.Fatal("named baseline inherited optimized certificate")
	}
}
