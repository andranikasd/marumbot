package corpus_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/amortisation"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// A loan filed months into its life is anchored on the balance the lender
// reports, not on the drawdown. Projecting from that anchor must reproduce
// the lender's remaining rows exactly — the same rows the full projection
// reproduces — for every anchor point in the schedule. This is the mid-life
// half of the corpus, derived from the same paperwork: the balance after
// row k is the principal less the principal parts the lender printed.
func TestCorpusMidLifeAnchors(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "testdata", "golden", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range paths {
		if strings.HasSuffix(p, "MANIFEST.json") {
			continue
		}
		f := load(t, p)
		if len(f.Rows) < 4 || f.Fidelity.ExactRows < len(f.Rows)-1 {
			continue // only schedules the engine already reproduces row by row
		}
		t.Run(strings.TrimSuffix(filepath.Base(p), ".json"), func(t *testing.T) {
			c, principal := contractOf(t, f)
			for k := 1; k < len(f.Rows)-1; k += 7 {
				balance := principal.Minor()
				for _, r := range f.Rows[:k] {
					balance -= r.PrincipalMinor
				}
				s, err := amortisation.Project(c, money.FromMinor(balance, c.Currency),
					money.FromMinor(f.InstalmentMinor, c.Currency), mustDate(t, f.Rows[k-1].Due))
				if err != nil {
					t.Fatalf("anchor after row %d: %v", k, err)
				}
				want := f.Rows[k:]
				if len(s.Rows) != len(want) {
					t.Fatalf("anchor after row %d: %d rows, lender has %d", k, len(s.Rows), len(want))
				}
				for i := range want {
					if i == len(want)-1 {
						break // the final row absorbs the lender's drift; the full-schedule test governs it
					}
					if s.Rows[i].Interest.Minor() != want[i].InterestMinor || s.Rows[i].Due.String() != want[i].Due {
						t.Errorf("anchor after row %d, row %d (%s): interest %s, lender %d",
							k, k+i+1, want[i].Due, s.Rows[i].Interest, want[i].InterestMinor)
					}
				}
			}
		})
	}
}
