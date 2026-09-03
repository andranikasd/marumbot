package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"

	"github.com/andranikasd/marumbot/pkg/core/money"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

type PlanVersion struct {
	ID        string       `json:"id"`
	Currency  string       `json:"currency"`
	Manifest  PlanManifest `json:"manifest"`
	CreatedAt string       `json:"created_at"`
	Active    bool         `json:"active"`
	Outdated  bool         `json:"outdated"`
}
type PlanActivation struct {
	ID       string `json:"id"`
	Revision int64  `json:"revision"`
}
type PlanActivationCommand struct {
	Proposal         string `json:"proposal"`
	Key              string `json:"idempotency_key"`
	ExpectedRevision int64  `json:"expected_revision"`
}
type PlanHistoryStore interface {
	PlanSources(context.Context, string) (string, error)
	PlanHistory(context.Context, string) ([]PlanVersion, int64, error)
	PlanVersion(context.Context, string, string) (PlanVersion, error)
	BeginPlanActivation(context.Context) (PlanActivationTransaction, error)
}
type PlanActivationTransaction interface {
	LockPlanSources(context.Context, string) (string, error)
	Receipt(context.Context, string, string) (PlanActivation, string, error)
	Activate(context.Context, string, PlanActivationCommand, PlanManifest) (PlanActivation, error)
	Commit(context.Context) error
	Rollback(context.Context) error
}
type planProposals struct {
	mu   sync.Mutex
	rows map[string]PlanManifest
}

func (p *planProposals) put(user string, m PlanManifest) (string, error) {
	raw, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	key := hex.EncodeToString(sum[:])
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.rows == nil {
		p.rows = make(map[string]PlanManifest)
	}
	if len(p.rows) >= 256 {
		for k := range p.rows {
			delete(p.rows, k)
			break
		}
	}
	p.rows[user+":"+key] = m
	return key, nil
}

func (p *planProposals) get(user, key string) (PlanManifest, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	m, ok := p.rows[user+":"+key]
	return m, ok
}

