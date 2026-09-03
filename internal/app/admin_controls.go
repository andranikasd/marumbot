package app

import (
	"context"
	"encoding/json"
	"strings"
)

// AdminControl is a versioned operator decision, with immutable revisions.
// Kind is fixed by each application operation; callers cannot choose storage.
type AdminControl struct {
	Kind     string
	ID       string
	Revision int64
	Body     json.RawMessage
}
type AdminControlTransaction interface {
	Control(context.Context, string, string) (AdminControl, error)
	SaveControl(context.Context, AdminControl, int64) error
	CaseEvidence(context.Context, AdminCase) (bool, error)
}

type AdminFlag struct {
	Environment     string
	Profile         string
	PlanningEnabled bool
	Reason          string
}
type AdminFlagReader interface {
	AdminProfileFlag(context.Context, string, string) (AdminFlag, error)
}

func (a *Admin) SetProfileFlag(ctx context.Context, flag AdminFlag, expected int64) error {
	if strings.TrimSpace(flag.Environment) == "" || strings.TrimSpace(flag.Profile) == "" || strings.TrimSpace(flag.Reason) == "" {
		return ErrAdminEvidenceRequired
	}
	// Length-prefixed tuple avoids collisions between environment/profile keys.
	keyBytes, _ := json.Marshal([]string{flag.Environment, flag.Profile})
	key := string(keyBytes)
	body, err := json.Marshal(flag)
	if err != nil {
		return err
	}
	return a.saveControl(ctx, AdminCapabilityFeatureFlags, AdminControl{Kind: "profile_flag", ID: key, Body: body, Revision: expected + 1}, expected, nil)
}

type AdminCase struct {
	ID         string
	UserID     string
	LoanID     string
	Category   string
	Note       string
	State      string
	Resolution string
	EvidenceID string
}

func validCaseCategory(category string) bool {
	switch category {
	case "input_error", "posting_date", "allocation_policy", "fee", "rounding", "schedule_reissue", "bank_correction", "engine_defect", "unknown":
		return true
	default:
		return false
	}
}

func (a *Admin) SaveCase(ctx context.Context, c AdminCase, expected int64) error {
	if c.ID == "" || c.UserID == "" || c.LoanID == "" || !validCaseCategory(c.Category) || strings.TrimSpace(c.Note) == "" {
		return ErrAdminEvidenceRequired
	}
	cap := AdminCapabilitySupportNotes
	switch c.State {
	case "open":
		if c.Resolution != "" || c.EvidenceID != "" {
			return ErrAdminEvidenceRequired
		}
	case "resolved":
		cap = AdminCapabilityReconciliation
		if c.Resolution != "anchor" && c.Resolution != "corrected_event" && c.Resolution != "policy_conclusion" {
			return ErrAdminEvidenceRequired
		}
		if c.EvidenceID == "" || expected == 0 {
			return ErrAdminEvidenceRequired
		}
	default:
		return ErrAdminEvidenceRequired
	}
	body, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return a.saveControl(ctx, cap, AdminControl{Kind: "case", ID: c.ID, Revision: expected + 1, Body: body}, expected, &c)
}

func (a *Admin) saveControl(ctx context.Context, cap AdminCapability, c AdminControl, expected int64, caseData *AdminCase) error {
	if !a.SecurityReady() {
		return ErrAdminSecurityUnavailable
	}
	tx, err := a.security.BeginAdmin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := a.authorizeTx(ctx, tx, cap, c.ID); err != nil {
		return err
	}
	controls, ok := tx.(AdminControlTransaction)
	if !ok {
		return ErrAdminSecurityUnavailable
	}
	if caseData != nil && expected > 0 {
		old, err := controls.Control(ctx, c.Kind, c.ID)
		if err != nil {
			return err
		}
		var previous AdminCase
		if err := json.Unmarshal(old.Body, &previous); err != nil {
			return err
		}
		if previous.Note != caseData.Note {
			caseData.Note = previous.Note + "\n\n" + caseData.Note
			body, err := json.Marshal(caseData)
			if err != nil {
				return err
			}
			c.Body = body
		}
		if previous.UserID != caseData.UserID || previous.LoanID != caseData.LoanID {
			return ErrAdminAccessDenied
		}
	}
	if caseData != nil {
		verified, err := controls.CaseEvidence(ctx, *caseData)
		if err != nil {
			return err
		}
		if !verified {
			return ErrAdminEvidenceRequired
		}
	}
	if err := controls.SaveControl(ctx, c, expected); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (a *Admin) SupportCase(ctx context.Context, id string) (AdminCase, int64, error) {
	if err := a.CheckAccess(ctx, AdminCapabilitySupportRead, id); err != nil {
		return AdminCase{}, 0, err
	}
	tx, err := a.security.BeginAdmin(ctx)
	if err != nil {
		return AdminCase{}, 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	controls, ok := tx.(AdminControlTransaction)
	if !ok {
		return AdminCase{}, 0, ErrAdminSecurityUnavailable
	}
	row, err := controls.Control(ctx, "case", id)
	if err != nil {
		return AdminCase{}, 0, err
	}
	var c AdminCase
	err = json.Unmarshal(row.Body, &c)
	// Free-text notes and evidence may contain financial or identifying details.
	// The support-reader surface exposes classification and status only.
	c.Note = ""
	c.EvidenceID = ""
	return c, row.Revision, err
}
