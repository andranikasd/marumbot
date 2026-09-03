package app

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestAuthorizeRoleMatrix(t *testing.T) {
	capabilities := []AdminCapability{
		AdminCapabilityIdentityRead, AdminCapabilityPolicyRead,
		AdminCapabilitySupportRead, AdminCapabilitySupportNotes, AdminCapabilityUserConfirmation,
		AdminCapabilitySafeReplay, AdminCapabilityFinancialRead, AdminCapabilityFixtureReview,
		AdminCapabilityReconciliation, AdminCapabilityPolicyReview, AdminCapabilityPolicyPublish,
		AdminCapabilityNotifications, AdminCapabilityHealth, AdminCapabilityEntitlements,
		AdminCapabilityAuditRead, AdminCapabilityAccessRead, AdminCapabilityExportVerification,
		AdminCapabilityDeletionVerification, AdminCapabilityRoleChanges, AdminCapabilityFeatureFlags,
		"", "unknown", "financial.write", "security.export", "security.delete",
	}
	cases := []struct {
		role    AdminRole
		allowed []AdminCapability
	}{
		{AdminRoleSupportReader, []AdminCapability{AdminCapabilitySupportRead}},
		{AdminRoleSupportOperator, []AdminCapability{AdminCapabilitySupportRead, AdminCapabilitySupportNotes, AdminCapabilityUserConfirmation, AdminCapabilitySafeReplay}},
		{AdminRoleFinancialVerifier, []AdminCapability{AdminCapabilityPolicyRead, AdminCapabilityFinancialRead, AdminCapabilityFixtureReview, AdminCapabilityReconciliation, AdminCapabilityPolicyReview}},
		{AdminRolePolicyPublisher, []AdminCapability{AdminCapabilityPolicyRead, AdminCapabilityPolicyPublish}},
		{AdminRoleOperations, []AdminCapability{AdminCapabilityNotifications, AdminCapabilityHealth}},
		{AdminRoleBillingOperator, []AdminCapability{AdminCapabilityEntitlements}},
		{AdminRoleSecurityAuditor, []AdminCapability{AdminCapabilityIdentityRead, AdminCapabilityAuditRead, AdminCapabilityAccessRead, AdminCapabilityExportVerification, AdminCapabilityDeletionVerification}},
		{AdminRoleAdministrator, []AdminCapability{AdminCapabilityIdentityRead, AdminCapabilityRoleChanges, AdminCapabilityFeatureFlags}},
		{"", nil},
		{"unknown", nil},
	}
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	for _, tc := range cases {
		t.Run(string(tc.role), func(t *testing.T) {
			for _, capability := range capabilities {
				t.Run(string(capability), func(t *testing.T) {
					actor := AdminActor{ID: "staff-1", Roles: []AdminRole{tc.role}, Purpose: "review support case", StepUpAt: now}
					err := Authorize(actor, capability, now)
					if slices.Contains(tc.allowed, capability) {
						if err != nil {
							t.Fatalf("allowed capability: %v", err)
						}
					} else if !errors.Is(err, ErrAdminAccessDenied) {
						t.Fatalf("want denial, got %v", err)
					}
				})
			}
		})
	}
}

func TestAuthorizeActorAndRoleUnion(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	for _, id := range []string{"", " ", " staff", "staff ", "staff member", "staff\n", "staff\x00", "\xff", "staff\u2003name"} {
		t.Run(id, func(t *testing.T) {
			err := Authorize(AdminActor{ID: id, Roles: []AdminRole{AdminRoleSupportReader}}, AdminCapabilitySupportRead, now)
			if !errors.Is(err, ErrAdminActorInvalid) {
				t.Fatalf("want invalid actor, got %v", err)
			}
		})
	}
	if err := Authorize(AdminActor{ID: "staff"}, AdminCapabilitySupportRead, now); !errors.Is(err, ErrAdminAccessDenied) {
		t.Fatalf("missing roles: %v", err)
	}
	actor := AdminActor{ID: "staff", Roles: []AdminRole{"unknown", AdminRoleAdministrator, AdminRoleFinancialVerifier}, Purpose: "review case"}
	before := actor
	before.Roles = slices.Clone(actor.Roles)
	for _, capability := range []AdminCapability{AdminCapabilityFinancialRead, AdminCapabilityFeatureFlags} {
		if err := Authorize(actor, capability, now); err != nil {
			t.Fatalf("explicit role union: %v", err)
		}
	}
	if err := Authorize(actor, AdminCapabilitySafeReplay, now); !errors.Is(err, ErrAdminAccessDenied) {
		t.Fatalf("union must not grant additional capabilities: %v", err)
	}
	if !reflect.DeepEqual(actor, before) {
		t.Fatal("authorization mutated actor")
	}
}

