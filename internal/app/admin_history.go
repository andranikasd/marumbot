package app

import (
	"context"

	"github.com/andranikasd/marumbot/pkg/core/plan"
)

type AdminHistoryStore interface {
	PlanHistory(context.Context, string) ([]PlanVersion, int64, error)
	PlanVersion(context.Context, string, string) (PlanVersion, error)
}

func (a *Admin) WithHistory(h AdminHistoryStore) *Admin { a.history = h; return a }
func (a *Admin) HistoricalPlans(ctx context.Context, user string) ([]PlanVersion, int64, error) {
	if err := a.CheckAccess(ctx, AdminCapabilityFinancialRead, user); err != nil {
		return nil, 0, err
	}
	if a.history == nil {
		return nil, 0, ErrAdminSecurityUnavailable
	}
	return a.history.PlanHistory(ctx, user)
}

func (a *Admin) ReplayHistoricalPlan(ctx context.Context, user, id string) (plan.Result, error) {
	if err := a.CheckAccess(ctx, AdminCapabilitySafeReplay, id); err != nil {
		return plan.Result{}, err
	}
	// Replay exposes financial values, so both explicit capabilities are needed.
	if err := a.CheckAccess(ctx, AdminCapabilityFinancialRead, id); err != nil {
		return plan.Result{}, err
	}
	if a.history == nil {
		return plan.Result{}, ErrAdminSecurityUnavailable
	}
	version, err := a.history.PlanVersion(ctx, user, id)
	if err != nil {
		return plan.Result{}, err
	}
	return ReplayManifest(version.Manifest)
}
