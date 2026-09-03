package miniapp

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/google/uuid"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

type scenarioListItem struct {
	ID       string              `json:"id"`
	Changes  app.ScenarioChanges `json:"changes"`
	Currency string              `json:"currency"`
	AsOf     string              `json:"as_of"`
}

type scenarioPlanner interface {
	PreviewScenario(context.Context, string, app.ScenarioCommand) (app.ScenarioView, error)
	SaveScenario(context.Context, string, app.ScenarioCommand) (app.ScenarioView, error)
	Scenario(context.Context, string, string) (app.ScenarioView, error)
	Scenarios(context.Context, string) ([]app.PlanScenario, error)
	ActivateScenario(context.Context, string, app.ScenarioActivationCommand) (app.PlanActivation, error)
}

// RegisterScenarioRoutes is called by the central server when mounting its API.
// Each handler uses the same Telegram authentication as the rest of the Mini App.
func (s *Server) RegisterScenarioRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/scenarios", s.scenarioHandler("list"))
	mux.Handle("GET /api/scenarios/{id}", s.scenarioHandler("get"))
	mux.Handle("POST /api/scenarios/preview", s.scenarioHandler("preview"))
	mux.Handle("POST /api/scenarios", s.scenarioHandler("save"))
	mux.Handle("POST /api/scenarios/{id}/activate", s.scenarioHandler("activate"))
}

func (s *Server) scenarioHandler(action string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, user, ok := s.authed(w, r)
		if !ok {
			return
		}
		planner, ok := s.Planner.(scenarioPlanner)
		if !ok {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		id := r.PathValue("id")
		if id != "" {
			if raw, err := hex.DecodeString(id); err != nil || len(raw) != 32 {
				paymentHTTPError(w, app.ErrPaymentInvalid)
				return
			}
		}
		var out any
		var err error
		switch action {
		case "list":
			var rows []app.PlanScenario
			rows, err = planner.Scenarios(ctx, user)
			// Originals remain server-side; history returns only user-facing metadata.
			items := make([]scenarioListItem, 0, len(rows))
			for _, row := range rows {
				items = append(items, scenarioListItem{ID: row.ID, Changes: row.Changes, Currency: row.Budget.Currency, AsOf: row.Original.Input.ValuationDate.String()})
			}
			out = map[string]any{"scenarios": items}
		case "get":
			out, err = planner.Scenario(ctx, user, id)
		case "preview", "save":
			var c app.ScenarioCommand
			if !decodeScenario(w, r, &c) {
				return
			}
			if c.VersionID != "" {
				if _, err := uuid.Parse(c.VersionID); err != nil {
					paymentHTTPError(w, app.ErrPaymentInvalid)
					return
				}
			}
			if action == "preview" {
				out, err = planner.PreviewScenario(ctx, user, c)
			} else {
				out, err = planner.SaveScenario(ctx, user, c)
			}
		case "activate":
			var c app.ScenarioActivationCommand
			if !decodeScenario(w, r, &c) {
				return
			}
			if c.ID != "" && c.ID != id {
				paymentHTTPError(w, app.ErrPaymentInvalid)
				return
			}
			c.ID = id
			out, err = planner.ActivateScenario(ctx, user, c)
		}
		if err != nil {
			scenarioHTTPError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
}

func decodeScenario(w http.ResponseWriter, r *http.Request, out any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequest))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		paymentHTTPError(w, app.ErrPaymentInvalid)
		return false
	}
	if err := d.Decode(new(any)); err != io.EOF {
		paymentHTTPError(w, app.ErrPaymentInvalid)
		return false
	}
	return true
}

func scenarioHTTPError(w http.ResponseWriter, err error) {
	var unsupported *plan.UnsupportedError
	var infeasible *plan.InfeasibleError
	var stale *plan.StaleBalanceError
	switch {
	case errors.As(err, &unsupported):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{jsonError: "unsupported", "reason": unsupported.Feature})
	case errors.As(err, &infeasible):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{jsonError: "infeasible"})
	case errors.As(err, &stale):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{jsonError: "stale_balance"})
	case errors.Is(err, app.ErrHistoricalEngine):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{jsonError: "historical_engine"})
	default:
		paymentHTTPError(w, err)
	}
}
