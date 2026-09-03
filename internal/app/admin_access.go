package app

import (
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// AdminRole is an explicit assignment from the authenticated admin identity.
type AdminRole string

const (
	AdminRoleSupportReader     AdminRole = "support_reader"
	AdminRoleSupportOperator   AdminRole = "support_operator"
	AdminRoleFinancialVerifier AdminRole = "financial_verifier"
	AdminRolePolicyPublisher   AdminRole = "policy_publisher"
	AdminRoleOperations        AdminRole = "operations"
	AdminRoleBillingOperator   AdminRole = "billing_operator"
	AdminRoleSecurityAuditor   AdminRole = "security_auditor"
	AdminRoleAdministrator     AdminRole = "administrator"
)

// AdminCapability names a single authorization boundary, not a route.
type AdminCapability string

const (
	AdminCapabilityIdentityRead         AdminCapability = "administrator.identity_read"
	AdminCapabilityPolicyRead           AdminCapability = "policy.read"
	AdminCapabilitySupportRead          AdminCapability = "support.read_redacted"
	AdminCapabilitySupportNotes         AdminCapability = "support.notes"
	AdminCapabilityUserConfirmation     AdminCapability = "support.user_confirmation"
	AdminCapabilitySafeReplay           AdminCapability = "support.safe_replay"
	AdminCapabilityFinancialRead        AdminCapability = "financial.read"
	AdminCapabilityFixtureReview        AdminCapability = "financial.fixture_review"
	AdminCapabilityReconciliation       AdminCapability = "financial.reconciliation_review"
	AdminCapabilityPolicyReview         AdminCapability = "financial.policy_review"
	AdminCapabilityPolicyPublish        AdminCapability = "policy.publish"
	AdminCapabilityNotifications        AdminCapability = "operations.notifications"
	AdminCapabilityHealth               AdminCapability = "operations.health"
	AdminCapabilityEntitlements         AdminCapability = "billing.entitlements"
	AdminCapabilityAuditRead            AdminCapability = "security.audit_read"
	AdminCapabilityAccessRead           AdminCapability = "security.access_read"
	AdminCapabilityExportVerification   AdminCapability = "security.export_verification"
	AdminCapabilityDeletionVerification AdminCapability = "security.deletion_verification"
	AdminCapabilityRoleChanges          AdminCapability = "administrator.role_changes"
	AdminCapabilityFeatureFlags         AdminCapability = "administrator.feature_flags"
)

// AdminActor must be supplied by a trusted authentication boundary. Authorize
// validates its shape and grants; it does not authenticate an ID or load roles.
// ID is opaque, nonempty UTF-8 without whitespace or control characters.
type AdminActor struct {
	ID       string
	Roles    []AdminRole
	Purpose  string
	StepUpAt time.Time
}

var (
	ErrAdminActorInvalid    = errors.New("invalid admin actor")
	ErrAdminAccessDenied    = errors.New("admin capability denied")
	ErrAdminPurposeRequired = errors.New("admin purpose must contain text and be at most 512 bytes")
	ErrAdminStepUpRequired  = errors.New("recent admin step-up required")
)

// Authorize is pure and denies every capability without an explicit role grant.
// It neither redacts returned data nor verifies publication approval, signatures,
// or separation of duties; those are obligations of the eventual use case.
// Financial reads (including evidence review) and replay need a purpose of at
// most 512 UTF-8 bytes. Sensitive actions require step-up within five minutes.
func Authorize(actor AdminActor, capability AdminCapability, now time.Time) error {
	if actor.ID == "" || !utf8.ValidString(actor.ID) || strings.ContainsFunc(actor.ID, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}) {
		return ErrAdminActorInvalid
	}
	granted := false
	for _, role := range actor.Roles {
		if adminRoleGrants(role, capability) {
			granted = true
			break
		}
	}
	if !granted {
		return ErrAdminAccessDenied
	}
	switch capability {
	case AdminCapabilitySupportRead, AdminCapabilitySupportNotes, AdminCapabilityUserConfirmation,
		AdminCapabilityPolicyRead, AdminCapabilityFinancialRead, AdminCapabilityFixtureReview,
		AdminCapabilityReconciliation, AdminCapabilityPolicyReview, AdminCapabilitySafeReplay:
		if strings.TrimSpace(actor.Purpose) == "" || len(actor.Purpose) > 512 || !utf8.ValidString(actor.Purpose) {
			return ErrAdminPurposeRequired
		}
	}
	switch capability {
	case AdminCapabilityExportVerification, AdminCapabilityDeletionVerification,
		AdminCapabilityPolicyPublish, AdminCapabilityRoleChanges:
		if actor.StepUpAt.IsZero() || now.IsZero() || actor.StepUpAt.After(now) || now.Sub(actor.StepUpAt) > 5*time.Minute {
			return ErrAdminStepUpRequired
		}
	}
	return nil
}

func adminRoleGrants(role AdminRole, capability AdminCapability) bool {
	switch role {
	case AdminRoleSupportReader:
		return capability == AdminCapabilitySupportRead
	case AdminRoleSupportOperator:
		return capability == AdminCapabilitySupportRead || capability == AdminCapabilitySupportNotes ||
			capability == AdminCapabilityUserConfirmation || capability == AdminCapabilitySafeReplay
	case AdminRoleFinancialVerifier:
		return capability == AdminCapabilityPolicyRead || capability == AdminCapabilityFinancialRead || capability == AdminCapabilityFixtureReview ||
			capability == AdminCapabilityReconciliation || capability == AdminCapabilityPolicyReview
	case AdminRolePolicyPublisher:
		return capability == AdminCapabilityPolicyRead || capability == AdminCapabilityPolicyPublish
	case AdminRoleOperations:
		return capability == AdminCapabilityNotifications || capability == AdminCapabilityHealth
	case AdminRoleBillingOperator:
		return capability == AdminCapabilityEntitlements
	case AdminRoleSecurityAuditor:
		return capability == AdminCapabilityIdentityRead || capability == AdminCapabilityAuditRead || capability == AdminCapabilityAccessRead ||
			capability == AdminCapabilityExportVerification || capability == AdminCapabilityDeletionVerification
	case AdminRoleAdministrator:
		return capability == AdminCapabilityIdentityRead || capability == AdminCapabilityRoleChanges || capability == AdminCapabilityFeatureFlags
	default:
		return false
	}
}

var AdminCapabilities = []AdminCapability{AdminCapabilityIdentityRead, AdminCapabilityPolicyRead, AdminCapabilitySupportRead, AdminCapabilitySupportNotes, AdminCapabilityUserConfirmation, AdminCapabilitySafeReplay, AdminCapabilityFinancialRead, AdminCapabilityFixtureReview, AdminCapabilityReconciliation, AdminCapabilityPolicyReview, AdminCapabilityPolicyPublish, AdminCapabilityNotifications, AdminCapabilityHealth, AdminCapabilityEntitlements, AdminCapabilityAuditRead, AdminCapabilityAccessRead, AdminCapabilityExportVerification, AdminCapabilityDeletionVerification, AdminCapabilityRoleChanges, AdminCapabilityFeatureFlags}
