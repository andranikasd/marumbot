package miniapp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/pkg/core/projection"
)

const projectionUnavailable = "projection_unavailable"

func (s *Server) ProjectionSettings() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, user, ok := s.authed(w, r)
		if !ok {
			return
		}
		reader, ok := s.Budgets.(app.ProjectionReader)
		if !ok {
			writeJSON(w, 503, map[string]string{jsonError: projectionUnavailable})
			return
		}
		if r.Method == http.MethodGet {
			settings, err := reader.ProjectionSettings(ctx, user)
			if err != nil {
				writeJSON(w, 503, map[string]string{jsonError: projectionUnavailable})
				return
			}
			writeJSON(w, 200, settings)
			return
		}
		store, ok := s.Budgets.(app.BudgetCommandStore)
		if !ok {
			writeJSON(w, 503, map[string]string{jsonError: projectionUnavailable})
			return
		}
		var body struct {
			Enabled         *bool  `json:"enabled"`
			ExpectedVersion *int64 `json:"expected_version"`
			Key             string `json:"idempotency_key"`
			Currencies      map[string]struct {
				Minor     *int64           `json:"extra_minor"`
				Overrides map[string]int64 `json:"overrides"`
			} `json:"currencies"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
		dec.DisallowUnknownFields()
		var tail any
		if dec.Decode(&body) != nil || dec.Decode(&tail) != io.EOF || body.Enabled == nil || body.ExpectedVersion == nil || body.Currencies == nil {
			writeJSON(w, 422, map[string]string{jsonError: "invalid_projection_settings"})
			return
		}
		in := app.ProjectionUpdate{Enabled: *body.Enabled, ExpectedVersion: *body.ExpectedVersion, Key: body.Key, Currencies: map[string]projection.Extra{}}
		for code, x := range body.Currencies {
			if x.Minor == nil {
				writeJSON(w, 422, map[string]string{jsonError: "invalid_projection_settings"})
				return
			}
			in.Currencies[code] = projection.Extra{Minor: *x.Minor, Overrides: x.Overrides}
		}
		version, err := (app.BudgetCommands{Store: store, Clock: s.Clock, Users: s.Users}).SetProjectionSettings(ctx, user, in)
		switch {
		case errors.Is(err, app.ErrConflict):
			writeJSON(w, 409, map[string]string{jsonError: "conflict"})
		case errors.Is(err, app.ErrPaymentInvalid):
			writeJSON(w, 422, map[string]string{jsonError: "invalid_projection_settings"})
		case err != nil:
			writeJSON(w, 503, map[string]string{jsonError: projectionUnavailable})
		default:
			writeJSON(w, 200, map[string]any{"saved": true, "version": version})
		}
	}
}

func (s *Server) Projection() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, user, ok := s.authed(w, r)
		if !ok {
			return
		}
		reader, ok := s.Budgets.(app.ProjectionReader)
		if !ok || s.Reader == nil {
			writeJSON(w, 503, map[string]string{jsonError: projectionUnavailable})
			return
		}
		result, err := (app.ProjectionService{Settings: reader, Loans: s.Reader, Clock: s.Clock, Users: s.Users}).Build(ctx, user)
		switch {
		case errors.Is(err, app.ErrPaymentReconciliation):
			writeJSON(w, 422, map[string]string{jsonError: errorPaymentReconciliation})
		case errors.Is(err, projection.ErrInvalid), errors.Is(err, app.ErrPaymentInvalid):
			writeJSON(w, 422, map[string]string{jsonError: "projection_source_invalid"})
		case err != nil:
			writeJSON(w, 503, map[string]string{jsonError: projectionUnavailable})
		default:
			writeJSON(w, 200, result)
		}
	}
}
