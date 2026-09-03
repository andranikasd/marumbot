package miniapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

type scenarioHTTPFake struct {
	app.Planner
	calls   int
	user    string
	err     error
	command app.ScenarioActivationCommand
}

func (f *scenarioHTTPFake) PlanSheet(context.Context, string, *plan.Goal) (app.Sheet, error) {
	return app.Sheet{}, nil
}

func (f *scenarioHTTPFake) PreviewScenario(_ context.Context, user string, _ app.ScenarioCommand) (app.ScenarioView, error) {
	f.calls++
	f.user = user
	return app.ScenarioView{}, f.err
}

func (f *scenarioHTTPFake) SaveScenario(ctx context.Context, user string, c app.ScenarioCommand) (app.ScenarioView, error) {
	return f.PreviewScenario(ctx, user, c)
}

func (f *scenarioHTTPFake) Scenario(context.Context, string, string) (app.ScenarioView, error) {
	return app.ScenarioView{}, f.err
}

func (f *scenarioHTTPFake) Scenarios(context.Context, string) ([]app.PlanScenario, error) {
	return []app.PlanScenario{}, f.err
}

func (f *scenarioHTTPFake) ActivateScenario(_ context.Context, user string, c app.ScenarioActivationCommand) (app.PlanActivation, error) {
	f.calls++
	f.user = user
	f.command = c
	return app.PlanActivation{}, f.err
}

func TestScenarioHTTPBoundary(t *testing.T) {
	f := &scenarioHTTPFake{}
	s := budgetTestServer(nil)
	s.Planner = f
	mux := http.NewServeMux()
	s.RegisterScenarioRoutes(mux)
	for _, tc := range []struct {
		name, body string
		auth       bool
		status     int
	}{
		{"auth", `{}`, false, 401},
		{"unknown", `{"user_id":"victim"}`, true, 422},
		{"trailing", `{} {}`, true, 422},
		{"fraction", `{"changes":{"monthly_minor":1.1}}`, true, 422},
		{"valid", `{"proposal":"proposal","changes":{"monthly_minor":100}}`, true, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequestWithContext(t.Context(), "POST", "/api/scenarios/preview", strings.NewReader(tc.body))
			if tc.auth {
				r.Header.Set("X-Telegram-Init-Data", knownInitData())
			}
			out := httptest.NewRecorder()
			mux.ServeHTTP(out, r)
			if out.Code != tc.status {
				t.Fatalf("status %d: %s", out.Code, out.Body.String())
			}
		})
	}
	if f.calls != 1 || f.user != "user-id" {
		t.Fatal("untrusted identity reached application")
	}
	for _, err := range []error{&plan.UnsupportedError{Feature: "dated funding"}, &plan.InfeasibleError{}, app.ErrHistoricalEngine} {
		out := httptest.NewRecorder()
		scenarioHTTPError(out, err)
		if out.Code != 422 {
			t.Fatal("domain refusal lost", out.Code)
		}
	}
	out := httptest.NewRecorder()
	scenarioHTTPError(out, app.ErrConflict)
	if out.Code != 409 {
		t.Fatal("conflict status", out.Code)
	}
}
