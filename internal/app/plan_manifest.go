package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/andranikasd/marumbot/pkg/core/plan"
)

var ErrHistoricalEngine = errors.New("historical calculation engine unavailable")

// PlanManifest contains original source inputs and the selected action policy.
// Results are deliberately not persisted: replay is the correctness check.
type PlanManifest struct {
	Sources       string      `json:"sources"`
	Schema        int         `json:"schema"`
	Engine        string      `json:"engine"`
	Input         plan.Input  `json:"input"`
	Goal          plan.Goal   `json:"goal"`
	Policy        plan.Policy `json:"policy"`
	InputHash     string      `json:"input_hash"`
	BudgetVersion int64       `json:"budget_version"`
	ResultHash    string      `json:"result_hash"`
}

func manifestFor(in plan.Input, g plan.Goal, r plan.Report, budgetVersion int64) (PlanManifest, error) {
	_, _, err := plan.Normalize(in)
	if err != nil {
		return PlanManifest{}, err
	}
	hash, err := resultHash(r.Best)
	if err != nil {
		return PlanManifest{}, err
	}
	return PlanManifest{Schema: 1, Engine: plan.EngineVersion, Input: in, Goal: g, Policy: r.Best.Policy, InputHash: searchFingerprint(in, plan.Goal{}), BudgetVersion: budgetVersion, ResultHash: hash}, nil
}

func resultHash(r plan.Result) (string, error) {
	raw, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func ReplayManifest(m PlanManifest) (plan.Result, error) {
	if m.Schema != 1 || m.Engine != plan.EngineVersion {
		return plan.Result{}, ErrHistoricalEngine
	}
	if m.InputHash != searchFingerprint(m.Input, plan.Goal{}) {
		return plan.Result{}, ErrConflict
	}
	normalized, assumed, err := plan.Normalize(m.Input)
	if err != nil {
		return plan.Result{}, err
	}
	r, err := plan.Run(normalized, m.Policy)
	if err != nil {
		return plan.Result{}, err
	}
	r.Assumed = assumed
	hash, err := resultHash(r)
	if err != nil {
		return plan.Result{}, err
	}
	if hash != m.ResultHash {
		return plan.Result{}, ErrConflict
	}
	return r, nil
}
