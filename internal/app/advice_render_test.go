package app

import (
	"strings"
	"testing"

	"github.com/andranikasd/marumbot/internal/i18n"
	"github.com/andranikasd/marumbot/pkg/core/allocation"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

// The plan card must fit a phone screen and never leak a raw catalogue key.
// This renders the real report body for a three-loan account in both
// languages and bounds its length; the methodology lives behind the why
// button and is rendered separately with the same checks.

func fixedReport(t *testing.T, goal plan.Goal) (plan.Report, date.Date) {
	t.Helper()
	amd := money.MustLookup("AMD")
	v := date.MustNew(2026, 1, 15)
	pos := func(id, name string, major, rate int64, years int) plan.Position {
		return plan.Position{
			ID: id, Name: name,
			Contract: model.Contract{
				LoanID: model.ID(id), Version: 1, Currency: amd, EffectiveFrom: v,
				NominalRate: money.RateFromPercent(rate, 0), DayCount: money.Actual365,
				Type: model.Annuity, StartDate: v, MaturityDate: date.MustNew(2026+years, 1, 15),
				PaymentDay: 15, Rounding: money.DefaultPolicy(amd),
			},
			Balance: money.FromMinor(major*100, amd), From: v,
			Excess: allocation.ExcessReducePrincipal, Trust: "user_entered",
		}
	}
	in := plan.Input{
		ValuationDate: v,
		Cash:          plan.CashPlan{Monthly: money.FromMinor(25_000_000, amd), PayDay: 1},
		Loans:         []plan.Position{pos("a", "Car", 1_200_000, 21, 3), pos("b", "Home", 4_000_000, 11, 10), pos("c", "Phone", 300_000, 26, 2)},
	}
	rep, err := plan.Search(in, goal)
	if err != nil {
		t.Fatal(err)
	}
	return rep, v
}

func renderCard(l i18n.Locale, rep plan.Report, today date.Date) string {
	var b strings.Builder
	req := rep.Best.Timeline[0].Required
	writeOutcome(&b, l, rep, req, today)
	b.WriteString("<b>" + i18n.T(l, "advice.this_month", bare(monthPaid(rep.Best.Timeline[0]))) + "</b>\n")
	writeActions(&b, l, rep.Best.Actions, today)
	return b.String()
}

func TestPlanCardIsCompactInBothLanguages(t *testing.T) {
	for _, goal := range []plan.Goal{{Kind: plan.LeastInterest}, {Kind: plan.Fastest}, {Kind: plan.FirstWin}} {
		rep, today := fixedReport(t, goal)
		for _, l := range []i18n.Locale{i18n.HY, i18n.EN} {
			out := renderCard(l, rep, today)
			lines := strings.Count(out, "\n")
			if lines > 14 {
				t.Errorf("%s/%s: card has %d lines:\n%s", goal, l, lines, out)
			}
			// A missing key renders as itself; none may reach a user.
			for _, frag := range []string{"advice.", "month.", "goal."} {
				if strings.Contains(out, frag) {
					t.Errorf("%s/%s: raw key in output:\n%s", goal, l, out)
					break
				}
			}
			if !strings.Contains(out, "<pre>") {
				t.Errorf("%s/%s: no payments table", goal, l)
			}
		}
		t.Logf("card %s (hy):\n%s", goal, renderCard(i18n.HY, rep, today))
	}
}

// Dates in the card are humanised: no ISO date may appear.
func TestPlanCardHumanisesDates(t *testing.T) {
	rep, today := fixedReport(t, plan.Goal{Kind: plan.LeastInterest})
	out := renderCard(i18n.EN, rep, today)
	if strings.Contains(out, "2026-") || strings.Contains(out, "2028-") {
		t.Fatalf("ISO date leaked:\n%s", out)
	}
	if !strings.Contains(out, "Feb") {
		t.Fatalf("no humanised date:\n%s", out)
	}
}
