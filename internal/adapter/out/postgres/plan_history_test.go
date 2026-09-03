package postgres_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

type historyClock struct{}

func (historyClock) Now() time.Time { return time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC) }
func TestPlanActivationReplayAndStaleness(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	owner := newUser(t, s)
	if _, err := s.CreateLoan(ctx, draft(owner, t)); err != nil {
		t.Fatal(err)
	}
	if err := s.SetBudgetConfiguration(ctx, app.BudgetConfiguration{UserID: owner, Currency: "AMD", MonthlyMinor: 600_000_00, PayDay: 5, OpeningAsOf: mustDate(t, "2026-08-01"), Funding: &app.BudgetFunding{MonthlyMinor: 600_000_00}}); err != nil {
		t.Fatal(err)
	}
	w := app.Worker{Users: s, Loans: s, Budgets: s, Plans: s, History: s, Clock: historyClock{}, DefaultCurrency: money.MustLookup("AMD"), Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	sh, err := w.PlanSheet(ctx, owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	command := app.PlanActivationCommand{Proposal: sh.Proposal, Key: uuid.NewString(), ExpectedRevision: sh.ActiveRevision}
	r, err := w.ActivateProposal(ctx, owner, command)
	if err != nil {
		t.Fatal(err)
	}
	again, err := w.ActivateProposal(ctx, owner, command)
	if err != nil || again != r {
		t.Fatalf("retry: %+v %v", again, err)
	}
	changed := command
	changed.ExpectedRevision++
	if _, err = w.ActivateProposal(ctx, owner, changed); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("changed retry: %v", err)
	}
	history, revision, err := w.PlanHistory(ctx, owner)
	if err != nil || revision != 1 || len(history) != 1 || !history[0].Active || history[0].Outdated {
		t.Fatalf("history: %+v %d %v", history, revision, err)
	}
	replay, err := w.HistoricalPlan(ctx, owner, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if replay.Summary.InterestMinor != sh.Summary.InterestMinor || replay.Summary.PayoffDate != sh.Summary.PayoffDate {
		t.Fatal("original answer changed")
	}
	if _, err = w.HistoricalPlan(ctx, newUser(t, s), r.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("ownership: %v", err)
	}
	if err = s.SetBudget(ctx, owner, "AMD", 700_000_00, 5); err != nil {
		t.Fatal(err)
	}
	history, _, err = w.PlanHistory(ctx, owner)
	if err != nil || !history[0].Outdated {
		t.Fatal("budget edit did not invalidate approval", err)
	}
	again, err = w.ActivateProposal(ctx, owner, command)
	if err != nil || again != r {
		t.Fatal("lost-response retry failed after edit", err)
	}
	stale := command
	stale.Key = uuid.NewString()
	stale.ExpectedRevision = 1
	if _, err = w.ActivateProposal(ctx, owner, stale); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("stale approval: %v", err)
	}
	replay, err = w.HistoricalPlan(ctx, owner, r.ID)
	if err != nil || replay.Summary.InterestMinor != sh.Summary.InterestMinor {
		t.Fatal("budget edit rewrote history", err)
	}
}
