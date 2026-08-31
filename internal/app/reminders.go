package app

import (
	"context"
	"fmt"
	"html"
	"time"

	"github.com/andranikasd/marumbot/pkg/core/plan"

	"github.com/andranikasd/marumbot/internal/i18n"
	"github.com/andranikasd/marumbot/pkg/core/amortisation"
)

// DueReminder is one reminder whose moment has arrived.
type DueReminder struct {
	ID         string
	UserID     string
	LoanID     string
	DueDate    string
	OffsetDays int
	LoanName   string
	Currency   string
}

// ReminderStore schedules and delivers reminders.
type ReminderStore interface {
	EnsureDefaultReminders(ctx context.Context, loanID string) error
	ScheduleReminders(ctx context.Context, due time.Time, loanID string) error
	DueReminders(ctx context.Context, limit int32) ([]DueReminder, error)
	MarkReminderSatisfied(ctx context.Context, id string) error
	CancelRemindersForLoan(ctx context.Context, loanID string) error
}

// remindHorizon is how far ahead occurrences are generated. Two weeks covers
// the earliest reminder offset with room to spare, and keeps the table small
// enough that a scheduling bug is visible rather than buried.
const remindHorizon = 14 * 24 * time.Hour

// UserLister names the accounts the reminder tick walks.
type UserLister interface {
	ActiveLoanUsers(ctx context.Context, limit int32) ([]string, error)
}

// remindEvery is how often the tick actually does the work. The scheduler
// calls every few minutes to keep the container warm; generating occurrences
// that often would be churn for rows that change daily.
const remindEvery = time.Hour

// TickReminders schedules upcoming occurrences and sends the due ones. It is
// called from the scheduler tick and rate-limits itself with the injected
// clock, so calling it every minute costs one comparison.
func (w *Worker) TickReminders(ctx context.Context, users UserLister) (int, error) {
	if w.Reminders == nil || users == nil {
		return 0, nil
	}
	now := w.Clock.Now()
	if !w.lastRemind.IsZero() && now.Sub(w.lastRemind) < remindEvery {
		return 0, nil
	}
	w.lastRemind = now
	ids, err := users.ActiveLoanUsers(ctx, 500)
	if err != nil {
		return 0, fmt.Errorf("listing accounts for reminders: %w", err)
	}
	for _, id := range ids {
		if err := w.ScheduleForUser(ctx, id); err != nil {
			// One account's broken loan must not silence everyone else's
			// reminders; it is logged and the walk continues.
			w.Log.WarnContext(ctx, "scheduling reminders failed", "user", id, "error", err)
		}
	}
	return w.SendDueReminders(ctx, 50)
}

// OnLoanFiled sets up reminders the moment a loan exists: the default rules,
// and the occurrences inside the horizon. Called by the Mini App after a
// successful create, so the first reminder does not wait for the next tick.
func (w *Worker) OnLoanFiled(ctx context.Context, userID, loanID string) error {
	if w.Reminders == nil {
		return nil
	}
	if err := w.Reminders.EnsureDefaultReminders(ctx, loanID); err != nil {
		return fmt.Errorf("default reminders: %w", err)
	}
	if err := w.ScheduleForUser(ctx, userID); err != nil {
		return err
	}
	// The conversation confirms what the form did. Best-effort: the loan
	// exists whether or not the message lands.
	w.OnLoanFiledMessage(ctx, userID)
	return nil
}

// ScheduleForUser generates occurrences for one borrower's loans.
func (w *Worker) ScheduleForUser(ctx context.Context, userID string) error {
	loans, err := w.Loans.LoansForUser(ctx, userID, plan.MaxLoans+1)
	if err != nil {
		return fmt.Errorf("listing loans: %w", err)
	}
	horizon := w.Clock.Now().Add(remindHorizon)

	for _, l := range loans {
		if l.Balance.Sign() <= 0 {
			continue
		}
		s, err := amortisation.Build(l.Contract, l.Balance, l.AsOf)
		if err != nil || len(s.Rows) == 0 {
			w.Log.WarnContext(ctx, "cannot project a loan for reminders",
				"loan", l.ID, "error", err)
			continue
		}
		// Only the instalments inside the horizon. Generating the whole
		// schedule would fill the table with rows nothing will read for years,
		// and every contract change would invalidate them.
		for _, row := range s.Rows {
			due := row.Due.AtLocal(0, 0, time.UTC)
			if due.After(horizon) {
				break
			}
			if err := w.Reminders.ScheduleReminders(ctx, due, l.ID); err != nil {
				return fmt.Errorf("scheduling %s: %w", l.ID, err)
			}
		}
	}
	return nil
}

