package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
)

func TestReconcilePaymentRestatesCashWithoutDoubleDeduction(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	owner := newUser(t, s)
	d := draft(owner, t)
	d.Contract.Type = model.Annuity
	loan, err := s.CreateLoan(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	service := app.PaymentService{Store: s, Clock: clock}
	today, err := service.BusinessDate(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetBudgetConfiguration(ctx, app.BudgetConfiguration{UserID: owner, Currency: "AMD", MonthlyMinor: 100000, OpeningAsOf: today, OpeningMinor: 80000, Funding: &app.BudgetFunding{MonthlyMinor: 100000}}); err != nil {
		t.Fatal(err)
	}
	budget, err := s.Budget(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	paid, err := service.Record(ctx, owner, app.PaymentCommand{LoanID: loan, Key: uuid.NewString(), AmountMinor: 20000, TransactionDate: today.String(), ValueDate: today.String()})
	if err != nil {
		t.Fatal(err)
	}
	c := app.ReconciliationCommand{LoanID: loan, Key: uuid.NewString(), ExpectedVersion: paid.Version, BudgetVersion: budget.Version, AsOf: today.String(), PrincipalMinor: 60000, NextDue: date.OnDayOfMonth(date.AddMonths(today, 1), 15).String(), NextPaymentMinor: 30000, CashMinor: 60000, SpentMinor: 40000, IncludePosted: true}
	low := c
	low.SpentMinor = 19999
	if _, err = service.Reconcile(ctx, owner, low); !errors.Is(err, app.ErrPaymentInvalid) {
		t.Fatalf("reported spending understated: %v", err)
	}
	result, err := service.Reconcile(ctx, owner, c)
	if err != nil {
		t.Fatal(err)
	}
	again, err := service.Reconcile(ctx, owner, c)
	if err != nil || again != result {
		t.Fatalf("retry: %v", err)
	}
	if _, err = service.Reconcile(ctx, newUser(t, s), c); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("ownership: %v", err)
	}
	changed := c
	changed.CashMinor++
	if _, err = service.Reconcile(ctx, owner, changed); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("changed retry: %v", err)
	}
	read, err := s.LoanForUser(ctx, loan, owner)
	if err != nil || read.UnreconciledPayments || read.Balance.Minor() != 60000 || read.Trust != "user_entered" {
		t.Fatalf("anchor: %v", err)
	}
	if read.Contract.NotBeforeDue.String() != c.NextDue || read.Contract.ScheduledPayment.Minor() != 30000 {
		t.Fatal("bank obligation lost")
	}
	budget, err = s.Budget(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	cash := budget.CashPlan(today)
	// Source statements already include the 20,000 payment: 60,000 cash and
	// 40,000 spent remain exactly those amounts, never 40,000 and 60,000.
	if cash.OpeningCash.Minor() != 60000 || cash.Spending.Spent.Minor() != 40000 || cash.CashThrough != today {
		t.Fatal("payment counted twice")
	}
	facts, err := s.BorrowerActivity(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range facts {
		if f.ID == paid.ID && f.Status != "reconciled" {
			t.Fatal("covered payment still pending")
		}
	}
	// Correcting a previously covered payment requires a new statement.
	_, err = service.Record(ctx, owner, app.PaymentCommand{LoanID: loan, Key: uuid.NewString(), ExpectedVersion: result.Version, TransactionDate: today.String(), Replaces: paid.ID, VoidOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	read, err = s.LoanForUser(ctx, loan, owner)
	if err != nil || !read.UnreconciledPayments {
		t.Fatal("old anchor survived a correction")
	}
}
