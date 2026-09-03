package app

import (
	"errors"
	"testing"
)

func TestNewPlanRequiresFundingDeclaration(t *testing.T) {
	w, _ := chartWorker(t)
	f := w.Budgets.(*shadowFakes)
	f.budget.Funding = nil
	_, err := w.PlanSheet(t.Context(), "chart-user", nil)
	if !errors.Is(err, ErrFundingRequired) {
		t.Fatalf("missing funding: %v", err)
	}
	// An explicit zero is a real declaration; the planner reports affordability,
	// rather than substituting permission as cash or asking for a declaration.
	f.budget.Funding = &BudgetFunding{MonthlyMinor: 0}
	_, err = w.PlanSheet(t.Context(), "chart-user", nil)
	if err == nil || errors.Is(err, ErrFundingRequired) {
		t.Fatalf("explicit zero must reach affordability checks: %v", err)
	}
}
