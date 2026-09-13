package app

import (
	"errors"
	"reflect"
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/allocation"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

func TestPlanningStartPreservesStatementsAndCyclePermissions(t *testing.T) {
	cur := money.MustLookup("USD")
	amount := func(n int64) money.Amount { return money.FromMinor(n, cur) }
	for _, day := range []int{1, 15, 31} {
		t.Run(string(rune('A'+day)), func(t *testing.T) {
			on := date.MustNew(2026, 9, 20)
			b := Budget{Funding: &BudgetFunding{PlanningStartMonth: "2026-10"}}
			cp := plan.CashPlan{Monthly: amount(900), OpeningCash: amount(500), ReserveFloor: amount(100), PayDay: 25, CashThrough: on, Spending: &plan.SpendingPlan{Monthly: amount(1000), Spent: amount(200), CycleDay: day, Overrides: map[string]money.Amount{"2026-10": amount(800)}, CarryRule: plan.CarryCash}}
			original := *cp.Spending
			got := b.applyPlanningStart(cp, on)
			if !reflect.DeepEqual(original, *cp.Spending) {
				t.Fatal("mutated original")
			}
			if got.Monthly != cp.Monthly || got.OpeningCash != cp.OpeningCash || got.ReserveFloor != cp.ReserveFloor || got.CashThrough != cp.CashThrough || got.Spending.Spent != original.Spent {
				t.Fatal("changed money statement")
			}
			for _, change := range got.Spending.Changes {
				want := int64(1000)
				if plan.MonthKey(original.PeriodStart(change.On)) == "2026-10" {
					want = 800
				}
				if change.On.Before(date.MustNew(2026, 10, 1)) {
					want = 0
					if original.PeriodStart(change.On).Equal(original.PeriodStart(on)) {
						want = 200
					}
				}
				if change.Limit.Minor() != want {
					t.Fatalf("%s limit=%d want=%d", change.On, change.Limit.Minor(), want)
				}
			}
		})
	}
}

func TestPlanningStartBothBudgetEntryPointsAndExpiredPreference(t *testing.T) {
	b := budgetPolicyFixture()
	b.Funding.PlanningStartMonth = "2026-02"
	on := date.MustNew(2026, 1, 1)
	first, second, err := b.CashPlans(on)
	if err != nil {
		t.Fatal(err)
	}
	direct := b.CashPlan(on)
	for _, cp := range []plan.CashPlan{first, second, direct} {
		if len(cp.Spending.Changes) == 0 || cp.Spending.Changes[0].Limit.Minor() != 400 {
			t.Fatal("pause missing or actual spent fabricated")
		}
	}
	first.Spending.Changes[0].Limit = money.Zero(b.Monthly.Currency())
	if second.Spending.Changes[0].Limit.Minor() != 400 {
		t.Fatal("inputs alias")
	}
	expired := b.CashPlan(date.MustNew(2026, 2, 1))
	if len(expired.Spending.Changes) != 0 {
		t.Fatal("expired preference still changes input")
	}
}

func TestPlanningStartDoesNotConcealOverspendingOrChangeApprovedLimit(t *testing.T) {
	b := budgetPolicyFixture()
	b.Funding.PlanningStartMonth = "2026-02"
	b.Funding.SpentMinor = 2000
	paused := b.CashPlan(date.MustNew(2026, 1, 1))
	if len(paused.Spending.Changes) != 0 || paused.Spending.Monthly.Minor() != 1000 {
		t.Fatal("pause concealed existing overspending")
	}
	b.Funding.SpentMinor = 400
	b.Policies = []BudgetPolicy{{Version: 8, EffectiveFrom: "2026-01-01", MonthlyMinor: 1500, CarryRule: plan.CarryCash, ReleasedPaymentRule: "roll_all"}}
	limit, err := b.PermissionOn(date.MustNew(2026, 1, 1))
	if err != nil || limit.Minor() != 1500 || b.Funding.PlanningStartMonth != "2026-02" {
		t.Fatal("approved permission or stored preference changed", err)
	}
	paused = b.CashPlan(date.MustNew(2026, 1, 1))
	if paused.Spending.Changes[0].Limit.Minor() != 400 {
		t.Fatal("policy permission ignored pause")
	}
}

// Hand-calculated zero-interest golden: the bank balance is 1,200 USD after
// September's payment. September cash is 500 opening + 300 payday, while the
// 100 already paid is a fact, not another deduction from cash or principal.
func TestPlanningStartPaymentGolden(t *testing.T) {
	cur := money.MustLookup("USD")
	amt := func(n int64) money.Amount { return money.FromMinor(n*100, cur) }
	today := date.MustNew(2026, 9, 20)
	b := Budget{Set: true, Currency: "USD", Monthly: amt(300), PayDay: 25, Opening: amt(500), OpeningAsOf: today, Funding: &BudgetFunding{MonthlyMinor: 30000, SpentMinor: 10000, CashThrough: today.String(), PlanningStartMonth: "2026-10"}}
	loan := plan.Position{ID: "fixture", Balance: amt(1200), From: today, Excess: allocation.ExcessReducePrincipal, Contract: model.Contract{LoanID: "fixture", Version: 1, Currency: cur, EffectiveFrom: today, StartDate: date.MustNew(2026, 9, 15), MaturityDate: date.MustNew(2027, 9, 15), NotBeforeDue: date.MustNew(2026, 10, 1), PaymentDay: 15, DayCount: money.Actual365, Type: model.Annuity, Rounding: money.DefaultPolicy(cur), HasScheduled: true, ScheduledPayment: amt(100)}}
	policy := plan.Policy{Order: []int{0}, Timing: []plan.Timing{plan.OnReceipt}, Effect: []model.PrepaymentEffect{model.PrepayShortenTerm}}
	for _, tc := range []struct {
		name, month string
		extra, cash int64
	}{{"next month", "2026-10", 0, 800}, {"today", "", 200, 600}} {
		t.Run(tc.name, func(t *testing.T) {
			b.Funding.PlanningStartMonth = tc.month
			in := plan.Input{ValuationDate: today, Cash: b.CashPlan(today), Loans: []plan.Position{loan}}
			result, err := plan.Run(in, policy)
			if err != nil {
				t.Fatal(err)
			}
			if result.TotalPaid != amt(1200) || result.TotalInterest.Sign() != 0 {
				t.Fatal("already paid was counted as new principal or interest")
			}
			first := result.Timeline[0]
			if first.Required.Sign() != 0 || first.Extra != amt(tc.extra) || first.Cash != amt(tc.cash) || first.Owed != amt(1200-tc.extra) {
				t.Fatalf("September golden got required %d extra %d cash %d owed %d", first.Required.Minor(), first.Extra.Minor(), first.Cash.Minor(), first.Owed.Minor())
			}
		})
	}
	// A debt newly added after choosing October must not disappear. The engine
	// refuses the September due instead of quietly pretending it was paid.
	b.Funding.PlanningStartMonth = "2026-10"
	unpaid := loan
	unpaid.ID = "unpaid"
	unpaid.Contract.LoanID = "unpaid"
	unpaid.Contract.PaymentDay = 25
	unpaid.Contract.StartDate = date.MustNew(2026, 8, 25)
	unpaid.Contract.MaturityDate = date.MustNew(2027, 8, 25)
	unpaid.Contract.NotBeforeDue = date.Date{}
	_, err := plan.Run(plan.Input{ValuationDate: today, Cash: b.CashPlan(today), Loans: []plan.Position{unpaid}}, policy)
	var infeasible *plan.InfeasibleError
	if !errors.As(err, &infeasible) || infeasible.Constraint != "spending_limit" {
		t.Fatal("unpaid September debt silently skipped", err)
	}
}
