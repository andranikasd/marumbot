package miniapp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andranikasd/marumbot/internal/app"
)

type accountStateUsers struct {
	budgetTestUsers
	err error
}

func (u accountStateUsers) ByTelegramTag(context.Context, string) (string, error) {
	return "user-id", u.err
}

type emptyAccountLoans struct{ app.LoanReader }

func (emptyAccountLoans) LoansForUser(context.Context, string, int32) ([]app.UserLoan, error) {
	return nil, nil
}

func TestMiniAppEmptyAccountAndLookupFailureAreDistinct(t *testing.T) {
	for _, test := range []struct {
		name   string
		err    error
		status int
		body   string
	}{
		{"empty account", nil, http.StatusOK, `"loans":[]`},
		{"not registered", app.ErrNotFound, http.StatusForbidden, `unknown account`},
		{"database unavailable", errors.New("database disconnected"), http.StatusServiceUnavailable, `unavailable`},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := budgetTestServer(nil)
			server.Users = accountStateUsers{err: test.err}
			server.Reader = emptyAccountLoans{}
			request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/loans", nil)
			request.Header.Set("X-Telegram-Init-Data", knownInitData())
			response := httptest.NewRecorder()
			server.Handler().ServeHTTP(response, request)
			if response.Code != test.status || !strings.Contains(response.Body.String(), test.body) {
				t.Fatal(response.Code, response.Body.String())
			}
			if strings.Contains(response.Body.String(), "disconnected") {
				t.Fatal("internal error leaked")
			}
		})
	}
}

type accountStateBudget struct {
	app.BudgetStore
	err error
}

func (b accountStateBudget) Budget(context.Context, string) (app.Budget, error) {
	return app.Budget{}, b.err
}

func TestBudgetFailureDoesNotBecomeAnEmptyBudget(t *testing.T) {
	for _, unavailable := range []bool{false, true} {
		server := budgetTestServer(nil)
		var err error
		if unavailable {
			err = errors.New("database disconnected")
		}
		server.Budgets = accountStateBudget{err: err}
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/budget", nil)
		request.Header.Set("X-Telegram-Init-Data", knownInitData())
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		want := http.StatusOK
		if unavailable {
			want = http.StatusServiceUnavailable
		}
		if response.Code != want {
			t.Fatal(response.Code, response.Body.String())
		}
		if !unavailable && strings.Contains(response.Body.String(), "unavailable") {
			t.Fatal("empty budget is not a failure")
		}
	}
}
