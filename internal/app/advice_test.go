package app

import (
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/money"
)

// The rate column stores a fraction: 0.140000000 means 14 per cent. Rendering
// it directly produced "Rate: 0.140000000%" in front of a user.
func TestPercentRendersAsAPersonReadsIt(t *testing.T) {
	cases := []struct {
		rate money.Rate
		want string
	}{
		{money.RateFromPercent(14, 0), "14"},
		{money.RateFromPercent(0, 0), "0"},
		{money.RateFromPercent(14, 500000), "14.5"},
		{money.RateFromPercent(9, 990000), "9.99"},
		{money.RateFromPercent(22, 250000), "22.25"},
	}
	for _, c := range cases {
		if got := percent(c.rate); got != c.want {
			t.Errorf("percent(%s) = %q, want %q", c.rate, got, c.want)
		}
	}
}
