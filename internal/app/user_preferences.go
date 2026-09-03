package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	_ "time/tzdata" // IANA validation also works in minimal production images.
)

var ErrInvalidPreferences = errors.New("invalid preferences")

// UserPreferences describes delivery timing; required reminders cannot be disabled.
// Quiet boundaries are local minutes since midnight, inclusive start/exclusive end.
type UserPreferences struct {
	Timezone     string `json:"timezone"`
	QuietEnabled bool   `json:"quiet_enabled"`
	QuietStart   int    `json:"quiet_start"`
	QuietEnd     int    `json:"quiet_end"`
	Version      int64  `json:"version"`
}

func (p UserPreferences) Validate() error {
	if p.Timezone == "" || p.Timezone == "Local" || strings.TrimSpace(p.Timezone) != p.Timezone {
		return ErrInvalidPreferences
	}
	if _, err := time.LoadLocation(p.Timezone); err != nil {
		return fmt.Errorf("%w: timezone", ErrInvalidPreferences)
	}
	if p.QuietStart < 0 || p.QuietStart >= 1440 || p.QuietEnd < 0 || p.QuietEnd >= 1440 || p.Version < 0 || p.QuietEnabled && p.QuietStart == p.QuietEnd {
		return fmt.Errorf("%w: quiet window", ErrInvalidPreferences)
	}
	return nil
}

func (p UserPreferences) QuietAt(now time.Time) bool {
	if !p.QuietEnabled {
		return false
	}
	loc, err := time.LoadLocation(p.Timezone)
	if err != nil {
		return true
	}
	local := now.In(loc)
	minute := local.Hour()*60 + local.Minute()
	if p.QuietStart < p.QuietEnd {
		return minute >= p.QuietStart && minute < p.QuietEnd
	}
	return minute >= p.QuietStart || minute < p.QuietEnd
}

type PreferenceCommand struct {
	UserPreferences
	Key string `json:"idempotency_key"`
}

type ReminderOccurrence struct {
	ID           string    `json:"id"`
	LoanID       string    `json:"loan_id"`
	DueDate      string    `json:"due_date"`
	TargetSendAt time.Time `json:"target_send_at"`
	Status       string    `json:"status"`
	Version      int64     `json:"version"`
	Required     bool      `json:"required"`
}
type SnoozeCommand struct {
	OccurrenceID    string    `json:"-"`
	Until           time.Time `json:"until"`
	ExpectedVersion int64     `json:"expected_version"`
	Key             string    `json:"idempotency_key"`
}

type UserPreferenceReader interface {
	UserPreferences(context.Context, string) (UserPreferences, error)
}
type UserPreferenceStore interface {
	UserPreferenceReader
	BeginPreferences(context.Context) (PreferenceTransaction, error)
	ReminderOccurrence(context.Context, string, string) (ReminderOccurrence, error)
}

// PreferenceTransaction serializes commands on the account, including receipts.
// The application opens and closes it; no network delivery runs inside it.
type PreferenceTransaction interface {
	LockUser(context.Context, string) error
	Receipt(context.Context, string, string) (string, []byte, error)
	SaveReceipt(context.Context, string, string, string, []byte) error
	SetPreferences(context.Context, string, UserPreferences) (UserPreferences, error)
	Snooze(context.Context, string, SnoozeCommand) (ReminderOccurrence, error)
	Commit(context.Context) error
	Rollback(context.Context) error
}
type PreferenceService struct {
	Store UserPreferenceStore
	Clock Clock
}

func (s PreferenceService) Save(ctx context.Context, user string, c PreferenceCommand) (UserPreferences, error) {
	var out UserPreferences
	if err := c.Validate(); err != nil {
		return out, err
	}
	payload, _ := json.Marshal(struct {
		Kind  string
		Value PreferenceCommand
	}{"preferences", c})
	err := s.command(ctx, user, c.Key, string(payload), &out, func(tx PreferenceTransaction) (any, error) { return tx.SetPreferences(ctx, user, c.UserPreferences) })
	return out, err
}

func (s PreferenceService) Snooze(ctx context.Context, user string, c SnoozeCommand) (ReminderOccurrence, error) {
	var out ReminderOccurrence
	// Normalize equal instants before computing command identity.
	c.Until = c.Until.UTC()
	payload, _ := json.Marshal(struct {
		Kind, Occurrence string
		Value            SnoozeCommand
	}{"snooze", c.OccurrenceID, c})
	err := s.command(ctx, user, c.Key, string(payload), &out, func(tx PreferenceTransaction) (any, error) {
		now := s.Clock.Now()
		if c.OccurrenceID == "" || c.ExpectedVersion < 0 || !c.Until.After(now) || c.Until.After(now.Add(7*24*time.Hour)) {
			return nil, ErrInvalidPreferences
		}
		return tx.Snooze(ctx, user, c)
	})
	return out, err
}

func (s PreferenceService) command(ctx context.Context, user, key, payload string, out any, apply func(PreferenceTransaction) (any, error)) error {
	if len(key) < 8 || len(key) > 128 || strings.TrimSpace(key) != key {
		return ErrInvalidPreferences
	}
	tx, err := s.Store.BeginPreferences(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }() // rollback is harmless after commit
	if err = tx.LockUser(ctx, user); err != nil {
		return err
	}
	old, raw, err := tx.Receipt(ctx, user, key)
	if err == nil {
		if old != payload {
			return ErrConflict
		}
		return json.Unmarshal(raw, out)
	}
	if !errors.Is(err, ErrNotFound) {
		return err
	}
	result, err := apply(tx)
	if err != nil {
		return err
	}
	raw, err = json.Marshal(result)
	if err != nil {
		return err
	}
	if err = tx.SaveReceipt(ctx, user, key, payload, raw); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}
