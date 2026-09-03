package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

type reminderUsersFake struct {
	UserStore
	prefs UserPreferences
}

func (f reminderUsersFake) Locale(context.Context, string) (string, string, error) {
	return "en", f.prefs.Timezone, nil
}

func (f reminderUsersFake) UserPreferences(context.Context, string) (UserPreferences, error) {
	return f.prefs, nil
}

type reminderDeliveryFake struct {
	reviseFakes
	due    []DueReminder
	marked int
	markup any
}

func (f *reminderDeliveryFake) DueReminders(context.Context, int32) ([]DueReminder, error) {
	return f.due, nil
}

func (f *reminderDeliveryFake) MarkReminderSatisfied(context.Context, string) error {
	f.marked++
	return nil
}

func (f *reminderDeliveryFake) SendMessage(_ context.Context, _ int64, text string, markup any) error {
	f.messages = append(f.messages, text)
	f.markup = markup
	return nil
}

func TestReminderQuietClockAndExactResolution(t *testing.T) {
	f := &reminderDeliveryFake{due: []DueReminder{{ID: "occurrence-a", UserID: "owner", LoanID: "loan-a", DueDate: "2026-09-15", LoanName: "Loan", Currency: "AMD"}}}
	f.loan = paidLoan(t)
	w := reviseWorker(t, &f.reviseFakes)
	w.Reminders = f
	w.Send = f
	w.Chats = menuChatsFake{}
	w.MiniApp = "https://example.test/app/"
	clock := &fixedClock{at: time.Date(2026, 9, 1, 18, 0, 0, 0, time.UTC)}
	w.Clock = clock
	w.Users = reminderUsersFake{prefs: UserPreferences{Timezone: "Asia/Yerevan", QuietEnabled: true, QuietStart: 1320, QuietEnd: 480}}
	n, err := w.SendDueReminders(t.Context(), 50)
	if err != nil || n != 0 || f.marked != 0 {
		t.Fatalf("quiet delivery: %d %v", n, err)
	}
	clock.at = time.Date(2026, 9, 2, 4, 0, 0, 0, time.UTC)
	n, err = w.SendDueReminders(t.Context(), 50)
	if err != nil || n != 1 || f.marked != 1 {
		t.Fatalf("quiet end: %d %v", n, err)
	}
	if !strings.Contains(f.messages[0], "Required payment") {
		t.Fatal("required label missing")
	}
	raw, _ := json.Marshal(f.markup)
	if !strings.Contains(string(raw), "screen=reminder") || !strings.Contains(string(raw), "id=occurrence-a") {
		t.Fatal("button does not resolve exact occurrence", string(raw))
	}
}

func TestQuietWindowsRespectTimezoneAndDST(t *testing.T) {
	for _, tt := range []struct {
		name, zone, instant string
		start, end          int
		quiet               bool
	}{
		{"overnight start", "Asia/Yerevan", "2026-09-01T18:00:00Z", 1320, 480, true},
		{"overnight end", "Asia/Yerevan", "2026-09-02T04:00:00Z", 1320, 480, false},
		{"daytime", "Asia/Kathmandu", "2026-09-01T06:15:00Z", 720, 780, true},
		{"daytime end", "Asia/Kathmandu", "2026-09-01T07:15:00Z", 720, 780, false},
		{"fall first hour", "America/New_York", "2026-11-01T05:30:00Z", 60, 120, true},
		{"fall repeated hour", "America/New_York", "2026-11-01T06:30:00Z", 60, 120, true},
		{"spring skipped end", "America/New_York", "2026-03-08T07:00:00Z", 60, 150, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			at, err := time.Parse(time.RFC3339, tt.instant)
			if err != nil {
				t.Fatal(err)
			}
			p := UserPreferences{Timezone: tt.zone, QuietEnabled: true, QuietStart: tt.start, QuietEnd: tt.end}
			if err = p.Validate(); err != nil {
				t.Fatal(err)
			}
			if got := p.QuietAt(at); got != tt.quiet {
				t.Fatalf("quiet=%v", got)
			}
		})
	}
	for _, zone := range []string{"", "Local", "UTC+4", "Mars/Base", " Asia/Yerevan"} {
		if err := (UserPreferences{Timezone: zone}).Validate(); err == nil {
			t.Fatalf("invalid zone accepted: %q", zone)
		}
	}
	if err := (UserPreferences{Timezone: "UTC", QuietEnabled: true, QuietStart: 60, QuietEnd: 60}).Validate(); err == nil {
		t.Fatal("empty quiet window accepted")
	}
}

type changedReminderFake struct {
	reminderDeliveryFake
	current    ReminderOccurrence
	selectedAt time.Time
}

func (f *changedReminderFake) ReadyReminders(_ context.Context, at time.Time, _ int32) ([]DueReminder, error) {
	f.selectedAt = at
	return f.due, nil
}

func (f *changedReminderFake) ReminderOccurrence(context.Context, string, string) (ReminderOccurrence, error) {
	return f.current, nil
}

func (f *changedReminderFake) MarkReminderDelivered(context.Context, string, time.Time) error {
	f.marked++
	return nil
}

func TestReminderRechecksSnoozeBeforeDelivery(t *testing.T) {
	clock := &fixedClock{at: time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)}
	f := &changedReminderFake{current: ReminderOccurrence{Status: "scheduled", TargetSendAt: clock.at.Add(time.Hour)}}
	f.due = []DueReminder{{ID: "occurrence", UserID: "owner", LoanID: "loan-a", DueDate: "2026-09-15"}}
	f.loan = paidLoan(t)
	w := reviseWorker(t, &f.reviseFakes)
	w.Reminders = f
	w.Send = f
	w.Clock = clock
	w.Chats = menuChatsFake{}
	w.Users = reminderUsersFake{prefs: UserPreferences{Timezone: "UTC"}}
	n, err := w.SendDueReminders(t.Context(), 50)
	if err != nil || n != 0 || f.marked != 0 || len(f.messages) != 0 {
		t.Fatal("stale selection delivered after snooze", n, err)
	}
	if !f.selectedAt.Equal(clock.at) {
		t.Fatal("store did not receive injected clock")
	}
}