// SendDueReminders delivers what is owed and marks each one satisfied.
//
// Delivery is at-least-once and cannot be made exactly-once: the gap between
// Telegram accepting a message and Marum recording that it did is unclosable.
// The wording therefore has to read correctly if it arrives twice, which is why
// a reminder states the due date rather than saying "tomorrow".
func (w *Worker) SendDueReminders(ctx context.Context, limit int32) (int, error) {
	due, err := w.Reminders.DueReminders(ctx, limit)
	if err != nil {
		return 0, fmt.Errorf("reading due reminders: %w", err)
	}
	sent := 0
	for _, d := range due {
		locale, _, err := w.Users.Locale(ctx, d.UserID)
		if err != nil {
			w.Log.WarnContext(ctx, "reminder: unknown user", "user", d.UserID, "error", err)
			continue
		}
		chat, err := w.Chats.ChatID(ctx, d.UserID)
		if err != nil {
			w.Log.WarnContext(ctx, "reminder: no chat", "user", d.UserID, "error", err)
			continue
		}
		l := i18n.Locale(locale)

		text := w.reminderText(ctx, l, d)

		if err := w.Send.SendMessage(ctx, chat, text, w.mainMenu(l)); err != nil {
			// Left scheduled, so the next tick tries again. A reminder that
			// failed to send is not a reminder that is no longer owed.
			w.Log.WarnContext(ctx, "reminder failed to send", "occurrence", d.ID, "error", err)
			continue
		}
		if err := w.Reminders.MarkReminderSatisfied(ctx, d.ID); err != nil {
			// Sent but not recorded. The next tick will send it again, which is
			// exactly why the wording has to survive arriving twice.
			w.Log.ErrorContext(ctx, "reminder sent but not recorded",
				"occurrence", d.ID, "error", err)
		}
		sent++
	}
	return sent, nil
}

// reminderText is the whole reminder: date, amount, loan. The amount is the
// instalment the schedule puts on that date; when it cannot be projected the
// reminder still goes out, without a figure, because a late reminder with a
// number is worse than a timely one without.
func (w *Worker) reminderText(ctx context.Context, l i18n.Locale, d DueReminder) string {
	name := html.EscapeString(d.LoanName)
	var text string
	if amount, ok := w.instalmentOn(ctx, d); ok {
		if d.OffsetDays < 0 {
			text = i18n.T(l, "reminder.due_soon", d.DueDate, amount, name)
		} else {
			text = i18n.T(l, "reminder.due_today", d.DueDate, amount, name)
		}
	} else if d.OffsetDays < 0 {
		text = i18n.T(l, "reminder.due_soon_plain", d.DueDate, name)
	} else {
		text = i18n.T(l, "reminder.due_today_plain", d.DueDate, name)
	}
	// The reminder knows the plan: other instalments on the same date are
	// named, so one message covers the day rather than four covering it in
	// pieces.
	for _, also := range w.alsoDue(ctx, d) {
		text += "\n" + also
	}
	return text
}

// alsoDue lists the other loans with an instalment on the reminder's date.
func (w *Worker) alsoDue(ctx context.Context, d DueReminder) []string {
	loans, err := w.Loans.LoansForUser(ctx, d.UserID, plan.MaxLoans+1)
	if err != nil {
		return nil
	}
	locale, _, err := w.Users.Locale(ctx, d.UserID)
	if err != nil {
		return nil
	}
	l := i18n.Locale(locale)
	var out []string
	for _, ln := range loans {
		if ln.ID == d.LoanID || ln.Balance.Sign() <= 0 {
			continue
		}
		s, err := amortisation.Build(ln.Contract, ln.Balance, ln.AsOf)
		if err != nil || len(s.Rows) == 0 || s.Rows[0].Due.String() != d.DueDate {
			continue
		}
		out = append(out, i18n.T(l, "reminder.also", s.Rows[0].Payment.String(), html.EscapeString(ln.Name)))
	}
	return out
}

// instalmentOn finds the scheduled payment falling on the reminder's date.
func (w *Worker) instalmentOn(ctx context.Context, d DueReminder) (string, bool) {
	if w.Editor == nil {
		return "", false
	}
	ln, err := w.Editor.LoanForUser(ctx, d.LoanID, d.UserID)
	if err != nil {
		return "", false
	}
	s, err := amortisation.Build(ln.Contract, ln.Balance, ln.AsOf)
	if err != nil {
		return "", false
	}
	for _, r := range s.Rows {
		if r.Due.String() == d.DueDate {
			return r.Payment.String(), true
		}
	}
	return "", false
}
