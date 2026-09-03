package app

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/andranikasd/marumbot/pkg/core/date"
)

func TestApprovedReminderSurvivesValuationMidnight(t *testing.T) {
	w, f, h, _ := optionalWorker(t)
	original, err := w.PaymentTimeline(t.Context(), "owner", "", "approved")
	if err != nil {
		t.Fatal(err)
	}
	w.Clock.(*fixedClock).at = w.Clock.Now().Add(24 * time.Hour)
	rows, _, err := w.PlanHistory(t.Context(), "owner")
	if err != nil || len(rows) != 1 || rows[0].Outdated {
		t.Fatalf("unchanged approval expired: %+v %v", rows, err)
	}
	if err := w.scheduleOptionalReminders(t.Context(), "owner"); err != nil {
		t.Fatal(err)
	}
	if len(f.active) != 1 || f.active[0] != "approved" {
		t.Fatal("midnight canceled approved reminders")
	}
	for index, action := range original.Payments {
		if action.Kind != "extra" || action.On <= h.manifest.Input.ValuationDate.String() {
			continue
		}
		on, err := date.Parse(action.On)
		if err != nil {
			t.Fatal(err)
		}
		w.Clock.(*fixedClock).at = on.AtLocal(12, 0, time.UTC)
		f.scheduled = nil
		if err := w.scheduleOptionalReminders(t.Context(), "owner"); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, scheduled := range f.scheduled {
			if scheduled.ActionIndex == index && scheduled.DueDate == action.On {
				found = true
			}
		}
		if !found {
			t.Fatal("approved future action not scheduled")
		}
		f.optional[0].ActionIndex = index
		f.optional[0].DueDate = action.On
		f.optional[0].LoanID = action.LoanID
		got, valid, err := w.optionalReminderAction(t.Context(), f.optional[0])
		if err != nil || !valid || !reflect.DeepEqual(got, action) {
			t.Fatalf("future action changed or suppressed: %+v %v", got, err)
		}
		n, err := w.SendDueReminders(t.Context(), 50)
		if err != nil || n != 1 || len(f.messages) != 1 || !strings.Contains(f.messages[0], "Extra payment (optional)") {
			t.Fatalf("future delivery: %d %v", n, err)
		}
		replay, err := w.PaymentTimeline(t.Context(), "owner", "", "approved")
		if err != nil || !reflect.DeepEqual(original, replay) {
			t.Fatal("clock advance replaced approved timeline")
		}
		historical, err := w.HistoricalPlan(t.Context(), "owner", "approved")
		if err != nil || historical.Outdated {
			t.Fatalf("historical approval expired: %v", err)
		}
		h.active = false
		rows, _, err = w.PlanHistory(t.Context(), "owner")
		if err != nil || !rows[0].Outdated {
			t.Fatal("superseded approval still current")
		}
		return
	}
	t.Fatal("fixture needs a future optional action")
}

func TestExpiredProposalStillCannotActivate(t *testing.T) {
	w, h, proposal, _ := comparisonWorker(t)
	w.Clock.(*fixedClock).at = w.Clock.Now().Add(24 * time.Hour)
	_, err := w.ActivateProposal(t.Context(), "owner", PlanActivationCommand{Proposal: proposal, Key: "expired-proposal-key", ExpectedRevision: 7})
	if !errors.Is(err, ErrConflict) || h.activated.Engine != "" {
		t.Fatal("yesterday's proposal activated", err)
	}
}

func TestNextDayPreviewPreservesApprovedBaseline(t *testing.T) {
	w, _ := chartWorker(t)
	h := &comparisonHistoryFake{sources: "unchanged-sources"}
	w.History = h
	original, err := w.PlanSheet(t.Context(), "chart-user", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.ActivateProposal(t.Context(), "chart-user", PlanActivationCommand{Proposal: original.Proposal, Key: "approved-baseline-key", ExpectedRevision: original.ActiveRevision})
	if err != nil {
		t.Fatal(err)
	}
	baseline := h.activated
	w.Clock = &fixedClock{at: w.Clock.Now().Add(24 * time.Hour)}
	preview, err := w.PlanSheet(t.Context(), "chart-user", nil)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Approved || !preview.AnyPlan || preview.Proposal == original.Proposal {
		t.Fatal("new valuation incorrectly presented as approved")
	}
	if !reflect.DeepEqual(h.activated, baseline) || h.begun != 1 {
		t.Fatal("preview replaced approved baseline")
	}
	rows, _, err := w.PlanHistory(t.Context(), "chart-user")
	if err != nil || len(rows) != 1 || rows[0].Outdated {
		t.Fatal("preview invalidated original approval", err)
	}
}
