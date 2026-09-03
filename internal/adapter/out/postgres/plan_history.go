package postgres

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/andranikasd/marumbot/internal/app"
)

type activationTx struct{ tx pgx.Tx }

func (s *Store) PlanSources(ctx context.Context, user string) (string, error) {
	var sources string
	err := s.pool.QueryRow(ctx, q("PlanSources"), user).Scan(&sources)
	return sources, paymentError(err)
}

func (s *Store) BeginPlanActivation(ctx context.Context) (app.PlanActivationTransaction, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return &activationTx{tx: tx}, nil
}
func (t *activationTx) Commit(ctx context.Context) error   { return t.tx.Commit(ctx) }
func (t *activationTx) Rollback(ctx context.Context) error { return t.tx.Rollback(ctx) }
func (t *activationTx) LockPlanSources(ctx context.Context, user string) (string, error) {
	var id string
	if err := t.tx.QueryRow(ctx, q("LockPlanUser"), user).Scan(&id); err != nil {
		return "", paymentError(err)
	}
	for _, name := range []string{"LockPlanLoans", "LockPlanBudgets"} {
		rows, err := t.tx.Query(ctx, q(name), user)
		if err != nil {
			return "", err
		}
		for rows.Next() {
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return "", err
		}
	}
	var sources string
	err := t.tx.QueryRow(ctx, q("PlanSources"), user).Scan(&sources)
	return sources, paymentError(err)
}

func (t *activationTx) Receipt(ctx context.Context, user, key string) (app.PlanActivation, string, error) {
	var r app.PlanActivation
	var p string
	err := t.tx.QueryRow(ctx, q("PlanActivationReceipt"), user, key).Scan(&r.ID, &r.Revision, &p)
	return r, p, paymentError(err)
}

func (t *activationTx) Activate(ctx context.Context, user string, c app.PlanActivationCommand, m app.PlanManifest) (app.PlanActivation, error) {
	var r app.PlanActivation
	var revision int64
	if err := t.tx.QueryRow(ctx, q("PlanActivationRevision"), user).Scan(&revision); err != nil {
		return r, err
	}
	if revision != c.ExpectedRevision {
		return r, app.ErrConflict
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return r, err
	}
	var id string
	err = t.tx.QueryRow(ctx, q("InsertPlanVersion"), uuid.NewString(), user, m.Input.Cash.Monthly.Currency().Code, raw).Scan(&id)
	if err != nil {
		return r, err
	}
	err = t.tx.QueryRow(ctx, q("InsertPlanActivation"), uuid.NewString(), user, id, revision+1, c.Key, c.Identity()).Scan(&r.ID, &r.Revision)
	return r, err
}

func (s *Store) PlanHistory(ctx context.Context, user string) ([]app.PlanVersion, int64, error) {
	return s.readPlanVersions(ctx, user, "ListPlanVersions")
}

func (s *Store) ActivePlanVersions(ctx context.Context, user string) ([]app.PlanVersion, int64, error) {
	return s.readPlanVersions(ctx, user, "ActivePlanVersions")
}

func (s *Store) readPlanVersions(ctx context.Context, user, name string) ([]app.PlanVersion, int64, error) {
	rows, err := s.pool.Query(ctx, q(name), user)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []app.PlanVersion{}
	for rows.Next() {
		var r app.PlanVersion
		var raw string
		if err := rows.Scan(&r.ID, &r.Currency, &raw, &r.CreatedAt, &r.Active); err != nil {
			return nil, 0, err
		}
		if err = json.Unmarshal([]byte(raw), &r.Manifest); err != nil {
			return nil, 0, err
		}
		out = append(out, r)
	}
	if err = rows.Err(); err != nil {
		return nil, 0, err
	}
	rows.Close()
	var revision int64
	err = s.pool.QueryRow(ctx, q("PlanActivationRevision"), user).Scan(&revision)
	return out, revision, err
}

func (s *Store) PlanVersion(ctx context.Context, user, id string) (app.PlanVersion, error) {
	var r app.PlanVersion
	var raw string
	err := s.pool.QueryRow(ctx, q("GetPlanVersion"), user, id).Scan(&r.ID, &r.Currency, &raw, &r.CreatedAt)
	if err != nil {
		return r, paymentError(err)
	}
	err = json.Unmarshal([]byte(raw), &r.Manifest)
	return r, err
}
