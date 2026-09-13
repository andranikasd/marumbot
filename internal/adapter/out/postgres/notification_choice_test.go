package postgres_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/andranikasd/marumbot/internal/app"
)

func TestNotificationChoiceSuppressesAndReplacesRules(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	user := newUser(t, s)
	clock := &preferenceClock{at: time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)}
	svc := app.PreferenceService{Store: s, Clock: clock}
	p, err := s.UserPreferences(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	if p.RemindersEnabled == nil || !*p.RemindersEnabled {
		t.Fatal("legacy default changed")
	}
	id, err := s.CreateLoan(ctx, draft(user, t))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.EnsureDefaultReminders(ctx, id); err != nil {
		t.Fatal(err)
	}
	due := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
	if err = s.ScheduleReminders(ctx, due, id); err != nil {
		t.Fatal(err)
	}
	count := func() int {
		rows, e := s.ReadyReminders(ctx, clock.at, 500)
		if e != nil {
			t.Fatal(e)
		}
		n := 0
		for _, r := range rows {
			if r.UserID == user {
				n++
				if r.OffsetDays != -1 {
					t.Fatal("old delivery rule survived explicit replacement")
				}
			}
		}
		return n
	}
	enabled := false
	p.RemindersEnabled = &enabled
	lead, minute := 1, 540
	p.ReminderLeadDays = &lead
	p.ReminderMinute = &minute
	saved, err := svc.Save(ctx, user, app.PreferenceCommand{UserPreferences: p, Key: uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	if count() != 0 {
		t.Fatal("opted-out reminder selected")
	}
	enabled = true
	p = saved
	p.RemindersEnabled = &enabled
	command := app.PreferenceCommand{UserPreferences: p, Key: uuid.NewString()}
	saved, err = svc.Save(ctx, user, command)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := svc.Save(ctx, user, command)
	if err != nil || !reflect.DeepEqual(saved, retry) {
		t.Fatal("notification retry changed source", err)
	}
	if err = s.ScheduleReminders(ctx, due, id); err != nil {
		t.Fatal(err)
	}
	if count() != 1 {
		t.Fatal("explicit one-day rule not generated")
	}
	id2, err := s.CreateLoan(ctx, draft(user, t))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.EnsureDefaultReminders(ctx, id2); err != nil {
		t.Fatal(err)
	}
	if err = s.ScheduleReminders(ctx, due, id2); err != nil {
		t.Fatal(err)
	}
	if count() != 2 {
		t.Fatal("new loan did not inherit selected rule")
	}
	// Clients using old preference fields cannot silently re-enable notifications.
	enabled = false
	p = saved
	p.RemindersEnabled = &enabled
	saved, err = svc.Save(ctx, user, app.PreferenceCommand{UserPreferences: p, Key: uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	p = saved
	p.RemindersEnabled = nil
	p.ReminderLeadDays = nil
	p.ReminderMinute = nil
	after, err := svc.Save(ctx, user, app.PreferenceCommand{UserPreferences: p, Key: uuid.NewString()})
	if err != nil || after.RemindersEnabled == nil || *after.RemindersEnabled {
		t.Fatal("legacy settings edit reenabled delivery", err)
	}
}

func TestNewIdentityMayOptOutWithoutChangingExistingChoice(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	in := freshUpsert()
	enabled := false
	in.RemindersEnabled = &enabled
	a, err := s.UpsertByTelegram(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.UserPreferences(ctx, a.ID)
	if err != nil || p.RemindersEnabled == nil || *p.RemindersEnabled {
		t.Fatal("new identity did not remain opt-in", err)
	}
	in.RemindersEnabled = nil
	if _, err = s.UpsertByTelegram(ctx, in); err != nil {
		t.Fatal(err)
	}
	p, err = s.UserPreferences(ctx, a.ID)
	if err != nil || *p.RemindersEnabled {
		t.Fatal("repeat contact reset choice", err)
	}
}
