package miniapp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

// SetBudgetFunding changes declared receipts without rewriting permission or
// reconciled cash/spending. The application command supplies a durable receipt.
func (s *Server) SetBudgetFunding() http.HandlerFunc {
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
		var in app.BudgetFundingUpdate
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

		err := (app.BudgetCommands{Store: store, Clock: s.Clock, Users: s.Users}).UpdateBudgetFunding(ctx, userID, in)
		var unsupported *plan.UnsupportedError
		switch {
		case errors.Is(err, app.ErrConflict):
			writeJSON(w, http.StatusConflict, map[string]string{jsonError: "conflict"})
		case errors.As(err, &unsupported):
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{jsonError: "unsupported", "reason": unsupported.Feature})
		case errors.Is(err, app.ErrBudgetFundingInvalid), errors.Is(err, app.ErrPaymentInvalid):
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{jsonError: "funding_rejected"})
		case err != nil:
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{jsonError: "funding_unavailable"})
		default:
			writeJSON(w, http.StatusOK, map[string]bool{"saved": true})
		}
	}
}
