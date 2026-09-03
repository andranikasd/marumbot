package miniapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andranikasd/marumbot/internal/app"
)

type planActualsHTTPFake struct {
	app.Planner
	user, month string
	calls       int
	err         error
}

func (f *planActualsHTTPFake) ActivePlanActuals(_ context.Context, user, month string) ([]app.PlanActualComparison, error) {
	f.user = user
	f.month = month
	f.calls++
	return []app.PlanActualComparison{{PlanID: "active-version", Currency: "AMD", Rows: []app.PlanActualRow{{LoanID: "loan", PlannedMinor: "12507940", Causes: []app.VarianceCause{app.VarianceMissing}}}}}, f.err
}

func TestPlanActualsHTTPAuthenticationBoundsAndCoverage(t *testing.T) {
	s := budgetTestServer(nil)
	f := &planActualsHTTPFake{}
	s.Planner = f
	s.Payments = &app.PaymentService{Clock: s.Clock, Users: s.Users}
	for _, tt := range []struct {
		month  string
		auth   bool
		status int
	}{
		{"2026-09", false, 401}, {"2026-13", true, 422}, {"2026-09-01", true, 422}, {"2026-09", true, 200},
	} {
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/plan-actuals?month="+tt.month, nil)
		if tt.auth {
			request.Header.Set("X-Telegram-Init-Data", knownInitData())
		}
		response := httptest.NewRecorder()
		s.Handler().ServeHTTP(response, request)
		if response.Code != tt.status {
			t.Fatalf("status: %d %s", response.Code, response.Body.String())
		}
		if tt.status == 200 {
			for _, part := range []string{`"basis":"reported_value_date"`, `"activation_boundary":"after_activation_day_and_transfer_date"`, `"posted_minor":null`, `"fee_delta_minor":null`, `"causes":["missing"]`} {
				if !strings.Contains(response.Body.String(), part) {
					t.Fatalf("missing %s: %s", part, response.Body.String())
				}
			}
		}
	}
	if f.calls != 1 || f.user != "user-id" || f.month != "2026-09" {
		t.Fatalf("scope: %+v", f)
	}
	for _, err := range []error{app.ErrHistoricalEngine, app.ErrPlanActualsCoverage} {
		f.err = err
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/plan-actuals?month=2026-09", nil)
		request.Header.Set("X-Telegram-Init-Data", knownInitData())
		response := httptest.NewRecorder()
		s.Handler().ServeHTTP(response, request)
		if response.Code != 422 {
			t.Fatal("unavailable history did not fail closed")
		}
		var body map[string]string
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body["error"] != "plan_actuals_unavailable" {
			t.Fatal(body)
		}
	}
}
