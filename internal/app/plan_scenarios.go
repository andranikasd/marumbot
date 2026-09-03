package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/money"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

// ScenarioChanges are declarations, never calculated reports. Nil preserves a field.
type ScenarioChanges struct {
	MonthlyMinor  *int64           `json:"monthly_minor,omitempty"`
	EffectiveFrom string           `json:"effective_from,omitempty"`
	PayDay        *int             `json:"pay_day,omitempty"`
	ReserveMinor  *int64           `json:"reserve_minor,omitempty"`
	OneTimeCash   *BudgetCashEvent `json:"one_time_cash,omitempty"`
}
type ScenarioCommand struct {
	Proposal   string          `json:"proposal"`
	VersionID  string          `json:"version_id,omitempty"`
	Changes    ScenarioChanges `json:"changes"`
	ResultHash string          `json:"result_hash,omitempty"`
}

// PlanScenario preserves the originals, the user's edits, and the selected policy.
// Candidate inputs and reports are rebuilt, not persisted.
type PlanScenario struct {
	ID         string          `json:"id"`
	Original   PlanManifest    `json:"original"`
	Budget     Budget          `json:"budget"`
	Changes    ScenarioChanges `json:"changes"`
	Policy     plan.Policy     `json:"policy"`
	ResultHash string          `json:"result_hash"`
}
type ScenarioView struct {
	ID         string          `json:"id"`
	Changes    ScenarioChanges `json:"changes"`
	Sheet      Sheet           `json:"sheet"`
	ResultHash string          `json:"result_hash"`
	Revision   int64           `json:"revision"`
	Outdated   bool            `json:"outdated"`
}
type ScenarioActivationCommand struct {
	ID               string `json:"id"`
	Key              string `json:"idempotency_key"`
	ExpectedRevision int64  `json:"expected_revision"`
}
type PlanScenarioStore interface {
	ScenarioBudget(context.Context, string, string, int64) (Budget, error)
	SaveScenario(context.Context, string, PlanScenario) (string, error)
	Scenario(context.Context, string, string) (PlanScenario, error)
	Scenarios(context.Context, string) ([]PlanScenario, error)
	BeginScenarioActivation(context.Context) (ScenarioTransaction, error)
}
type ScenarioTransaction interface {
	PlanActivationTransaction
	ApplyScenarioBudget(context.Context, string, Budget, int64) (int64, error)
}

func (w *Worker) scenarioStore() (PlanScenarioStore, error) {
	s, ok := w.History.(PlanScenarioStore)
	if !ok {
		return nil, ErrNotFound
	}
	return s, nil
}

// scenarioBudget deep-clones all maps, policy slices, funding events and pointers.
func scenarioBudget(s PlanScenario) (Budget, error) {
	raw, err := json.Marshal(s.Budget)
	if err != nil {
		return Budget{}, err
	}
	var b Budget
	if err = json.Unmarshal(raw, &b); err != nil {
		return b, err
	}
	c := s.Changes
	cur := b.Monthly.Currency()
	on := s.Original.Input.ValuationDate
	if c.MonthlyMinor == nil && c.EffectiveFrom != "" {
		return b, ErrPaymentInvalid
	}
	if c.MonthlyMinor != nil {
		if *c.MonthlyMinor < 0 {
			return b, ErrPaymentInvalid
		}
		effective := on
		if c.EffectiveFrom != "" {
			effective, err = date.Parse(c.EffectiveFrom)
			if err != nil {
				return b, ErrPaymentInvalid
			}
		}
		if effective.Before(on) {
			return b, &plan.UnsupportedError{Feature: "retroactive scenario budget"}
		}
		if len(b.Policies) == 0 && effective == on {
			// Legacy recurring budgets remain compatible; an explicit current
			// period replacement must not be shadowed by that month's override.
			b.Monthly = money.FromMinor(*c.MonthlyMinor, cur)
			delete(b.Overrides, plan.MonthKey(on))
		} else {
			if b.Funding == nil {
				return b, &plan.UnsupportedError{Feature: "dated budget changes require explicit funding"}
			}
			p, e := b.PolicyForMonthlyChange(effective, *c.MonthlyMinor)
			if e != nil {
				return b, e
			}
			p.Version = b.Version + 1
			b.Policies = append(b.Policies, p)
		}
	}
	if c.PayDay != nil {
		if *c.PayDay < 0 || *c.PayDay > 31 {
			return b, ErrPaymentInvalid
		}
		b.PayDay = *c.PayDay
	}
	if c.ReserveMinor != nil {
		if *c.ReserveMinor < 0 {
			return b, ErrPaymentInvalid
		}
		b.Reserve = money.FromMinor(*c.ReserveMinor, cur)
	}
	if c.OneTimeCash != nil {
		e := *c.OneTimeCash
		d, eerr := date.Parse(e.On)
		if eerr != nil || e.Minor <= 0 || d.Before(on) {
			return b, ErrPaymentInvalid
		}
		if b.Funding == nil {
			return b, &plan.UnsupportedError{Feature: "one-time cash requires explicit funding"}
		}
		if through, eerr := date.Parse(b.Funding.CashThrough); eerr == nil && !d.After(through) {
			return b, &plan.UnsupportedError{Feature: "cash already included in the opening statement"}
		}
		b.Funding.Events = append(b.Funding.Events, e)
	}
	return b, nil
}

