package miniapp

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/andranikasd/marumbot/internal/app"
)

type fundingUnavailableStore struct{ app.BudgetStore }

func (fundingUnavailableStore) BeginBudgetCommand(context.Context) (app.BudgetCommandTransaction, error) {
	return nil, io.ErrUnexpectedEOF
}

func TestBudgetFundingTransientFailureRemainsRetryable(t *testing.T) {
	s := budgetTestServer(nil)
	s.Budgets = fundingUnavailableStore{}
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"transient", `{"idempotency_key":"stable-command-key","currency":"USD","expected_version":1,"pay_day":10,"monthly_minor":0,"events":[]}`, http.StatusServiceUnavailable},
		{"invalid key", `{"idempotency_key":"short","currency":"USD","expected_version":1,"pay_day":10,"monthly_minor":0,"events":[]}`, http.StatusUnprocessableEntity},
		{"unknown route field", `{"idempotency_key":"stable-command-key","currency":"USD","expected_version":1,"pay_day":10,"monthly_minor":0,"events":[{"routing":{"other_loan":"x"}}]}`, http.StatusUnprocessableEntity},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/budget/funding", bytes.NewBufferString(tc.body))
			r.Header.Set("X-Telegram-Init-Data", knownInitData())
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("got %d want %d: %s", w.Code, tc.status, w.Body.String())
			}
		})
	}
}
