package app

import (
	"context"
	"errors"
	"testing"
	"time"
)

var adminTestTime = time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)

type securityFake struct {
	ids        map[string]AdminIdentity
	policies   map[string]AdminPolicy
	audit      []AdminAudit
	published  []AdminPolicy
	auditError error
}
type securityTxFake struct {
	*securityFake
	parent *securityFake
}

func (f *securityFake) BeginAdmin(context.Context) (AdminSecurityTransaction, error) {
	copyIDs := map[string]AdminIdentity{}
	for k, v := range f.ids {
		copyIDs[k] = v
	}
	copyPolicies := map[string]AdminPolicy{}
	for k, v := range f.policies {
		copyPolicies[k] = v
	}
	return &securityTxFake{securityFake: &securityFake{ids: copyIDs, policies: copyPolicies, audit: append([]AdminAudit(nil), f.audit...), published: append([]AdminPolicy(nil), f.published...), auditError: f.auditError}, parent: f}, nil
}

func (f *securityFake) AdminIdentityByUsername(_ context.Context, name string) (AdminIdentity, error) {
	for _, v := range f.ids {
		if v.Username == name {
			return v, nil
		}
	}
	return AdminIdentity{}, ErrNotFound
}

func (f *securityFake) BootstrapAdmin(_ context.Context, id AdminIdentity) error {
	if len(f.ids) == 0 {
		f.ids[id.ID] = id
	}
	return nil
}
func (*securityFake) ConsumeAdminOTP(context.Context, string, int64, int64) error { return nil }
func (f *securityTxFake) Identity(_ context.Context, id string) (AdminIdentity, error) {
	v, ok := f.ids[id]
	if !ok {
		return v, ErrNotFound
	}
	return v, nil
}

func (f *securityTxFake) SaveIdentity(_ context.Context, id AdminIdentity, expected int64) error {
	if f.ids[id.ID].Version != expected {
		return ErrAdminConflict
	}
	f.ids[id.ID] = id
	return nil
}

func (f *securityTxFake) AppendAudit(_ context.Context, event AdminAudit) error {
	if f.auditError != nil {
		return f.auditError
	}
	f.audit = append(f.audit, event)
	return nil
}

func (f *securityTxFake) Policy(_ context.Context, id string) (AdminPolicy, error) {
	v, ok := f.policies[id]
	if !ok {
		return v, ErrNotFound
	}
	return v, nil
}

func (f *securityTxFake) SavePolicy(_ context.Context, p AdminPolicy, expected int64) error {
	if f.policies[p.ID].Revision != expected {
		return ErrAdminConflict
	}
	f.policies[p.ID] = p
	return nil
}

func (f *securityTxFake) PublishPolicy(_ context.Context, p AdminPolicy) error {
	f.published = append(f.published, p)
	return nil
}
func (f *securityTxFake) AuditEvents(context.Context) ([]AdminAudit, error) { return f.audit, nil }
func (f *securityTxFake) Commit(context.Context) error                      { *f.parent = *f.securityFake; return nil }
func (*securityTxFake) Rollback(context.Context) error                      { return nil }
func adminFixture() (*Admin, *securityFake) {
	f := &securityFake{ids: map[string]AdminIdentity{}, policies: map[string]AdminPolicy{}}
	for id, roles := range map[string][]AdminRole{"author": {AdminRoleFinancialVerifier, AdminRolePolicyPublisher}, "reviewer": {AdminRoleFinancialVerifier, AdminRolePolicyPublisher}, "admin": {AdminRoleAdministrator}, "support": {AdminRoleSupportReader}, "auditor": {AdminRoleSecurityAuditor}} {
		f.ids[id] = AdminIdentity{ID: id, Username: id, Roles: roles, Version: 1, Enabled: true, TOTPSecret: "enrolled"}
	}
	return NewAdmin(nil).WithSecurity(f, func() time.Time { return adminTestTime }).WithPolicySigner(func(hash string) (string, error) { return "test-signature:" + hash, nil }), f
}

func adminContext(id string) context.Context {
	return WithAdminSession(context.Background(), AdminSession{IdentityID: id, Version: 1, Strong: true, StepUpAt: adminTestTime, Purpose: "investigate support case"})
}

func TestAdminServiceDeniesBypasses(t *testing.T) {
	a, f := adminFixture()
	// Nil stores deliberately panic if a denied request reaches data access.
	for name, ctx := range map[string]context.Context{"anonymous": context.Background(), "administrator": adminContext("admin"), "redacted_support": adminContext("support"), "weak_auth": WithAdminSession(context.Background(), AdminSession{IdentityID: "author", Version: 1}), "stale_session": WithAdminSession(context.Background(), AdminSession{IdentityID: "author", Version: 2, Strong: true})} {
		t.Run(name, func(t *testing.T) {
			if _, err := a.Loans(ctx); err == nil {
				t.Fatal("read authorized")
			}
			if err := a.EraseUser(ctx, "user"); err == nil {
				t.Fatal("erase authorized")
			}
		})
	}
	id := f.ids["author"]
	id.Enabled = false
	f.ids["author"] = id
	if err := a.CheckAccess(adminContext("author"), AdminCapabilityFinancialRead, "loan"); !errors.Is(err, ErrAdminAccessDenied) {
		t.Fatal(err)
	}
}

