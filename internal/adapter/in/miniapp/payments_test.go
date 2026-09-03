package miniapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

type paymentContextFake struct{ owner, loan string }

func (p *paymentContextFake) PaymentContext(_ context.Context, loan, owner string) (app.PaymentContext, error) {
	p.owner, p.loan = owner, loan
	return app.PaymentContext{LoanID: loan, Version: 3, Currency: "AMD", CurrencyExponent: 2}, nil
}

func TestPaymentEndpointsRequireAuthenticationAndOwnership(t *testing.T) {
	server := budgetTestServer(nil)
	server.Payments = &app.PaymentService{Clock: server.Clock, Users: server.Users}
	reader := &paymentContextFake{}
	server.PaymentReader = reader
	const loan = "deeb7199-21ea-4436-91cc-d093e5ce3c32"
	path := "/api/loans/" + loan + "/payments"
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), method, path, nil))
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous %s: %d", method, response.Code)
		}
	}
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
	request.Header.Set("X-Telegram-Init-Data", knownInitData())
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || reader.owner != "user-id" || reader.loan != loan {
		t.Fatalf("owner scope missing: %d %+v", response.Code, reader)
	}
}

func TestPaymentRequestRejectsUnknownAndTrailingJSON(t *testing.T) {
	server := budgetTestServer(nil)
	server.Payments = &app.PaymentService{}
	for _, body := range []string{`{"trust":"bank_confirmed"}`, `{} {}`, `{"amount_minor":1.5}`} {
		request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/loans/deeb7199-21ea-4436-91cc-d093e5ce3c32/payments", strings.NewReader(body))
		request.Header.Set("X-Telegram-Init-Data", knownInitData())
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("invalid body accepted: %d", response.Code)
		}
	}
}

func TestPaymentBusinessDateUsesAccountTimezone(t *testing.T) {
	service := app.PaymentService{Clock: budgetTestClock{now: time.Date(2026, time.September, 2, 21, 0, 0, 0, time.UTC)}, Users: budgetTestUsers{}}
	today, err := service.BusinessDate(t.Context(), "user")
	if err != nil || today.String() != "2026-09-03" {
		t.Fatalf("Armenian business date: %s %v", today, err)
	}
}

type paymentLoansFake struct{}

func (paymentLoansFake) LoansForUser(context.Context, string, int32) ([]app.UserLoan, error) {
	return []app.UserLoan{{UnreconciledPayments: true, Contract: model.Contract{Currency: money.MustLookup("AMD")}, Balance: money.FromMinor(100, money.MustLookup("AMD"))}}, nil
}

func TestLoanResponseDoesNotProjectUnreconciledPayment(t *testing.T) {
	server := budgetTestServer(nil)
	server.Reader = paymentLoansFake{}
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/loans", nil)
	request.Header.Set("X-Telegram-Init-Data", knownInitData())
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"needs_reconciliation":true`) {
		t.Fatal("missing payment warning")
	}
	if strings.Contains(response.Body.String(), `"next_due"`) || strings.Contains(response.Body.String(), `"next_payment_major"`) {
		t.Fatal("stale projection exposed")
	}
}
