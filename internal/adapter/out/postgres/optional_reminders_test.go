package postgres_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// This sender has no transport. Integration tests must never contact Telegram.
type optionalProofSender struct {
	app.Sender
	messages []string
}

func (s *optionalProofSender) SendMessage(_ context.Context, _ int64, text string, _ any) error {
	s.messages = append(s.messages, text)
	return nil
}

type optionalProofChats struct{}

func (optionalProofChats) ChatID(context.Context, string) (int64, error) { return 1, nil }

func TestOptionalReminderPersistenceQuietSnoozeAndStaleSuppression(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	owner := newUser(t, s)
	loan, err := s.CreateLoan(ctx, draft(owner, t))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetBudgetConfiguration(ctx, app.BudgetConfiguration{UserID: owner, Currency: "AMD", MonthlyMinor: 600_000_00, PayDay: 1, OpeningAsOf: mustDate(t, "2026-08-01"), Funding: &app.BudgetFunding{MonthlyMinor: 600_000_00}}); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureDefaultReminders(ctx, loan); err != nil {
		t.Fatal(err)
	}
	send := &optionalProofSender{}
	clock := &preferenceClock{at: time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)}
	w := app.Worker{Users: s, Loans: s, Budgets: s, Plans: s, History: s, Reminders: s, Clock: clock, Send: send, Chats: optionalProofChats{}, DefaultCurrency: money.MustLookup("AMD"), Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	sheet, err := w.PlanSheet(ctx, owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	approved, err := w.ActivateProposal(ctx, owner, app.PlanActivationCommand{Proposal: sheet.Proposal, Key: uuid.NewString(), ExpectedRevision: sheet.ActiveRevision})
	if err != nil {
		t.Fatal(err)
	}
	timeline, err := w.PaymentTimeline(ctx, owner, "", approved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.ScheduleForUser(ctx, owner); err != nil {
		t.Fatal(err)
	}
	ready, err := s.DueOptionalReminders(ctx, clock.at, 500)
	if err != nil {
		t.Fatal(err)
	}
	var occurrence app.OptionalReminder
	for _, d := range ready {
		if d.UserID == owner {
			occurrence = d
			break
		}
	}
	if occurrence.ID == "" {
		t.Fatalf("no on-day optional occurrence: %+v", timeline.Payments)
	}
	action := timeline.Payments[occurrence.ActionIndex]
	if action.Kind != "extra" || action.On != occurrence.DueDate || occurrence.PlanID != approved.ID {
		t.Fatal("wrong approved action reference")
	}
	// Repeated concurrent scheduling cannot create two notifications for one action.
	var wg sync.WaitGroup
	failures := make(chan error, 5)
	for range 5 {
		wg.Go(func() {
			failures <- s.ScheduleOptionalReminder(ctx, owner, approved.ID, loan, occurrence.ActionIndex, occurrence.DueDate)
		})
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	ready, err = s.DueOptionalReminders(ctx, clock.at, 500)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, d := range ready {
		if d.ID == occurrence.ID {
			count++
		}
	}
	if count != 1 {
		t.Fatal("optional occurrence duplicated")
	}
	required, err := s.ReadyReminders(ctx, clock.at, 500)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range required {
		if d.ID == occurrence.ID {
			t.Fatal("optional leaked into required queue")
		}
	}
	meta, err := s.ReminderOccurrence(ctx, owner, occurrence.ID)
	if err != nil || meta.Required {
		t.Fatalf("optional mislabeled: %+v %v", meta, err)
	}
	if _, err := s.ReminderOccurrence(ctx, newUser(t, s), occurrence.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("foreign occurrence exposed")
	}
	prefs, err := s.UserPreferences(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	// Quiet changes do not invalidate financial sources. Optional and required
	// reminders obey the same local window and the same boundary semantics.
	loc, err := time.LoadLocation(prefs.Timezone)
	if err != nil {
		t.Fatal(err)
	}
	local := clock.at.In(loc)
	minute := local.Hour()*60 + local.Minute()
	prefs.QuietEnabled = true
	prefs.QuietStart = minute
	prefs.QuietEnd = (minute + 60) % 1440
	svc := app.PreferenceService{Store: s, Clock: clock}
	prefs, err = svc.Save(ctx, owner, app.PreferenceCommand{UserPreferences: prefs, Key: uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	ready, err = s.DueOptionalReminders(ctx, clock.at, 500)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range ready {
		if d.ID == occurrence.ID {
			t.Fatal("quiet optional selected")
		}
	}
	prefs.QuietEnabled = false
	if _, err := svc.Save(ctx, owner, app.PreferenceCommand{UserPreferences: prefs, Key: uuid.NewString()}); err != nil {
		t.Fatal(err)
	}
	snooze := app.SnoozeCommand{OccurrenceID: meta.ID, Until: clock.at.Add(time.Hour), ExpectedVersion: meta.Version, Key: uuid.NewString()}
	snoozed, err := svc.Snooze(ctx, owner, snooze)
	if err != nil || snoozed.Required || snoozed.DueDate != meta.DueDate {
		t.Fatalf("snooze changed action: %+v %v", snoozed, err)
	}
	if again, err := svc.Snooze(ctx, owner, snooze); err != nil || again != snoozed {
		t.Fatal("snooze retry differs")
	}
	ready, err = s.DueOptionalReminders(ctx, clock.at, 500)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range ready {
		if d.ID == occurrence.ID {
			t.Fatal("snooze ignored")
		}
	}
	clock.at = clock.at.Add(time.Hour)
	n, err := w.SendDueReminders(ctx, 500)
	if err != nil || n == 0 {
		t.Fatalf("delivery: %d %v", n, err)
	}
	found := false
	for _, text := range send.messages {
		if strings.Contains(text, "optional") || strings.Contains(text, "ոչ պարտադիր") {
			found = true
		}
	}
	if !found {
		t.Fatal("optional label absent")
	}
	meta, err = s.ReminderOccurrence(ctx, owner, occurrence.ID)
	if err != nil || meta.Status != "satisfied" {
		t.Fatalf("delivery not recorded: %+v %v", meta, err)
	}
	if err := w.ScheduleForUser(ctx, owner); err != nil {
		t.Fatal(err)
	}
	ready, err = s.DueOptionalReminders(ctx, clock.at, 500)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range ready {
		if d.ID == occurrence.ID {
			t.Fatal("delivered action rescheduled")
		}
	}
	// An explicit snooze may reopen delivery, but a changed budget must still
	// suppress it without altering required reminders or the approved version.
	snooze.Key = uuid.NewString()
	snooze.ExpectedVersion = meta.Version
	snooze.Until = clock.at.Add(time.Hour)
	if _, err := svc.Snooze(ctx, owner, snooze); err != nil {
		t.Fatal(err)
	}
	if err := s.SetBudget(ctx, owner, "AMD", 700_000_00, 1); err != nil {
		t.Fatal(err)
	}
	clock.at = clock.at.Add(time.Hour)
	before := len(send.messages)
	if _, err := w.SendDueReminders(ctx, 500); err != nil {
		t.Fatal(err)
	}
	if len(send.messages) != before {
		t.Fatal("outdated optional instruction sent")
	}
	meta, err = s.ReminderOccurrence(ctx, owner, occurrence.ID)
	if err != nil || meta.Status != "canceled" {
		t.Fatal("outdated occurrence not canceled")
	}
	if err := w.ScheduleForUser(ctx, owner); err != nil {
		t.Fatal(err)
	}
	history, _, err := s.PlanHistory(ctx, owner)
	if err != nil || len(history) != 1 || !history[0].Active {
		t.Fatal("reminder changed plan activation")
	}
	if err := s.ScheduleReminders(ctx, clock.at, loan); err != nil {
		t.Fatal(err)
	}
	required, err = s.ReadyReminders(ctx, clock.at, 500)
	if err != nil {
		t.Fatal(err)
	}
	hasRequired := false
	for _, d := range required {
		if d.LoanID == loan {
			hasRequired = true
		}
	}
	if !hasRequired {
		t.Fatal("stale plan suppressed required reminder")
	}
}
