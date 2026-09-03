package miniapp

import (
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"

	"github.com/andranikasd/marumbot/internal/app"
)

// Parent registers GET /api/plan/stress with s.planStress(). The
// comparison is scoped by authenticated user and an opaque issued proposal.
func (s *Server) planStress() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, user, ok := s.authed(w, r)
		if !ok {
			return
		}
		service, ok := s.Planner.(app.PlanStressReader)
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
		var increaseBP int64
		if values, exists := r.URL.Query()["required_increase_bp"]; exists {
			if len(values) != 1 {
				paymentHTTPError(w, app.ErrPaymentInvalid)
				return
			}
			increaseBP, err = strconv.ParseInt(values[0], 10, 64)
			if err != nil || increaseBP < 1 || increaseBP > 10000 {
				paymentHTTPError(w, app.ErrPaymentInvalid)
				return
			}
		}
		report, err := service.PlanStress(ctx, user, proposal, increaseBP)
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