func scenarioCalculation(s PlanScenario, selectPolicy bool) (PlanManifest, Sheet, error) {
	if _, err := ReplayManifest(s.Original); err != nil {
		return PlanManifest{}, Sheet{}, err
	}
	raw, err := json.Marshal(s.Original)
	if err != nil {
		return PlanManifest{}, Sheet{}, err
	}
	var m PlanManifest
	if err = json.Unmarshal(raw, &m); err != nil {
		return m, Sheet{}, err
	}
	// Refuse a source whose budget declaration cannot reproduce its cash input.
	if !reflect.DeepEqual(s.Budget.CashPlan(m.Input.ValuationDate), m.Input.Cash) {
		return m, Sheet{}, &plan.UnsupportedError{Feature: "scenario source cash differs from its budget declaration"}
	}
	b, err := scenarioBudget(s)
	if err != nil {
		return m, Sheet{}, err
	}
	// In a what-if calculation the newly declared expected receipt is an
	// explicit assumption. Existing expected receipts remain excluded. Activation
	// refuses this assumption until the user creates a confirmed declaration.
	if s.Changes.OneTimeCash != nil && s.Changes.OneTimeCash.Expected {
		b.Funding.Events[len(b.Funding.Events)-1].Expected = false
	}
	m.Input.Cash, _, err = b.CashPlans(m.Input.ValuationDate)
	if err != nil {
		return m, Sheet{}, err
	}
	report, err := plan.Search(m.Input, m.Goal)
	if err != nil {
		return m, Sheet{}, err
	}
	if selectPolicy {
		m, err = manifestFor(m.Input, m.Goal, report, s.Original.BudgetVersion)
		m.Sources = s.Original.Sources
	} else {
		m.InputHash = searchFingerprint(m.Input, plan.Goal{})
		m.Policy = s.Policy
		m.ResultHash = s.ResultHash
		var selected plan.Result
		selected, err = ReplayManifest(m)
		if err == nil {
			report = withSelectedPolicy(report, selected)
		}
	}
	if err != nil {
		return m, Sheet{}, err
	}
	owed := money.Zero(b.Monthly.Currency())
	for _, p := range m.Input.Loans {
		owed, err = owed.Add(p.Balance)
		if err != nil {
			return m, Sheet{}, err
		}
	}
	permission, err := b.PermissionOn(m.Input.ValuationDate)
	if err != nil {
		return m, Sheet{}, err
	}
	sheet, err := sheetFromReport(m.Input, m.Goal, report, owed, permission, nil)
	return m, sheet, err
}

func (w *Worker) prepareScenario(ctx context.Context, user string, c ScenarioCommand) (PlanScenario, ScenarioView, error) {
	var s PlanScenario
	var v ScenarioView
	if _, err := w.scenarioStore(); err != nil {
		return s, v, err
	}
	if (c.Proposal == "") == (c.VersionID == "") {
		return s, v, ErrPaymentInvalid
	}
	if c.VersionID != "" {
		old, err := w.History.PlanVersion(ctx, user, c.VersionID)
		if err != nil {
			return s, v, err
		}
		s.Original = old.Manifest
	} else {
		var ok bool
		s.Original, ok = w.proposals.get(user, c.Proposal)
		if !ok {
			return s, v, ErrConflict
		}
	}
	// Resolve the immutable declaration matching the original manifest.
	store, err := w.scenarioStore()
	if err != nil {
		return s, v, err
	}
	b, err := store.ScenarioBudget(ctx, user, s.Original.Input.Cash.Monthly.Currency().Code, s.Original.BudgetVersion)
	if err != nil {
		return s, v, err
	}
	if b.Version != s.Original.BudgetVersion || b.Currency != s.Original.Input.Cash.Monthly.Currency().Code {
		return s, v, ErrConflict
	}
	s.Budget = b
	s.Changes = c.Changes
	current := w.scenarioCurrent(ctx, user, s)
	if current != nil && !errors.Is(current, ErrConflict) {
		return s, v, current
	}
	s.Budget = s.Budget.WithReleaseFacts(s.Original.Input.Cash.Spending)
	m, sheet, err := scenarioCalculation(s, true)
	if err != nil {
		return s, v, err
	}
	s.Policy = m.Policy
	s.ResultHash = m.ResultHash
	raw, err := json.Marshal(s)
	if err != nil {
		return s, v, err
	}
	sum := sha256.Sum256(raw)
	s.ID = hex.EncodeToString(sum[:])
	if c.ResultHash != "" && c.ResultHash != s.ResultHash {
		return s, v, ErrConflict
	}
	_, revision, err := w.History.PlanHistory(ctx, user)
	if err != nil {
		return s, v, err
	}
	current = w.scenarioCurrent(ctx, user, s)
	if current != nil && !errors.Is(current, ErrConflict) {
		return s, v, current
	}
	v = ScenarioView{ID: s.ID, Changes: s.Changes, Sheet: sheet, ResultHash: s.ResultHash, Revision: revision, Outdated: current != nil}
	return s, v, nil
}

