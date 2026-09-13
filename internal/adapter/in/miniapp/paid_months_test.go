package miniapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

type paidMonthsBoundaryStore struct {
	loanCommandBoundaryStore
	user, loan string
}

func (s *paidMonthsBoundaryStore) LoanCommandCurrency(_ context.Context, id, user string) (money.Currency, error) {
	s.user, s.loan = user, id
	return money.MustLookup("AMD"), nil
}

func (*paidMonthsBoundaryStore) BeginLoanCommand(context.Context) (app.LoanCommandTransaction, error) {
	panic("invalid input reached mutation transaction")
}

type paidMonthsEditor struct {
	app.LoanEditor
	user, loan string
	err        error
}

func (e *paidMonthsEditor) LoanForUser(_ context.Context, id, user string) (app.UserLoan, error) {
	e.user, e.loan = user, id
	cur := money.MustLookup("AMD")
	return app.UserLoan{Name: "Loan", Balance: money.FromMinor(60000, cur), Contract: model.Contract{Currency: cur, StartDate: date.MustNew(2026, 1, 1), MaturityDate: date.MustNew(2027, 12, 15), PaymentDay: 15}}, e.err
}

func TestPaidMonthHTTPAuthenticationAndOwnerScope(t *testing.T) {
	server := budgetTestServer(nil)
	editor := &paidMonthsEditor{}
	server.Editor = editor
	const path = "/api/loans/deeb7199-21ea-4436-91cc-d093e5ce3c32/paid-months"
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		out := httptest.NewRecorder()
		server.Handler().ServeHTTP(out, httptest.NewRequestWithContext(t.Context(), method, path, nil))
		if out.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous %s accepted: %d", method, out.Code)
		}
	}
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
	req.Header.Set("X-Telegram-Init-Data", knownInitData())
	out := httptest.NewRecorder()
	server.Handler().ServeHTTP(out, req)
	if out.Code != http.StatusOK || editor.user != "user-id" || editor.loan != "deeb7199-21ea-4436-91cc-d093e5ce3c32" {
		t.Fatal("metadata not scoped to authenticated borrower", out.Code)
	}
	editor.err = app.ErrNotFound
	out = httptest.NewRecorder()
	server.Handler().ServeHTTP(out, req)
	if out.Code != http.StatusNotFound {
		t.Fatal("ownership error not concealed", out.Code)
	}
}

func TestPaidMonthHTTPRejectsMalformedAndUnsafeAmounts(t *testing.T) {
	server := budgetTestServer(nil)
	store := &paidMonthsBoundaryStore{}
	server.Loans = store
	for _, tt := range []struct {
		name, body string
		status     int
	}{
		{"unknown field", `{"trust":"bank_confirmed"}`, http.StatusBadRequest},
		{"multiple objects", `{} {}`, http.StatusBadRequest},
		{"oversize", `{"month":"` + strings.Repeat("x", 2048) + `"}`, http.StatusBadRequest},
		{"null body", `null`, http.StatusUnprocessableEntity},
		{"missing amounts", `{}`, http.StatusUnprocessableEntity},
		{"null amount", `{"balance_major":null,"payment_major":0}`, http.StatusUnprocessableEntity},
		{"excess decimal", `{"balance_major":1.001,"payment_major":1}`, http.StatusUnprocessableEntity},
		{"negative", `{"balance_major":-1,"payment_major":1}`, http.StatusUnprocessableEntity},
		{"unsafe precision", `{"balance_major":90071992547409.92,"payment_major":1}`, http.StatusUnprocessableEntity},
		{"missing payment", `{"balance_major":0}`, http.StatusUnprocessableEntity},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/loans/deeb7199-21ea-4436-91cc-d093e5ce3c32/paid-months", strings.NewReader(tt.body))
			req.Header.Set("X-Telegram-Init-Data", knownInitData())
			req.Header.Set("Idempotency-Key", "test-paid-months-0001")
			req.Header.Set("If-Match", "1")
			out := httptest.NewRecorder()
			server.Handler().ServeHTTP(out, req)
			if out.Code != tt.status {
				t.Fatalf("status=%d want=%d body=%s", out.Code, tt.status, out.Body.String())
			}
			if store.user != "user-id" {
				t.Fatal("amount currency lookup not owner scoped")
			}
		})
	}
}
