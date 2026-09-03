package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/andranikasd/marumbot/internal/i18n"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/money"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

type optionalHistoryFake struct {
	PlanHistoryStore
	manifest PlanManifest
	sources  string
	active   bool
	err      error
}

func (h *optionalHistoryFake) PlanSources(context.Context, string) (string, error) {
	return h.sources, h.err
}

func (h *optionalHistoryFake) PlanHistory(context.Context, string) ([]PlanVersion, int64, error) {
	return []PlanVersion{{ID: "approved", Active: h.active, Manifest: h.manifest}}, 1, h.err
}

func (h *optionalHistoryFake) PlanVersion(_ context.Context, user, id string) (PlanVersion, error) {
	if user != "owner" || id != "approved" {
		return PlanVersion{}, ErrNotFound
	}
	return PlanVersion{ID: id, Manifest: h.manifest}, h.err
}

type optionalDeliveryFake struct {
	changedReminderFake
	optional         []OptionalReminder
	scheduled        []OptionalReminder
	canceled         []string
	active           []string
	scheduleRequired int
	optionalErr      error
	failSend         bool
	onRead           func()
}

func (f *optionalDeliveryFake) ScheduleReminders(context.Context, time.Time, string) error {
	f.scheduleRequired++
	return nil
}

func (f *optionalDeliveryFake) ScheduleOptionalReminder(_ context.Context, user, planID, loan string, index int, on string) error {
	f.scheduled = append(f.scheduled, OptionalReminder{DueReminder: DueReminder{UserID: user, LoanID: loan, DueDate: on}, PlanID: planID, ActionIndex: index})
	return nil
}

func (f *optionalDeliveryFake) CancelObsoleteOptionalReminders(_ context.Context, _ string, active []string) error {
	f.active = active
	return nil
}

func (f *optionalDeliveryFake) CancelOptionalReminder(_ context.Context, id string) error {
	f.canceled = append(f.canceled, id)
	return nil
}

func (f *optionalDeliveryFake) DueOptionalReminders(context.Context, time.Time, int32) ([]OptionalReminder, error) {
	if f.onRead != nil {
		f.onRead()
	}
	return f.optional, f.optionalErr
}

func (f *optionalDeliveryFake) SendMessage(ctx context.Context, chat int64, text string, markup any) error {
	if f.failSend {
		return errors.New("test delivery unavailable")
	}
	return f.reminderDeliveryFake.SendMessage(ctx, chat, text, markup)
}

func optionalWorker(t *testing.T) (*Worker, *optionalDeliveryFake, *optionalHistoryFake, PlanPayment) {
	t.Helper()
	in := cacheInput(t)
	in.Cash.PayDay = in.ValuationDate.Day()
	in.Cash.OpeningCash = money.FromMinor(20_000_000, in.Cash.Monthly.Currency())
	report, err := plan.Search(in, plan.Goal{Kind: plan.LeastInterest})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := manifestFor(in, plan.Goal{Kind: plan.LeastInterest}, report, 1)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Sources = "sources"
	h := &optionalHistoryFake{manifest: manifest, sources: manifest.Sources, active: true}
	f := &optionalDeliveryFake{}
	f.loan = paidLoan(t)
	f.current = ReminderOccurrence{Status: "scheduled", TargetSendAt: in.ValuationDate.AtLocal(10, 0, time.UTC)}
	w := reviseWorker(t, &f.reviseFakes)
	w.Reminders = f
	w.Send = f
	w.Chats = menuChatsFake{}
	w.Users = reminderUsersFake{prefs: UserPreferences{Timezone: "UTC"}}
	w.History = h
	w.MiniApp = "https://example.test/app/"
	w.Clock = &fixedClock{at: in.ValuationDate.AtLocal(12, 0, time.UTC)}
	timeline, err := w.PaymentTimeline(t.Context(), "owner", "", "approved")
	if err != nil {
		t.Fatal(err)
	}
	for index, a := range timeline.Payments {
		if a.Kind == "extra" && a.On == in.ValuationDate.String() {
			f.optional = []OptionalReminder{{DueReminder: DueReminder{ID: "optional-occurrence", UserID: "owner", LoanID: a.LoanID, DueDate: a.On, LoanName: a.Loan, Currency: timeline.Currency}, PlanID: "approved", ActionIndex: index}}
			return w, f, h, a
		}
	}
	t.Fatal("fixture has no on-receipt extra on valuation date")
	return nil, nil, nil, PlanPayment{}
}

