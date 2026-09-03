package miniapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

type countedBudgetReader struct {
	app.BudgetStore
	value app.Budget
	calls int
}

func (f *countedBudgetReader) Budget(context.Context, string) (app.Budget, error) {
	f.calls++
	return f.value, nil
}

type countedBudgetUsers struct {
	budgetTestUsers
	identityReads, localeReads int
}

func (f *countedBudgetUsers) ByTelegramTag(ctx context.Context, tag string) (string, error) {
	f.identityReads++
	return f.budgetTestUsers.ByTelegramTag(ctx, tag)
}

func (f *countedBudgetUsers) Locale(ctx context.Context, user string) (string, string, error) {
	f.localeReads++
	return f.budgetTestUsers.Locale(ctx, user)
}

type countedBudgetRequired struct{ calls int }

func (f *countedBudgetRequired) RequiredThisMonth(context.Context, string) (money.Amount, money.Currency, error) {
	f.calls++
	cur := money.MustLookup("USD")
	return money.FromMinor(300, cur), cur, nil
}

func budgetReadFixture() app.Budget {
	cur := money.MustLookup("USD")
	fixed, capMinor, delta := int64(200), int64(1800), int64(-100)
	return app.Budget{
		Set: true, Version: 7, Currency: "USD", Monthly: money.FromMinor(1000, cur), PayDay: 1,
		Opening: money.FromMinor(700, cur), OpeningAsOf: date.MustNew(2026, 1, 1), Reserve: money.FromMinor(100, cur),
		Funding: &app.BudgetFunding{MonthlyMinor: 500, SpentMinor: 400, CashThrough: "2026-01-01", Events: []app.BudgetCashEvent{{On: "2026-01-01", Minor: 500}, {On: "2026-01-20", Minor: 900, Expected: true}}},
		Policies: []app.BudgetPolicy{{
			Version: 7, EffectiveFrom: "2026-01-01", MonthlyMinor: 1500, CarryRule: "carry_cash", ReleasedPaymentRule: "roll_all",
			Growth:      &app.BudgetPolicyGrowth{EveryMonths: 12, StartsOn: "2026-01-01", FixedMinor: &fixed, MaximumMinor: &capMinor},
			Adjustments: []app.BudgetPolicyAdjustment{{Month: "2026-01", DeltaMinor: &delta}},
		}},
	}
}

