package amortisation

import (
	"fmt"
	"strings"

	"github.com/andranikasd/marumbot/pkg/core/money"
)

// Explain renders the arithmetic behind one row, in the terms the lender's own
// contract uses.
//
// This exists because "trust the number" is not an argument a borrower can
// check, and it is the argument the product had been making. A schedule that
// shows only the answer is indistinguishable from a schedule that is wrong.
//
// What it prints is the whole calculation: the balance it started from, the
// days it ran, the rate applied, and the unit it was rounded to. Anyone holding
// their own paperwork can follow it line by line and find the disagreement
// themselves -- which is the only way they ever come to believe the agreement.
func Explain(c Row, rate money.Rate, dc money.DayCount, p money.Policy) string {
	var b strings.Builder

	fmt.Fprintf(&b, "%s · %d days\n", c.Due, c.Days)
	fmt.Fprintf(&b, "  balance    %s\n", c.Opening)
	fmt.Fprintf(&b, "  interest   %s × %s%% × %d/%d = %s\n",
		c.Opening, ratePercent(rate), c.Days, dc.Denominator(), c.Interest)

	if p.Unit > 1 {
		fmt.Fprintf(&b, "             rounded to %s\n", quantum(p, c.Opening.Currency()))
	}
	fmt.Fprintf(&b, "  payment    %s\n", c.Payment)
	fmt.Fprintf(&b, "  of which   %s principal, %s interest\n", c.Principal, c.Interest)
	fmt.Fprintf(&b, "  leaves     %s\n", c.Closing)
	return b.String()
}

// ratePercent renders a parts-per-billion rate as a person reads it: the engine
// holds 171500000 for a contract that says 17.15%.
func ratePercent(r money.Rate) string {
	hundredths := (int64(r)*10000 + 500_000_000) / 1_000_000_000
	whole, frac := hundredths/100, hundredths%100
	switch {
	case frac == 0:
		return fmt.Sprintf("%d", whole)
	case frac%10 == 0:
		return fmt.Sprintf("%d.%d", whole, frac/10)
	default:
		return fmt.Sprintf("%d.%02d", whole, frac)
	}
}

// quantum names the unit interest is rounded to, in the currency's own terms.
// Saying "0.10 AMD" is checkable; saying "10 minor units" is not.
func quantum(p money.Policy, cur money.Currency) string {
	return money.FromMinor(p.Unit, cur).String()
}
