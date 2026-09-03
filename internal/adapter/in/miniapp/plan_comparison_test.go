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

type comparisonPlannerFake struct{ owner, proposal string }

func (f *comparisonPlannerFake) PlanSheet(context.Context, string, *plan.Goal) (app.Sheet, error) {
	return app.Sheet{}, nil
}

func (f *comparisonPlannerFake) ApprovePlanFor(context.Context, string, plan.Goal) (app.ApprovedPlan, error) {
	return app.ApprovedPlan{}, nil
}

func (f *comparisonPlannerFake) PlanComparisons(_ context.Context, user, proposal string) (app.PlanComparisonSheet, error) {
	f.owner = user
	f.proposal = proposal
	return app.PlanComparisonSheet{Proposal: proposal, ActiveRevision: 4}, nil
}

func TestPlanComparisonsHandlerUsesAuthenticatedProposal(t *testing.T) {
	server := budgetTestServer(nil)
	fake := &comparisonPlannerFake{}
	server.Planner = fake
	handler := server.planComparisons()
	proposal := strings.Repeat("a", 64)
	for _, tc := range []struct {
		auth     bool
		proposal string
		want     int
	}{{false, proposal, 401}, {true, "invalid", 422}, {true, proposal, 200}} {
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/plan/comparisons?proposal="+tc.proposal, nil)
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
