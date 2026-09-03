package plan

import (
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

func TestExpandBudgetRulesFixtures(t *testing.T) {
	usd := money.MustLookup("USD")
	amount := func(n int64) money.Amount { return money.FromMinor(n, usd) }
	ptr := func(n int64) *money.Amount { a := amount(n); return &a }
	jan := date.MustNew(2026, 1, 1)
	tests := []struct {
		name        string
		base        money.Amount
		from        date.Date
		growth      *BudgetGrowth
		adjustments []BudgetAdjustment
		want        []int64
	}{
		{name: "zero base", base: amount(0), from: jan, want: []int64{0, 0, 0}},
		{name: "unchanged", base: amount(101), from: jan, want: []int64{101, 101}},
		{name: "zero percent", base: amount(101), from: jan, growth: &BudgetGrowth{EveryMonths: 1, StartsOn: jan}, want: []int64{101, 101}},
		{name: "fixed every two months", base: amount(100), from: jan, growth: &BudgetGrowth{EveryMonths: 2, StartsOn: date.AddMonths(jan, 1), Fixed: amount(20)}, want: []int64{100, 120, 120, 140, 140}},
		// 101*1.5=151.5 ->152; 152*1.5=228; 228*1.5=342.
		{name: "compound percent half up", base: amount(101), from: jan, growth: &BudgetGrowth{EveryMonths: 1, StartsOn: jan, PercentPPB: 500_000_000}, want: []int64{152, 228, 342}},
		{name: "fixed cap", base: amount(100), from: jan, growth: &BudgetGrowth{EveryMonths: 1, StartsOn: jan, Fixed: amount(30), Maximum: amount(150)}, want: []int64{130, 150, 150}},
		{name: "percent cap", base: amount(100), from: jan, growth: &BudgetGrowth{EveryMonths: 1, StartsOn: jan, PercentPPB: 500_000_000, Maximum: amount(200)}, want: []int64{150, 200, 200}},
		{name: "explicit zero cap", base: amount(100), from: jan, growth: &BudgetGrowth{EveryMonths: 1, StartsOn: jan, Fixed: amount(10), Maximum: amount(0)}, want: []int64{0, 0}},
		{name: "adjustments do not compound", base: amount(100), from: jan, growth: &BudgetGrowth{EveryMonths: 1, StartsOn: jan, Fixed: amount(20)}, adjustments: []BudgetAdjustment{{Month: "2026-01", Delta: ptr(50)}, {Month: "2026-02", Delta: ptr(-40)}, {Month: "2026-03", Replacement: ptr(0)}}, want: []int64{170, 100, 0, 180}},
		{name: "adjustment exceeds growth cap", base: amount(100), from: jan, growth: &BudgetGrowth{EveryMonths: 1, StartsOn: jan, Fixed: amount(20), Maximum: amount(130)}, adjustments: []BudgetAdjustment{{Month: "2026-01", Replacement: ptr(900)}}, want: []int64{900, 130, 130}},
		{name: "historical recurrences", base: amount(100), from: date.MustNew(2026, 3, 1), growth: &BudgetGrowth{EveryMonths: 1, StartsOn: jan, Fixed: amount(10)}, want: []int64{130, 140}},
		{name: "inclusive end", base: amount(100), from: jan, growth: &BudgetGrowth{EveryMonths: 1, StartsOn: jan, EndsOn: date.MustNew(2026, 3, 1), Fixed: amount(10)}, want: []int64{110, 120, 130, 130}},
		{name: "end before next occurrence", base: amount(100), from: jan, growth: &BudgetGrowth{EveryMonths: 1, StartsOn: jan, EndsOn: date.MustNew(2026, 2, 28), Fixed: amount(10)}, want: []int64{110, 120, 120, 120}},
		{name: "outside adjustments excluded", base: amount(100), from: jan, adjustments: []BudgetAdjustment{{Month: "2025-12", Delta: ptr(-1000)}, {Month: "2026-03", Replacement: ptr(0)}}, want: []int64{100, 100}},
		// AMD settles in units of ten: 110*1.05=115.5 ->120;
		// 120*1.05=126 ->130; 130*1.05=136.5 ->140.
		{name: "AMD settlement", base: money.FromMinor(110, money.AMD), from: jan, growth: &BudgetGrowth{EveryMonths: 1, StartsOn: jan, PercentPPB: 50_000_000}, want: []int64{120, 130, 140}},
		{name: "fixed settlement", base: money.FromMinor(110, money.AMD), from: jan, growth: &BudgetGrowth{EveryMonths: 1, StartsOn: jan, Fixed: money.FromMinor(5, money.AMD)}, want: []int64{120, 130}},
		{name: "huge frequency bounded", base: amount(100), from: jan, growth: &BudgetGrowth{EveryMonths: math.MaxInt, StartsOn: jan, Fixed: amount(1)}, want: []int64{101, 101}},
		{name: "cap avoids intermediate overflow", base: amount(math.MaxInt64), from: jan, growth: &BudgetGrowth{EveryMonths: 1, StartsOn: jan, PercentPPB: math.MaxInt64, Maximum: amount(math.MaxInt64)}, want: []int64{math.MaxInt64}},
		{name: "large representable percentage", base: amount(4_000_000_000_000_000_000), from: jan, growth: &BudgetGrowth{EveryMonths: 1, StartsOn: jan, PercentPPB: 1_000_000_000}, want: []int64{8_000_000_000_000_000_000}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var before BudgetGrowth
			if tt.growth != nil {
				before = *tt.growth
			}
			got, err := ExpandBudgetRules(tt.base, tt.from, len(tt.want), tt.growth, tt.adjustments)
			if err != nil {
				t.Fatal(err)
			}
			want := make(map[string]money.Amount, len(tt.want))
			for i, n := range tt.want {
				want[MonthKey(date.AddMonths(tt.from, i))] = money.FromMinor(n, tt.base.Currency())
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got %v; want %v", got, want)
			}
			if tt.growth != nil && *tt.growth != before {
				t.Fatal("growth mutated")
			}
		})
	}
}

