package miniapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/andranikasd/marumbot/internal/app"
)

type reconciliationBoundaryStore struct{ calls int }

func (s *reconciliationBoundaryStore) BeginPayment(context.Context) (app.PaymentTransaction, error) {
	s.calls++
	return nil, errors.New("transaction boundary reached")
}

func TestReconciliationRequiresExplicitAmountsAndVersions(t *testing.T) {
	base := map[string]any{
		"idempotency_key": "reconcile-zero-boundary", "as_of": "2026-01-01",
		"expected_version": 0, "budget_version": 1, "include_posted": true,
		"principal_minor": 0, "next_payment_minor": 0, "cash_minor": 0, "spent_minor": 0,
	}
	post := func(t *testing.T, body map[string]any) (int, int) {
		t.Helper()
		store := &reconciliationBoundaryStore{}
		server := budgetTestServer(nil)
		server.Payments = &app.PaymentService{Clock: server.Clock, Users: server.Users, Store: store}
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/loans/deeb7199-21ea-4436-91cc-d093e5ce3c32/reconcile", bytes.NewReader(raw))
		r.Header.Set("X-Telegram-Init-Data", knownInitData())
		w := httptest.NewRecorder()
		server.Handler().ServeHTTP(w, r)
		return w.Code, store.calls
	}
	for _, field := range []string{"principal_minor", "next_payment_minor", "cash_minor", "spent_minor", "expected_version", "budget_version"} {
		for _, shape := range []string{"missing", "null"} {
			t.Run(field+"/"+shape, func(t *testing.T) {
				body := make(map[string]any, len(base))
				for k, v := range base {
					body[k] = v
				}
				if shape == "missing" {
					delete(body, field)
				} else {
					body[field] = nil
				}
				if status, calls := post(t, body); status != http.StatusUnprocessableEntity || calls != 0 {
					t.Fatalf("incomplete statement: status=%d transactions=%d", status, calls)
				}
			})
		}
	}
	// Explicit zero balances still support closing a repaid loan. The fake
	// store deliberately fails after validation to avoid inventing ledger data.
	if status, calls := post(t, base); status != http.StatusInternalServerError || calls != 1 {
		t.Fatalf("explicit zero rejected before transaction: status=%d transactions=%d", status, calls)
	}
}
