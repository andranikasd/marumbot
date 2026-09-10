package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/andranikasd/marumbot/internal/app"
)

func TestLoanFiledOutboxIdempotencyFencingRetryAndPause(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	user, pausedUser := newUser(t, s), newUser(t, s)
	service := app.LoanCommands{Store: s, Clock: clock, Users: s}
	d := draft(user, t)
	key := uuid.NewString()
	receipt, err := service.Create(ctx, key, d)
	if err != nil {
		t.Fatal(err)
	}
	again, err := service.Create(ctx, key, d)
	if err != nil || again != receipt {
		t.Fatalf("lost-response retry: %v", err)
	}
	pausedLoan, err := service.Create(ctx, uuid.NewString(), draft(pausedUser, t))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetUserAccess(ctx, pausedUser, "paused"); err != nil {
		t.Fatal(err)
	}
	now := clock.Now().Add(time.Minute)
	lease := func(at time.Time, loan string, want int) app.LoanFiledNotification {
		t.Helper()
		rows, err := s.LeaseLoanFiled(ctx, at, 1000)
		if err != nil {
			t.Fatal(err)
		}
		var found []app.LoanFiledNotification
		for _, row := range rows {
			if row.LoanID == loan {
				found = append(found, row)
			}
			if row.UserID == pausedUser && loan != pausedLoan.ID {
				t.Fatal("paused account received a notification lease")
			}
		}
		if len(found) != want {
			t.Fatalf("notification count = %d, want %d", len(found), want)
		}
		if want == 0 {
			return app.LoanFiledNotification{}
		}
		return found[0]
	}
	first := lease(now, receipt.ID, 1)
	if first.UserID != user || first.Token == "" {
		t.Fatal("lease owner/token missing")
	}
	wrong := first
	wrong.Token = uuid.NewString()
	if err := s.CompleteLoanFiled(ctx, wrong, now); err != nil {
		t.Fatal(err)
	}
	if err := s.RetryLoanFiled(ctx, wrong, now); err != nil {
		t.Fatal(err)
	}
	lease(now.Add(61*time.Second), receipt.ID, 0)
	second := lease(now.Add(121*time.Second), receipt.ID, 1)
	if second.ID != first.ID || second.Token == first.Token {
		t.Fatal("expired lease was not fenced with a fresh token")
	}
	if err := s.CompleteLoanFiled(ctx, first, now.Add(121*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := s.RetryLoanFiled(ctx, first, now.Add(121*time.Second)); err != nil {
		t.Fatal(err)
	}
	lease(now.Add(182*time.Second), receipt.ID, 0)
	if err := s.RetryLoanFiled(ctx, second, now.Add(182*time.Second)); err != nil {
		t.Fatal(err)
	}
	lease(now.Add(301*time.Second), receipt.ID, 0)
	third := lease(now.Add(302*time.Second), receipt.ID, 1)
	if err := s.CompleteLoanFiled(ctx, third, now.Add(302*time.Second)); err != nil {
		t.Fatal(err)
	}
	lease(now.Add(24*time.Hour), receipt.ID, 0)
	if err := s.SetUserAccess(ctx, pausedUser, "active"); err != nil {
		t.Fatal(err)
	}
	restored := lease(now.Add(24*time.Hour), pausedLoan.ID, 1)
	if restored.UserID != pausedUser {
		t.Fatal("notification crossed account boundary")
	}
	if err := s.CompleteLoanFiled(ctx, restored, now.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
}