func (w *Worker) scenarioCurrent(ctx context.Context, user string, s PlanScenario) error {
	sources, err := w.History.PlanSources(ctx, user)
	if err != nil {
		return err
	}
	today, err := (PaymentService{Clock: w.Clock, Users: w.Users}).BusinessDate(ctx, user)
	if err != nil {
		return err
	}
	if sources != s.Original.Sources || today != s.Original.Input.ValuationDate {
		return ErrConflict
	}
	return nil
}

func (w *Worker) PreviewScenario(ctx context.Context, user string, c ScenarioCommand) (ScenarioView, error) {
	_, v, err := w.prepareScenario(ctx, user, c)
	return v, err
}

func (w *Worker) SaveScenario(ctx context.Context, user string, c ScenarioCommand) (ScenarioView, error) {
	if len(c.ResultHash) != 64 {
		return ScenarioView{}, ErrPaymentInvalid
	}
	s, v, err := w.prepareScenario(ctx, user, c)
	if err != nil {
		return v, err
	}
	store, err := w.scenarioStore()
	if err != nil {
		return v, err
	}
	v.ID, err = store.SaveScenario(ctx, user, s)
	return v, err
}

func (w *Worker) Scenario(ctx context.Context, user, id string) (ScenarioView, error) {
	store, err := w.scenarioStore()
	if err != nil {
		return ScenarioView{}, err
	}
	s, err := store.Scenario(ctx, user, id)
	if err != nil {
		return ScenarioView{}, err
	}
	_, sheet, err := scenarioCalculation(s, false)
	if err != nil {
		return ScenarioView{}, err
	}
	_, revision, err := w.History.PlanHistory(ctx, user)
	if err != nil {
		return ScenarioView{}, err
	}
	current := w.scenarioCurrent(ctx, user, s)
	if current != nil && !errors.Is(current, ErrConflict) {
		return ScenarioView{}, current
	}
	return ScenarioView{ID: s.ID, Changes: s.Changes, Sheet: sheet, ResultHash: s.ResultHash, Revision: revision, Outdated: current != nil}, nil
}

func (w *Worker) Scenarios(ctx context.Context, user string) ([]PlanScenario, error) {
	store, err := w.scenarioStore()
	if err != nil {
		return nil, err
	}
	return store.Scenarios(ctx, user)
}

func (w *Worker) ActivateScenario(ctx context.Context, user string, c ScenarioActivationCommand) (PlanActivation, error) {
	var out PlanActivation
	if len(c.ID) != 64 || len(c.Key) < 16 || len(c.Key) > 100 || c.ExpectedRevision < 0 {
		return out, ErrPaymentInvalid
	}
	store, err := w.scenarioStore()
	if err != nil {
		return out, err
	}
	s, err := store.Scenario(ctx, user, c.ID)
	if err != nil {
		return out, err
	}
	// Precompute without a transaction or active writes. Receipt lookup below
	// precedes freshness validation so a lost response remains retryable.
	m, _, err := scenarioCalculation(s, false)
	if err != nil {
		return out, err
	}
	today, err := (PaymentService{Clock: w.Clock, Users: w.Users}).BusinessDate(ctx, user)
	if err != nil {
		return out, err
	}
	tx, err := store.BeginScenarioActivation(ctx)
	if err != nil {
		return out, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	sources, err := tx.LockPlanSources(ctx, user)
	if err != nil {
		return out, err
	}
	command := PlanActivationCommand{Proposal: "scenario:" + c.ID, Key: c.Key, ExpectedRevision: c.ExpectedRevision}
	old, identity, err := tx.Receipt(ctx, user, c.Key)
	if err == nil {
		if identity != command.Identity() {
			return out, ErrConflict
		}
		return old, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return out, err
	}
	if sources != s.Original.Sources || today != s.Original.Input.ValuationDate {
		return out, ErrConflict
	}
	if s.Changes.OneTimeCash != nil && s.Changes.OneTimeCash.Expected {
		return out, &plan.UnsupportedError{Feature: "confirm expected one-time cash before activation"}
	}
	b, err := scenarioBudget(s)
	if err != nil {
		return out, err
	}
	m.BudgetVersion, err = tx.ApplyScenarioBudget(ctx, user, b, s.Original.BudgetVersion)
	if err != nil {
		return out, err
	}
	m.Sources, err = tx.LockPlanSources(ctx, user)
	if err != nil {
		return out, err
	}
	out, err = tx.Activate(ctx, user, command, m)
	if err != nil {
		return out, err
	}
	return out, tx.Commit(ctx)
}
