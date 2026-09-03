package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/andranikasd/marumbot/internal/app"
)

type adminTransaction struct{ tx pgx.Tx }

func (s *Store) BeginAdmin(ctx context.Context) (app.AdminSecurityTransaction, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return &adminTransaction{tx: tx}, nil
}

func (s *Store) AdminIdentityByUsername(ctx context.Context, username string) (app.AdminIdentity, error) {
	return scanAdminIdentity(s.pool.QueryRow(ctx, q("AdminIdentityByUsername"), username))
}

func (s *Store) BootstrapAdmin(ctx context.Context, id app.AdminIdentity) error {
	var count int64
	return s.pool.QueryRow(ctx, q("BootstrapAdminIdentity"), id.ID, id.Username, id.PasswordHash).Scan(&count)
}

func scanAdminIdentity(row pgx.Row) (app.AdminIdentity, error) {
	var id app.AdminIdentity
	var roles []string
	err := row.Scan(&id.ID, &id.Username, &id.PasswordHash, &id.TOTPSecret, &roles, &id.Version, &id.Enabled)
	for _, role := range roles {
		id.Roles = append(id.Roles, app.AdminRole(role))
	}
	return id, adminStoreError(err)
}

func (t *adminTransaction) Identity(ctx context.Context, id string) (app.AdminIdentity, error) {
	return scanAdminIdentity(t.tx.QueryRow(ctx, q("AdminIdentity"), id))
}

func (t *adminTransaction) SaveIdentity(ctx context.Context, id app.AdminIdentity, expected int64) error {
	roles := make([]string, len(id.Roles))
	for i, r := range id.Roles {
		roles[i] = string(r)
	}
	args := []any{id.ID, id.Username, id.PasswordHash, id.TOTPSecret, roles, id.Version, id.Enabled}
	name := "CreateAdminIdentity"
	if expected > 0 {
		name = "UpdateAdminIdentity"
		args = append(args, expected)
	}
	tag, err := t.tx.Exec(ctx, q(name), args...)
	if err != nil {
		return adminStoreConflict(err)
	}
	if tag.RowsAffected() != 1 {
		return app.ErrAdminConflict
	}
	return nil
}

func (t *adminTransaction) AppendAudit(ctx context.Context, e app.AdminAudit) error {
	_, err := t.tx.Exec(ctx, q("AppendAdminAudit"), e.ActorID, e.Action, e.Target, e.Purpose, e.Outcome, e.At)
	return err
}

func (t *adminTransaction) Policy(ctx context.Context, id string) (app.AdminPolicy, error) {
	var body []byte
	var p app.AdminPolicy
	if err := t.tx.QueryRow(ctx, q("AdminPolicy"), id).Scan(&body); err != nil {
		return p, adminStoreError(err)
	}
	err := json.Unmarshal(body, &p)
	return p, err
}

func (t *adminTransaction) SavePolicy(ctx context.Context, p app.AdminPolicy, expected int64) error {
	body, err := json.Marshal(p)
	if err != nil {
		return err
	}
	args := []any{p.ID, body, p.Revision}
	name := "CreateAdminPolicy"
	if expected > 0 {
		name = "UpdateAdminPolicy"
		args = append(args, expected)
	}
	tag, err := t.tx.Exec(ctx, q(name), args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return app.ErrAdminConflict
	}
	return nil
}

func (t *adminTransaction) PublishPolicy(ctx context.Context, p app.AdminPolicy) error {
	_, err := t.tx.Exec(ctx, q("InsertPolicy"), p.ID, p.Key, p.Version, []byte(p.Definition), p.Excess, p.Source)
	return adminStoreConflict(err)
}

func (t *adminTransaction) AuditEvents(ctx context.Context) ([]app.AdminAudit, error) {
	rows, err := t.tx.Query(ctx, q("AdminAuditEvents"))
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[app.AdminAudit])
}
func (t *adminTransaction) Commit(ctx context.Context) error   { return t.tx.Commit(ctx) }
func (t *adminTransaction) Rollback(ctx context.Context) error { return t.tx.Rollback(ctx) }
func adminStoreError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrNotFound
	}
	return err
}

