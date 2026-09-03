package app

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrAdminSecurityUnavailable = errors.New("admin security is not configured")
	ErrAdminConflict            = errors.New("admin version conflict")
	ErrAdminEvidenceRequired    = errors.New("policy requires evidence and independent review")
)

type AdminIdentity struct {
	ID           string
	Username     string
	PasswordHash string `json:"-"`
	TOTPSecret   string `json:"-"`
	Roles        []AdminRole
	Version      int64
	Enabled      bool
}

type AdminAudit struct {
	ActorID string
	Action  string
	Target  string
	Purpose string
	Outcome string
	At      time.Time
}

// AdminSession is created only by the authentication adapter. Roles are always
// reloaded from storage; callers cannot grant themselves permissions in context.
type AdminSession struct {
	IdentityID string
	Version    int64
	Strong     bool
	StepUpAt   time.Time
	Purpose    string
}
type adminSessionKey struct{}

func WithAdminSession(ctx context.Context, session AdminSession) context.Context {
	return context.WithValue(ctx, adminSessionKey{}, session)
}

type AdminSecurityStore interface {
	AdminIdentityByUsername(context.Context, string) (AdminIdentity, error)
	BootstrapAdmin(context.Context, AdminIdentity) error
	BeginAdmin(context.Context) (AdminSecurityTransaction, error)
	ConsumeAdminOTP(context.Context, string, int64, int64) error
}

type AdminSecurityTransaction interface {
	Identity(context.Context, string) (AdminIdentity, error)
	SaveIdentity(context.Context, AdminIdentity, int64) error
	AppendAudit(context.Context, AdminAudit) error
	Policy(context.Context, string) (AdminPolicy, error)
	SavePolicy(context.Context, AdminPolicy, int64) error
	PublishPolicy(context.Context, AdminPolicy) error
	AuditEvents(context.Context) ([]AdminAudit, error)
	Commit(context.Context) error
	Rollback(context.Context) error
}

func (a *Admin) WithSecurity(s AdminSecurityStore, now func() time.Time) *Admin {
	a.security, a.now = s, now
	return a
}
func (a *Admin) SecurityReady() bool { return a.security != nil && a.now != nil }
func (a *Admin) BootstrapIdentity(ctx context.Context, id AdminIdentity) error {
	if !a.SecurityReady() {
		return ErrAdminSecurityUnavailable
	}
	id.Roles = []AdminRole{AdminRoleAdministrator}
	id.Version, id.Enabled = 1, true
	if id.ID == "" || id.Username == "" || id.PasswordHash == "" {
		return ErrAdminActorInvalid
	}
	return a.security.BootstrapAdmin(ctx, id)
}

func (a *Admin) LoginIdentity(ctx context.Context, username string) (AdminIdentity, error) {
	if !a.SecurityReady() {
		return AdminIdentity{}, ErrAdminSecurityUnavailable
	}
	return a.security.AdminIdentityByUsername(ctx, username)
}

func (a *Admin) authorizeTx(ctx context.Context, tx AdminSecurityTransaction, cap AdminCapability, target string) (AdminActor, error) {
	session, ok := ctx.Value(adminSessionKey{}).(AdminSession)
	if !ok || !session.Strong {
		return AdminActor{}, a.denyAdmin(ctx, tx, cap, target, "", ErrAdminAccessDenied)
	}
	id, err := tx.Identity(ctx, session.IdentityID)
	if err != nil {
		return AdminActor{}, a.denyAdmin(ctx, tx, cap, target, "", ErrAdminAccessDenied)
	}
	if !id.Enabled || id.Version != session.Version || id.TOTPSecret == "" {
		return AdminActor{}, a.denyAdmin(ctx, tx, cap, target, id.ID, ErrAdminAccessDenied)
	}
	actor := AdminActor{ID: id.ID, Roles: id.Roles, StepUpAt: session.StepUpAt, Purpose: session.Purpose}
	err = Authorize(actor, cap, a.now())
	outcome := "authorized_attempt"
	if err != nil {
		outcome = "denied"
	}
	// Do not record invalid/unbounded caller strings. The audit stores purpose,
	// never request bodies, credentials, or financial amounts.
	purpose := actor.Purpose
	if len(purpose) > 512 || !utf8.ValidString(purpose) {
		purpose = ""
	}
	auditErr := tx.AppendAudit(ctx, AdminAudit{ActorID: id.ID, Action: string(cap), Target: target, Purpose: purpose, Outcome: outcome, At: a.now()})
	if auditErr != nil {
		return AdminActor{}, auditErr
	}
	if err != nil {
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return AdminActor{}, commitErr
		}
	}
	return actor, err
}

