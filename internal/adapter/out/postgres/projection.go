package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/pkg/core/projection"
)

func (s *Store) ProjectionSettings(ctx context.Context, user string) (app.ProjectionSettings, error) {
	out := app.ProjectionSettings{Currencies: map[string]projection.Extra{}}
	var raw []byte
	err := s.pool.QueryRow(ctx, q("ProjectionSettings"), user).Scan(&out.Version, &out.Enabled, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	err = json.Unmarshal(raw, &out.Currencies)
	return out, err
}

func (t *budgetCommandTx) SaveProjectionSettings(ctx context.Context, user string, in app.ProjectionUpdate) (int64, error) {
	raw, err := json.Marshal(in.Currencies)
	if err != nil {
		return 0, err
	}
	if in.Currencies == nil {
		raw = []byte("{}")
	}
	var version int64
	err = t.tx.QueryRow(ctx, q("SaveProjectionSettings"), user, in.ExpectedVersion, in.Enabled, raw).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, app.ErrConflict
	}
	return version, err
}
