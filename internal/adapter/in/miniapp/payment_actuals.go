package miniapp

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/andranikasd/marumbot/internal/app"
)

// Central route wiring: GET /api/activity -> allocatedActivity(), and
// GET /api/payment-actuals -> paymentActuals(). Existing Editor supplies the port.
func (s *Server) allocatedActivity() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, user, ok := s.authed(w, r)
		if !ok {
			return
		}
		reader, ok := s.Editor.(app.PaymentActualsReader)
		if !ok {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		cursor := r.URL.Query().Get("after")
		if cursor != "" {
			if _, err := uuid.Parse(cursor); err != nil {
				http.Error(w, "invalid cursor", http.StatusBadRequest)
				return
			}
		}
		facts, err := reader.BorrowerAllocatedActivity(ctx, user, cursor)
		if err != nil {
			paymentHTTPError(w, err)
			return
		}
		next := ""
		if len(facts) == 100 {
			next = facts[len(facts)-1].ID
		}
		writeJSON(w, http.StatusOK, map[string]any{"facts": facts, "next_cursor": next})
	})
}

func (s *Server) paymentActuals() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, user, ok := s.authed(w, r)
		if !ok {
			return
		}
		reader, ok := s.Editor.(app.PaymentActualsReader)
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
		totals, err := reader.MonthlyPaymentActuals(ctx, user, month)
		if err != nil {
			paymentHTTPError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"month": month, "basis": "transaction_date", "totals": totals})
	})
}