func (w *Worker) ActivateProposal(ctx context.Context, user string, c PlanActivationCommand) (PlanActivation, error) {
	if w.History == nil {
		return PlanActivation{}, ErrNotFound
	}
	if len(c.Key) < 16 || len(c.Key) > 100 || len(c.Proposal) != 64 || c.ExpectedRevision < 0 {
		return PlanActivation{}, ErrConflict
	}
	today, err := (PaymentService{Clock: w.Clock, Users: w.Users}).BusinessDate(ctx, user)
	if err != nil {
		return PlanActivation{}, err
	}
	tx, err := w.History.BeginPlanActivation(ctx)
	if err != nil {
		return PlanActivation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	sources, err := tx.LockPlanSources(ctx, user)
	if err != nil {
		return PlanActivation{}, err
	}
	old, proposal, err := tx.Receipt(ctx, user, c.Key)
	if err == nil {
		if proposal != activationIdentity(c) {
			return PlanActivation{}, ErrConflict
		}
		return old, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return PlanActivation{}, err
	}
	manifest, ok := w.proposals.get(user, c.Proposal)
	if !ok {
		return PlanActivation{}, ErrConflict
	}
	if sources != manifest.Sources || today != manifest.Input.ValuationDate {
		return PlanActivation{}, ErrConflict
	}
	if _, err = ReplayManifest(manifest); err != nil {
		return PlanActivation{}, err
	}
	out, err := tx.Activate(ctx, user, c, manifest)
	if err != nil {
		return PlanActivation{}, err
	}
	return out, tx.Commit(ctx)
}

type activePlanReader interface {
	ActivePlanVersions(context.Context, string) ([]PlanVersion, int64, error)
}

func (w *Worker) activePlans(ctx context.Context, user string) ([]PlanVersion, int64, error) {
	if reader, ok := w.History.(activePlanReader); ok {
		return w.planHistoryRows(ctx, user, reader.ActivePlanVersions)
	}
	return w.PlanHistory(ctx, user)
}

func (w *Worker) PlanHistory(ctx context.Context, user string) ([]PlanVersion, int64, error) {
	if w.History == nil {
		return nil, 0, ErrNotFound
	}
	return w.planHistoryRows(ctx, user, w.History.PlanHistory)
}

func (w *Worker) planHistoryRows(ctx context.Context, user string, read func(context.Context, string) ([]PlanVersion, int64, error)) ([]PlanVersion, int64, error) {
	if w.History == nil {
		return nil, 0, ErrNotFound
	}
	rows, revision, err := read(ctx, user)
	if err != nil {
		return nil, 0, err
	}
	sources, err := w.History.PlanSources(ctx, user)
	if err != nil {
		return nil, 0, err
	}
	for i := range rows {
		rows[i].Outdated = approvedPlanOutdated(rows[i], sources)
	}
	return rows, revision, nil
}

func (w *Worker) HistoricalPlan(ctx context.Context, user, id string) (Sheet, error) {
	if w.History == nil {
		return Sheet{}, ErrNotFound
	}
	stored, err := w.History.PlanVersion(ctx, user, id)
	if err != nil {
		return Sheet{}, err
	}
	m := stored.Manifest
	result, err := ReplayManifest(m)
	if err != nil {
		return Sheet{}, err
	}
	report, err := w.plans.search(m.Input, m.Goal, w.Clock.Now())
	if err != nil {
		return Sheet{}, err
	}
	report = withSelectedPolicy(report, result)
	owed := money.Zero(m.Input.Cash.Monthly.Currency())
	for _, p := range m.Input.Loans {
		owed, err = owed.Add(p.Balance)
		if err != nil {
			return Sheet{}, err
		}
	}
	monthly := m.Input.Cash.Monthly
	if spending := m.Input.Cash.Spending; spending != nil {
		monthly = spending.Monthly
		if override, ok := spending.Overrides[plan.MonthKey(spending.PeriodStart(m.Input.ValuationDate))]; ok {
			monthly = override
		}
		// Changes supersede the cycle override only once effective.
		for _, change := range spending.Changes {
			if change.On.After(m.Input.ValuationDate) {
				break
			}
			monthly = change.Limit
		}
	} else if override, ok := m.Input.Cash.MonthlyOverrides[plan.MonthKey(m.Input.ValuationDate)]; ok {
		monthly = override
	}
	sheet, err := sheetFromReport(m.Input, m.Goal, report, owed, monthly, nil)
	if err != nil {
		return Sheet{}, err
	}
	sources, err := w.History.PlanSources(ctx, user)
	if err != nil {
		return Sheet{}, err
	}
	// The single-version reader returns the immutable manifest, without current
	// activation state. Resolve membership separately before marking supersession.
	versions, _, err := w.activePlans(ctx, user)
	if err != nil {
		return Sheet{}, err
	}
	stored.Active = false
	for _, version := range versions {
		if version.ID == id && version.Active {
			stored.Active = true
			break
		}
	}
	sheet.Historical = true
	sheet.Outdated = approvedPlanOutdated(stored, sources)
	return sheet, nil
}

func activationIdentity(c PlanActivationCommand) string {
	raw, _ := json.Marshal(c)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (c PlanActivationCommand) Identity() string { return activationIdentity(c) }

// A user-selected baseline cannot inherit the optimizer winner's certificate.
func withSelectedPolicy(report plan.Report, result plan.Result) plan.Report {
	if report.Best.Policy.ID() != result.Policy.ID() {
		report.Certificate = plan.Certificate{Strength: plan.NamedStrategiesOnly, Policies: 1, FeasiblePolicies: 1, BestCost: result.Cost(), EngineVersion: plan.EngineVersion, Quantum: result.TotalPaid.Currency().SettlementUnit, AssumedPayments: result.Assumed, Positions: report.Certificate.Positions}
	}
	report.Best = result
	return report
}

// Approved timelines keep their original dates across midnight. Only changed
// source facts or supersession invalidate them; proposal activation separately
// requires today's valuation and previews still require an exact input hash.
func approvedPlanOutdated(version PlanVersion, sources string) bool {
	return !version.Active || version.Manifest.Sources != sources
}
