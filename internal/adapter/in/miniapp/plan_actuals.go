package miniapp

import (
	"context"
	"errors"
	"net/http"

	"github.com/andranikasd/marumbot/internal/app"
)

type activePlanActualsReader interface {
	ActivePlanActuals(context.Context, string, string) ([]app.PlanActualComparison, error)
}

// Parent route: GET /api/plan-actuals -> s.planActuals(). Existing Planner is Worker.
func (s *Server) planActuals() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, user, ok := s.authed(w, r)
		if !ok {
			return
		}
		reader, ok := s.Planner.(activePlanActualsReader)
		if !ok || s.Payments == nil {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		month := r.URL.Query().Get("month")
		if month == "" {
			today, err := s.Payments.BusinessDate(ctx, user)
			if err != nil {
				paymentHTTPError(w, err)
				return
			}
			month = today.String()[:7]
		}
		if err := app.ValidatePaymentMonth(month); err != nil {
			paymentHTTPError(w, err)
			return
		}
		comparisons, err := reader.ActivePlanActuals(ctx, user, month)
		if errors.Is(err, app.ErrHistoricalEngine) || errors.Is(err, app.ErrPlanActualsCoverage) {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "plan_actuals_unavailable"})
			return
		}
		if err != nil {
			paymentHTTPError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"month": month, "basis": "reported_value_date", "activation_boundary": "after_activation_day_and_transfer_date", "comparisons": comparisons})
	})
}
