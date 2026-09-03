package app

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/andranikasd/marumbot/pkg/core/plan"

	"github.com/andranikasd/marumbot/internal/i18n"
	"github.com/andranikasd/marumbot/pkg/core/amortisation"
	"github.com/andranikasd/marumbot/pkg/core/date"
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

// ReminderDateBatcher expands the same rules for multiple instalments at once.
type ReminderDateBatcher interface {
	ScheduleReminderDates(context.Context, string, []time.Time) error
}

// remindHorizon is how far ahead occurrences are generated. Two weeks covers
// the earliest reminder offset with room to spare, and keeps the table small
// enough that a scheduling bug is visible rather than buried.
const remindHorizon = 14 * 24 * time.Hour

// UserLister names the accounts the reminder tick walks.
type UserLister interface {
	ActiveLoanUsers(ctx context.Context, limit int32) ([]string, error)
}

// remindEvery is how often the tick generates occurrences. The scheduler
// calls every few minutes to keep the container warm; generating occurrences
// that often would be churn for rows that change daily.
const remindEvery = time.Hour

// TickReminders schedules upcoming occurrences and sends the due ones. It is
// called from the scheduler tick. Occurrence generation is hourly; already-due
// deliveries are drained on every tick, including after a send retry.
func (w *Worker) TickReminders(ctx context.Context, users UserLister) (int, error) {
	if w.Reminders == nil || users == nil {
		return 0, nil
	}
	if !w.reminding.CompareAndSwap(false, true) {
		return 0, nil // another tick won the walk
	}
	defer w.reminding.Store(false)
	now := w.Clock.Now()
	// Existing deliveries get a turn even when generation repeatedly times out.
	sent, deliveryErr := w.SendDueReminders(ctx, 50)
	last := w.lastRemind.Load()
	if last != 0 && now.Sub(time.Unix(0, last)) < remindEvery {
		return sent, deliveryErr
	}
	generationCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	generationErr := w.generateReminders(generationCtx, users, now)
	if generationErr == nil {
		w.lastRemind.Store(now.UnixNano())
	}
	return sent, errors.Join(deliveryErr, generationErr)
}

func (w *Worker) generateReminders(ctx context.Context, users UserLister, now time.Time) error {
	// Housekeeping rides the hourly walk: completed inbox rows only serve
	// update-id dedup, and Telegram's retries span minutes, so a week of
	// retention is generous. Optional -- a fake store simply skips it.
	if j, ok := w.Inbox.(InboxJanitor); ok {
		if n, err := j.PurgeCompletedBefore(ctx, now.Add(-inboxRetention)); err != nil {
			w.Log.WarnContext(ctx, "purging completed commands failed", "error", err)
		} else if n > 0 {
			w.Log.InfoContext(ctx, "purged completed commands", "rows", n)
		}
	}
	ids, err := users.ActiveLoanUsers(ctx, 500)
	if err != nil {
		return fmt.Errorf("listing accounts for reminders: %w", err)
	}
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := w.ScheduleForUser(ctx, id); err != nil {
			// One account's broken loan must not silence everyone else's
			// reminders; it is logged and the walk continues.
			w.Log.WarnContext(ctx, "scheduling reminders failed", "error", err)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
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
		s, err := l.Schedule()
		if err != nil || len(s.Rows) == 0 {
			w.Log.WarnContext(ctx, "cannot project a loan for reminders", "error", err)
			continue
		}
		// Only the instalments inside the horizon. Generating the whole
		// schedule would fill the table with rows nothing will read for years,
		// and every contract change would invalidate them.
		var dates []time.Time
		for _, row := range s.Rows {
			due := row.Due.AtLocal(0, 0, time.UTC)
			if due.After(horizon) {
				break
			}
			dates = append(dates, due)
		}
		if batcher, ok := w.Reminders.(ReminderDateBatcher); ok {
			if err := batcher.ScheduleReminderDates(ctx, l.ID, dates); err != nil {
				return fmt.Errorf("scheduling loan reminders: %w", err)
			}
		} else {
			for _, due := range dates {
				if err := w.Reminders.ScheduleReminders(ctx, due, l.ID); err != nil {
					return fmt.Errorf("scheduling loan reminder: %w", err)
				}
			}
		}
	}
	return w.scheduleOptionalReminders(ctx, userID)
}

// ReminderDeliveryStore applies the injected clock and quiet-window filtering
// before the limit, so quiet accounts cannot starve everyone behind them.
type ReminderDeliveryStore interface {
	ReadyReminders(context.Context, time.Time, int32) ([]DueReminder, error)
	MarkReminderDelivered(context.Context, string, time.Time) error
	ReminderOccurrence(context.Context, string, string) (ReminderOccurrence, error)
}

