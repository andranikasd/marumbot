package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/andranikasd/marumbot/internal/app"
)

// RecordShadow stores one silent recommendation. False without error means a
// row for this account, day and goal already exists -- the expected outcome
// on every walk after the day's first.
func (s *Store) RecordShadow(ctx context.Context, r app.ShadowRecommendation) (bool, error) {
	var id string
	err := s.pool.QueryRow(ctx, q("RecordShadowRecommendation"),
		uuid.NewString(), r.UserID, r.ComputedOn, r.Goal, r.Engine, r.Fingerprint, r.Sheet,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// ShadowDays reports how much shadow evidence an account has: distinct days
// and the span they cover. The field gates read this.
func (s *Store) ShadowDays(ctx context.Context, userID string) (int64, string, string, error) {
	var n int64
	var first, last string
	err := s.pool.QueryRow(ctx, q("ShadowRecommendationDays"), userID).Scan(&n, &first, &last)
	return n, first, last, err
}
