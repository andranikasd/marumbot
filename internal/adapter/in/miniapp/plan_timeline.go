package miniapp

import (
	"context"
	"encoding/csv"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/andranikasd/marumbot/internal/app"
)

type timelinePlanner interface {
	PaymentTimeline(context.Context, string, string, string) (app.PlanTimeline, error)
}

func (s *Server) planTimeline() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, user, ok := s.authed(w, r)
		if !ok {
			return
		}
		planner, ok := s.Planner.(timelinePlanner)
		if !ok {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		id := r.URL.Query().Get("id")
		if id != "" {
			if _, err := uuid.Parse(id); err != nil {
				paymentHTTPError(w, app.ErrPaymentInvalid)
				return
			}
		}
		out, err := planner.PaymentTimeline(ctx, user, r.URL.Query().Get("proposal"), id)
		if err != nil {
			paymentHTTPError(w, err)
			return
		}
		if r.URL.Query().Get("format") != "csv" {
			writeJSON(w, 200, out)
			return
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="marum-plan.csv"`)
		w.Header().Set("Cache-Control", "no-store")
		writer := csv.NewWriter(w)
		_ = writer.Write([]string{"date", "loan", "kind", "amount_minor", "fee_minor", keyCurrency, "currency_exponent", "settlement_quantum", "engine", "input_hash"})
		for _, p := range out.Payments {
			_ = writer.Write([]string{p.On, csvText(p.Loan), p.Kind, strconv.FormatInt(p.AmountMinor, 10), strconv.FormatInt(p.FeeMinor, 10), out.Currency, strconv.Itoa(int(out.Exponent)), strconv.FormatInt(out.Quantum, 10), out.Engine, out.InputHash})
		}
		writer.Flush()
	})
}

func csvText(s string) string {
	if strings.ContainsAny(strings.TrimSpace(s)[:min(len(strings.TrimSpace(s)), 1)], "=+-@") || strings.HasPrefix(s, "\t") || strings.HasPrefix(s, "\r") {
		return "'" + s
	}
	return s
}