// SendDueReminders delivers what is owed and marks each one satisfied.
//
// Delivery is at-least-once and cannot be made exactly-once: the gap between
// Telegram accepting a message and Marum recording that it did is unclosable.
// The wording therefore has to read correctly if it arrives twice, which is why
// a reminder states the due date rather than saying "tomorrow".
func (w *Worker) SendDueReminders(ctx context.Context, limit int32) (int, error) {
	now := w.Clock.Now()
	var due []DueReminder
	var err error
	delivery, modern := w.Reminders.(ReminderDeliveryStore)
	if modern {
		due, err = delivery.ReadyReminders(ctx, now, limit)
	} else {
		due, err = w.Reminders.DueReminders(ctx, limit)
	}
	if err != nil {
		return 0, fmt.Errorf("reading due reminders: %w", err)
	}
	optional := map[string]OptionalReminder{}
	optionalStore, supportsOptional := w.Reminders.(OptionalReminderStore)
	if supportsOptional && modern && int32(len(due)) < limit {
		extra, readErr := optionalStore.DueOptionalReminders(ctx, now, limit-int32(len(due)))
		if readErr != nil {
			w.Log.WarnContext(ctx, "reading optional reminders failed", "error", readErr)
		} else {
			for _, item := range extra {
				optional[item.ID] = item
				due = append(due, item.DueReminder)
			}
		}
	}
	// One locale, one chat and one schedule build per user, not per reminder:
	// fifty due reminders used to mean hundreds of queries and repeated
	// projections of the same loans.
	books := map[string]*reminderBook{}
	sent := 0
	for _, d := range due {
		book, ok := books[d.UserID]
		if !ok {
			book = w.reminderBook(ctx, d.UserID)
			books[d.UserID] = book
		}
		if book == nil || book.preferences.QuietAt(w.Clock.Now()) {
			continue
		}

		// A snooze or cancellation may have happened while this batch was loading.
		// Recheck just before delivery; the guarded mark also preserves a snooze
		// arriving while Telegram is accepting the message.
		if modern {
			current, readErr := delivery.ReminderOccurrence(ctx, d.UserID, d.ID)
			if readErr != nil || current.Status != "scheduled" || current.TargetSendAt.After(w.Clock.Now()) {
				continue
			}
		}
		loc, _ := time.LoadLocation(book.preferences.Timezone)
		if loc == nil {
			loc = time.UTC
		}
		text := reminderText(book, d, date.From(w.Clock.Now(), loc))
		extra, isOptional := optional[d.ID]
		if isOptional {
			if book.chat <= 0 {
				continue
			}
			action, fresh, checkErr := w.optionalReminderAction(ctx, extra)
			if checkErr != nil {
				w.Log.WarnContext(ctx, "checking optional reminder failed", "error", checkErr)
				continue
			}
			if !fresh {
				if cancelErr := optionalStore.CancelOptionalReminder(ctx, d.ID); cancelErr != nil {
					w.Log.WarnContext(ctx, "canceling optional reminder failed", "error", cancelErr)
				}
				continue
			}
			text = optionalReminderText(book.locale, extra, action)
			if text == "" {
				continue
			}
		}

		// The deployed Mini App resolves this exact occurrence. Older bot-only
		// installations retain their payment callback.
		markup := w.mainMenu(book.locale)
		if w.Balances != nil && !isOptional {
			markup = paidMarkup(book.locale, d.LoanID)
		}
		if w.MiniApp != "" {
			label := "Required payment · review / snooze"
			if book.locale != i18n.EN {
				label = "Պարտադիր վճարում · դիտել / հետաձգել"
			}
			if isOptional {
				label = "Optional extra payment · review / snooze"
				if book.locale != i18n.EN {
					label = "Լրացուցիչ վճարում · դիտել / հետաձգել"
				}
			}
			markup = map[string]any{keyInline: [][]map[string]any{{webAppButton(label, w.miniURL("reminder")+"&id="+url.QueryEscape(d.ID))}}}
		}
		if err := w.Send.SendMessage(ctx, book.chat, text, markup); err != nil {
			// Left scheduled, so the next tick tries again. A reminder that
			// failed to send is not a reminder that is no longer owed.
			w.Log.WarnContext(ctx, "reminder failed to send", "error", err)
			continue
		}
		if modern {
			err = delivery.MarkReminderDelivered(ctx, d.ID, now)
		} else {
			err = w.Reminders.MarkReminderSatisfied(ctx, d.ID)
		}
		if err != nil {
			// Sent but not recorded. The next tick will send it again, which is
			// exactly why the wording has to survive arriving twice.
			w.Log.ErrorContext(ctx, "reminder sent but not recorded",
				"error", err)
		}
		sent++
	}
	return sent, nil
}

