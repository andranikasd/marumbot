package miniapp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/google/uuid"

	"github.com/andranikasd/marumbot/internal/app"
)

func paymentHTTPError(w http.ResponseWriter, err error) {
	code, message := http.StatusInternalServerError, "internal"
	switch {
	case errors.Is(err, app.ErrPaymentInvalid):
		code, message = http.StatusUnprocessableEntity, err.Error()
	case errors.Is(err, app.ErrNotFound):
		code, message = http.StatusNotFound, "not_found"
	case errors.Is(err, app.ErrConflict):
		code, message = http.StatusConflict, "version_conflict"
	case errors.Is(err, app.ErrPaymentDuplicate):
		code, message = http.StatusConflict, "possible_duplicate_payment"
	}
	writeJSON(w, code, map[string]string{"error": message})
}

func (s *Server) paymentContext() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, userID, ok := s.authed(w, r)
		if !ok {
			return
		}
		if s.PaymentReader == nil || s.Payments == nil {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		if _, err := uuid.Parse(r.PathValue("id")); err != nil {
			paymentHTTPError(w, app.ErrNotFound)
			return
		}
		out, err := s.PaymentReader.PaymentContext(ctx, r.PathValue("id"), userID)
		if err != nil {
			paymentHTTPError(w, err)
			return
		}
		today, err := s.Payments.BusinessDate(ctx, userID)
		if err != nil {
			paymentHTTPError(w, err)
			return
		}
		out.Today = today.String()
		writeJSON(w, http.StatusOK, out)
	})
}

func (s *Server) recordPayment() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, userID, ok := s.authed(w, r)
		if !ok {
			return
		}
		if s.Payments == nil {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		var c app.PaymentCommand
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequest))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&c); err != nil {
			paymentHTTPError(w, app.ErrPaymentInvalid)
			return
		}
		var trailing any
		if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
			paymentHTTPError(w, app.ErrPaymentInvalid)
			return
		}
		c.LoanID = r.PathValue("id")
		if _, err := uuid.Parse(c.LoanID); err != nil {
			paymentHTTPError(w, app.ErrNotFound)
			return
		}
		if c.Replaces != "" {
			if _, err := uuid.Parse(c.Replaces); err != nil {
				paymentHTTPError(w, app.ErrPaymentInvalid)
				return
			}
		}
		receipt, err := s.Payments.Record(ctx, userID, c)
		if err != nil {
			paymentHTTPError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, receipt)
	})
}
