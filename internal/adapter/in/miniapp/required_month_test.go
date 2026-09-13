package miniapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/money"
)

type zeroMonthRequired struct{}

func (zeroMonthRequired) RequiredThisMonth(context.Context, string) (money.Amount, money.Currency, error) {
	cur := money.MustLookup("USD")
	return money.Zero(cur), cur, nil
}

func TestBudgetShowsConfirmedZeroRequired(t *testing.T) {
	s := budgetTestServer(nil)
	s.Budgets = &countedBudgetReader{value: budgetReadFixture()}
	s.Required = zeroMonthRequired{}
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/budget", nil)
	req.Header.Set("X-Telegram-Init-Data", knownInitData())
	res := httptest.NewRecorder()
	s.getBudget().ServeHTTP(res, req)
	var body map[string]json.RawMessage
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if res.Code != http.StatusOK || string(body["required_major"]) != "0" {
		t.Fatalf("confirmed zero must differ from unknown: %d %s", res.Code, res.Body.String())
	}
}