func (s *Store) ConsumeAdminOTP(ctx context.Context, id string, version, counter int64) error {
	tag, err := s.pool.Exec(ctx, q("ConsumeAdminOTP"), id, version, counter)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return app.ErrAdminAccessDenied
	}
	return nil
}

func (t *adminTransaction) Control(ctx context.Context, kind, id string) (app.AdminControl, error) {
	var c app.AdminControl
	err := t.tx.QueryRow(ctx, q("AdminControl"), kind, id).Scan(&c.Kind, &c.ID, &c.Revision, &c.Body)
	return c, adminStoreError(err)
}

func (t *adminTransaction) SaveControl(ctx context.Context, c app.AdminControl, expected int64) error {
	name := "CreateAdminControl"
	args := []any{c.Kind, c.ID, c.Revision, []byte(c.Body)}
	if expected > 0 {
		name = "UpdateAdminControl"
		args = append(args, expected)
	}
	tag, err := t.tx.Exec(ctx, q(name), args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return app.ErrAdminConflict
	}
	return nil
}

func (t *adminTransaction) CaseEvidence(ctx context.Context, c app.AdminCase) (bool, error) {
	// Lock first, then read evidence in a fresh statement snapshot. Concurrent
	// financial commands cannot invalidate evidence before this case commits.
	var loan string
	if err := t.tx.QueryRow(ctx, q("AdminCaseLoan"), c.LoanID, c.UserID).Scan(&loan); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if c.State == "open" {
		return true, nil
	}
	var verified bool
	err := t.tx.QueryRow(ctx, q("AdminCaseEvidence"), c.UserID, c.LoanID, c.Resolution, c.EvidenceID).Scan(&verified)
	return verified, err
}

func (s *Store) AdminProfileFlag(ctx context.Context, environment, profile string) (app.AdminFlag, error) {
	key, _ := json.Marshal([]string{environment, profile})
	var body []byte
	var flag app.AdminFlag
	if err := s.pool.QueryRow(ctx, q("AdminProfileFlag"), string(key)).Scan(&body); err != nil {
		return flag, adminStoreError(err)
	}
	err := json.Unmarshal(body, &flag)
	return flag, err
}

func adminStoreConflict(err error) error {
	var conflict *pgconn.PgError
	if errors.As(err, &conflict) && conflict.Code == "23505" {
		return app.ErrAdminConflict
	}
	return err
}

func (t *adminTransaction) Identities(ctx context.Context) ([]app.AdminIdentityView, error) {
	rows, err := t.tx.Query(ctx, q("AdminIdentityDirectory"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []app.AdminIdentityView
	for rows.Next() {
		var v app.AdminIdentityView
		var roles []string
		if err := rows.Scan(&v.ID, &v.Username, &roles, &v.Version, &v.Enabled, &v.Enrolled); err != nil {
			return nil, err
		}
		for _, role := range roles {
			v.Roles = append(v.Roles, app.AdminRole(role))
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

func (t *adminTransaction) Policies(ctx context.Context) ([]app.AdminPolicy, error) {
	rows, err := t.tx.Query(ctx, q("AdminPolicyDirectory"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []app.AdminPolicy
	for rows.Next() {
		var body []byte
		if err := rows.Scan(&body); err != nil {
			return nil, err
		}
		var p app.AdminPolicy
		if err := json.Unmarshal(body, &p); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func (t *adminTransaction) Controls(ctx context.Context, kind string) ([]app.AdminControl, error) {
	rows, err := t.tx.Query(ctx, q("AdminControlDirectory"), kind)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[app.AdminControl])
}

func (t *adminTransaction) CaseEvidenceOptions(ctx context.Context, user, loan string) ([]app.AdminEvidenceOption, error) {
	rows, err := t.tx.Query(ctx, q("AdminCaseEvidenceChoices"), user, loan)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[app.AdminEvidenceOption])
}