func TestBudgetReadResponseAndQueryCounts(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*app.Budget, map[string]any)
	}{
		{"policy growth and adjustment", func(*app.Budget, map[string]any) {}},
		{"legacy unfunded override", func(b *app.Budget, want map[string]any) {
			b.Policies = nil
			b.Funding = nil
			b.Overrides = map[string]int64{"2026-01": 0}
			want["monthly_major"] = 0
			want["funding"] = nil
			want["overrides"] = map[string]int{"2026-01": 0}
		}},
		{"legacy funded", func(b *app.Budget, want map[string]any) {
			b.Policies = nil
			want["monthly_major"] = 10
		}},
		{"future policy", func(b *app.Budget, want map[string]any) {
			b.Policies[0].EffectiveFrom = "2026-02-01"
			b.Policies[0].Growth.StartsOn = "2026-02-01"
			b.Policies[0].Adjustments = nil
			want["monthly_major"] = 10
		}},
		{"explicit zero funding", func(b *app.Budget, want map[string]any) {
			b.Funding = &app.BudgetFunding{}
			want["funding"] = &app.BudgetFunding{}
		}},
		{"explicit zero permission", func(b *app.Budget, want map[string]any) {
			b.Policies[0].MonthlyMinor = 0
			b.Policies[0].Growth = nil
			b.Policies[0].Adjustments = nil
			want["monthly_major"] = 0
		}},
		{"stale cash statement", func(b *app.Budget, want map[string]any) {
			b.OpeningAsOf = date.MustNew(2025, 12, 1)
			want["opening_as_of"] = "2025-12-01"
			want["opening_major"] = 0
			f := *b.Funding
			f.SpentMinor = 0
			f.CashThrough = ""
			want["funding"] = &f
		}},
		{"user cycle across calendar month", func(b *app.Budget, want map[string]any) {
			b.Policies[0].EffectiveFrom = "2025-12-15"
			b.Policies[0].CycleDay = 15
			b.Policies[0].Growth = nil
			b.Policies[0].Adjustments = nil
			b.OpeningAsOf = date.MustNew(2025, 12, 20)
			b.Funding.SpentPeriodStart = "2025-12-15"
			b.Funding.CashThrough = "2025-12-20"
			want["monthly_major"] = 15
			want["opening_as_of"] = "2025-12-20"
			f := *b.Funding
			want["funding"] = &f
		}},
		{"past routed cash retained", func(b *app.Budget, want map[string]any) {
			b.Funding.Events = append(b.Funding.Events, app.BudgetCashEvent{On: "2025-12-25", Minor: 100, Routing: &app.BudgetCashRouting{LoanID: "loan-a"}}, app.BudgetCashEvent{On: "2025-12-26", Minor: 200, FromOpening: true}, app.BudgetCashEvent{On: "2025-12-27", Minor: 300})
			f := *b.Funding
			f.Events = f.Events[:4]
			want["funding"] = &f
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			budget := budgetReadFixture()
			want := map[string]any{"today": "2026-01-01", "currency": "USD", "currency_exponent": 2, "monthly_major": 16, "base_monthly_major": 10, "pay_day": 1, "version": 7, "opening_major": 7, "opening_as_of": "2026-01-01", "reserve_major": 1, "funding": budget.Funding, "required_major": 3}
			tc.change(&budget, want)
			before, _ := json.Marshal(budget)
			store := &countedBudgetReader{value: budget}
			users := &countedBudgetUsers{}
			required := &countedBudgetRequired{}
			s := budgetTestServer(nil)
			s.Budgets = store
			s.Users = users
			s.Required = required
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/budget", nil)
			req.Header.Set("X-Telegram-Init-Data", knownInitData())
			res := httptest.NewRecorder()
			s.getBudget().ServeHTTP(res, req)
			expected, _ := json.Marshal(want)
			if res.Code != 200 || res.Body.String() != string(expected)+"\n" {
				t.Fatalf("status %d\ngot  %s\nwant %s", res.Code, res.Body.String(), expected)
			}
			after, _ := json.Marshal(store.value)
			if !reflect.DeepEqual(before, after) {
				t.Fatal("read mutated stored budget")
			}
			if users.identityReads != 1 || users.localeReads != 1 || store.calls != 1 || required.calls != 1 {
				t.Fatalf("reads: identity=%d locale=%d budget=%d required=%d", users.identityReads, users.localeReads, store.calls, required.calls)
			}
		})
	}
}

func TestBudgetReadInvalidPolicyStillRejects(t *testing.T) {
	for _, change := range []func(*app.Budget){
		func(b *app.Budget) { b.Funding = nil },
		func(b *app.Budget) { b.Policies[0].CarryRule = "invalid" },
		// Growth masks the negative adjustment; the no-growth validation must still fail.
		func(b *app.Budget) {
			delta := int64(-1600)
			b.Policies[0].Adjustments[0].DeltaMinor = &delta
		},
	} {
		budget := budgetReadFixture()
		change(&budget)
		s := budgetTestServer(nil)
		s.Budgets = &countedBudgetReader{value: budget}
		required := &countedBudgetRequired{}
		s.Required = required
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/budget", nil)
		req.Header.Set("X-Telegram-Init-Data", knownInitData())
		res := httptest.NewRecorder()
		s.getBudget().ServeHTTP(res, req)
		if res.Code != 422 || res.Body.String() != "invalid budget policy\n" || required.calls != 0 {
			t.Fatalf("invalid policy: %d %s required=%d", res.Code, res.Body.String(), required.calls)
		}
	}
}

// Exercises the real authenticated GET handler, with in-memory reads so the
// measurement isolates CPU/allocations rather than network or database noise.
func BenchmarkBudgetReadPolicy(b *testing.B) {
	s := budgetTestServer(nil)
	s.Budgets = &countedBudgetReader{value: budgetReadFixture()}
	s.Required = &countedBudgetRequired{}
	handler := s.getBudget()
	req := httptest.NewRequestWithContext(b.Context(), http.MethodGet, "/api/budget", nil)
	req.Header.Set("X-Telegram-Init-Data", knownInitData())
	b.ReportAllocs()
	for b.Loop() {
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != 200 {
			b.Fatal(res.Code, res.Body.String())
		}
	}
}
