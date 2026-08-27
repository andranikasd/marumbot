// Package corpus replays the schedules real lenders issued.
//
// It lives outside pkg/core because it reads files, and pkg/core performs no
// I/O -- the engine must stay compilable and testable with nothing but its own
// arithmetic. The evidence that the engine is right is not part of the engine.
package corpus_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/amortisation"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// The correctness corpus.
//
// Each fixture is a schedule a real lender issued. This replays every one row
// by row and fails on a single minor unit of disagreement, because that is the
// standard the product claims: the same inputs always produce the same answer,
// and the answer is the one on the paperwork.
//
// A failure here is not a test to relax. It means the engine and a real bank
// disagree about money, and one of them is wrong in front of a borrower.

type fixture struct {
	Source        string  `json:"source"`
	Contract      string  `json:"contract"`
	Note          string  `json:"note"`
	Currency      string  `json:"currency"`
	RatePercent   float64 `json:"nominal_rate_percent"`
	RepaymentType string  `json:"repayment_type"`
	Accrual       struct {
		DayCount     string `json:"day_count"`
		QuantumMinor int64  `json:"quantum_minor"`
		Rounding     string `json:"rounding"`
	} `json:"accrual"`
	Anchor struct {
		AsOf           string `json:"as_of"`
		PrincipalMinor int64  `json:"principal_minor"`
	} `json:"anchor"`
	Fidelity struct {
		ExactRows            int   `json:"exact_rows"`
		TotalRows            int   `json:"total_rows"`
		ToleranceMinor       int64 `json:"tolerance_minor"`
		FinalRowAbsorbsDrift bool  `json:"final_row_absorbs_drift"`
	} `json:"fidelity"`
	InstalmentMinor int64  `json:"instalment_minor"`
	PaymentDay      int    `json:"payment_day"`
	Maturity        string `json:"maturity"`
	Totals          struct {
		PrincipalMinor int64 `json:"principal_minor"`
		InterestMinor  int64 `json:"interest_minor"`
	} `json:"totals"`
	Rows []struct {
		Due            string `json:"due"`
		PrincipalMinor int64  `json:"principal_minor"`
		InterestMinor  int64  `json:"interest_minor"`
	} `json:"rows"`
}

func TestCorpus(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "testdata", "golden", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("the corpus is empty; every calculation claim is then unbacked")
	}

	for _, p := range paths {
		t.Run(strings.TrimSuffix(filepath.Base(p), ".json"), func(t *testing.T) {
			f := load(t, p)
			if len(f.Rows) == 0 {
				t.Skip("fixture carries no rows")
			}
			c, principal := contractOf(t, f)

			s, err := amortisation.Project(c, principal,
				money.FromMinor(f.InstalmentMinor, c.Currency), mustDate(t, f.Anchor.AsOf))
			if err != nil {
				t.Fatalf("projecting %s: %v", f.Contract, err)
			}
			if len(s.Rows) != len(f.Rows) {
				t.Fatalf("produced %d rows, the lender published %d", len(s.Rows), len(f.Rows))
			}

			// Two numbers, because they mean different things.
			//
			// A row outside tolerance is a disagreement about money and fails
			// outright. A row inside it is a rounding boundary the lender
			// resolves from more internal precision than they publish, which is
			// not something this engine can or should reproduce.
			//
			// exact is then a ratchet. It is recorded in the fixture, and a
			// change that lowers it has made the engine agree with this lender
			// less often -- which is a regression even when every row still
			// sits inside tolerance.
			tol := f.Fidelity.ToleranceMinor
			exact, within := 0, 0
			reported := 0

			for i, want := range f.Rows {
				got := s.Rows[i]
				last := i == len(f.Rows)-1

				if got.Due.String() != want.Due {
					t.Errorf("row %d falls due %s, the lender says %s", i+1, got.Due, want.Due)
					continue
				}

				di := got.Interest.Minor() - want.InterestMinor
				dp := got.Principal.Minor() - want.PrincipalMinor

				// The final row is where a lender puts the drift accumulated by
				// rounding every earlier row, so its principal legitimately
				// differs by more than one quantum. Its interest does not.
				if last && f.Fidelity.FinalRowAbsorbsDrift {
					if abs64(di) > tol {
						t.Errorf("final row interest %s, the lender says %s (off by %s)",
							minor(got.Interest.Minor()), minor(want.InterestMinor), minor(di))
					}
					t.Logf("final row absorbs drift: principal %s against the lender's %s (%s)",
						minor(got.Principal.Minor()), minor(want.PrincipalMinor), minor(dp))
					continue
				}

				switch {
				case di == 0 && dp == 0:
					exact++
				case abs64(di) <= tol && abs64(dp) <= tol:
					within++
				default:
					if reported < 8 {
						t.Errorf("row %d (%s, %d days) is outside tolerance\n"+
							"  interest  %14s  lender %14s  off by %s\n"+
							"  principal %14s  lender %14s  off by %s",
							i+1, want.Due, got.Days,
							minor(got.Interest.Minor()), minor(want.InterestMinor), minor(di),
							minor(got.Principal.Minor()), minor(want.PrincipalMinor), minor(dp))
					}
					reported++
				}
			}
			if reported > 8 {
				t.Errorf("... and %d further rows are outside tolerance", reported-8)
			}

			t.Logf("%d rows: %d exact, %d within %s, %d outside",
				len(f.Rows), exact, within, minor(tol), reported)

			if want := f.Fidelity.ExactRows; want > 0 && exact < want {
				t.Errorf("%d rows reproduce exactly; this fixture recorded %d. "+
					"The engine now agrees with this lender less often than it did.",
					exact, want)
			}

			if reported == 0 && !f.Fidelity.FinalRowAbsorbsDrift {
				assertTotals(t, f, s)
			}
		})
	}
}

