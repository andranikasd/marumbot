package app

import (
	"errors"
	"testing"
)

func TestInverseRefusalDoesNotInventMinimum(t *testing.T) {
	w, h, key, _ := comparisonWorker(t)
	result, err := w.BudgetByDate(t.Context(), "owner", key, "2027-01-01")
	if err != nil || result.Supported || result.MinimumMinor != 0 || result.Reason != "unproven_domain" {
		t.Fatalf("unsupported inverse: %+v %v", result, err)
	}
	if h.begun != 0 {
		t.Fatal("inverse preview changed active state")
	}
	if _, err = w.BudgetByDate(t.Context(), "stranger", key, "2027-01-01"); !errors.Is(err, ErrConflict) {
		t.Fatal("foreign proposal accepted", err)
	}
	h.sources = "changed"
	if _, err = w.BudgetByDate(t.Context(), "owner", key, "2027-01-01"); !errors.Is(err, ErrConflict) {
		t.Fatal("stale proposal accepted", err)
	}
}
