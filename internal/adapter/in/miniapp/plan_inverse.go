package miniapp

import (
	"context"
	"net/http"

	"github.com/andranikasd/marumbot/internal/app"
)

type inversePlanner interface {
	BudgetByDate(context.Context, string, string, string) (app.InverseBudget, error)
}

func (s *Server) inverseBudget() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, user, ok := s.authed(w, r)
		if !ok {
			return
		}
		planner, ok := s.Planner.(inversePlanner)
		if !ok {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		out, err := planner.BudgetByDate(ctx, user, r.URL.Query().Get("proposal"), r.URL.Query().Get("target"))
		if err != nil {
			paymentHTTPError(w, err)
			return
		}
		writeJSON(w, 200, out)
	})
}
