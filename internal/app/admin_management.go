package app

import (
	"context"
	"encoding/json"
	"time"
)

// AdminIdentityView deliberately contains no password hash or authenticator secret.
type AdminIdentityView struct {
	ID       string
	Username string
	Roles    []AdminRole
	Version  int64
	Enabled  bool
	Enrolled bool
}
type AdminCaseView struct {
	Case     AdminCase
	Revision int64
}
type AdminFlagView struct {
	Flag     AdminFlag
	Revision int64
}
type AdminEvidenceOption struct {
	Kind  string
	ID    string
	Label string
}
type AdminManagementReader interface {
	Identities(context.Context) ([]AdminIdentityView, error)
	Policies(context.Context) ([]AdminPolicy, error)
	Controls(context.Context, string) ([]AdminControl, error)
	CaseEvidenceOptions(context.Context, string, string) ([]AdminEvidenceOption, error)
}

// GrantedCapabilities supports navigation only. Operations still call CheckAccess
// so a visible button never substitutes for purpose, step-up or current roles.
func (a *Admin) GrantedCapabilities(ctx context.Context) (map[AdminCapability]bool, error) {
	if !a.SecurityReady() {
		return nil, ErrAdminSecurityUnavailable
	}
	s, ok := ctx.Value(adminSessionKey{}).(AdminSession)
	if !ok || !s.Strong {
		return nil, ErrAdminAccessDenied
	}
	tx, err := a.security.BeginAdmin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	id, err := tx.Identity(ctx, s.IdentityID)
	if err != nil {
		return nil, err
	}
	if !id.Enabled || id.Version != s.Version || id.TOTPSecret == "" {
		return nil, ErrAdminAccessDenied
	}
	grants := map[AdminCapability]bool{}
	for _, role := range id.Roles {
		for _, cap := range AdminCapabilities {
			if adminRoleGrants(role, cap) {
				grants[cap] = true
			}
		}
	}
	return grants, nil
}

func (a *Admin) checkAnyAccess(ctx context.Context, target string, caps ...AdminCapability) error {
	grants, err := a.GrantedCapabilities(ctx)
	if err != nil {
		return err
	}
	for _, cap := range caps {
		if grants[cap] {
			return a.CheckAccess(ctx, cap, target)
		}
	}
	return a.CheckAccess(ctx, caps[0], target)
}

func (a *Admin) managementRead(ctx context.Context) (AdminSecurityTransaction, AdminManagementReader, error) {
	tx, err := a.security.BeginAdmin(ctx)
	if err != nil {
		return nil, nil, err
	}
	reader, ok := tx.(AdminManagementReader)
	if !ok {
		_ = tx.Rollback(ctx)
		return nil, nil, ErrAdminSecurityUnavailable
	}
	return tx, reader, nil
}

func (a *Admin) Identities(ctx context.Context) ([]AdminIdentityView, error) {
	if err := a.CheckAccess(ctx, AdminCapabilityIdentityRead, "identities"); err != nil {
		return nil, err
	}
	tx, r, err := a.managementRead(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	return r.Identities(ctx)
}

func (a *Admin) PolicyRegistry(ctx context.Context) ([]AdminPolicy, error) {
	if err := a.CheckAccess(ctx, AdminCapabilityPolicyRead, "policy_registry"); err != nil {
		return nil, err
	}
	tx, r, err := a.managementRead(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	return r.Policies(ctx)
}

func (a *Admin) Cases(ctx context.Context) ([]AdminCaseView, error) {
	if err := a.checkAnyAccess(ctx, "cases", AdminCapabilityFinancialRead, AdminCapabilitySupportRead); err != nil {
		return nil, err
	}
	grants, err := a.GrantedCapabilities(ctx)
	if err != nil {
		return nil, err
	}
	tx, r, err := a.managementRead(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := r.Controls(ctx, "case")
	if err != nil {
		return nil, err
	}
	result := make([]AdminCaseView, 0, len(rows))
	for _, row := range rows {
		var c AdminCase
		if err := json.Unmarshal(row.Body, &c); err != nil {
			return nil, err
		}
		if !grants[AdminCapabilityFinancialRead] {
			c.Note = ""
			c.EvidenceID = ""
		}
		result = append(result, AdminCaseView{Case: c, Revision: row.Revision})
	}
	return result, nil
}

func (a *Admin) CaseDetail(ctx context.Context, id string) (AdminCaseView, error) {
	if err := a.checkAnyAccess(ctx, id, AdminCapabilityFinancialRead, AdminCapabilitySupportRead); err != nil {
		return AdminCaseView{}, err
	}
	grants, err := a.GrantedCapabilities(ctx)
	if err != nil {
		return AdminCaseView{}, err
	}
	tx, err := a.security.BeginAdmin(ctx)
	if err != nil {
		return AdminCaseView{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	r, ok := tx.(AdminControlTransaction)
	if !ok {
		return AdminCaseView{}, ErrAdminSecurityUnavailable
	}
	row, err := r.Control(ctx, "case", id)
	if err != nil {
		return AdminCaseView{}, err
	}
	var c AdminCase
	if err := json.Unmarshal(row.Body, &c); err != nil {
		return AdminCaseView{}, err
	}
	if !grants[AdminCapabilityFinancialRead] {
		c.Note = ""
		c.EvidenceID = ""
	}
	return AdminCaseView{Case: c, Revision: row.Revision}, nil
}

func (a *Admin) CaseEvidenceChoices(ctx context.Context, c AdminCase) ([]AdminEvidenceOption, error) {
	if err := a.CheckAccess(ctx, AdminCapabilityReconciliation, c.ID); err != nil {
		return nil, err
	}
	tx, r, err := a.managementRead(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	return r.CaseEvidenceOptions(ctx, c.UserID, c.LoanID)
}

func (a *Admin) ProfileFlags(ctx context.Context) ([]AdminFlagView, error) {
	if err := a.CheckAccess(ctx, AdminCapabilityFeatureFlags, "profile_flags"); err != nil {
		return nil, err
	}
	tx, r, err := a.managementRead(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := r.Controls(ctx, "profile_flag")
	if err != nil {
		return nil, err
	}
	result := make([]AdminFlagView, 0, len(rows))
	for _, row := range rows {
		var flag AdminFlag
		if err := json.Unmarshal(row.Body, &flag); err != nil {
			return nil, err
		}
		result = append(result, AdminFlagView{Flag: flag, Revision: row.Revision})
	}
	return result, nil
}

type AdminEntitlement struct {
	ID                string
	AccessState       string
	TrialEndsAt       time.Time
	DeletionRequested bool
}

func (a *Admin) Entitlements(ctx context.Context) ([]AdminEntitlement, error) {
	if err := a.CheckAccess(ctx, AdminCapabilityEntitlements, "entitlements"); err != nil {
		return nil, err
	}
	rows, err := a.store.ListUsers(ctx, 200)
	if err != nil {
		return nil, err
	}
	result := make([]AdminEntitlement, 0, len(rows))
	for _, row := range rows {
		result = append(result, AdminEntitlement{ID: row.ID, AccessState: row.AccessState, TrialEndsAt: row.TrialEndsAt, DeletionRequested: row.DeletionRequested})
	}
	return result, nil
}
