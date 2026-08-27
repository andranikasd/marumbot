package app

import (
	"context"
	"time"
)

// Account is a borrower, identified only by an opaque uuid.
type Account struct {
	ID       string
	Locale   string
	Timezone string
	Created  bool // true when this call created the account
}

// UserStore resolves a Telegram identity to an account.
//
// The port takes tags and ciphertexts rather than raw Telegram ids, so the
// database adapter never sees an identifier it could store by mistake. Doing the
// encryption above this line is what makes that a compile-time property instead
// of a review comment.
type UserStore interface {
	UpsertByTelegram(ctx context.Context, in UpsertUser) (Account, error)
	Locale(ctx context.Context, userID string) (locale, timezone string, err error)
	SetLocale(ctx context.Context, userID, locale string) error
}

// UpsertUser carries an already-encrypted identity.
type UpsertUser struct {
	UserTag    string
	UserSealed []byte
	ChatTag    string
	ChatSealed []byte
	KeyVersion int
	NewID      string
	Locale     string
	Timezone   string
	TrialEnds  time.Time
}

// TrialPeriod is how long a new account can use Marum before it needs an
// entitlement. Long enough to see a full billing cycle of reminders, since a
// planner that has not yet reminded anyone of anything has not been evaluated.
const TrialPeriod = 45 * 24 * time.Hour
