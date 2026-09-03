package app

import (
	"context"
	"fmt"
	"time"

	"github.com/andranikasd/marumbot/internal/i18n"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// OptionalReminder references one position in the full approved dated timeline.
// It is notification state, never evidence that a payment happened.
type OptionalReminder struct {
	DueReminder
	PlanID      string
	ActionIndex int
}

// OptionalReminderStore owns notification identity and delivery state only.
type OptionalReminderStore interface {
	ScheduleOptionalReminder(context.Context, string, string, string, int, string) error
	CancelObsoleteOptionalReminders(context.Context, string, []string) error
	CancelOptionalReminder(context.Context, string) error
	DueOptionalReminders(context.Context, time.Time, int32) ([]OptionalReminder, error)
}

func (w *Worker) scheduleOptionalReminders(ctx context.Context, user string) error {
	store, ok := w.Reminders.(OptionalReminderStore)
	if !ok || w.History == nil {
		return nil
	}
	plans, _, err := w.activePlans(ctx, user)
	if err != nil {
		return err
	}
	active := []string{}
	for _, p := range plans {
		if p.Active && !p.Outdated {
			active = append(active, p.ID)
		}
	}
	if err := store.CancelObsoleteOptionalReminders(ctx, user, active); err != nil {
		return err
	}
	today, err := (PaymentService{Clock: w.Clock, Users: w.Users}).BusinessDate(ctx, user)
	if err != nil {
		return err
	}
	horizon := date.AddDays(today, 14)
	for _, p := range plans {
		if !p.Active || p.Outdated {
			continue
		}
		timeline, err := w.PaymentTimeline(ctx, user, "", p.ID)
		if err != nil {
			return err
		}
		for index, action := range timeline.Payments {
			if action.Kind != "extra" || action.AmountMinor <= 0 || action.On < today.String() || action.On > horizon.String() {
				continue
			}
			if err := store.ScheduleOptionalReminder(ctx, user, p.ID, action.LoanID, index, action.On); err != nil {
				return err
			}
		}
	}
	return nil
}

// optionalReminderAction rechecks approval and source freshness for each send,
// not only when the batch was selected. A stale or past action is suppressed;
// unavailable history fails closed without inventing an amount.
func (w *Worker) optionalReminderAction(ctx context.Context, d OptionalReminder) (PlanPayment, bool, error) {
	if w.History == nil {
		return PlanPayment{}, false, nil
	}
	plans, _, err := w.activePlans(ctx, d.UserID)
	if err != nil {
		return PlanPayment{}, false, err
	}
	fresh := false
	for _, p := range plans {
		if p.ID == d.PlanID && p.Active && !p.Outdated {
			fresh = true
			break
		}
	}
	if !fresh {
		return PlanPayment{}, false, nil
	}
	timeline, err := w.PaymentTimeline(ctx, d.UserID, "", d.PlanID)
	if err != nil {
		return PlanPayment{}, false, err
	}
	if d.ActionIndex < 0 || d.ActionIndex >= len(timeline.Payments) {
		return PlanPayment{}, false, nil
	}
	action := timeline.Payments[d.ActionIndex]
	today, err := (PaymentService{Clock: w.Clock, Users: w.Users}).BusinessDate(ctx, d.UserID)
	if err != nil {
		return PlanPayment{}, false, err
	}
	if action.Kind != "extra" || action.AmountMinor <= 0 || action.LoanID != d.LoanID || action.On != d.DueDate || timeline.Currency != d.Currency || action.On < today.String() {
		return PlanPayment{}, false, nil
	}
	return action, true, nil
}

func optionalReminderText(locale i18n.Locale, d OptionalReminder, a PlanPayment) string {
	cur, err := money.Lookup(d.Currency)
	if err != nil {
		return ""
	}
	title, notice, fees := "Extra payment (optional)", "From your approved plan; this is not a required instalment.", "Fees included"
	if locale != i18n.EN {
		title = "Լրացուցիչ վճարում (ոչ պարտադիր)"
		notice = "Ձեր հաստատված պլանից է․ սա պարտադիր վճարում չէ։"
		fees = "Ներառված վճարներ"
	}
	return fmt.Sprintf("<b>%s</b>\n%s\n%s\n%s\n%s: %s", title, a.On,
		figures([][2]string{{clip(d.LoanName, 18), money.FromMinor(a.AmountMinor, cur).String()}}), notice, fees, money.FromMinor(a.FeeMinor, cur).String())
}
