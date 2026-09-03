package postgres_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/andranikasd/marumbot/internal/app"
)

type allocationClock struct{}

func (allocationClock) Now() time.Time { return time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC) }

func TestPaymentAllocationActualsCorrectionAndCoverage(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	owner := newUser(t, s)
	loan, err := s.CreateLoan(ctx, draft(owner, t))
	if err != nil {
		t.Fatal(err)
	}
	service := app.PaymentService{Store: s, Clock: allocationClock{}}
	principal, interest, fees := int64(5603410), int64(6904530), int64(0)
	c := app.PaymentCommand{LoanID: loan, Key: uuid.NewString(), AmountMinor: 12507940, TransactionDate: "2026-09-24", ValueDate: "2026-09-24", Allocation: &app.PaymentAllocation{PrincipalMinor: &principal, InterestMinor: &interest, FeesMinor: &fees}}
	first, err := service.Record(ctx, owner, c)
	if err != nil {
		t.Fatal(err)
	}
	if retry, err := service.Record(ctx, owner, c); err != nil || retry != first {
		t.Fatalf("retry: %v", err)
	}
	changed := c
	changed.Allocation = nil
	if _, err := service.Record(ctx, owner, changed); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("split not hashed: %v", err)
	}
	// A pending source transfer has unknown allocation, never zero interest.
	unknown := app.PaymentCommand{LoanID: loan, Key: uuid.NewString(), ExpectedVersion: 1, AmountMinor: 100, TransactionDate: "2026-09-25"}
	if _, err := service.Record(ctx, owner, unknown); err != nil {
		t.Fatal(err)
	}
	totals, err := s.MonthlyPaymentActuals(ctx, owner, "2026-09")
	if err != nil {
		t.Fatal(err)
	}
	if len(totals) != 1 {
		t.Fatalf("totals: %+v", totals)
	}
	a := totals[0]
	if a.PaidMinor != "12508040" || a.UnknownPaidMinor != "100" || a.KnownCount != 1 || a.UnknownCount != 1 || a.PendingCount != 1 || a.InterestMinor == nil || *a.InterestMinor != "6904530" || a.FeesMinor == nil || *a.FeesMinor != "0" {
		t.Fatalf("coverage: %+v", a)
	}
	// Correction removes the old known split from September and moves the
	// replacement to October. Original history and its allocation survive.
	replacement := c
	replacement.Key = uuid.NewString()
	replacement.ExpectedVersion = 2
	replacement.Replaces = first.ID
	replacement.TransactionDate = "2026-10-24"
	replacement.ValueDate = "2026-10-24"
	corrected, err := service.Record(ctx, owner, replacement)
	if err != nil {
		t.Fatal(err)
	}
	if corrected.Version != 4 {
		t.Fatal("correction did not append two events")
	}
	if retry, err := service.Record(ctx, owner, replacement); err != nil || retry != corrected {
		t.Fatalf("correction retry: %v", err)
	}
	totals, err = s.MonthlyPaymentActuals(ctx, owner, "2026-09")
	if err != nil {
		t.Fatal(err)
	}
	if len(totals) != 1 || totals[0].PaidMinor != "100" || totals[0].InterestMinor != nil || totals[0].FeesMinor != nil || totals[0].KnownCount != 0 || totals[0].UnknownCount != 1 {
		t.Fatalf("unknown became zero: %+v", totals)
	}
	facts, err := s.BorrowerAllocatedActivity(ctx, owner, "")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range facts {
		if f.ID == first.ID {
			found = true
			if !f.Voided || f.Allocation == nil || *f.Allocation.InterestMinor != interest {
				t.Fatal("original rewritten")
			}
		}
	}
	if !found {
		t.Fatal("missing original")
	}
	foreign := newUser(t, s)
	totals, err = s.MonthlyPaymentActuals(ctx, foreign, "2026-10")
	if err != nil || len(totals) != 0 {
		t.Fatalf("foreign actuals: %v", err)
	}
	facts, err = s.BorrowerAllocatedActivity(ctx, foreign, first.ID)
	if err != nil || len(facts) != 0 {
		t.Fatalf("foreign activity: %v", err)
	}
	void := app.PaymentCommand{LoanID: loan, Key: uuid.NewString(), ExpectedVersion: 4, TransactionDate: "2026-10-25", Replaces: corrected.ID, VoidOnly: true}
	if _, err := service.Record(ctx, owner, void); err != nil {
		t.Fatal(err)
	}
	totals, err = s.MonthlyPaymentActuals(ctx, owner, "2026-10")
	if err != nil || len(totals) != 0 {
		t.Fatalf("void in totals: %v", err)
	}
}
