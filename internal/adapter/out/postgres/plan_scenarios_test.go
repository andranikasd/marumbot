package postgres_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

func TestScenarioActivationAtomicOwnershipAndIdempotency(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	owner := newUser(t, s)
	other := newUser(t, s)
	if _, err := s.CreateLoan(ctx, draft(owner, t)); err != nil {
		t.Fatal(err)
	}
	cfg := app.BudgetConfiguration{UserID: owner, Currency: "AMD", MonthlyMinor: 600_000_00, PayDay: 5, OpeningAsOf: mustDate(t, "2026-08-01"), Funding: &app.BudgetFunding{MonthlyMinor: 600_000_00, CashThrough: "2026-08-01"}}
	if err := s.SetBudgetConfiguration(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	w := app.Worker{Users: s, Loans: s, Budgets: s, Plans: s, History: s, Clock: historyClock{}, DefaultCurrency: money.MustLookup("AMD"), Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	before, err := s.Budget(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	sheet, err := w.PlanSheet(ctx, owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	monthly := int64(700_000_00)
	c := app.ScenarioCommand{Proposal: sheet.Proposal, Changes: app.ScenarioChanges{MonthlyMinor: &monthly}}
	preview, err := w.PreviewScenario(ctx, owner, c)
	if err != nil {
		t.Fatal(err)
	}
	c.ResultHash = preview.ResultHash
	saved, err := w.SaveScenario(ctx, owner, c)
	if err != nil {
		t.Fatal(err)
	}
	same, err := w.SaveScenario(ctx, owner, c)
	if err != nil || same.ID != saved.ID {
		t.Fatal("save retry duplicated scenario", err)
	}
	after, err := s.Budget(ctx, owner)
	if err != nil || after.Version != before.Version || after.Monthly != before.Monthly {
		t.Fatal("preview/save changed active budget", err)
	}
	history, rev, err := s.PlanHistory(ctx, owner)
	if err != nil || len(history) != 0 || rev != 0 {
		t.Fatal("save activated a plan", err)
	}
	if _, err = w.Scenario(ctx, other, saved.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("ownership read", err)
	}
	command := app.ScenarioActivationCommand{ID: saved.ID, Key: uuid.NewString(), ExpectedRevision: 0}
	if _, err = w.ActivateScenario(ctx, other, command); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("ownership activation", err)
	}
	// Force failure after the budget write: the plan revision check must roll back both.
	wrong := command
	wrong.ExpectedRevision = 9
	if _, err = w.ActivateScenario(ctx, owner, wrong); !errors.Is(err, app.ErrConflict) {
		t.Fatal("stale revision", err)
	}
	after, err = s.Budget(ctx, owner)
	if err != nil || after.Version != before.Version || after.Monthly != before.Monthly {
		t.Fatal("budget survived failed activation", err)
	}
	// Concurrent identical requests must share one immutable receipt.
	var wg sync.WaitGroup
	results := make([]app.PlanActivation, 2)
	errs := make([]error, 2)
	for i := range results {
		wg.Add(1)
		go func(i int) { defer wg.Done(); results[i], errs[i] = w.ActivateScenario(ctx, owner, command) }(i)
	}
	wg.Wait()
	if errs[0] != nil || errs[1] != nil || results[0] != results[1] {
		t.Fatalf("concurrent retry: %v %v %+v", errs[0], errs[1], results)
	}
	after, err = s.Budget(ctx, owner)
	if err != nil || after.Version != before.Version+1 {
		t.Fatal("budget not applied exactly once", err)
	}
	permission, err := after.PermissionOn(mustDate(t, "2026-08-01"))
	if err != nil || permission.Minor() != monthly || after.Funding.MonthlyMinor != cfg.Funding.MonthlyMinor || after.Opening.Minor() != 0 {
		t.Fatal("activation must change effective permission without inventing cash", err)
	}
	history, rev, err = s.PlanHistory(ctx, owner)
	if err != nil || len(history) != 1 || rev != 1 {
		t.Fatal("activation not atomic", err)
	}
	sources, err := s.PlanSources(ctx, owner)
	if err != nil || history[0].Manifest.Sources != sources || history[0].Manifest.BudgetVersion != after.Version {
		t.Fatal("activation retained pre-write sources", err)
	}
	replay, err := w.HistoricalPlan(ctx, owner, results[0].ID)
	if err != nil || !reflect.DeepEqual(replay.Summary, preview.Sheet.Summary) {
		t.Fatal("activated calculation differs", err)
	}
	changed := command
	changed.ExpectedRevision = 1
	if _, err = w.ActivateScenario(ctx, owner, changed); !errors.Is(err, app.ErrConflict) {
		t.Fatal("changed retry accepted", err)
	}
	policy, err := after.PolicyForMonthlyChange(mustDate(t, "2026-08-01"), 800_000_00)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AppendBudgetPolicy(ctx, owner, "AMD", after.Version, policy); err != nil {
		t.Fatal(err)
	}
	retried, err := w.ActivateScenario(ctx, owner, command)
	if err != nil || retried != results[0] {
		t.Fatal("lost response retry after source change", err)
	}
	changed.Key = uuid.NewString()
	if _, err = w.ActivateScenario(ctx, owner, changed); !errors.Is(err, app.ErrConflict) {
		t.Fatal("stale scenario activated", err)
	}
	original, err := w.Scenario(ctx, owner, saved.ID)
	if err != nil || !original.Outdated || !reflect.DeepEqual(original.Sheet.Summary, preview.Sheet.Summary) {
		t.Fatal("saved original changed", err)
	}
	// Original proposal still clones its own historical budget after current edits.
	historical, err := w.PreviewScenario(ctx, owner, c)
	if err != nil || !historical.Outdated || !reflect.DeepEqual(historical.Sheet.Summary, preview.Sheet.Summary) {
		t.Fatal("historical budget lost", err)
	}
}

func TestScenarioFutureFundingActivation(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	owner := newUser(t, s)
	if _, err := s.CreateLoan(ctx, draft(owner, t)); err != nil {
		t.Fatal(err)
	}
	cfg := app.BudgetConfiguration{UserID: owner, Currency: "AMD", MonthlyMinor: 60000000, PayDay: 5, OpeningMinor: 60000000, OpeningAsOf: mustDate(t, "2026-08-01"), Funding: &app.BudgetFunding{MonthlyMinor: 60000000, CashThrough: "2026-08-01"}}
	if err := s.SetBudgetConfiguration(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	w := app.Worker{Users: s, Loans: s, Budgets: s, Plans: s, History: s, Clock: historyClock{}, DefaultCurrency: money.MustLookup("AMD"), Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	sheet, err := w.PlanSheet(ctx, owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	monthly := int64(70000000)
	reserve := int64(100000)
	payday := 4
	c := app.ScenarioCommand{Proposal: sheet.Proposal, Changes: app.ScenarioChanges{MonthlyMinor: &monthly, EffectiveFrom: "2026-09-01", PayDay: &payday, ReserveMinor: &reserve, OneTimeCash: &app.BudgetCashEvent{On: "2026-09-01", Minor: 1000000, Expected: true}}}
	preview, err := w.PreviewScenario(ctx, owner, c)
	if err != nil {
		t.Fatal(err)
	}
	c.ResultHash = preview.ResultHash
	saved, err := w.SaveScenario(ctx, owner, c)
	if err != nil {
		t.Fatal(err)
	}
	command := app.ScenarioActivationCommand{ID: saved.ID, Key: uuid.NewString()}
	if _, err = w.ActivateScenario(ctx, owner, command); err == nil {
		t.Fatal("expected cash activated")
	}
	b, err := s.Budget(ctx, owner)
	if err != nil || b.Version != 1 {
		t.Fatal("refusal changed budget", err)
	}
	c.Changes.OneTimeCash.Expected = false
	c.ResultHash = ""
	preview, err = w.PreviewScenario(ctx, owner, c)
	if err != nil {
		t.Fatal(err)
	}
	c.ResultHash = preview.ResultHash
	saved, err = w.SaveScenario(ctx, owner, c)
	if err != nil {
		t.Fatal(err)
	}
	command.ID = saved.ID
	receipt, err := w.ActivateScenario(ctx, owner, command)
	if err != nil {
		t.Fatal(err)
	}
	b, err = s.Budget(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if b.Version != 2 || b.PayDay != 4 || b.Reserve.Minor() != reserve || len(b.Policies) != 1 || b.Policies[0].EffectiveFrom != "2026-09-01" || len(b.Funding.Events) != 1 || b.Funding.Events[0].Expected {
		t.Fatalf("declarations not activated: %+v", b)
	}
	if b.Funding.MonthlyMinor != cfg.Funding.MonthlyMinor || b.Opening.Minor() != cfg.OpeningMinor || b.Funding.CashThrough != cfg.Funding.CashThrough {
		t.Fatal("scenario changed unrelated funding facts")
	}
	replay, err := w.HistoricalPlan(ctx, owner, receipt.ID)
	if err != nil || !reflect.DeepEqual(replay.Summary, preview.Sheet.Summary) {
		t.Fatal("funded activation differs from preview", err)
	}
	current, err := w.PlanSheet(ctx, owner, nil)
	if err != nil || !reflect.DeepEqual(current.Summary, preview.Sheet.Summary) {
		t.Fatal("active budget source shape disagrees with preview", err)
	}
}

func TestScenarioExistingPolicyAppendsOneVersion(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	owner := newUser(t, s)
	if _, err := s.CreateLoan(ctx, draft(owner, t)); err != nil {
		t.Fatal(err)
	}
	cfg := app.BudgetConfiguration{UserID: owner, Currency: "AMD", MonthlyMinor: 60000000, PayDay: 5, OpeningMinor: 60000000, OpeningAsOf: mustDate(t, "2026-08-01"), Funding: &app.BudgetFunding{MonthlyMinor: 60000000, CashThrough: "2026-08-01"}}
	if err := s.SetBudgetConfiguration(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	replace := int64(90000000)
	currentPolicy := app.BudgetPolicy{EffectiveFrom: "2026-08-01", MonthlyMinor: 60000000, CarryRule: "carry_cash", ReleasedPaymentRule: "roll_all", Adjustments: []app.BudgetPolicyAdjustment{{Month: "2026-08", ReplacementMinor: &replace}}}
	version, err := s.AppendBudgetPolicy(ctx, owner, "AMD", 1, currentPolicy)
	if err != nil {
		t.Fatal(err)
	}
	futurePolicy := app.BudgetPolicy{EffectiveFrom: "2026-10-01", MonthlyMinor: 80000000, CarryRule: "carry_cash", ReleasedPaymentRule: "roll_all"}
	version, err = s.AppendBudgetPolicy(ctx, owner, "AMD", version, futurePolicy)
	if err != nil {
		t.Fatal(err)
	}
	before, err := s.Budget(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	w := app.Worker{Users: s, Loans: s, Budgets: s, Plans: s, History: s, Clock: historyClock{}, DefaultCurrency: money.MustLookup("AMD"), Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	sheet, err := w.PlanSheet(ctx, owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	minor := int64(70000000)
	payday := 4
	c := app.ScenarioCommand{Proposal: sheet.Proposal, Changes: app.ScenarioChanges{MonthlyMinor: &minor, PayDay: &payday}}
	preview, err := w.PreviewScenario(ctx, owner, c)
	if err != nil {
		t.Fatal(err)
	}
	c.ResultHash = preview.ResultHash
	if preview.Sheet.Summary.BudgetMinor != minor {
		t.Fatal("existing adjustment masked requested permission")
	}
	saved, err := w.SaveScenario(ctx, owner, c)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := w.ActivateScenario(ctx, owner, app.ScenarioActivationCommand{ID: saved.ID, Key: uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	after, err := s.Budget(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if after.Version != version+1 || len(after.Policies) != len(before.Policies)+1 || !reflect.DeepEqual(after.Policies[:len(before.Policies)], before.Policies) {
		t.Fatal("policy history replaced or version advanced twice")
	}
	if after.Monthly != before.Monthly || after.Opening != before.Opening || !reflect.DeepEqual(after.Funding, before.Funding) || after.PayDay != 4 {
		t.Fatal("unrelated source facts changed")
	}
	active, err := w.PlanSheet(ctx, owner, nil)
	if err != nil || !reflect.DeepEqual(active.Summary, preview.Sheet.Summary) {
		t.Fatal("active permissions differ from scenario", err)
	}
	replay, err := w.HistoricalPlan(ctx, owner, receipt.ID)
	if err != nil || !reflect.DeepEqual(replay.Summary, preview.Sheet.Summary) {
		t.Fatal("historical policy replay differs", err)
	}
	future, err := after.PermissionOn(mustDate(t, "2026-10-01"))
	if err != nil || future.Minor() != 80000000 {
		t.Fatal("future declaration lost")
	}
}
