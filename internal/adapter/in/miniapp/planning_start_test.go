package miniapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/pkg/core/amortisation"
)

func TestPlanningStartTransientAndMalformedRequests(t *testing.T) {
	s := budgetTestServer(nil)
	s.Budgets = fundingUnavailableStore{}
	for _, tc := range []struct {
		body   string
		status int
	}{
		{`{"idempotency_key":"stable-command-key","expected_version":1,"planning_start_month":"2026-10"}`, http.StatusServiceUnavailable},
		{`{"idempotency_key":"short","expected_version":1,"planning_start_month":"2026-10"}`, http.StatusUnprocessableEntity},
		{`{"idempotency_key":"stable-command-key","expected_version":1,"planning_start_month":"2026-10","spent_minor":0}`, http.StatusUnprocessableEntity},
		{`{"idempotency_key":"stable-command-key"} {}`, http.StatusUnprocessableEntity},
	} {
		r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/budget/planning-start", bytes.NewBufferString(tc.body))
		r.Header.Set("X-Telegram-Init-Data", knownInitData())
		w := httptest.NewRecorder()
		s.SetPlanningStart().ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("got %d want %d: %s", w.Code, tc.status, w.Body.String())
		}
	}
}

type planningStartFailureStore struct {
	app.BudgetStore
	err error
}

func (s planningStartFailureStore) BeginBudgetCommand(context.Context) (app.BudgetCommandTransaction, error) {
	return nil, s.err
}

func TestPlanningStartDeterministicRefusalsPermitCorrection(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code string
	}{
		{fmt.Errorf("wrapped: %w", app.ErrPaymentReconciliation), "payment_reconciliation_required"},
		{fmt.Errorf("wrapped: %w", amortisation.ErrUnsolvable), "loan_schedule_invalid"},
	} {
		s := budgetTestServer(nil)
		s.Budgets = planningStartFailureStore{err: tc.err}
		r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/budget/planning-start", bytes.NewBufferString(`{"idempotency_key":"stable-command-key","expected_version":1,"planning_start_month":"2026-10"}`))
		r.Header.Set("X-Telegram-Init-Data", knownInitData())
		w := httptest.NewRecorder()
		s.SetPlanningStart().ServeHTTP(w, r)
		var body map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if w.Code != http.StatusUnprocessableEntity || body["error"] != tc.code {
			t.Fatalf("got %d %v", w.Code, body)
		}
	}
}