func TestOptionalRemindersScheduleOnlyApprovedDatedExtras(t *testing.T) {
	w, f, h, _ := optionalWorker(t)
	if err := w.scheduleOptionalReminders(t.Context(), "owner"); err != nil {
		t.Fatal(err)
	}
	if len(f.scheduled) == 0 {
		t.Fatal("no optional actions scheduled")
	}
	timeline, err := w.PaymentTimeline(t.Context(), "owner", "", "approved")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range f.scheduled {
		a := timeline.Payments[d.ActionIndex]
		if a.Kind != "extra" || a.On != d.DueDate || a.LoanID != d.LoanID || d.PlanID != "approved" {
			t.Fatal("not an exact approved action")
		}
	}
	h.sources = "changed"
	f.scheduled = nil
	if err := w.scheduleOptionalReminders(t.Context(), "owner"); err != nil {
		t.Fatal(err)
	}
	if len(f.scheduled) != 0 || len(f.active) != 0 {
		t.Fatal("outdated plan scheduled extras")
	}
	h.sources = h.manifest.Sources
	h.active = false
	if err := w.scheduleOptionalReminders(t.Context(), "owner"); err != nil {
		t.Fatal(err)
	}
	if len(f.scheduled) != 0 {
		t.Fatal("proposal scheduled extras")
	}
}

func TestOptionalRemindersLabelExactAmountAndOccurrence(t *testing.T) {
	w, f, _, a := optionalWorker(t)
	n, err := w.SendDueReminders(t.Context(), 50)
	if err != nil || n != 1 || f.marked != 1 {
		t.Fatalf("delivery: %d %v", n, err)
	}
	if !strings.Contains(f.messages[0], "Extra payment (optional)") || !strings.Contains(f.messages[0], money.FromMinor(a.AmountMinor, money.MustLookup("AMD")).String()) || strings.Contains(f.messages[0], "<b>Required payment") {
		t.Fatal("optional message mislabeled")
	}
	raw, _ := json.Marshal(f.markup)
	if !strings.Contains(string(raw), "id=optional-occurrence") || !strings.Contains(string(raw), "Optional extra payment") {
		t.Fatal("wrong resolution", string(raw))
	}
	if !strings.Contains(optionalReminderText(i18n.HY, f.optional[0], a), "ոչ պարտադիր") {
		t.Fatal("Armenian optional label missing")
	}
}

func TestOptionalRemindersSuppressStaleBeforeSendPreserveRequired(t *testing.T) {
	for _, reason := range []string{"sources", "deactivated", "tomorrow", "missing history", "engine", "wrong action", "read failure"} {
		t.Run(reason, func(t *testing.T) {
			w, f, h, _ := optionalWorker(t)
			f.due = []DueReminder{{ID: "required-occurrence", UserID: "owner", LoanID: "loan-a", DueDate: "2026-09-15", LoanName: "Required", Currency: "AMD"}}
			f.onRead = func() {
				switch reason {
				case "sources":
					h.sources = "changed"
				case "deactivated":
					h.active = false
				case "tomorrow":
					w.Clock.(*fixedClock).at = w.Clock.Now().Add(24 * time.Hour)
				case "missing history":
					w.History = nil
				case "engine":
					h.manifest.Engine = "unavailable"
				case "wrong action":
					f.optional[0].ActionIndex = -1
				case "read failure":
					f.optionalErr = errors.New("optional read failed")
				}
			}
			n, err := w.SendDueReminders(t.Context(), 50)
			if err != nil || n != 1 || len(f.messages) != 1 || !strings.Contains(f.messages[0], "Required payment") {
				t.Fatalf("required suppressed or stale optional delivered: %d %v %+v", n, err, f.messages)
			}
		})
	}
}

func TestOptionalReminderDeliveryFailureAndSnoozeRemainRetryable(t *testing.T) {
	w, f, _, _ := optionalWorker(t)
	f.failSend = true
	n, err := w.SendDueReminders(t.Context(), 50)
	if err != nil || n != 0 || f.marked != 0 || len(f.canceled) != 0 {
		t.Fatal("failed delivery consumed occurrence")
	}
	f.failSend = false
	f.current.TargetSendAt = w.Clock.Now().Add(time.Hour)
	if n, err = w.SendDueReminders(t.Context(), 50); err != nil || n != 0 {
		t.Fatal("snooze ignored")
	}
	f.current.TargetSendAt = w.Clock.Now()
	if n, err = w.SendDueReminders(t.Context(), 50); err != nil || n != 1 {
		t.Fatal("retry lost")
	}
}

func TestOptionalReminderGoldenText(t *testing.T) {
	// Exact bank fixture components from the allocation corpus; notification text
	// formats the approved cash outflow and never labels it as a paid fact.
	timeline, fact := planActualFixture(t)
	a := timeline.Payments[0]
	a.Kind = "extra"
	d := OptionalReminder{DueReminder: DueReminder{LoanName: "Fixture", Currency: "AMD"}}
	text := optionalReminderText(i18n.EN, d, a)
	if !strings.Contains(text, money.FromMinor(fact.AmountMinor, money.MustLookup("AMD")).String()) || !strings.Contains(text, date.MustNew(2026, 9, 24).String()) {
		t.Fatal("golden amount/date changed")
	}
}