func TestAuthorizePurpose(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	for _, capability := range []AdminCapability{AdminCapabilitySupportRead, AdminCapabilitySupportNotes, AdminCapabilityUserConfirmation, AdminCapabilityFinancialRead, AdminCapabilityFixtureReview, AdminCapabilityReconciliation, AdminCapabilityPolicyReview, AdminCapabilitySafeReplay} {
		t.Run(string(capability), func(t *testing.T) {
			for _, tc := range []struct {
				name, purpose string
				allowed       bool
			}{
				{"missing", "", false},
				{"whitespace", " \t\n\u2003", false},
				{"text", "review case", true},
				{"boundary", strings.Repeat("x", 512), true},
				{"too long", strings.Repeat("x", 513), false},
				{"unicode boundary", strings.Repeat("é", 256), true},
				{"unicode too long", strings.Repeat("é", 257), false},
				{"invalid utf8", "\xff", false},
			} {
				t.Run(tc.name, func(t *testing.T) {
					actor := AdminActor{ID: "staff", Roles: []AdminRole{AdminRoleFinancialVerifier, AdminRoleSupportOperator}, Purpose: tc.purpose}
					err := Authorize(actor, capability, now)
					if tc.allowed {
						if err != nil {
							t.Fatal(err)
						}
					} else if !errors.Is(err, ErrAdminPurposeRequired) {
						t.Fatalf("want purpose error, got %v", err)
					}
				})
			}
		})
	}
	if err := Authorize(AdminActor{ID: "staff", Roles: []AdminRole{AdminRoleSupportReader}}, AdminCapabilitySupportRead, now); !errors.Is(err, ErrAdminPurposeRequired) {
		t.Fatalf("redacted read: %v", err)
	}
}

func TestAuthorizeStepUp(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	for _, capability := range []AdminCapability{AdminCapabilityExportVerification, AdminCapabilityDeletionVerification, AdminCapabilityPolicyPublish, AdminCapabilityRoleChanges} {
		t.Run(string(capability), func(t *testing.T) {
			for _, tc := range []struct {
				name        string
				stepUp, now time.Time
				allowed     bool
			}{
				{"missing", time.Time{}, now, false},
				{"current", now, now, true},
				{"recent", now.Add(-time.Minute), now, true},
				{"boundary", now.Add(-5 * time.Minute), now, true},
				{"expired", now.Add(-5*time.Minute - time.Nanosecond), now, false},
				{"future", now.Add(time.Nanosecond), now, false},
				{"zero clock", now, time.Time{}, false},
				{"both zero", time.Time{}, time.Time{}, false},
			} {
				t.Run(tc.name, func(t *testing.T) {
					actor := AdminActor{ID: "staff", Roles: []AdminRole{AdminRoleSecurityAuditor, AdminRolePolicyPublisher, AdminRoleAdministrator}, StepUpAt: tc.stepUp}
					err := Authorize(actor, capability, tc.now)
					if tc.allowed {
						if err != nil {
							t.Fatal(err)
						}
					} else if !errors.Is(err, ErrAdminStepUpRequired) {
						t.Fatalf("want step-up error, got %v", err)
					}
				})
			}
		})
	}
	for _, tc := range []struct {
		role       AdminRole
		capability AdminCapability
	}{
		{AdminRoleSupportReader, AdminCapabilitySupportRead},
		{AdminRoleSupportOperator, AdminCapabilitySafeReplay},
		{AdminRoleFinancialVerifier, AdminCapabilityFinancialRead},
		{AdminRoleOperations, AdminCapabilityHealth},
		{AdminRoleBillingOperator, AdminCapabilityEntitlements},
		{AdminRoleAdministrator, AdminCapabilityFeatureFlags},
	} {
		actor := AdminActor{ID: "staff", Roles: []AdminRole{tc.role}, Purpose: "review case"}
		if err := Authorize(actor, tc.capability, now); err != nil {
			t.Fatalf("ordinary capability %s: %v", tc.capability, err)
		}
	}
}
