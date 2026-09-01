package miniapp

import (
	"errors"
	"math"
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/money"
)

func TestBudgetRequestValidate(t *testing.T) {
	t.Parallel()

	r := BudgetRequest{MonthlyMajor: 250_000.25, Currency: "AMD", PayDay: 31}
	code, minor, payDay, err := r.Validate()
	if err != nil {
		t.Fatal(err)
	}
	if code != "AMD" || minor != 25_000_025 || payDay != 31 {
		t.Fatalf("Validate() = %s, %d, %d; want AMD, 25000025, 31", code, minor, payDay)
	}
}

func TestBudgetRequestValidateRejectsInvalidConfiguration(t *testing.T) {
	t.Parallel()

	cases := map[string]BudgetRequest{
		"unknown currency": {MonthlyMajor: 1, Currency: "XYZ"},
		"zero monthly":     {MonthlyMajor: 0, Currency: "AMD"},
		"negative monthly": {MonthlyMajor: -1, Currency: "AMD"},
		"NaN monthly":      {MonthlyMajor: math.NaN(), Currency: "AMD"},
		"infinite monthly": {MonthlyMajor: math.Inf(1), Currency: "AMD"},
		"huge monthly":     {MonthlyMajor: float64(math.MaxInt64), Currency: "AMD"},
		"negative pay day": {MonthlyMajor: 1, Currency: "AMD", PayDay: -1},
		"pay day 32":       {MonthlyMajor: 1, Currency: "AMD", PayDay: 32},
	}
	for name, r := range cases {
		r := r
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, _, _, err := r.Validate(); !errors.Is(err, ErrInvalid) {
				t.Fatalf("Validate() error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestBudgetRequestValidateOpening(t *testing.T) {
	t.Parallel()

	amd := money.MustLookup("AMD")
	for name, major := range map[string]float64{"zero clears": 0, "rounds minor units": 12.345} {
		major := major
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := BudgetRequest{OpeningMajor: &major}
			got, err := r.ValidateOpening(amd)
			if err != nil {
				t.Fatal(err)
			}
			want := int64(0)
			if name == "rounds minor units" {
				want = 1235
			}
			if got != want {
				t.Fatalf("ValidateOpening() = %d, want %d", got, want)
			}
		})
	}
	for name, major := range map[string]float64{
		"negative": -1, "NaN": math.NaN(), "infinite": math.Inf(1),
		"too large": float64(math.MaxInt64),
	} {
		major := major
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := BudgetRequest{OpeningMajor: &major}
			if _, err := r.ValidateOpening(amd); !errors.Is(err, ErrInvalid) {
				t.Fatalf("ValidateOpening() error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestBudgetRequestValidateOverrides(t *testing.T) {
	t.Parallel()

	amd := money.MustLookup("AMD")
	r := BudgetRequest{Overrides: map[string]float64{"2026-09": 0, "2026-10": 100_000.25}}
	got, err := r.ValidateOverrides(amd)
	if err != nil {
		t.Fatal(err)
	}
	if got["2026-09"] != 0 || got["2026-10"] != 10_000_025 {
		t.Fatalf("ValidateOverrides() = %#v", got)
	}
}

func TestBudgetRequestValidateOverridesRejectsInvalidDocument(t *testing.T) {
	t.Parallel()

	tooMany := make(map[string]float64, maxOverrideMonths+1)
	for i := 0; i <= maxOverrideMonths; i++ {
		tooMany["invalid-"+string(rune('a'+i))] = 1
	}
	cases := map[string]map[string]float64{
		"bad month": {"2026-13": 1},
		"negative":  {"2026-09": -1},
		"NaN":       {"2026-09": math.NaN()},
		"infinite":  {"2026-09": math.Inf(1)},
		"too large": {"2026-09": float64(math.MaxInt64)},
		"too many":  tooMany,
	}
	amd := money.MustLookup("AMD")
	for name, overrides := range cases {
		overrides := overrides
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := BudgetRequest{Overrides: overrides}
			if _, err := r.ValidateOverrides(amd); !errors.Is(err, ErrInvalid) {
				t.Fatalf("ValidateOverrides() error = %v, want ErrInvalid", err)
			}
		})
	}
}
