package miniapp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/pkg/core/projection"
)

type projectionSettingsFake struct {
	app.BudgetStore
	err   error
	calls int
	user  string
}

func (f *projectionSettingsFake) ProjectionSettings(_ context.Context, user string) (app.ProjectionSettings, error) {
	f.calls++
	f.user = user
	return app.ProjectionSettings{Currencies: map[string]projection.Extra{}}, f.err
}

func TestProjectionFirstUseAndAccountIsolation(t *testing.T) {
	s := budgetTestServer(nil)
	f := &projectionSettingsFake{}
	s.Budgets = f
	s.Reader = emptyAccountLoans{}
	for _, path := range []string{"/api/projection/settings", "/api/projection"} {
		r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != 401 || f.calls != 0 {
			t.Fatal("anonymous projection read")
		}
	}
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/projection?user=another-account", nil)
	r.Header.Set("X-Telegram-Init-Data", knownInitData())
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	var out app.ProjectionResult
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || out.Enabled || len(out.Currencies) != 0 || f.user != "user-id" {
		t.Fatalf("first-use projection: %d %s", w.Code, w.Body)
	}
	f.err = errors.New("storage offline")
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 503 {
		t.Fatal("outage represented as empty plan")
	}
}
