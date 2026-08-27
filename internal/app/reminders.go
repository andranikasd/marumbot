package app

import (
	"context"
	"fmt"
	"time"

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

// ScheduleForUser generates occurrences for one borrower's loans.
func (w *Worker) ScheduleForUser(ctx context.Context, userID string) error {
	loans, err := w.Loans.LoansForUser(ctx, userID, 50)
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

		key := "reminder.due_today"
		if d.OffsetDays < 0 {
			key = "reminder.due_soon"
		}
		text := i18n.T(l, key, d.LoanName, d.DueDate)

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
