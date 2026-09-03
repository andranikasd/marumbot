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
		rows, revision, err := history.PlanHistory(ctx, user)
		if err != nil {
			paymentHTTPError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"plans": rows, "revision": revision})
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
