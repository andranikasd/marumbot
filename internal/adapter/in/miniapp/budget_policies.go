package miniapp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/pkg/core/money"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

// BudgetPolicies returns a separate declaration document. Register GET and
// POST /api/budget/policies with these methods in Server.Handler.
func (s *Server) BudgetPolicies() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, userID, ok := s.authed(w, r)
		if !ok {
			return
		}
		b, err := s.Budgets.Budget(ctx, userID)
		if err != nil {
			http.Error(w, "budget unavailable", http.StatusServiceUnavailable)
			return
		}
		today, err := (app.PaymentService{Clock: s.Clock, Users: s.Users}).BusinessDate(ctx, userID)
		if err != nil {
			http.Error(w, "date unavailable", http.StatusServiceUnavailable)
			return
		}
		cur, err := money.Lookup(b.Currency)
		if err != nil {
			http.Error(w, "set a budget first", http.StatusConflict)
			return
		}
		permission, err := b.PermissionOn(today)
		if err != nil {
			http.Error(w, "invalid budget policy", http.StatusUnprocessableEntity)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(struct {
			Currency         string             `json:"currency"`
			Version          int64              `json:"version"`
			Today            string             `json:"today"`
			ExplicitFunding  bool               `json:"explicit_funding"`
			CurrencyExponent int                `json:"currency_exponent"`
			ActiveLimitMinor int64              `json:"active_limit_minor"`
			MonthlyMinor     int64              `json:"monthly_minor"`
			Policies         []app.BudgetPolicy `json:"policies"`
		}{b.Currency, b.Version, today.String(), b.Funding != nil, int(cur.Exponent), permission.Minor(), b.Monthly.Minor(), b.Policies})
	}
}

func (s *Server) SetBudgetPolicy() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, userID, ok := s.authed(w, r)
		if !ok {
			return
		}
		store, ok := s.Budgets.(app.BudgetCommandStore)
		if !ok {
			http.Error(w, "budget policies unavailable", http.StatusServiceUnavailable)
			return
		}
		var in struct {
			Key             string           `json:"idempotency_key"`
			Currency        string           `json:"currency"`
			ExpectedVersion int64            `json:"expected_version"`
			Policy          app.BudgetPolicy `json:"policy"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&in); err != nil {
			http.Error(w, "invalid policy", http.StatusUnprocessableEntity)
			return
		}
		var trailing any
		if decoder.Decode(&trailing) != io.EOF {
			http.Error(w, "invalid policy", http.StatusUnprocessableEntity)
			return
		}
		version, err := (app.BudgetCommands{Store: store, Clock: s.Clock, Users: s.Users}).SavePolicy(ctx, userID, in.Currency, in.ExpectedVersion, in.Key, in.Policy)
		if errors.Is(err, app.ErrConflict) {
			http.Error(w, "budget changed; reload", http.StatusConflict)
			return
		}
		var unsupported *plan.UnsupportedError
		if errors.As(err, &unsupported) {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{jsonError: "unsupported", "reason": unsupported.Feature})
			return
		}
		if err != nil {
			http.Error(w, "invalid or unsupported policy", http.StatusUnprocessableEntity)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]int64{"version": version})
	}
}
