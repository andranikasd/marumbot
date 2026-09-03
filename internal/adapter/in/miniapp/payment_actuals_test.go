package miniapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andranikasd/marumbot/internal/app"
)

type actualsReaderFake struct {
	app.LoanEditor
	user, month, cursor string
	calls               int
}

func (f *actualsReaderFake) MonthlyPaymentActuals(_ context.Context, user, month string) ([]app.PaymentActuals, error) {
	f.user = user
	f.month = month
	f.calls++
	return []app.PaymentActuals{{Currency: "AMD", CurrencyExponent: 2, PaymentCount: 1, UnknownCount: 1, PendingCount: 1, PaidMinor: "12507940"}}, nil
}

func (f *actualsReaderFake) BorrowerAllocatedActivity(_ context.Context, user, cursor string) ([]app.AllocatedActivityFact, error) {
	f.user = user
	f.cursor = cursor
	f.calls++
	return []app.AllocatedActivityFact{{ActivityFact: app.ActivityFact{ID: "payment"}}}, nil
}

func TestPaymentActualsAuthenticationMonthAndUnknownCoverage(t *testing.T) {
	s := budgetTestServer(nil)
	reader := &actualsReaderFake{}
	s.Editor = reader
	s.Payments = &app.PaymentService{Clock: s.Clock, Users: s.Users}
	for _, tt := range []struct {
		path   string
		auth   bool
		status int
	}{
		{"/api/payment-actuals?month=2026-09", false, 401},
		{"/api/payment-actuals?month=2026-13", true, 422},
		{"/api/payment-actuals?month=2026-09-01", true, 422},
		{"/api/payment-actuals?month=2026-09", true, 200},
	} {
		r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, tt.path, nil)
		if tt.auth {
			r.Header.Set("X-Telegram-Init-Data", knownInitData())
		}
		w := httptest.NewRecorder()
		s.paymentActuals().ServeHTTP(w, r)
		if w.Code != tt.status {
			t.Fatalf("status %d: %s", w.Code, w.Body.String())
		}
		if tt.status == 200 {
			for _, part := range []string{`"basis":"transaction_date"`, `"unknown_count":1`, `"interest_minor":null`, `"fees_minor":null`, `"paid_minor":"12507940"`} {
				if !strings.Contains(w.Body.String(), part) {
					t.Fatalf("missing %s: %s", part, w.Body.String())
				}
			}
		}
	}
	if reader.calls != 1 || reader.user != "user-id" || reader.month != "2026-09" {
		t.Fatalf("scope: %+v", reader)
	}
}

func TestPaymentAllocatedActivityAuthenticationAndCursor(t *testing.T) {
	s := budgetTestServer(nil)
	reader := &actualsReaderFake{}
	s.Editor = reader
	for _, tt := range []struct {
		path   string
		auth   bool
		status int
	}{
		{"/api/activity", false, 401}, {"/api/activity?after=bad", true, 400}, {"/api/activity", true, 200},
	} {
		r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, tt.path, nil)
		if tt.auth {
			r.Header.Set("X-Telegram-Init-Data", knownInitData())
		}
		w := httptest.NewRecorder()
		s.allocatedActivity().ServeHTTP(w, r)
		if w.Code != tt.status {
			t.Fatalf("status: %d", w.Code)
		}
		if tt.status == 200 && !strings.Contains(w.Body.String(), `"allocation":null`) {
			t.Fatal("unknown allocation was lost")
		}
	}
	if reader.calls != 1 || reader.user != "user-id" {
		t.Fatalf("scope: %+v", reader)
	}
}
