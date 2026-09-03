package app

import (
	"errors"
	"reflect"
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/plan"
)

func TestPlanStressPreservesOriginalProposal(t *testing.T) {
	w, h, proposal, original := comparisonWorker(t)
	got, err := w.PlanStress(t.Context(), "owner", proposal, 500)
	if err != nil {
		t.Fatal(err)
	}
	normalized, assumed, err := plan.Normalize(original.Input)
	if err != nil {
		t.Fatal(err)
	}
	if len(assumed) == 0 {
		t.Fatal("fixture must exercise normalization")
	}
	wantHash := searchFingerprint(normalized, plan.Goal{})
	if wantHash == original.InputHash {
		t.Fatal("fixture must distinguish raw and normalized hashes")
	}
	report, err := plan.Search(original.Input, original.Goal)
	if err != nil {
		t.Fatal(err)
	}
	sheet, err := sheetFromReport(original.Input, original.Goal, report, original.Input.Cash.Monthly, original.Input.Cash.Monthly, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.InputHash != sheet.InputHash {
		t.Fatal("stress and main sheet normalized identities differ")
	}
	after, ok := w.proposals.get("owner", proposal)
	if !ok || !reflect.DeepEqual(after, original) || h.begun != 0 || got.Proposal != proposal || got.InputHash != wantHash || len(got.Cases) != 5 {
		t.Fatal("stress modified original or lost identity")
	}
	if _, err = w.PlanStress(t.Context(), "other", proposal, 0); !errors.Is(err, ErrConflict) {
		t.Fatalf("ownership: %v", err)
	}
	h.sources = "changed"
	if _, err = w.PlanStress(t.Context(), "owner", proposal, 0); !errors.Is(err, ErrConflict) {
		t.Fatalf("staleness: %v", err)
	}
}
