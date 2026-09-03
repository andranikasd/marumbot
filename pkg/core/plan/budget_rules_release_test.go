package plan

import (
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/money"
)

func TestReleasedReductionIndependentFixtures(t *testing.T) {
	cur := money.MustLookup("USD")
	amount := func(n int64) money.Amount { return money.FromMinor(n, cur) }
	for _, tc := range []struct {
		name, rule                     string
		before, after, keep, ppb, want int64
		confirmed                      bool
	}{
		{"unconfirmed", "release_all", 100, 0, 0, 0, 0, false},
		{"all rolled", "roll_all", 100, 0, 0, 0, 0, true},
		{"all released", "release_all", 100, 0, 0, 0, 100, true},
		{"reissue reduction", "release_all", 100, 60, 0, 0, 40, true},
		{"no release on increase", "release_all", 100, 120, 0, 0, 0, true},
		{"fixed retained", "roll_amount", 100, 0, 30, 0, 70, true},
		{"retained capped to released", "roll_amount", 100, 90, 30, 0, 0, true},
		{"percentage retained", "roll_percent", 100, 0, 0, 250000000, 75, true},
		{"percentage half up", "roll_percent", 101, 0, 0, 500000000, 50, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ReleasedReduction(amount(tc.before), amount(tc.after), tc.rule, amount(tc.keep), tc.ppb, tc.confirmed)
			if err != nil || got.Minor() != tc.want {
				t.Fatalf("got %v %v want %d", got, err, tc.want)
			}
		})
	}
}
