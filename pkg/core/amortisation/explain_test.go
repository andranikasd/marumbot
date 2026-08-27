package amortisation_test

import (
	"strings"
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/amortisation"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// The explanation must contain every input to the arithmetic, or it is not an
// explanation: a borrower checking it against their own paperwork needs the
// balance, the days, the rate and the rounding unit to reach the same figure.
func TestExplainShowsEveryInput(t *testing.T) {
	c, principal := amd5m()
	s, err := amortisation.SolveAndProject(c, principal, c.StartDate)
	if err != nil {
		t.Fatal(err)
	}
	out := amortisation.Explain(s.Rows[0], c.NominalRate, c.DayCount, c.Rounding)

	for _, want := range []string{
		s.Rows[0].Due.String(),       // when
		"31 days",                    // how long
		"14%",                        // at what rate
		"365",                        // over what year
		s.Rows[0].Opening.String(),   // from what balance
		s.Rows[0].Interest.String(),  // to what interest
		s.Rows[0].Principal.String(), // and what principal
		s.Rows[0].Closing.String(),   // leaving what
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the explanation omits %q\n%s", want, out)
		}
	}
}

// A rounding unit above one minor unit changes the answer, so it must be named.
// Inecobank rounds interest to 0.10 AMD; a borrower who does not know that
// cannot reproduce the row and concludes the figure is wrong.
func TestExplainNamesTheRoundingUnit(t *testing.T) {
	c, principal := amd5m()
	c.Rounding = money.Policy{Mode: money.HalfUp, Unit: 10}
	s, err := amortisation.SolveAndProject(c, principal, c.StartDate)
	if err != nil {
		t.Fatal(err)
	}
	out := amortisation.Explain(s.Rows[0], c.NominalRate, c.DayCount, c.Rounding)
	if !strings.Contains(out, "0.10") {
		t.Errorf("the explanation does not name the 0.10 rounding unit\n%s", out)
	}
}

// Rounding to the minor unit is not a step worth mentioning, and mentioning it
// would train readers to skip a line that sometimes matters.
func TestExplainOmitsATrivialRounding(t *testing.T) {
	c, principal := amd5m()
	c.Rounding = money.Policy{Mode: money.HalfUp, Unit: 1}
	s, err := amortisation.SolveAndProject(c, principal, c.StartDate)
	if err != nil {
		t.Fatal(err)
	}
	if out := amortisation.Explain(s.Rows[0], c.NominalRate, c.DayCount, c.Rounding); strings.Contains(out, "rounded to") {
		t.Errorf("a one-unit rounding should not be mentioned\n%s", out)
	}
}