func TestExpandBudgetRulesValidation(t *testing.T) {
	usd := money.MustLookup("USD")
	amount := func(n int64) money.Amount { return money.FromMinor(n, usd) }
	ptr := func(n int64) *money.Amount { a := amount(n); return &a }
	jan := date.MustNew(2026, 1, 1)
	other := money.Zero(money.AMD)
	invalid := money.Amount{}
	forged := usd
	forged.SettlementUnit = 100
	tests := []struct {
		name        string
		base        money.Amount
		from        date.Date
		months      int
		growth      *BudgetGrowth
		adjustments []BudgetAdjustment
		sentinel    error
	}{
		{name: "zero currency", base: invalid, from: jan, months: 1, sentinel: money.ErrUnknownCurrency},
		{name: "forged currency", base: money.FromMinor(1, forged), from: jan, months: 1},
		{name: "negative base", base: amount(-1), from: jan, months: 1, sentinel: money.ErrNegative},
		{name: "zero date", base: amount(1), months: 1},
		{name: "invalid ISO year", base: amount(1), from: date.MustNew(-1, 1, 1), months: 1},
		{name: "zero horizon", base: amount(1), from: jan},
		{name: "excess horizon", base: amount(1), from: jan, months: 601},
		{name: "horizon exceeds ISO dates", base: amount(1), from: date.MustNew(9999, 12, 1), months: 2},
		{name: "zero frequency", base: amount(1), from: jan, months: 1, growth: &BudgetGrowth{StartsOn: jan}},
		{name: "negative frequency", base: amount(1), from: jan, months: 1, growth: &BudgetGrowth{StartsOn: jan, EveryMonths: -1}},
		{name: "mid-month growth refused", base: amount(1), from: jan, months: 1, growth: &BudgetGrowth{EveryMonths: 1, StartsOn: date.MustNew(2026, 1, 15), Fixed: amount(1)}},
		{name: "missing start", base: amount(1), from: jan, months: 1, growth: &BudgetGrowth{EveryMonths: 1}},
		{name: "end before start", base: amount(1), from: jan, months: 1, growth: &BudgetGrowth{EveryMonths: 1, StartsOn: jan, EndsOn: date.AddMonths(jan, -1)}},
		{name: "two growth operations", base: amount(1), from: jan, months: 1, growth: &BudgetGrowth{EveryMonths: 1, StartsOn: jan, Fixed: amount(1), PercentPPB: 1}},
		{name: "negative percent", base: amount(1), from: jan, months: 1, growth: &BudgetGrowth{EveryMonths: 1, StartsOn: jan, PercentPPB: -1}},
		{name: "negative fixed", base: amount(1), from: jan, months: 1, growth: &BudgetGrowth{EveryMonths: 1, StartsOn: jan, Fixed: amount(-1)}},
		{name: "negative maximum", base: amount(1), from: jan, months: 1, growth: &BudgetGrowth{EveryMonths: 1, StartsOn: jan, Maximum: amount(-1)}},
		{name: "fixed currency", base: amount(1), from: jan, months: 1, growth: &BudgetGrowth{EveryMonths: 1, StartsOn: jan, Fixed: other}},
		{name: "maximum currency", base: amount(1), from: jan, months: 1, growth: &BudgetGrowth{EveryMonths: 1, StartsOn: jan, Maximum: other}},
		{name: "no operation", base: amount(1), from: jan, months: 1, adjustments: []BudgetAdjustment{{Month: "2026-01"}}},
		{name: "two operations", base: amount(1), from: jan, months: 1, adjustments: []BudgetAdjustment{{Month: "2026-01", Replacement: ptr(0), Delta: ptr(0)}}},
		{name: "duplicate", base: amount(1), from: jan, months: 1, adjustments: []BudgetAdjustment{{Month: "2026-01", Delta: ptr(1)}, {Month: "2026-01", Replacement: ptr(2)}}},
		{name: "unknown month", base: amount(1), from: jan, months: 1, adjustments: []BudgetAdjustment{{Month: "2026-13", Delta: ptr(1)}}},
		{name: "noncanonical month", base: amount(1), from: jan, months: 1, adjustments: []BudgetAdjustment{{Month: "2026-1", Delta: ptr(1)}}},
		{name: "negative replacement", base: amount(1), from: jan, months: 1, adjustments: []BudgetAdjustment{{Month: "2026-01", Replacement: ptr(-1)}}},
		{name: "negative effective", base: amount(1), from: jan, months: 1, adjustments: []BudgetAdjustment{{Month: "2026-01", Delta: ptr(math.MinInt64)}}, sentinel: money.ErrNegative},
		{name: "replacement currency", base: amount(1), from: jan, months: 1, adjustments: []BudgetAdjustment{{Month: "2026-01", Replacement: &other}}},
		{name: "delta missing currency", base: amount(1), from: jan, months: 1, adjustments: []BudgetAdjustment{{Month: "2026-01", Delta: &invalid}}},
		{name: "fixed overflow", base: amount(math.MaxInt64), from: jan, months: 1, growth: &BudgetGrowth{EveryMonths: 1, StartsOn: jan, Fixed: amount(1)}, sentinel: money.ErrOverflow},
		{name: "percentage overflow", base: amount(math.MaxInt64), from: jan, months: 1, growth: &BudgetGrowth{EveryMonths: 1, StartsOn: jan, PercentPPB: 1_000_000_000}, sentinel: money.ErrOverflow},
		{name: "delta overflow", base: amount(math.MaxInt64), from: jan, months: 1, adjustments: []BudgetAdjustment{{Month: "2026-01", Delta: ptr(1)}}, sentinel: money.ErrOverflow},
		{name: "settlement overflow", base: money.FromMinor(math.MaxInt64-1, money.AMD), from: jan, months: 1, growth: &BudgetGrowth{EveryMonths: 1, StartsOn: jan, Fixed: money.FromMinor(1, money.AMD)}, sentinel: money.ErrOverflow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExpandBudgetRules(tt.base, tt.from, tt.months, tt.growth, tt.adjustments)
			if err == nil || got != nil {
				t.Fatalf("got %v, %v; want nil and error", got, err)
			}
			if tt.sentinel != nil && !errors.Is(err, tt.sentinel) {
				t.Fatalf("got %v; want %v", err, tt.sentinel)
			}
		})
	}
}

func TestExpandBudgetRulesMaximumHorizon(t *testing.T) {
	base := money.FromMinor(1, money.MustLookup("USD"))
	got, err := ExpandBudgetRules(base, date.MustNew(2026, 1, 1), 600, nil, nil)
	if err != nil || len(got) != 600 || got["2075-12"] != base {
		t.Fatalf("horizon: len=%d err=%v", len(got), err)
	}
}
