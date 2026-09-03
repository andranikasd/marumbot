package postgres_test

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

type actualsProofClock struct{ on date.Date }

func (c actualsProofClock) Now() time.Time { return c.on.AtLocal(12, 0, time.UTC) }

func TestPlanActualsStoreScopeCorrectionAndActivationBoundary(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	owner := newUser(t, s)
	loan, err := s.CreateLoan(ctx, draft(owner, t))
	if err != nil {
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
	activated, err := w.ActivateProposal(ctx, owner, app.PlanActivationCommand{Proposal: sheet.Proposal, Key: uuid.NewString(), ExpectedRevision: sheet.ActiveRevision})
	if err != nil {
		t.Fatal(err)
	}
	baselines, err := s.ActiveActualBaselines(ctx, owner)
	if err != nil || len(baselines) != 1 || baselines[0].PlanID != activated.ID {
		t.Fatalf("baseline: %+v %v", baselines, err)
	}
	baseline := baselines[0]
	_, zone, err := s.Locale(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		t.Fatal(err)
	}
	activationDay := date.From(baseline.ActivatedAt, location)
	postedDay := date.AddDays(activationDay, 1)
	today := date.AddDays(postedDay, 1)
	month := postedDay.String()[:7]
	w.Clock = actualsProofClock{today}
	service := app.PaymentService{Store: s, Clock: w.Clock, Users: s}
	principal, interest, fees := int64(5603410), int64(6904530), int64(0)
	command := app.PaymentCommand{LoanID: loan, Key: uuid.NewString(), AmountMinor: 12507940, TransactionDate: postedDay.String(), ValueDate: postedDay.String(), Allocation: &app.PaymentAllocation{PrincipalMinor: &principal, InterestMinor: &interest, FeesMinor: &fees}}
	first, err := service.Record(ctx, owner, command)
	if err != nil {
		t.Fatal(err)
	}
	unknown := app.PaymentCommand{LoanID: loan, Key: uuid.NewString(), ExpectedVersion: 1, AmountMinor: 100, TransactionDate: postedDay.String(), ValueDate: postedDay.String()}
	if _, err := service.Record(ctx, owner, unknown); err != nil {
		t.Fatal(err)
	}
	earlier := unknown
	earlier.Key = uuid.NewString()
	earlier.ExpectedVersion = 2
	earlier.AmountMinor = 50
	earlier.TransactionDate = activationDay.String()
	if _, err := service.Record(ctx, owner, earlier); err != nil {
		t.Fatal(err)
	}
	pending := unknown
	pending.Key = uuid.NewString()
	pending.ExpectedVersion = 3
	pending.AmountMinor = 200
	pending.ValueDate = ""
	if _, err := service.Record(ctx, owner, pending); err != nil {
		t.Fatal(err)
	}
	facts, err := s.PlanActualFacts(ctx, owner, baseline, month)
	if err != nil || len(facts) != 4 {
		t.Fatalf("facts: %+v %v", facts, err)
	}
	progress, err := w.ActivePlanActuals(ctx, owner, month)
	if err != nil {
		t.Fatal(err)
	}
	if len(progress) != 1 || progress[0].ExcludedBeforeActivationCount != 1 || progress[0].PendingCount != 1 {
		t.Fatalf("coverage: %+v", progress)
	}
	if len(progress[0].Rows) != 1 {
		t.Fatal(progress)
	}
	row := progress[0].Rows[0]
	if row.PostedMinor == nil || *row.PostedMinor != "12508040" || row.KnownInterestMinor == nil || *row.KnownInterestMinor != "6904530" || row.MissingAllocationCount != 1 || row.FeeDeltaMinor != nil {
		t.Fatalf("actuals: %+v", row)
	}
	correction := command
	correction.Key = uuid.NewString()
	correction.ExpectedVersion = 4
	correction.Replaces = first.ID
	correction.Allocation = nil
	corrected, err := service.Record(ctx, owner, correction)
	if err != nil {
		t.Fatal(err)
	}
	if corrected.Version != 6 {
		t.Fatal("correction was not append-only")
	}
	progress, err = w.ActivePlanActuals(ctx, owner, month)
	if err != nil {
		t.Fatal(err)
	}
	row = progress[0].Rows[0]
	if row.MissingAllocationCount != 2 || row.KnownInterestMinor != nil || *row.PostedMinor != "12508040" {
		t.Fatalf("corrected coverage: %+v", row)
	}
	after, err := s.ActiveActualBaselines(ctx, owner)
	if err != nil || len(after) != 1 || after[0] != baseline {
		t.Fatal("comparison changed the active baseline")
	}
	foreign := newUser(t, s)
	facts, err = s.PlanActualFacts(ctx, foreign, baseline, month)
	if err != nil || len(facts) != 0 {
		t.Fatalf("foreign facts: %v", err)
	}
	after, err = s.ActiveActualBaselines(ctx, foreign)
	if err != nil || len(after) != 0 {
		t.Fatal("foreign active plan exposed")
	}
}
