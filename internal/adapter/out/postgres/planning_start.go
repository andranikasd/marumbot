package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/andranikasd/marumbot/internal/app"
)

func (t *budgetCommandTx) SetPlanningStart(ctx context.Context, user, currency string, expected int64, month string) (int64, error) {
	var version int64
	err := t.tx.QueryRow(ctx, q("SetPlanningStart"), user, currency, expected, month).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, app.ErrConflict
	}
	return version, err
}
