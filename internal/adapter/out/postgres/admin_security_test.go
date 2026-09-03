package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/andranikasd/marumbot/internal/adapter/out/postgres"
	"github.com/andranikasd/marumbot/internal/app"
)

func TestAdminSecurityPolicyTransaction(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := clock.Now()
	a := app.NewAdmin(s).WithSecurity(s, func() time.Time { return now }).WithPolicySigner(func(hash string) (string, error) { return "test-signature:" + hash, nil })
	author, reviewer := uuid.NewString(), uuid.NewString()
	tx, err := s.BeginAdmin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, id := range []string{author, reviewer} {
		if err := tx.SaveIdentity(ctx, app.AdminIdentity{ID: id, Username: id, PasswordHash: "not-a-login-fixture", TOTPSecret: "enrolled", Roles: []app.AdminRole{app.AdminRoleFinancialVerifier, app.AdminRolePolicyPublisher}, Enabled: true, Version: 1}, 0); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	actor := func(id string) context.Context {
		return app.WithAdminSession(ctx, app.AdminSession{IdentityID: id, Version: 1, Strong: true, StepUpAt: now, Purpose: "test independent policy publication"})
	}
	p := app.AdminPolicy{ID: uuid.NewString(), Key: "admin-proof/" + uuid.NewString(), Version: 1, Definition: []byte(`{"order":["interest","principal"]}`), Excess: "reduce_principal", Source: "redacted fixture", Evidence: "verified fixture revision"}
	if err := a.DraftPolicy(actor(author), p, 0); err != nil {
		t.Fatal(err)
	}
	p, err = a.PolicyDraft(actor(author), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.ReviewPolicy(actor(reviewer), p.ID, 1, p.ContentHash); err != nil {
		t.Fatal(err)
	}
	if err := a.PublishPolicy(actor(author), p.ID, 2, p.ContentHash); !errors.Is(err, app.ErrAdminEvidenceRequired) {
		t.Fatal("self publish", err)
	}
	if err := a.PublishPolicy(actor(reviewer), p.ID, 2, p.ContentHash); err != nil {
		t.Fatal(err)
	}
	if err := a.PublishPolicy(actor(reviewer), p.ID, 2, p.ContentHash); !errors.Is(err, app.ErrAdminConflict) {
		t.Fatal("stale publication", err)
	}
	rows, err := s.ListPolicies(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, row := range rows {
		if row.ID == p.ID {
			found++
		}
	}
	if found != 1 {
		t.Fatalf("published rows: %d", found)
	}
	duplicate := p
	duplicate.ID = uuid.NewString()
	if err := a.DraftPolicy(actor(author), duplicate, 0); err != nil {
		t.Fatal(err)
	}
	duplicate, err = a.PolicyDraft(actor(author), duplicate.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.ReviewPolicy(actor(reviewer), duplicate.ID, 1, duplicate.ContentHash); err != nil {
		t.Fatal(err)
	}
	if err := a.PublishPolicy(actor(reviewer), duplicate.ID, 2, duplicate.ContentHash); !errors.Is(err, app.ErrAdminConflict) {
		t.Fatal("duplicate policy version", err)
	}
	duplicate, err = a.PolicyDraft(actor(author), duplicate.ID)
	if err != nil || duplicate.State != app.AdminPolicyReviewed || duplicate.Revision != 2 {
		t.Fatal("publication rollback", duplicate, err)
	}
	if err := s.ConsumeAdminOTP(ctx, author, 1, 100); err != nil {
		t.Fatal(err)
	}
	if err := s.ConsumeAdminOTP(ctx, author, 1, 100); !errors.Is(err, app.ErrAdminAccessDenied) {
		t.Fatal("OTP replay", err)
	}
	if err := s.ConsumeAdminOTP(ctx, author, 2, 101); !errors.Is(err, app.ErrAdminAccessDenied) {
		t.Fatal("stale session", err)
	}
}

func TestAdminControlsAndDeletionGuard(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := clock.Now()
	id := uuid.NewString()
	tx, err := s.BeginAdmin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := tx.SaveIdentity(ctx, app.AdminIdentity{ID: id, Username: id, PasswordHash: "not-a-login-fixture", TOTPSecret: "enrolled", Enabled: true, Version: 1, Roles: []app.AdminRole{app.AdminRoleAdministrator, app.AdminRoleSupportOperator, app.AdminRoleFinancialVerifier, app.AdminRoleSecurityAuditor}}, 0); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	a := app.NewAdmin(s).WithModeration(s).WithSecurity(s, func() time.Time { return now })
	actor := app.WithAdminSession(ctx, app.AdminSession{IdentityID: id, Version: 1, Strong: true, StepUpAt: now, Purpose: "test evidence and deletion guard"})
	flag := app.AdminFlag{Environment: "test", Profile: uuid.NewString(), PlanningEnabled: false, Reason: "profile verification failed"}
	if err := a.SetProfileFlag(actor, flag, 0); err != nil {
		t.Fatal(err)
	}
	if err := a.SetProfileFlag(actor, flag, 0); !errors.Is(err, app.ErrAdminConflict) {
		t.Fatal("stale flag update", err)
	}
	got, err := s.AdminProfileFlag(ctx, flag.Environment, flag.Profile)
	if err != nil || got.PlanningEnabled {
		t.Fatal(got, err)
	}
	if _, err := s.AdminProfileFlag(ctx, flag.Environment, "unrelated-profile"); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("profile isolation", err)
	}
	user := newUser(t, s)
	loan, err := s.CreateLoan(ctx, draft(user, t))
	if err != nil {
		t.Fatal(err)
	}
	c := app.AdminCase{ID: uuid.NewString(), UserID: user, LoanID: loan, Category: "bank_correction", Note: "private financial details", State: "open"}
	noPurpose := app.WithAdminSession(ctx, app.AdminSession{IdentityID: id, Version: 1, Strong: true, StepUpAt: now})
	if err := a.SaveCase(noPurpose, c, 0); !errors.Is(err, app.ErrAdminPurposeRequired) {
		t.Fatal("support write without purpose", err)
	}
	if _, _, err := a.SupportCase(actor, c.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("denied write persisted case", err)
	}
	audit, err := a.Audit(actor)
	if err != nil {
		t.Fatal(err)
	}
	foundDenial := false
	for _, event := range audit {
		if event.ActorID == id && event.Action == string(app.AdminCapabilitySupportNotes) && event.Outcome == "denied" {
			foundDenial = true
		}
	}
	if !foundDenial {
		t.Fatal("support-purpose denial was not durably audited")
	}
	mismatched := c
	mismatched.UserID = newUser(t, s)
	if err := a.SaveCase(actor, mismatched, 0); !errors.Is(err, app.ErrAdminEvidenceRequired) {
		t.Fatal("case accepted another user's loan", err)
	}
	if err := a.SaveCase(actor, c, 0); err != nil {
		t.Fatal(err)
	}
	redacted, revision, err := a.SupportCase(actor, c.ID)
	if err != nil || redacted.Note != "" || revision != 1 {
		t.Fatal("case redaction", redacted, revision, err)
	}

	identities, err := a.Identities(actor)
	if err != nil || len(identities) == 0 {
		t.Fatal("identity directory", err)
	}
	registry, err := a.PolicyRegistry(actor)
	if err != nil {
		t.Fatal("policy directory", err)
	}
	_ = registry
	flags, err := a.ProfileFlags(actor)
	if err != nil || len(flags) == 0 {
		t.Fatal("flag directory", err)
	}
	cases, err := a.Cases(actor)
	if err != nil || len(cases) == 0 {
		t.Fatal("case directory", err)
	}
	choices, err := a.CaseEvidenceChoices(actor, c)
	if err != nil || len(choices) != 0 {
		t.Fatal("opening-only snapshot offered as resolution", err)
	}
	c.State = "resolved"
	c.Resolution = "anchor"
	c.EvidenceID = uuid.NewString()
	if err := a.SaveCase(actor, c, 1); !errors.Is(err, app.ErrAdminEvidenceRequired) {
		t.Fatal("nonexistent evidence closed case", err)
	}
	snapshots, err := s.SnapshotsForLoan(ctx, loan)
	if err != nil || len(snapshots) == 0 {
		t.Fatal("fixture snapshot missing", err)
	}
	c.EvidenceID = snapshots[0].ID
	if err := a.SaveCase(actor, c, 1); !errors.Is(err, app.ErrAdminEvidenceRequired) {
		t.Fatal("opening snapshot closed case", err)
	}
	otherUser := newUser(t, s)
	otherLoan, err := s.CreateLoan(ctx, draft(otherUser, t))
	if err != nil {
		t.Fatal(err)
	}
	c.EvidenceID = adminReconciliationAnchor(t, s, otherUser, otherLoan)
	if err := a.SaveCase(actor, c, 1); !errors.Is(err, app.ErrAdminEvidenceRequired) {
		t.Fatal("foreign anchor closed case", err)
	}
	c.EvidenceID = adminReconciliationAnchor(t, s, user, loan)
	c.Resolution = "corrected_event"
	if err := a.SaveCase(actor, c, 1); !errors.Is(err, app.ErrAdminEvidenceRequired) {
		t.Fatal("misclassified anchor closed case", err)
	}
	c.Resolution = "anchor"
	if err := a.SaveCase(actor, c, 1); err != nil {
		t.Fatal(err)
	}
	if err := a.SaveCase(actor, c, 1); !errors.Is(err, app.ErrAdminConflict) {
		t.Fatal("stale case update", err)
	}
	// Exercise the actual borrower correction path, whose entries have no
	// legacy source_command_id: request hash and exact :void keys bind the pair.
	correctionLoan, err := s.CreateLoan(ctx, draft(user, t))
	if err != nil {
		t.Fatal(err)
	}
	paymentService := app.PaymentService{Store: s, Clock: clock}
	original, err := paymentService.Record(ctx, user, app.PaymentCommand{LoanID: correctionLoan, Key: uuid.NewString(), AmountMinor: 123, TransactionDate: "2026-08-02"})
	if err != nil {
		t.Fatal(err)
	}
	replacement, err := paymentService.Record(ctx, user, app.PaymentCommand{LoanID: correctionLoan, Key: uuid.NewString(), ExpectedVersion: original.Version, Replaces: original.ID, AmountMinor: 234, TransactionDate: "2026-08-02", ValueDate: "2026-08-03"})
	if err != nil {
		t.Fatal(err)
	}
	correctionCase := app.AdminCase{ID: uuid.NewString(), UserID: user, LoanID: correctionLoan, Category: "posting_date", Note: "Verify borrower correction", State: "open"}
	if err = a.SaveCase(actor, correctionCase, 0); err != nil {
		t.Fatal(err)
	}
	choices, err = a.CaseEvidenceChoices(actor, correctionCase)
	if err != nil {
		t.Fatal(err)
	}
	foundReplacement := false
	for _, option := range choices {
		if option.Kind == "corrected_event" && option.ID == replacement.ID {
			foundReplacement = true
		}
	}
	if !foundReplacement {
		t.Fatal("real borrower correction missing from evidence selector")
	}
	correctionCase.State = "resolved"
	correctionCase.Resolution = "corrected_event"
	correctionCase.EvidenceID = original.ID
	if err = a.SaveCase(actor, correctionCase, 1); !errors.Is(err, app.ErrAdminEvidenceRequired) {
		t.Fatal("original event treated as replacement", err)
	}
	correctionCase.EvidenceID = replacement.ID
	wrongLoanCase := correctionCase
	wrongLoanCase.ID = c.ID
	wrongLoanCase.LoanID = loan
	if err = a.SaveCase(actor, wrongLoanCase, 2); !errors.Is(err, app.ErrAdminEvidenceRequired) {
		t.Fatal("cross-loan replacement accepted", err)
	}
	if err = a.SaveCase(actor, correctionCase, 1); err != nil {
		t.Fatal("real borrower correction rejected", err)
	}
	if _, err = paymentService.Record(ctx, user, app.PaymentCommand{LoanID: correctionLoan, Key: uuid.NewString(), ExpectedVersion: replacement.Version, Replaces: replacement.ID, VoidOnly: true, TransactionDate: "2026-08-04"}); err != nil {
		t.Fatal(err)
	}
	if err = a.SaveCase(actor, correctionCase, 2); !errors.Is(err, app.ErrAdminEvidenceRequired) {
		t.Fatal("voided replacement closed case", err)
	}
	choices, err = a.CaseEvidenceChoices(actor, correctionCase)
	if err != nil {
		t.Fatal(err)
	}
	for _, option := range choices {
		if option.ID == replacement.ID {
			t.Fatal("voided replacement offered by selector")
		}
	}
	if err := a.EraseUser(actor, user); !errors.Is(err, app.ErrAdminAccessDenied) {
		t.Fatal("erasure without prior request", err)
	}
	if err := s.DeleteUser(ctx, user); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("SQL allowed erasure without prior request", err)
	}
}

func adminReconciliationAnchor(t *testing.T, s *postgres.Store, user, loan string) string {
	t.Helper()
	ctx := t.Context()
	service := app.PaymentService{Store: s, Clock: clock}
	today, err := service.BusinessDate(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetBudgetConfiguration(ctx, app.BudgetConfiguration{UserID: user, Currency: "AMD", MonthlyMinor: 10000, OpeningAsOf: today, Funding: &app.BudgetFunding{MonthlyMinor: 10000}}); err != nil {
		t.Fatal(err)
	}
	budget, err := s.Budget(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	payment, err := s.PaymentContext(ctx, loan, user)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := service.Reconcile(ctx, user, app.ReconciliationCommand{LoanID: loan, Key: uuid.NewString(), ExpectedVersion: payment.Version, BudgetVersion: budget.Version, AsOf: today.String(), PrincipalMinor: 0, CashMinor: 0, SpentMinor: 0, IncludePosted: true})
	if err != nil {
		t.Fatal(err)
	}
	return receipt.ID
}