// A schedule that matches row by row must also match in aggregate. It is a
// weaker check, and it catches a transcription error the row check cannot: a
// row copied twice still sums wrong.
func assertTotals(t *testing.T, f fixture, s amortisation.Schedule) {
	t.Helper()
	if got := s.TotalInterest.Minor(); got != f.Totals.InterestMinor {
		t.Errorf("total interest %s, the lender says %s (off by %s)",
			minor(got), minor(f.Totals.InterestMinor), minor(got-f.Totals.InterestMinor))
	}
	var principal int64
	for _, r := range s.Rows {
		principal += r.Principal.Minor()
	}
	if principal != f.Totals.PrincipalMinor {
		t.Errorf("total principal %s, the lender says %s (off by %s)",
			minor(principal), minor(f.Totals.PrincipalMinor),
			minor(principal-f.Totals.PrincipalMinor))
	}
}

// The convention is read from the fixture rather than assumed, because it is a
// property of the lender's system and not of Armenian law. Nothing in the Civil
// Code, the Consumer Lending Law, the Mortgage Law or CBA Regulations 8/01 and
// 8/05 prescribes a rounding rule for a payment.
func contractOf(t *testing.T, f fixture) (model.Contract, money.Amount) {
	t.Helper()
	cur, err := money.Lookup(f.Currency)
	if err != nil {
		t.Fatalf("currency %q: %v", f.Currency, err)
	}
	quantum := f.Accrual.QuantumMinor
	if quantum <= 0 {
		quantum = 1
	}
	typ := model.Annuity
	if f.RepaymentType == "declining" {
		typ = model.DecliningPrincipal
	}
	dayCount := money.Actual365
	switch f.Accrual.DayCount {
	case "act/360":
		dayCount = money.Actual360
	case "30/360":
		dayCount = money.Thirty360
	}
	mode := money.HalfUp
	switch f.Accrual.Rounding {
	case "down":
		mode = money.Down
	case "up":
		mode = money.Up
	case "half_even":
		mode = money.HalfEven
	}

	anchor := mustDate(t, f.Anchor.AsOf)
	return model.Contract{
		LoanID: model.ID(f.Contract), Version: 1, Currency: cur,
		NominalRate: rateOf(f.RatePercent), DayCount: dayCount, Type: typ,
		StartDate: anchor, MaturityDate: mustDate(t, f.Maturity),
		PaymentDay: f.PaymentDay,
		Rounding:   money.Policy{Mode: mode, Unit: quantum},
	}, money.FromMinor(f.Anchor.PrincipalMinor, cur)
}

// rateOf converts a percentage to parts per billion without passing through a
// float wide enough to lose a digit: 17.15 is exactly 171_500_000 ppb, and
// arriving at 171_499_999 would shift every row.
func rateOf(percent float64) money.Rate {
	micro := int64(percent*1_000_000 + 0.5)
	return money.RateFromPercent(micro/1_000_000, micro%1_000_000)
}

func load(t *testing.T, path string) fixture {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var f fixture
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return f
}

func mustDate(t *testing.T, s string) date.Date {
	t.Helper()
	d, err := date.Parse(s)
	if err != nil {
		t.Fatalf("date %q: %v", s, err)
	}
	return d
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// minor renders a minor-unit figure the way the paperwork does, so a failure
// can be compared to the document without arithmetic.
func minor(v int64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	s := fmt.Sprintf("%d.%02d", v/100, v%100)
	if neg {
		return "-" + s
	}
	return s
}
