package miniapp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/pkg/core/date"
)

const (
	keyVersion       = "version"
	jsonReason       = "reason"
	errorUnsupported = "unsupported"
)

func (s *Server) PaidMonths() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, user, ok := s.authed(w, r)
		if !ok {
			return
		}
		id := r.PathValue("id")
		if r.Method == http.MethodPost {
			commands, key, version, ok := s.loanCommandRequest(w, r, true)
			if !ok {
				return
			}
			currency, err := commands.Currency(ctx, id, user)
			if err != nil {
				paymentHTTPError(w, err)
				return
			}
			var input struct {
				Month     string      `json:"month"`
				AsOf      string      `json:"as_of"`
				Balance   json.Number `json:"balance_major"`
				Payment   json.Number `json:"payment_major"`
				Confirmed bool        `json:"confirmed"`
			}
			decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&input); err != nil || decoder.Decode(new(any)) != io.EOF {
				writeJSON(w, http.StatusBadRequest, map[string]string{jsonError: "invalid_statement"})
				return
			}
			if input.Balance == "" || input.Payment == "" {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]string{jsonError: "invalid_amount"})
				return
			}
			balance, err := budgetMinor(input.Balance, currency, true)
			if err != nil {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]string{jsonError: "invalid_amount"})
				return
			}
			payment, err := budgetMinor(input.Payment, currency, true)
			if err != nil {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]string{jsonError: "invalid_amount"})
				return
			}
			receipt, err := commands.MarkMonthsPaid(ctx, user, id, key, version, app.PaidMonthStatement{Month: input.Month, AsOf: input.AsOf, PrincipalMinor: balance, NextPaymentMinor: payment, Confirmed: input.Confirmed})
			if errors.Is(err, app.ErrPaymentReconciliation) {
				writeJSON(w, http.StatusConflict, map[string]string{jsonError: "payment_reconciliation_required"})
				return
			}
			if errors.Is(err, app.ErrPaymentInvalid) {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]string{jsonError: "invalid_paid_month"})
				return
			}
			if err != nil {
				if !loanCommandError(w, err) {
					paymentHTTPError(w, err)
				}
				return
			}
			writeJSON(w, http.StatusOK, receipt)
			return
		}
		if s.Editor == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{jsonError: errorUnavailable})
			return
		}
		loan, err := s.Editor.LoanForUser(ctx, id, user)
		if err != nil {
			paymentHTTPError(w, err)
			return
		}
		today, err := (app.PaymentService{Clock: s.Clock, Users: s.Users}).BusinessDate(ctx, user)
		if err != nil {
			paymentHTTPError(w, err)
			return
		}
		nextDates := map[string]string{}
		for offset := -1; offset <= 0; offset++ {
			month := date.AddMonths(date.OnDayOfMonth(today, 1), offset).String()[:7]
			if next, err := app.PaidMonthNextDue(loan, month, today); err == nil {
				nextDates[month] = next.String()
			}
		}
		out := map[string]any{"id": id, "name": loan.Name, keyVersion: loan.MutationVersion, "today": today.String(), "currency": loan.Contract.Currency.Code, "currency_exponent": loan.Contract.Currency.Exponent, "balance_major": major(loan.Balance), "next_dates": nextDates, "needs_reconciliation": loan.UnreconciledPayments, "paid_through": paidThrough(loan)}
		if loan.Contract.HasScheduled {
			out["payment_major"] = major(loan.Contract.ScheduledPayment)
		}
		writeJSON(w, http.StatusOK, out)
	})
}

func paidThrough(loan app.UserLoan) string {
	if loan.Contract.NotBeforeDue.IsZero() {
		return ""
	}
	return date.AddMonths(date.OnDayOfMonth(loan.Contract.NotBeforeDue, 1), -1).String()[:7]
}
