package miniapp

import (
	"encoding/hex"
	"errors"
	"net/http"

	"github.com/andranikasd/marumbot/internal/app"
)

// Parent registers GET /api/plan/comparisons with s.planComparisons(). The
// comparison is scoped by authenticated user and an opaque issued proposal.
func (s *Server) planComparisons() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, user, ok := s.authed(w, r)
		if !ok {
			return
		}
		service, ok := s.Planner.(app.PlanComparer)
		if !ok {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		proposal := r.URL.Query().Get("proposal")
		raw, err := hex.DecodeString(proposal)
		if err != nil || len(raw) != 32 {
			paymentHTTPError(w, app.ErrPaymentInvalid)
			return
		}
		report, err := service.PlanComparisons(ctx, user, proposal)
		if errors.Is(err, app.ErrHistoricalEngine) {
			http.Error(w, "historical engine unavailable", http.StatusUnprocessableEntity)
			return
		}
		if err != nil {
			paymentHTTPError(w, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, report)
	})
}
