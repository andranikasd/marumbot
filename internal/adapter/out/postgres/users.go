package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/andranikasd/marumbot/internal/app"
)

// UpsertByTelegram finds the account behind a Telegram identity, creating one on
// first contact. The whole thing is one statement so two updates arriving at
// once cannot both decide they are the first.
func (s *Store) UpsertByTelegram(ctx context.Context, in app.UpsertUser) (app.Account, error) {
	var a app.Account
	err := s.pool.QueryRow(ctx, q("UpsertUserByTelegram"),
		in.UserTag, in.NewID, in.Locale, in.Timezone, in.TrialEnds,
		in.UserSealed, in.ChatTag, in.ChatSealed, in.KeyVersion,
	).Scan(&a.ID, &a.Created)
	if err != nil {
		return app.Account{}, err
	}
	a.Locale, a.Timezone = in.Locale, in.Timezone
	if !a.Created {
		// An existing account keeps its own preferences; the ones on the update
		// describe the user's phone, not their choice.
		if l, tz, err := s.Locale(ctx, a.ID); err == nil {
			a.Locale, a.Timezone = l, tz
		}
	}
	return a, nil
}

// Locale returns an account's language and timezone.
func (s *Store) Locale(ctx context.Context, userID string) (string, string, error) {
	var locale, tz string
	err := s.pool.QueryRow(ctx, q("GetUserLocale"), userID).Scan(&locale, &tz)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", errors.New("postgres: no such account")
	}
	return locale, tz, err
}

// SetLocale records a language the user chose.
func (s *Store) SetLocale(ctx context.Context, userID, locale string) error {
	var id string
	err := s.pool.QueryRow(ctx, q("SetUserLocale"), userID, locale).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return errors.New("postgres: no such account")
	}
	return err
}

// ChatCipher returns the encrypted chat id for an account, with the key version
// it was sealed under. Decryption is the caller's job: this package holds the
// database connection, not the key.
func (s *Store) ChatCipher(ctx context.Context, userID string) ([]byte, int, error) {
	var box []byte
	var ver int
	err := s.pool.QueryRow(ctx, q("GetChatCipher"), userID).Scan(&box, &ver)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, errors.New("postgres: no identity for account")
	}
	return box, ver, err
}

// ByTelegramTag finds the account behind a hashed Telegram identifier.
func (s *Store) ByTelegramTag(ctx context.Context, tag string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, q("GetUserByTelegramTag"), tag).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errors.New("postgres: no account for that identity")
	}
	return id, err
}

// SetState records what the bot is waiting for from this user.
func (s *Store) SetState(ctx context.Context, userID, state string) error {
	var got string
	return s.pool.QueryRow(ctx, q("SetConversationState"), userID, state).Scan(&got)
}

// State returns what the bot is waiting for, or empty when it is waiting for
// nothing. An expired state reads as empty rather than as an error.
func (s *Store) State(ctx context.Context, userID string) (string, error) {
	var name string
	err := s.pool.QueryRow(ctx, q("GetConversationState"), userID).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return name, err
}

// ClearState forgets what the bot was waiting for.
func (s *Store) ClearState(ctx context.Context, userID string) error {
	var got string
	err := s.pool.QueryRow(ctx, q("ClearConversationState"), userID).Scan(&got)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	return err
}

// MenuUsers returns a stable page of accounts whose Telegram menu may need a
// new deployment URL.
func (s *Store) MenuUsers(ctx context.Context, after string, limit int32) ([]app.MenuUser, error) {
	rows, err := s.pool.Query(ctx, q("MenuUsers"), after, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[app.MenuUser])
}
