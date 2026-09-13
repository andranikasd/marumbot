package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

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
	if err = s.EnsureDefaultReminders(ctx, loan); err != nil {
		t.Fatal(err)
	}
	oldDue := today.AtLocal(0, 0, time.UTC)
	nextDue, err := date.Parse(c.NextDue)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ScheduleReminderDates(ctx, loan, []time.Time{oldDue, nextDue.AtLocal(0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	before, err := s.ReadyReminders(ctx, nextDue.AtLocal(23, 59, time.UTC), 500)
	if err != nil {
		t.Fatal(err)
	}
	var stale, upcoming []string
	for _, reminder := range before {
		if reminder.LoanID != loan {
			continue
		}
		if reminder.DueDate == today.String() {
			stale = append(stale, reminder.ID)
		} else {
			upcoming = append(upcoming, reminder.ID)
		}
	}
	if len(stale) != 2 || len(upcoming) != 2 {
		t.Fatal("missing scheduled reminder fixtures")
	}
	delivered := stale[0]
	stale = stale[1:]
	if err = s.MarkReminderSatisfied(ctx, delivered); err != nil {
		t.Fatal(err)
	}
	low := c
	low.SpentMinor = 19999
	if _, err = service.Reconcile(ctx, owner, low); !errors.Is(err, app.ErrPaymentInvalid) {
		t.Fatalf("reported spending understated: %v", err)
	}
	result, err := service.Reconcile(ctx, owner, c)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range stale {
		reminder, readErr := s.ReminderOccurrence(ctx, owner, id)
		if readErr != nil || reminder.Status != "canceled" {
			t.Fatal("paid instalment reminder remains active", readErr)
		}
	}
	for _, id := range upcoming {
		reminder, readErr := s.ReminderOccurrence(ctx, owner, id)
		if readErr != nil || reminder.Status != "scheduled" {
			t.Fatal("future obligation reminder was canceled", readErr)
		}
	}
	deliveredOccurrence, err := s.ReminderOccurrence(ctx, owner, delivered)
	if err != nil {
		t.Fatal(err)
	}
	preferences := app.PreferenceService{Store: s, Clock: clock}
	_, err = preferences.Snooze(ctx, owner, app.SnoozeCommand{OccurrenceID: delivered, Until: clock.Now().Add(time.Hour), ExpectedVersion: deliveredOccurrence.Version, Key: uuid.NewString()})
	if !errors.Is(err, app.ErrConflict) {
		t.Fatal("paid reminder was resurrected", err)
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
	// Restating only the balance must not bring a paid instalment back.
	if err = s.RecordBalance(ctx, loan, owner, 60000, today.String()); err != nil {
		t.Fatal(err)
	}
	balance := int64(60000)
	if err = s.ApplyLoanRevision(ctx, loan, owner, app.LoanRevision{BalanceMinor: &balance, BalanceAsOf: today, EffectiveFrom: today}); err != nil {
		t.Fatal(err)
	}
	read, err = s.LoanForUser(ctx, loan, owner)
	if err != nil || read.Contract.NotBeforeDue.String() != c.NextDue || read.Contract.ScheduledPayment.Minor() != c.NextPaymentMinor {
		t.Fatal("balance edit lost confirmed next obligation", err)
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
	// Changed terms invalidate the old obligation rather than copying it.
	contract := read.Contract
	contract.PaymentDay = 16
	if err = s.ApplyLoanRevision(ctx, loan, owner, app.LoanRevision{Contract: &contract, BalanceMinor: &balance, BalanceAsOf: today, EffectiveFrom: today}); err != nil {
		t.Fatal(err)
	}
	read, err = s.LoanForUser(ctx, loan, owner)
	if err != nil || !read.Contract.NotBeforeDue.IsZero() {
		t.Fatal("old obligation survived changed terms", err)
	}
}