// reminderBook is everything one user's reminders read: locale, chat, and each
// live loan with its projected schedule, built once.
type reminderBook struct {
	preferences UserPreferences
	locale      i18n.Locale
	chat        int64
	loans       []scheduledLoan
	pending     map[string]bool
}

type scheduledLoan struct {
	loan  UserLoan
	sched amortisation.Schedule
}

// reminderBook loads one user's reminder context, or nil when the user cannot
// receive reminders at all; the cause is logged here, once.
func (w *Worker) reminderBook(ctx context.Context, userID string) *reminderBook {
	locale, timezone, err := w.Users.Locale(ctx, userID)
	if err != nil {
		w.Log.WarnContext(ctx, "reminder: unknown user", "error", err)
		return nil
	}
	chat, err := w.Chats.ChatID(ctx, userID)
	if err != nil {
		w.Log.WarnContext(ctx, "reminder: no chat", "error", err)
		return nil
	}
	prefs := UserPreferences{Timezone: timezone}
	if reader, ok := w.Users.(UserPreferenceReader); ok {
		prefs, err = reader.UserPreferences(ctx, userID)
		if err != nil {
			w.Log.WarnContext(ctx, "reminder: preferences unavailable", "error", err)
			return nil
		}
	}
	book := &reminderBook{preferences: prefs, locale: i18n.Locale(locale), chat: chat, pending: map[string]bool{}}
	loans, err := w.Loans.LoansForUser(ctx, userID, plan.MaxLoans+1)
	if err != nil {
		// Reminders still go out, without figures: a late reminder with a
		// number is worse than a timely one without.
		w.Log.WarnContext(ctx, "reminder: listing loans failed", "error", err)
		return book
	}
	for _, ln := range loans {
		if ln.UnreconciledPayments {
			book.pending[ln.ID] = true
			continue
		}
		if ln.Balance.Sign() <= 0 {
			continue
		}
		s, err := ln.Schedule()
		if err != nil || len(s.Rows) == 0 {
			continue
		}
		book.loans = append(book.loans, scheduledLoan{loan: ln, sched: s})
	}
	return book
}

// reminderText is the whole reminder: a titled date, then one aligned row
// per loan due that day. The amount is the instalment the schedule puts on
// that date; when it cannot be projected the reminder still goes out, with
// the loan named and the amount left blank.
//
// The date is humanised: "15 Sep" reads as a day, "2026-09-15" reads as an
// ID. The stored ISO form still keys the same-day lookup below. The title
// states the date rather than "today" or "tomorrow", so a reminder that
// arrives twice still reads correctly.
func reminderText(book *reminderBook, d DueReminder, today date.Date) string {
	l := book.locale
	when := shortDate(l, mustParseDate(d.DueDate), today)
	rows := [][2]string{}
	if amount, ok := instalmentOn(book, d); ok {
		rows = append(rows, [2]string{clip(d.LoanName, 18), amount})
	} else {
		rows = append(rows, [2]string{clip(d.LoanName, 18), "—"})
	}
	// The reminder knows the plan: other instalments on the same date are
	// named, so one message covers the day rather than four covering it in
	// pieces.
	for _, s := range book.loans {
		if s.loan.ID == d.LoanID {
			continue
		}
		if len(s.sched.Rows) > 0 && s.sched.Rows[0].Due.String() == d.DueDate {
			rows = append(rows, [2]string{clip(s.loan.Name, 18), s.sched.Rows[0].Payment.String()})
		}
	}
	required := "Required payment"
	if l != i18n.EN {
		required = "Պարտադիր վճարում"
	}
	text := "<b>" + required + "</b>\n<b>" + i18n.T(l, "reminder.title", when) + "</b>\n" + strings.TrimRight(figures(rows), "\n")
	if book.pending[d.LoanID] {
		text += "\n" + i18n.T(l, "payment.reconcile")
	}
	return text
}

// instalmentOn finds the scheduled payment falling on the reminder's date.
func instalmentOn(book *reminderBook, d DueReminder) (string, bool) {
	for _, s := range book.loans {
		if s.loan.ID != d.LoanID {
			continue
		}
		for _, r := range s.sched.Rows {
			if r.Due.String() == d.DueDate {
				return r.Payment.String(), true
			}
		}
	}
	return "", false
}
