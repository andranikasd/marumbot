package miniapp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/google/uuid"

	"github.com/andranikasd/marumbot/internal/app"
)

type planHistory interface {
	ActivateProposal(context.Context, string, app.PlanActivationCommand) (app.PlanActivation, error)
	PlanHistory(context.Context, string) ([]app.PlanVersion, int64, error)
	HistoricalPlan(context.Context, string, string) (app.Sheet, error)
}

func (s *Server) planHistory() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, user, ok := s.authed(w, r)
		if !ok {
			return
		}
		history, ok := s.Planner.(planHistory)
		if !ok {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		after := r.URL.Query().Get("after")
		if after != "" {
			if _, err := uuid.Parse(after); err != nil {
				http.Error(w, "invalid cursor", 400)
				return
			}
		}
		var rows []app.PlanVersion
		var revision int64
		var err error
		if paged, ok := s.Planner.(interface {
			PlanHistoryPage(context.Context, string, string) ([]app.PlanVersion, int64, error)
		}); ok {
			rows, revision, err = paged.PlanHistoryPage(ctx, user, after)
		} else {
			rows, revision, err = history.PlanHistory(ctx, user)
		}
		if err != nil {
			paymentHTTPError(w, err)
			return
		}
		next := ""
		if len(rows) > 50 {
			rows = rows[:50]
			next = rows[len(rows)-1].ID
		}
		type metadata struct {
			ID        string `json:"id"`
			Currency  string `json:"currency"`
			CreatedAt string `json:"created_at"`
			Active    bool   `json:"active"`
			Outdated  bool   `json:"outdated"`
		}
		out := make([]metadata, 0, len(rows))
		for _, row := range rows {
			out = append(out, metadata{row.ID, row.Currency, row.CreatedAt, row.Active, row.Outdated})
		}
		writeJSON(w, 200, map[string]any{"plans": out, "revision": revision, "next_cursor": next})
	})
}

func (s *Server) historicalPlan() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, user, ok := s.authed(w, r)
		if !ok {
			return
		}
		history, ok := s.Planner.(planHistory)
		if !ok {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		id := r.PathValue("id")
		if _, err := uuid.Parse(id); err != nil {
			paymentHTTPError(w, app.ErrPaymentInvalid)
			return
		}
		sheet, err := history.HistoricalPlan(ctx, user, id)
		if errors.Is(err, app.ErrHistoricalEngine) {
			http.Error(w, "historical engine unavailable", http.StatusUnprocessableEntity)
			return
		}
		if err != nil {
			paymentHTTPError(w, err)
			return
		}
		writeJSON(w, 200, sheet)
	})
}

func (s *Server) activateProposal() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, user, ok := s.authed(w, r)
		if !ok {
			return
		}
		history, ok := s.Planner.(planHistory)
		if !ok {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		var c app.PlanActivationCommand
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequest))
		d.DisallowUnknownFields()
		if err := d.Decode(&c); err != nil {
			paymentHTTPError(w, app.ErrPaymentInvalid)
			return
		}
		if err := d.Decode(new(any)); err != io.EOF {
			paymentHTTPError(w, app.ErrPaymentInvalid)
			return
		}
		receipt, err := history.ActivateProposal(ctx, user, c)
		if err != nil {
			paymentHTTPError(w, err)
			return
		}
		writeJSON(w, 200, receipt)
	})
}
