package postgres_test

import (
	"context"
	"io"
	"log/slog"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/pkg/core/money"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

func TestPlanHistoryPagesAreBoundedStableAndMetadataOnly(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	owner, foreign := newUser(t, s), newUser(t, s)
	if _, err := s.CreateLoan(ctx, draft(owner, t)); err != nil {
		t.Fatal(err)
	}
	if err := s.SetBudgetConfiguration(ctx, app.BudgetConfiguration{UserID: owner, Currency: "AMD", MonthlyMinor: 600_000_00, PayDay: 5, OpeningAsOf: mustDate(t, "2026-08-01"), Funding: &app.BudgetFunding{MonthlyMinor: 600_000_00}}); err != nil {
		t.Fatal(err)
	}
	w := app.Worker{Users: s, Loans: s, Budgets: s, Plans: s, History: s, Clock: historyClock{}, DefaultCurrency: money.MustLookup("AMD"), Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	sheet, err := w.PlanSheet(ctx, owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	activation, err := w.ActivateProposal(ctx, owner, app.PlanActivationCommand{Proposal: sheet.Proposal, Key: uuid.NewString(), ExpectedRevision: 0})
	if err != nil {
		t.Fatal(err)
	}
	original, err := s.PlanVersion(ctx, owner, activation.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Reuse one proven manifest; pagination must not require dozens of searches.
	for revision := int64(1); revision < 57; revision++ {
		tx, err := s.BeginPlanActivation(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = tx.LockPlanSources(ctx, owner); err == nil {
			_, err = tx.Activate(ctx, owner, app.PlanActivationCommand{Proposal: sheet.Proposal, Key: uuid.NewString(), ExpectedRevision: revision}, original.Manifest)
		}
		if err != nil {
			_ = tx.Rollback(ctx)
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	first, revision, err := s.PlanHistoryPage(ctx, owner, "")
	if err != nil || len(first) != 51 || revision != 57 {
		t.Fatalf("first page length=%d revision=%d: %v", len(first), revision, err)
	}
	again, _, err := s.PlanHistoryPage(ctx, owner, "")
	if err != nil || !reflect.DeepEqual(first, again) {
		t.Fatal("unchanged history page is unstable")
	}
	second, _, err := s.PlanHistoryPage(ctx, owner, first[len(first)-1].ID)
	if err != nil || len(second) != 6 {
		t.Fatalf("second page length=%d: %v", len(second), err)
	}
	seen := make(map[string]bool)
	for _, page := range [][]app.PlanVersion{first, second} {
		for _, version := range page {
			if seen[version.ID] {
				t.Fatal("duplicate history entry across pages")
			}
			seen[version.ID] = true
			if !reflect.DeepEqual(version.Manifest.Input, plan.Input{}) {
				t.Fatal("metadata page retained full manifest input")
			}
		}
	}
	full, err := s.PlanVersion(ctx, owner, original.ID)
	if err != nil || !reflect.DeepEqual(full.Manifest, original.Manifest) || reflect.DeepEqual(full.Manifest.Input, plan.Input{}) {
		t.Fatal("full historical manifest was altered or omitted")
	}
	foreignPage, _, err := s.PlanHistoryPage(ctx, foreign, original.ID)
	if err != nil || len(foreignPage) != 0 {
		t.Fatal("foreign cursor crossed account boundary")
	}
	last, _, err := s.PlanHistoryPage(ctx, owner, second[len(second)-1].ID)
	if err != nil || len(last) != 0 {
		t.Fatal("history continued beyond final page")
	}
}
