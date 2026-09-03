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

type stressPlannerFake struct{ owner, proposal string }

func (f *stressPlannerFake) PlanSheet(context.Context, string, *plan.Goal) (app.Sheet, error) {
	return app.Sheet{}, nil
}

func (f *stressPlannerFake) ApprovePlanFor(context.Context, string, plan.Goal) (app.ApprovedPlan, error) {
	return app.ApprovedPlan{}, nil
}

func (f *stressPlannerFake) PlanStress(_ context.Context, user, proposal string, _ int64) (app.PlanStressSheet, error) {
	f.owner = user
	f.proposal = proposal
	return app.PlanStressSheet{Proposal: proposal, RequiredIncreaseBP: 0}, nil
}

func TestPlanStressHandlerUsesAuthenticatedProposal(t *testing.T) {
	server := budgetTestServer(nil)
	fake := &stressPlannerFake{}
	server.Planner = fake
	handler := server.planStress()
	proposal := strings.Repeat("a", 64)
	for _, tc := range []struct {
		auth     bool
		proposal string
		want     int
	}{{false, proposal, 401}, {true, "invalid", 422}, {true, proposal, 200}} {
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/plan/stress?proposal="+tc.proposal, nil)
		if tc.auth {
			request.Header.Set("X-Telegram-Init-Data", knownInitData())
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != tc.want {
			t.Fatalf("status %d want %d: %s", response.Code, tc.want, response.Body)
		}
		if response.Code == 200 && (fake.owner != "user-id" || fake.proposal != proposal || response.Header().Get("Cache-Control") != "no-store") {
			t.Fatal("missing ownership or no-store")
		}
	}
}
