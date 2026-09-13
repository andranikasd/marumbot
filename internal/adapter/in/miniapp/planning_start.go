package miniapp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/pkg/core/amortisation"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

// SetPlanningStart records a spending pause without changing paid balances.
func (s *Server) SetPlanningStart() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, userID, ok := s.authed(w, r)
		if !ok {
			return
		}
		store, ok := s.Budgets.(app.BudgetCommandStore)
		if !ok {
			http.Error(w, "funding unavailable", http.StatusServiceUnavailable)
			return
		}
		var in app.PlanningStartUpdate
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&in); err != nil {
			http.Error(w, "invalid funding", http.StatusUnprocessableEntity)
			return
		}
		var trailing any
		if decoder.Decode(&trailing) != io.EOF {
			http.Error(w, "invalid funding", http.StatusUnprocessableEntity)
			return
		}

		version, err := (app.BudgetCommands{Store: store, Clock: s.Clock, Users: s.Users}).SetPlanningStart(ctx, userID, in)
		var unsupported *plan.UnsupportedError
		switch {
		case errors.Is(err, app.ErrConflict):
			writeJSON(w, http.StatusConflict, map[string]string{jsonError: "conflict"})
		case errors.As(err, &unsupported):
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{jsonError: errorUnsupported, jsonReason: unsupported.Feature})
		case errors.Is(err, app.ErrPaymentReconciliation):
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{jsonError: errorPaymentReconciliation})
		case errors.Is(err, amortisation.ErrUnsolvable):
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{jsonError: "loan_schedule_invalid"})
		case errors.Is(err, app.ErrFundingRequired):
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{jsonError: "funding_required"})
		case errors.Is(err, app.ErrPaymentInvalid):
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{jsonError: "planning_start_rejected"})
		case err != nil:
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{jsonError: "planning_start_unavailable"})
		default:
			writeJSON(w, http.StatusOK, map[string]any{"saved": true, keyVersion: version, "planning_start_month": in.Month})
		}
	}
}
