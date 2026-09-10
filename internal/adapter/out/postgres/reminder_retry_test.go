package postgres_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/andranikasd/marumbot/internal/app"
)

func TestReminderRetryBackoffSnoozeAndPausedAccount(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	user := newUser(t, s)
	loan, err := s.CreateLoan(ctx, draft(user, t))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.EnsureDefaultReminders(ctx, loan); err != nil {
		t.Fatal(err)
	}
	due := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	now := due.Add(12 * time.Hour)
	if err = s.ScheduleReminders(ctx, due, loan); err != nil {
		t.Fatal(err)
	}
	ready, err := s.ReadyReminders(ctx, now, 10000)
	if err != nil {
		t.Fatal(err)
	}
	id := ""
	for _, row := range ready {
		if row.UserID == user {
			id = row.ID
			break
		}
	}
	if id == "" {
		t.Fatal("fresh required reminder was not due")
	}
	assertDue := func(at time.Time, want bool) {
		t.Helper()
		rows, readErr := s.ReadyReminders(ctx, at, 10000)
		if readErr != nil {
			t.Fatal(readErr)
		}
		found := false
		for _, row := range rows {
			if row.ID == id {
				found = true
			}
		}
		if found != want {
			t.Fatalf("due at %s = %v; want %v", at, found, want)
		}
	}
	if err = s.DeferReminderDelivery(ctx, id, now); err != nil {
		t.Fatal(err)
	}
	assertDue(now.Add(59*time.Second), false)
	assertDue(now.Add(time.Minute), true)
	second := now.Add(time.Minute)
	if err = s.DeferReminderDelivery(ctx, id, second); err != nil {
		t.Fatal(err)
	}
	assertDue(second.Add(119*time.Second), false)
	assertDue(second.Add(2*time.Minute), true)
	if err = s.SetUserAccess(ctx, user, "paused"); err != nil {
		t.Fatal(err)
	}
	assertDue(second.Add(2*time.Minute), false)
	if err = s.SetUserAccess(ctx, user, "active"); err != nil {
		t.Fatal(err)
	}
	assertDue(second.Add(2*time.Minute), true)
	original, err := s.ReminderOccurrence(ctx, user, id)
	if err != nil {
		t.Fatal(err)
	}
	svc := app.PreferenceService{Store: s, Clock: &preferenceClock{at: now}}
	until := now.Add(time.Hour)
	if _, err = svc.Snooze(ctx, user, app.SnoozeCommand{OccurrenceID: id, Until: until, ExpectedVersion: original.Version, Key: uuid.NewString()}); err != nil {
		t.Fatal(err)
	}
	// A late failure from the old send must not defer or overwrite the snooze.
	if err = s.DeferReminderDelivery(ctx, id, now); err != nil {
		t.Fatal(err)
	}
	assertDue(until.Add(-time.Second), false)
	assertDue(until, true)
	// Snooze resets attempts: its first failure waits one minute again.
	if err = s.DeferReminderDelivery(ctx, id, until); err != nil {
		t.Fatal(err)
	}
	assertDue(until.Add(59*time.Second), false)
	assertDue(until.Add(time.Minute), true)
}
