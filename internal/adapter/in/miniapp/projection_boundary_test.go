package miniapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/pkg/core/projection"
)

type projectionBoundaryStore struct{ app.BudgetStore }

func (*projectionBoundaryStore) ProjectionSettings(context.Context, string) (app.ProjectionSettings, error) {
	return app.ProjectionSettings{Currencies: map[string]projection.Extra{}}, nil
}

func (*projectionBoundaryStore) BeginBudgetCommand(context.Context) (app.BudgetCommandTransaction, error) {
	panic("malformed request reached mutation")
}

func TestProjectionHTTPRequiresExplicitSource(t *testing.T) {
	s := budgetTestServer(nil)
	s.Budgets = &projectionBoundaryStore{}
	for _, body := range []string{`null`, `{}`, `{"enabled":true,"expected_version":0,"idempotency_key":"test-source-123456","currencies":{"AMD":{}}}`, `{"enabled":true,"expected_version":0,"idempotency_key":"test-source-123456","currencies":{"AMD":{"extra_minor":null}}}`, `{"enabled":true,"expected_version":0,"currencies":{},"user_id":"other"}`, `{} {}`, `{"enabled":true,"expected_version":0,"currencies":null}`} {
		r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/projection/settings", strings.NewReader(body))
		r.Header.Set("X-Telegram-Init-Data", knownInitData())
		out := httptest.NewRecorder()
		s.Handler().ServeHTTP(out, r)
		if out.Code != 422 {
			t.Fatalf("missing/extra source accepted %d %s", out.Code, body)
		}
	}
}
