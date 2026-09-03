package plan

import (
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

func TestBudgetPermissionSurvivesReportingReset(t *testing.T) {
	cur := money.MustLookup("USD")
	amount := func(n int64) money.Amount { return money.FromMinor(n, cur) }
	first := date.MustNew(2026, 2, 15)
	s := sim{
		cur: cur, budget: amount(10000), cash: amount(50000), periodSpent: amount(0),
		pol: Policy{RequiredOnly: true},
		res: Result{PeakRequired: amount(0)},
		in: Input{ValuationDate: first, Cash: CashPlan{Spending: &SpendingPlan{
			CycleDay: 15, Monthly: amount(10000), Spent: amount(1000),
			Changes: []SpendingChange{{On: date.MustNew(2026, 2, 25), Limit: amount(12000)}},
		}}},
	}
	s.period(first)
	// Independent ledger: observed 10 + required 20 + optional 28 and fee 2
	// leaves 60 from revised permission 120, regardless of reporting resets.
	s.spendPermission(amount(2000))
	s.spendPermission(amount(3000))
	for _, reportingDate := range []date.Date{date.MustNew(2026, 2, 10), date.MustNew(2026, 3, 10)} {
		s.openCycle(reportingDate)
		s.period(date.MustNew(2026, 2, 25))
		if s.err != nil || s.periodLeft.Minor() != 6000 {
			t.Fatalf("report reset %s restored spending: remaining=%s err=%v", reportingDate, s.periodLeft, s.err)
		}
	}
	s.period(date.MustNew(2026, 3, 15))
	if s.err != nil || s.periodLeft.Minor() != 12000 {
		t.Fatalf("new spending period must reset exactly once: %s / %v", s.periodLeft, s.err)
	}
}
