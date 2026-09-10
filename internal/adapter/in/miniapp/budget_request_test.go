package miniapp

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/money"
)

func TestBudgetRequestValidate(t *testing.T) {
	t.Parallel()

	r := BudgetRequest{MonthlyMajor: "250000.25", Currency: "AMD", PayDay: 31}
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
		"unknown currency": {MonthlyMajor: "1", Currency: "XYZ"},
		"zero monthly":     {MonthlyMajor: "0", Currency: "AMD"},
		"negative monthly": {MonthlyMajor: "-1", Currency: "AMD"},
		"NaN monthly":      {MonthlyMajor: "NaN", Currency: "AMD"},
		"infinite monthly": {MonthlyMajor: "Infinity", Currency: "AMD"},
		"huge monthly":     {MonthlyMajor: "9223372036854775807", Currency: "AMD"},
		"negative pay day": {MonthlyMajor: "1", Currency: "AMD", PayDay: -1},
		"pay day 32":       {MonthlyMajor: "1", Currency: "AMD", PayDay: 32},
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
	for name, major := range map[string]json.Number{"zero clears": "0", "exact minor units": "12.35"} {
		major := major
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := BudgetRequest{OpeningMajor: &major}
			got, err := r.ValidateOpening(amd)
			if err != nil {
				t.Fatal(err)
			}
			want := int64(0)
			if name == "exact minor units" {
				want = 1235
			}
			if got != want {
				t.Fatalf("ValidateOpening() = %d, want %d", got, want)
			}
		})
	}
	for name, major := range map[string]json.Number{
		"excess precision": "12.345", "negative": "-1", "NaN": "NaN", "infinite": "Infinity",
		"too large": "9223372036854775807",
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

func TestBudgetRequestValidateReserve(t *testing.T) {
	t.Parallel()

	amd := money.MustLookup("AMD")
	major := json.Number("100000.25")
	r := BudgetRequest{ReserveMajor: &major}
	got, err := r.ValidateReserve(amd)
	if err != nil {
		t.Fatal(err)
	}
	if got != 10_000_025 {
		t.Fatalf("ValidateReserve() = %d, want 10000025", got)
	}
	negative := json.Number("-1")
	r.ReserveMajor = &negative
	if _, err := r.ValidateReserve(amd); !errors.Is(err, ErrInvalid) {
		t.Fatalf("negative reserve error = %v, want ErrInvalid", err)
	}
}

func TestBudgetRequestValidateOverrides(t *testing.T) {
	t.Parallel()

	amd := money.MustLookup("AMD")
	r := BudgetRequest{Overrides: map[string]json.Number{"2026-09": "0", "2026-10": "100000.25"}}
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

	tooMany := make(map[string]json.Number, maxOverrideMonths+1)
	for i := 0; i <= maxOverrideMonths; i++ {
		tooMany["invalid-"+string(rune('a'+i))] = "1"
	}
	cases := map[string]map[string]json.Number{
		"bad month": {"2026-13": "1"},
		"negative":  {"2026-09": "-1"},
		"NaN":       {"2026-09": "NaN"},
		"infinite":  {"2026-09": "Infinity"},
		"too large": {"2026-09": "9223372036854775807"},
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
