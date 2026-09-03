package postgres_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/andranikasd/marumbot/internal/app"
)

type preferenceClock struct{ at time.Time }

func (c *preferenceClock) Now() time.Time { return c.at }

func TestUserPreferencesCASReplayAndSnoozeOwnership(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	owner := newUser(t, s)
	other := newUser(t, s)
	clock := &preferenceClock{at: time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)}
	svc := app.PreferenceService{Store: s, Clock: clock}
	p, err := s.UserPreferences(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	p.Timezone = "America/New_York"
	p.QuietEnabled = true
	p.QuietStart = 1320
	p.QuietEnd = 480
	command := app.PreferenceCommand{UserPreferences: p, Key: uuid.NewString()}
	saved, err := svc.Save(ctx, owner, command)
	if err != nil || saved.Version != 1 {
		t.Fatal(saved, err)
	}
	retry, err := svc.Save(ctx, owner, command)
	if err != nil || retry != saved {
		t.Fatal("replay", retry, err)
	}
	changed := command
	changed.Timezone = "UTC"
	if _, err = svc.Save(ctx, owner, changed); !errors.Is(err, app.ErrConflict) {
		t.Fatal("mutated key", err)
	}
	stale := command
	stale.Key = uuid.NewString()
	if _, err = svc.Save(ctx, owner, stale); !errors.Is(err, app.ErrConflict) {
		t.Fatal("stale CAS", err)
	}
	_, zone, err := s.Locale(ctx, owner)
	if err != nil || zone != saved.Timezone {
		t.Fatal("business timezone not synchronized", zone, err)
	}
	loan, err := s.CreateLoan(ctx, draft(owner, t))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.EnsureDefaultReminders(ctx, loan); err != nil {
		t.Fatal(err)
	}
	due := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	if err = s.ScheduleReminders(ctx, due, loan); err != nil {
		t.Fatal(err)
	}
	ready, err := s.ReadyReminders(ctx, clock.at, 500)
	if err != nil {
		t.Fatal(err)
	}
	var id string
	for _, r := range ready {
		if r.UserID == owner {
			id = r.ID
			break
		}
	}
	if id == "" {
		t.Fatal("missing occurrence")
	}
	original, err := s.ReminderOccurrence(ctx, owner, id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ReminderOccurrence(ctx, other, id); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("read ownership", err)
	}
	if err = s.MarkReminderSatisfied(ctx, id); err != nil {
		t.Fatal(err)
	}
	snooze := app.SnoozeCommand{OccurrenceID: id, Until: clock.at.Add(24 * time.Hour), ExpectedVersion: original.Version, Key: uuid.NewString()}
	if _, err = svc.Snooze(ctx, other, snooze); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("snooze ownership", err)
	}
	result, err := svc.Snooze(ctx, owner, snooze)
	if err != nil {
		t.Fatal(err)
	}
	if result.DueDate != original.DueDate || !result.TargetSendAt.Equal(snooze.Until) || result.Status != "scheduled" || !result.Required {
		t.Fatal("snooze changed contract", result)
	}
	// A delivery selected before snooze must not satisfy its new future send.
	if err = s.MarkReminderDelivered(ctx, id, clock.at); err != nil {
		t.Fatal(err)
	}
	after, err := s.ReminderOccurrence(ctx, owner, id)
	if err != nil || after.Status != "scheduled" {
		t.Fatal("stale delivery erased snooze", after, err)
	}
	p = saved
	p.Timezone = "Asia/Yerevan"
	p.QuietEnabled = false
	if _, err = svc.Save(ctx, owner, app.PreferenceCommand{UserPreferences: p, Key: uuid.NewString()}); err != nil {
		t.Fatal(err)
	}
	if err = s.ScheduleReminders(ctx, due, loan); err != nil {
		t.Fatal(err)
	}
	after, err = s.ReminderOccurrence(ctx, owner, id)
	if err != nil || after.DueDate != original.DueDate || !after.TargetSendAt.Equal(snooze.Until) {
		t.Fatal("regeneration or timezone reset snooze", after, err)
	}
	clock.at = snooze.Until.Add(time.Hour)
	replay, err := svc.Snooze(ctx, owner, snooze)
	if err != nil || replay != result {
		t.Fatal("expired retry must replay", replay, err)
	}
	snooze.Key = uuid.NewString()
	snooze.Until = clock.at.Add(time.Hour)
	if _, err = svc.Snooze(ctx, owner, snooze); !errors.Is(err, app.ErrConflict) {
		t.Fatal("stale snooze CAS", err)
	}
}

func TestUserPreferencesConcurrentCAS(t *testing.T) {
	s := testStore(t)
	user := newUser(t, s)
	p, err := s.UserPreferences(t.Context(), user)
	if err != nil {
		t.Fatal(err)
	}
	svc := app.PreferenceService{Store: s}
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, zone := range []string{"UTC", "Europe/Paris"} {
		wg.Add(1)
		go func(zone string) {
			defer wg.Done()
			c := app.PreferenceCommand{UserPreferences: p, Key: uuid.NewString()}
			c.Timezone = zone
			_, err := svc.Save(t.Context(), user, c)
			results <- err
		}(zone)
	}
	wg.Wait()
	close(results)
	wins, conflicts := 0, 0
	for err := range results {
		switch {
		case err == nil:
			wins++
		case errors.Is(err, app.ErrConflict):
			conflicts++
		default:
			t.Fatal(err)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatalf("wins=%d conflicts=%d", wins, conflicts)
	}
}

func TestUserPreferencesQuietFilteringBeforeLimit(t *testing.T) {
	s := testStore(t)
	owner := newUser(t, s)
	ctx := t.Context()
	clock := &preferenceClock{at: time.Date(2026, 9, 1, 18, 0, 0, 0, time.UTC)}
	loan, err := s.CreateLoan(ctx, draft(owner, t))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.EnsureDefaultReminders(ctx, loan); err != nil {
		t.Fatal(err)
	}
	if err = s.ScheduleReminders(ctx, clock.at, loan); err != nil {
		t.Fatal(err)
	}
	p, err := s.UserPreferences(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	p.QuietEnabled = true
	if _, err = (app.PreferenceService{Store: s, Clock: clock}).Save(ctx, owner, app.PreferenceCommand{UserPreferences: p, Key: uuid.NewString()}); err != nil {
		t.Fatal(err)
	}
	rows, err := s.ReadyReminders(ctx, clock.at, 500)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.UserID == owner {
			t.Fatal("quiet reminder selected")
		}
	}
	clock.at = clock.at.Add(10 * time.Hour)
	rows, err = s.ReadyReminders(ctx, clock.at, 500)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.UserID == owner {
			return
		}
	}
	t.Fatal("quiet end did not release reminder")
}