func TestAdminReadRequiresDurablePurposeAudit(t *testing.T) {
	a, f := adminFixture()
	noPurpose := WithAdminSession(context.Background(), AdminSession{IdentityID: "author", Version: 1, Strong: true})
	if _, err := a.Loans(noPurpose); !errors.Is(err, ErrAdminPurposeRequired) {
		t.Fatal(err)
	}
	if len(f.audit) != 1 || f.audit[0].Outcome != "denied" {
		t.Fatal("denial not recorded")
	}
	f.auditError = errors.New("audit unavailable")
	if _, err := a.Loans(adminContext("author")); !errors.Is(err, f.auditError) {
		t.Fatal("read proceeded without audit", err)
	}
	f.auditError = nil
	if err := a.CheckAccess(adminContext("author"), AdminCapabilityFinancialRead, "loan"); err != nil {
		t.Fatal(err)
	}
	last := f.audit[len(f.audit)-1]
	if last.ActorID != "author" || last.Purpose != "investigate support case" || last.Target != "loan" {
		t.Fatal(last)
	}
}

func TestAdminPolicyIndependentPublication(t *testing.T) {
	a, f := adminFixture()
	p := AdminPolicy{ID: "policy", Key: "lender/profile", Version: 1, Definition: []byte(`{"order":["interest","principal"]}`), Excess: "reduce_principal", Source: "contract source", Evidence: "fixture revision and reviewed source"}
	if err := a.DraftPolicy(adminContext("author"), p, 0); err != nil {
		t.Fatal(err)
	}
	p = f.policies[p.ID]
	if len(f.published) != 0 {
		t.Fatal("draft became active")
	}
	if err := a.ReviewPolicy(adminContext("author"), p.ID, 1, p.ContentHash); !errors.Is(err, ErrAdminEvidenceRequired) {
		t.Fatal("self review", err)
	}
	if err := a.PublishPolicy(adminContext("reviewer"), p.ID, 1, p.ContentHash); !errors.Is(err, ErrAdminEvidenceRequired) {
		t.Fatal("publish without review", err)
	}
	if err := a.ReviewPolicy(adminContext("reviewer"), p.ID, 2, p.ContentHash); !errors.Is(err, ErrAdminConflict) {
		t.Fatal("stale revision", err)
	}
	if err := a.ReviewPolicy(adminContext("reviewer"), p.ID, 1, "wrong hash"); !errors.Is(err, ErrAdminConflict) {
		t.Fatal("wrong hash", err)
	}
	if err := a.ReviewPolicy(adminContext("reviewer"), p.ID, 1, p.ContentHash); err != nil {
		t.Fatal(err)
	}
	if err := a.PublishPolicy(adminContext("author"), p.ID, 2, p.ContentHash); !errors.Is(err, ErrAdminEvidenceRequired) {
		t.Fatal("self publish", err)
	}
	expired := WithAdminSession(context.Background(), AdminSession{IdentityID: "reviewer", Version: 1, Strong: true, StepUpAt: adminTestTime.Add(-6 * time.Minute)})
	if err := a.PublishPolicy(expired, p.ID, 2, p.ContentHash); !errors.Is(err, ErrAdminStepUpRequired) {
		t.Fatal("expired stepup", err)
	}
	if err := a.PublishPolicy(adminContext("reviewer"), p.ID, 2, p.ContentHash); err != nil {
		t.Fatal(err)
	}
	if len(f.published) != 1 || f.policies[p.ID].State != AdminPolicyPublished {
		t.Fatal("publication missing")
	}
	if err := a.PublishPolicy(adminContext("reviewer"), p.ID, 2, p.ContentHash); !errors.Is(err, ErrAdminConflict) {
		t.Fatal("duplicate publish", err)
	}
	if err := a.DraftPolicy(adminContext("author"), p, 3); !errors.Is(err, ErrAdminAccessDenied) {
		t.Fatal("published version editable", err)
	}
}

func TestAdminPolicyEditInvalidatesReview(t *testing.T) {
	a, f := adminFixture()
	p := AdminPolicy{ID: "p", Key: "profile", Version: 1, Definition: []byte(`{}`), Excess: "unknown", Source: "source", Evidence: "evidence"}
	if err := a.DraftPolicy(adminContext("author"), p, 0); err != nil {
		t.Fatal(err)
	}
	p = f.policies[p.ID]
	oldHash := p.ContentHash
	if err := a.ReviewPolicy(adminContext("reviewer"), p.ID, 1, p.ContentHash); err != nil {
		t.Fatal(err)
	}
	p.Evidence = "different evidence"
	if err := a.DraftPolicy(adminContext("author"), p, 2); err != nil {
		t.Fatal(err)
	}
	if f.policies[p.ID].Reviewer != "" || f.policies[p.ID].ContentHash == oldHash {
		t.Fatal("approval survives edit")
	}
	if err := a.PublishPolicy(adminContext("reviewer"), p.ID, 3, oldHash); !errors.Is(err, ErrAdminConflict) {
		t.Fatal(err)
	}
}
