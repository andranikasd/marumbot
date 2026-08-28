package plan_test

import (
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/plan"
)

func five() []plan.Position {
	return append(three(), pos("d", "Fridge", 500_000, 24, 2), pos("e", "Study", 2_000_000, 14, 5))
}

func BenchmarkSearchFive(b *testing.B) {
	in := input(five(), 300_000, 1)
	for b.Loop() {
		if _, err := plan.Search(in, plan.Goal{Kind: plan.LeastInterest}); err != nil {
			b.Fatal(err)
		}
	}
}
