package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

const (
	AdminPolicyDraft     = "draft"
	AdminPolicyReviewed  = "reviewed"
	AdminPolicyPublished = "published"
)

type AdminPolicy struct {
	Signature   string
	ID          string
	Key         string
	Version     int32
	Definition  json.RawMessage
	Excess      string
	Source      string
	Evidence    string
	ContentHash string
	Author      string
	Reviewer    string
	Publisher   string
	State       string
	Revision    int64
}

// PolicyContentHash binds the entire publishable payload and its evidence,
// including identity/version; changing any field invalidates prior approval.
func PolicyContentHash(p AdminPolicy) (string, error) {
	// Decode with UseNumber to preserve exact numeric values.
	var definition any
	dec := json.NewDecoder(strings.NewReader(string(p.Definition)))
	dec.UseNumber()
	if err := dec.Decode(&definition); err != nil {
		return "", err
	}
	canonical, err := json.Marshal(struct {
		ID         string
		Key        string
		Version    int32
		Definition any
		Excess     string
		Source     string
		Evidence   string
	}{p.ID, p.Key, p.Version, definition, p.Excess, p.Source, p.Evidence})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

func validAdminPolicy(p AdminPolicy) bool {
	if p.ID == "" || strings.TrimSpace(p.Key) == "" || p.Version < 1 || !json.Valid(p.Definition) || strings.TrimSpace(p.Source) == "" {
		return false
	}
	switch p.Excess {
	case "reduce_principal", "hold_as_advance", "requires_bank_request", "unknown":
		return true
	default:
		return false
	}
}

func (a *Admin) DraftPolicy(ctx context.Context, p AdminPolicy, expected int64) error {
	if !a.SecurityReady() {
		return ErrAdminSecurityUnavailable
	}
	if !validAdminPolicy(p) {
		return ErrAdminEvidenceRequired
	}
	tx, err := a.security.BeginAdmin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	actor, err := a.authorizeTx(ctx, tx, AdminCapabilityPolicyReview, p.ID)
	if err != nil {
		return err
	}
	if expected > 0 {
		old, err := tx.Policy(ctx, p.ID)
		if err != nil {
			return err
		}
		if old.Author != actor.ID || old.State == AdminPolicyPublished {
			return ErrAdminAccessDenied
		}
	}
	p.Signature = ""
	p.Author, p.Reviewer, p.Publisher, p.State, p.Revision = actor.ID, "", "", AdminPolicyDraft, expected+1
	p.ContentHash, err = PolicyContentHash(p)
	if err != nil {
		return err
	}
	if err := tx.SavePolicy(ctx, p, expected); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (a *Admin) ReviewPolicy(ctx context.Context, id string, expected int64, hash string) error {
	return a.advancePolicy(ctx, id, expected, hash, false)
}

func (a *Admin) PublishPolicy(ctx context.Context, id string, expected int64, hash string) error {
	return a.advancePolicy(ctx, id, expected, hash, true)
}

func (a *Admin) advancePolicy(ctx context.Context, id string, expected int64, hash string, publish bool) error {
	if !a.SecurityReady() {
		return ErrAdminSecurityUnavailable
	}
	tx, err := a.security.BeginAdmin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	cap := AdminCapabilityPolicyReview
	if publish {
		cap = AdminCapabilityPolicyPublish
	}
	actor, err := a.authorizeTx(ctx, tx, cap, id)
	if err != nil {
		return err
	}
	p, err := tx.Policy(ctx, id)
	if err != nil {
		return err
	}
	computed, err := PolicyContentHash(p)
	if err != nil {
		return err
	}
	if p.Revision != expected || hash != p.ContentHash || hash != computed {
		return ErrAdminConflict
	}
	if strings.TrimSpace(p.Evidence) == "" || actor.ID == p.Author {
		return ErrAdminEvidenceRequired
	}
	if publish {
		// The verifier may publish only with an explicit publisher role. The
		// author can never publish their own work, even with both roles.
		if p.State != AdminPolicyReviewed || p.Reviewer == "" || p.Reviewer == p.Author {
			return ErrAdminEvidenceRequired
		}
		if a.signPolicy == nil {
			return ErrAdminSecurityUnavailable
		}
		p.Signature, err = a.signPolicy(p.ContentHash)
		if err != nil {
			return err
		}
		p.State, p.Publisher = AdminPolicyPublished, actor.ID
		if err := tx.PublishPolicy(ctx, p); err != nil {
			return err
		}
	} else {
		if p.State != AdminPolicyDraft {
			return ErrAdminConflict
		}
		p.State, p.Reviewer = AdminPolicyReviewed, actor.ID
	}
	p.Revision++
	if err := tx.SavePolicy(ctx, p, expected); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (a *Admin) PolicyDraft(ctx context.Context, id string) (AdminPolicy, error) {
	if err := a.CheckAccess(ctx, AdminCapabilityPolicyRead, id); err != nil {
		return AdminPolicy{}, err
	}
	tx, err := a.security.BeginAdmin(ctx)
	if err != nil {
		return AdminPolicy{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	return tx.Policy(ctx, id)
}

func (a *Admin) WithPolicySigner(sign func(string) (string, error)) *Admin {
	a.signPolicy = sign
	return a
}
