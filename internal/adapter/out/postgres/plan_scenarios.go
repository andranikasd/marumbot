package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

var _ app.PlanScenarioStore = (*Store)(nil)

type scenarioTx struct{ *activationTx }

func (s *Store) SaveScenario(ctx context.Context, user string, scenario app.PlanScenario) (string, error) {
	raw, err := json.Marshal(scenario)
	if err != nil {
		return "", err
	}
	_, err = s.pool.Exec(ctx, q("SavePlanScenario"), user, scenario.ID, raw)
	return scenario.ID, err
}

func (s *Store) Scenario(ctx context.Context, user, id string) (app.PlanScenario, error) {
	var out app.PlanScenario
	var raw string
	err := s.pool.QueryRow(ctx, q("GetPlanScenario"), user, id).Scan(&raw)
	if err != nil {
		return out, paymentError(err)
	}
	err = json.Unmarshal([]byte(raw), &out)
	return out, err
}

func (s *Store) Scenarios(ctx context.Context, user string) ([]app.PlanScenario, error) {
	rows, err := s.pool.Query(ctx, q("ListPlanScenarios"), user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []app.PlanScenario{}
	for rows.Next() {
		var raw string
		var s app.PlanScenario
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (s *Store) BeginScenarioActivation(ctx context.Context) (app.ScenarioTransaction, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return &scenarioTx{activationTx: &activationTx{tx: tx}}, nil
}

func (t *scenarioTx) ApplyScenarioBudget(ctx context.Context, user string, b app.Budget, version int64) (int64, error) {
	var funding any
	if b.Funding != nil {
		raw, err := json.Marshal(b.Funding)
		if err != nil {
			return 0, err
		}
		funding = string(raw)
	}
	policies := []app.BudgetPolicy{}
	for _, p := range b.Policies {
		if p.Version > version {
			if p.Version != version+1 {
				return 0, app.ErrConflict
			}
			policies = append(policies, p)
		}
	}
	if len(policies) > 1 {
		return 0, app.ErrConflict
	}
	overrides := b.Overrides
	if overrides == nil {
		overrides = map[string]int64{}
	}
	overrideJSON, err := json.Marshal(overrides)
	if err != nil {
		return 0, err
	}
	raw, err := json.Marshal(policies)
	if err != nil {
		return 0, err
	}
	var next int64
	err = t.tx.QueryRow(ctx, q("ApplyScenarioBudget"), user, b.Currency, version, b.Monthly.Minor(), b.PayDay, b.Reserve.Minor(), funding, string(raw), string(overrideJSON)).Scan(&next)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, app.ErrConflict
	}
	return next, err
}

// ScenarioBudget reads the exact immutable declaration behind the proposal,
// including stale history, never the user's newest unrelated currency budget.
func (s *Store) ScenarioBudget(ctx context.Context, user, currency string, version int64) (app.Budget, error) {
	var b app.Budget
	var monthly, opening, reserve int64
	var overrides, policies string
	var asof, funding *string
	err := s.pool.QueryRow(ctx, q("ScenarioBudgetVersion"), user, currency, version).Scan(&b.Currency, &monthly, &b.PayDay, &overrides, &opening, &asof, &reserve, &funding, &b.Version, &policies)
	if err != nil {
		return b, paymentError(err)
	}
	cur, err := money.Lookup(b.Currency)
	if err != nil {
		return b, err
	}
	b.Monthly = money.FromMinor(monthly, cur)
	b.Opening = money.FromMinor(opening, cur)
	b.Reserve = money.FromMinor(reserve, cur)
	b.Set = true
	if err = json.Unmarshal([]byte(overrides), &b.Overrides); err != nil {
		return b, err
	}
	if err = json.Unmarshal([]byte(policies), &b.Policies); err != nil {
		return b, err
	}
	if funding != nil {
		if err = json.Unmarshal([]byte(*funding), &b.Funding); err != nil {
			return b, err
		}
	}
	if asof != nil {
		b.OpeningAsOf, err = date.Parse(*asof)
	}
	return b, err
}
