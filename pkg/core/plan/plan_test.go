package plan_test

import (
	"errors"
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/money"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

var amd = money.MustLookup("AMD")

func amt(major int64) money.Amount { return money.FromMinor(major*100, amd) }

// Three loans of deliberately different shapes, so the three goals genuinely
// disagree about which to target.
func loans() []plan.Loan {
	return []plan.Loan{
		{ID: "big-cheap", Balance: amt(3_000_000), Rate: money.RateFromPercent(9, 0), Required: amt(90_000)},
		{ID: "small-dear", Balance: amt(200_000), Rate: money.RateFromPercent(22, 0), Required: amt(20_000)},
		{ID: "mid-heavy", Balance: amt(600_000), Rate: money.RateFromPercent(14, 0), Required: amt(75_000)},
	}
}

// The property the whole package exists for. Whatever the goal, the surplus
// lands on exactly ONE loan -- never spread across them in proportion to their
// balances, which is what people do unaided and what costs them most.
func TestSurplusIsNeverSpread(t *testing.T) {
	for _, g := range []plan.Goal{plan.PayLeastInterest, plan.FinishSoonest, plan.FreeUpMonthly} {
		p, err := plan.Allocate(loans(), amt(250_000), g)
		if err != nil {
			t.Fatalf("%s: %v", g, err)
		}
		funded := 0
		for _, a := range p.Allocations {
			if a.Extra.Sign() > 0 {
				funded++
			}
		}
		if funded != 1 {
			t.Errorf("%s: surplus went to %d loans, want exactly 1", g, funded)
		}
	}
}

// The three goals must actually differ, or offering them is theatre.
func TestGoalsTargetDifferentLoans(t *testing.T) {
	want := map[plan.Goal]string{
		plan.PayLeastInterest: "small-dear", // 22% is the highest rate
		plan.FinishSoonest:    "small-dear", // and also the smallest balance
		plan.FreeUpMonthly:    "mid-heavy",  // 75,000 freed per 600,000 repaid
	}
	for g, id := range want {
		p, err := plan.Allocate(loans(), amt(250_000), g)
		if err != nil {
			t.Fatalf("%s: %v", g, err)
		}
		if p.Target != id {
			t.Errorf("%s targeted %s, want %s", g, p.Target, id)
		}
	}
}

// Required payments come first, always. A plan that underpays one loan to
// accelerate another manufactures arrears, and Armenian penalty interest runs
// to four times the Central Bank's rate.
func TestRequiredPaymentsAreAlwaysMet(t *testing.T) {
	ls := loans()
	p, err := plan.Allocate(ls, amt(250_000), plan.PayLeastInterest)
	if err != nil {
		t.Fatal(err)
	}
	for i, a := range p.Allocations {
		if a.Total.Cmp(ls[i].Required) < 0 {
			t.Errorf("%s allocated %s, below its required %s", a.LoanID, a.Total, ls[i].Required)
		}
	}
	// And the whole budget is accounted for: nothing invented, nothing lost.
	total := money.Zero(amd)
	for _, a := range p.Allocations {
		total, _ = total.Add(a.Total)
	}
	total, _ = total.Add(p.Unspent)
	if total.Cmp(amt(250_000)) != 0 {
		t.Errorf("allocations plus unspent = %s, want the budget %s", total, amt(250_000))
	}
}

// A budget that cannot cover the minimums is refused rather than fudged.
func TestBudgetBelowRequiredIsRefused(t *testing.T) {
	_, err := plan.Allocate(loans(), amt(100_000), plan.PayLeastInterest)
	if !errors.Is(err, plan.ErrBudget) {
		t.Errorf("got %v, want ErrBudget", err)
	}
}

func TestExactBudgetLeavesNoSurplus(t *testing.T) {
	p, err := plan.Allocate(loans(), amt(185_000), plan.PayLeastInterest) // 90+20+75
	if err != nil {
		t.Fatal(err)
	}
	if p.Surplus.Sign() != 0 || p.Target != "" {
		t.Errorf("surplus %s targeted %q, want none", p.Surplus, p.Target)
	}
}

// Prepaying is only worth it when the interest avoided beats the fee. Armenian
// consumer credit forbids the fee outright, but mortgages may charge one for
// three years, so the engine must be able to decline.
func TestPrepaymentFeeCanMakeHoldingBetter(t *testing.T) {
	ls := []plan.Loan{{
		ID: "fee-heavy", Balance: amt(1_000_000),
		Rate: money.RateFromPercent(3, 0), Required: amt(20_000),
		PrepaymentFeeBP: 600, // 6%, far above a year of interest at 3%
	}}
	p, err := plan.Allocate(ls, amt(120_000), plan.PayLeastInterest)
	if err != nil {
		t.Fatal(err)
	}
	if p.Target != "" {
		t.Errorf("targeted %s despite the fee outweighing the saving", p.Target)
	}
	if p.Unspent.Cmp(amt(100_000)) != 0 {
		t.Errorf("unspent = %s, want the whole surplus held", p.Unspent)
	}
	if p.Note == "" {
		t.Error("no explanation given for withholding the surplus")
	}
}

// A fee small enough to be worth paying must not block the plan.
func TestSmallFeeStillWorthPaying(t *testing.T) {
	ls := []plan.Loan{{
		ID: "fee-light", Balance: amt(1_000_000),
		Rate: money.RateFromPercent(18, 0), Required: amt(20_000),
		PrepaymentFeeBP: 60, // 0.6%, the Armenian mortgage first-year cap
	}}
	p, err := plan.Allocate(ls, amt(120_000), plan.PayLeastInterest)
	if err != nil {
		t.Fatal(err)
	}
	if p.Target != "fee-light" {
		t.Errorf("target = %q, want the loan funded: 0.6%% is far below a year at 18%%", p.Target)
	}
}

// Never pay more than the loan is worth.
func TestSurplusBeyondTheBalanceIsNotSpent(t *testing.T) {
	ls := []plan.Loan{
		{ID: "tiny", Balance: amt(30_000), Rate: money.RateFromPercent(25, 0), Required: amt(5_000)},
	}
	p, err := plan.Allocate(ls, amt(500_000), plan.PayLeastInterest)
	if err != nil {
		t.Fatal(err)
	}
	if p.Allocations[0].Extra.Cmp(amt(30_000)) != 0 {
		t.Errorf("extra = %s, want the balance %s", p.Allocations[0].Extra, amt(30_000))
	}
	if p.Unspent.Sign() <= 0 {
		t.Error("the remainder above the balance should be reported as unspent")
	}
}

// Allocating one currency's budget across another's loan needs an exchange
// rate, and Marum has no validated source for one.
func TestMixedCurrenciesAreRefused(t *testing.T) {
	usd := money.MustLookup("USD")
	ls := []plan.Loan{{
		ID: "dollar", Balance: money.FromMinor(100_000, usd),
		Rate: money.RateFromPercent(10, 0), Required: money.FromMinor(10_000, usd),
	}}
	if _, err := plan.Allocate(ls, amt(100_000), plan.PayLeastInterest); err == nil {
		t.Error("expected a refusal, got none")
	}
}

// A settled loan must not attract the surplus.
func TestSettledLoansAreSkipped(t *testing.T) {
	ls := []plan.Loan{
		{ID: "done", Balance: amt(0), Rate: money.RateFromPercent(30, 0), Required: amt(0)},
		{ID: "live", Balance: amt(100_000), Rate: money.RateFromPercent(10, 0), Required: amt(10_000)},
	}
	p, err := plan.Allocate(ls, amt(50_000), plan.PayLeastInterest)
	if err != nil {
		t.Fatal(err)
	}
	if p.Target != "live" {
		t.Errorf("target = %q, want live", p.Target)
	}
}