// CheckAccess is also the seam for historical replay: call before loading any
// report, using SafeReplay and the report ID. Failure must abort the read.
func (a *Admin) CheckAccess(ctx context.Context, cap AdminCapability, target string) error {
	if !a.SecurityReady() {
		return ErrAdminSecurityUnavailable
	}
	tx, err := a.security.BeginAdmin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, accessErr := a.authorizeTx(ctx, tx, cap, target)
	if accessErr != nil {
		return accessErr
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return accessErr
}

func (a *Admin) ChangeIdentity(ctx context.Context, id AdminIdentity, expected int64) error {
	if !a.SecurityReady() {
		return ErrAdminSecurityUnavailable
	}
	if id.ID == "" || id.Username == "" || (id.PasswordHash == "" && expected == 0) || len(id.Roles) == 0 {
		return ErrAdminActorInvalid
	}
	for _, role := range id.Roles {
		switch role {
		case AdminRoleSupportReader, AdminRoleSupportOperator, AdminRoleFinancialVerifier, AdminRolePolicyPublisher, AdminRoleOperations, AdminRoleBillingOperator, AdminRoleSecurityAuditor, AdminRoleAdministrator:
		default:
			return ErrAdminActorInvalid
		}
	}
	tx, err := a.security.BeginAdmin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	actor, err := a.authorizeTx(ctx, tx, AdminCapabilityRoleChanges, id.ID)
	if err != nil {
		return err
	}
	// Protect the bootstrap administrator from accidental lockout. Another
	// administrator can change this identity after setup has been completed.
	if actor.ID == id.ID {
		return ErrAdminAccessDenied
	}
	if expected > 0 {
		old, err := tx.Identity(ctx, id.ID)
		if err != nil {
			return err
		}
		id.TOTPSecret = old.TOTPSecret
		if id.PasswordHash == "" {
			id.PasswordHash = old.PasswordHash
		}
	}
	id.Version = expected + 1
	if err := tx.SaveIdentity(ctx, id, expected); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// EnrollIdentity accepts only the password-authenticated enrollment session.
// It can set the first factor secret once, never reset an enrolled identity.
func (a *Admin) EnrollIdentity(ctx context.Context, secret string) error {
	if !a.SecurityReady() {
		return ErrAdminSecurityUnavailable
	}
	session, ok := ctx.Value(adminSessionKey{}).(AdminSession)
	if !ok || session.Strong || strings.TrimSpace(secret) == "" {
		return ErrAdminAccessDenied
	}
	tx, err := a.security.BeginAdmin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	id, err := tx.Identity(ctx, session.IdentityID)
	if err != nil {
		return err
	}
	if !id.Enabled || id.TOTPSecret != "" || id.Version != session.Version {
		return ErrAdminAccessDenied
	}
	id.TOTPSecret = secret
	id.Version++
	if err := tx.SaveIdentity(ctx, id, session.Version); err != nil {
		return err
	}
	if err := tx.AppendAudit(ctx, AdminAudit{ActorID: id.ID, Action: "identity.enroll", Target: id.ID, Outcome: "completed", At: a.now()}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (a *Admin) Audit(ctx context.Context) ([]AdminAudit, error) {
	if err := a.CheckAccess(ctx, AdminCapabilityAuditRead, "audit"); err != nil {
		return nil, err
	}
	tx, err := a.security.BeginAdmin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	return tx.AuditEvents(ctx)
}

func (a *Admin) ConsumeOTP(ctx context.Context, id string, version, counter int64) error {
	if !a.SecurityReady() {
		return ErrAdminSecurityUnavailable
	}
	return a.security.ConsumeAdminOTP(ctx, id, version, counter)
}

// Denials commit only their audit entry. No protected operation has run yet.
func (a *Admin) denyAdmin(ctx context.Context, tx AdminSecurityTransaction, cap AdminCapability, target, id string, cause error) error {
	if err := tx.AppendAudit(ctx, AdminAudit{ActorID: id, Action: string(cap), Target: target, Outcome: "denied", At: a.now()}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return cause
}

func (a *Admin) RecordAuthentication(ctx context.Context, id, outcome string) error {
	if !a.SecurityReady() {
		return ErrAdminSecurityUnavailable
	}
	tx, err := a.security.BeginAdmin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := tx.AppendAudit(ctx, AdminAudit{ActorID: id, Action: "identity.authentication", Outcome: outcome, At: a.now()}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
