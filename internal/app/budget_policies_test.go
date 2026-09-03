package app

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/money"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

func budgetPolicyFixture() Budget {
	cur := money.MustLookup("USD")
	return Budget{
		Set: true, Version: 7, Currency: "USD", Monthly: money.FromMinor(1000, cur), PayDay: 1,
		Opening: money.FromMinor(700, cur), OpeningAsOf: date.MustNew(2026, 1, 1), Reserve: money.FromMinor(100, cur),
		Funding: &BudgetFunding{MonthlyMinor: 500, SpentMinor: 400, CashThrough: "2026-01-01", Events: []BudgetCashEvent{{On: "2026-01-01", Minor: 500}, {On: "2026-01-20", Minor: 900, Expected: true}}},
	}
}

func TestBudgetPolicyIndependentGrowthAndFundingFixture(t *testing.T) {
	b := budgetPolicyFixture()
	fixed, capMinor, delta := int64(200), int64(1800), int64(-200)
	b.Policies = []BudgetPolicy{{
		Version: 8, EffectiveFrom: "2026-01-15", MonthlyMinor: 1500, CarryRule: "carry_cash", ReleasedPaymentRule: "roll_all",
		Growth:      &BudgetPolicyGrowth{EveryMonths: 1, StartsOn: "2026-02-01", FixedMinor: &fixed, MaximumMinor: &capMinor},
		Adjustments: []BudgetPolicyAdjustment{{Month: "2026-02", DeltaMinor: &delta}},
	}}
	base, fallback, err := b.CashPlans(date.MustNew(2026, 1, 1))
	if err != nil {
		t.Fatal(err)
	}
	// Hand-computed: Jan starts 1000, Jan15 1500, Feb 1500+200-200,
	// March capped at 1800. Without growth: Feb 1500-200, March 1500.
	for _, tc := range []struct {
		cp   plan.CashPlan
		want []int64
	}{{base, []int64{1000, 1500, 1500, 1800}}, {fallback, []int64{1000, 1500, 1300, 1500}}} {
		for i, want := range tc.want {
			if tc.cp.Spending.Changes[i].Limit.Minor() != want {
				t.Fatalf("change %d: got %d want %d", i, tc.cp.Spending.Changes[i].Limit.Minor(), want)
			}
		}
		if tc.cp.Monthly.Minor() != 500 || tc.cp.OpeningCash.Minor() != 700 || tc.cp.Spending.Spent.Minor() != 400 || tc.cp.ReserveFloor.Minor() != 100 || tc.cp.CashThrough.String() != "2026-01-01" {
			t.Fatal("policy modified cash facts")
		}
		if len(tc.cp.Lumps) != 1 || !tc.cp.Lumps[0].Expected {
			t.Fatal("cash-through or expected class lost")
		}
	}
	if !base.Spending.Changes[1].On.Equal(date.MustNew(2026, 1, 15)) {
		t.Fatal("activation moved early")
	}
	base.Spending.Changes[0].Limit = money.Zero(b.Monthly.Currency())
	if fallback.Spending.Changes[0].Limit.Minor() != 1000 {
		t.Fatal("comparison inputs alias")
	}
	if b.Funding.SpentMinor != 400 || *b.Policies[0].Growth.FixedMinor != 200 {
		t.Fatal("source mutation")
	}
}

func TestBudgetPoliciesSameDateLatestVersionWins(t *testing.T) {
	b := budgetPolicyFixture()
	p := BudgetPolicy{Version: 8, EffectiveFrom: "2026-01-01", MonthlyMinor: 1500, CarryRule: "carry_cash", ReleasedPaymentRule: "roll_all"}
	next := p
	next.Version = 9
	next.MonthlyMinor = 2000
	b.Policies = []BudgetPolicy{next, p}
	cp := b.CashPlan(date.MustNew(2026, 1, 1))
	if cp.Spending.Changes[0].Limit.Minor() != 2000 {
		t.Fatal("latest effective declaration not chosen")
	}
	if b.Policies[0].Version != 9 {
		t.Fatal("source declaration order mutated")
	}
}

func TestBudgetPoliciesRefuseUnsupportedSemantics(t *testing.T) {
	b := budgetPolicyFixture()
	p := BudgetPolicy{Version: 8, EffectiveFrom: "2026-01-01", MonthlyMinor: 1500, CarryRule: "carry_cash", ReleasedPaymentRule: "roll_all"}
	for _, rule := range []string{"batch_until", "carry_to_date", "invented"} {
		bad := p
		bad.CarryRule = rule
		if bad.Validate("USD") == nil {
			t.Fatal("accepted carry", rule)
		}
	}
	for _, rule := range []string{"roll_percent", "roll_amount", "until_goal_then_release"} {
		bad := p
		bad.ReleasedPaymentRule = rule
		if bad.Validate("USD") == nil {
			t.Fatal("accepted release", rule)
		}
	}
	b.Policies = []BudgetPolicy{p}
	b.Funding = nil
	cp := b.CashPlan(date.MustNew(2026, 1, 1))
	if cp.Spending == nil || cp.Spending.RuleError == "" {
		t.Fatal("invalid policy became legacy funding")
	}
}

type budgetPolicyFake struct {
	b      Budget
	writes int
}

func (f *budgetPolicyFake) Budget(context.Context, string) (Budget, error)            { return f.b, nil }
func (*budgetPolicyFake) SetBudget(context.Context, string, string, int64, int) error { return nil }
func (f *budgetPolicyFake) AppendBudgetPolicy(_ context.Context, _, _ string, version int64, p BudgetPolicy) (int64, error) {
	if version != f.b.Version {
		return 0, ErrConflict
	}
	f.writes++
	f.b.Version++
	f.b.Policies = append(f.b.Policies, p)
	return f.b.Version, nil
}

func TestSaveBudgetPolicyPreservesReconciliationFacts(t *testing.T) {
	f := &budgetPolicyFake{b: budgetPolicyFixture()}
	before := *f.b.Funding
	p := BudgetPolicy{EffectiveFrom: "2026-01-15", MonthlyMinor: 1500, CarryRule: "carry_cash", ReleasedPaymentRule: "roll_all"}
	today := date.MustNew(2026, 1, 1)
	version, err := SaveBudgetPolicy(t.Context(), f, "owner", "USD", 7, today, p)
	if err != nil || version != 8 || f.writes != 1 {
		t.Fatalf("save %d %v", version, err)
	}
	if !reflect.DeepEqual(before, *f.b.Funding) || f.b.Opening.Minor() != 700 {
		t.Fatal("reconciliation facts changed")
	}
	if _, err := SaveBudgetPolicy(t.Context(), f, "owner", "USD", 7, today, p); !errors.Is(err, ErrConflict) {
		t.Fatal("stale version accepted")
	}
	p.EffectiveFrom = "2025-12-31"
	if _, err := SaveBudgetPolicy(t.Context(), f, "owner", "USD", 8, today, p); err == nil {
		t.Fatal("backdated rule accepted")
	}
	if f.writes != 1 {
		t.Fatal("invalid declaration written")
	}
}

func TestBudgetUserCycleRequiresExplicitSpendingPeriod(t *testing.T) {
	b := budgetPolicyFixture()
	b.OpeningAsOf = date.MustNew(2026, 1, 20)
	b.Policies = []BudgetPolicy{{Version: 8, EffectiveFrom: "2026-01-15", CycleDay: 15, MonthlyMinor: 1500, CarryRule: "carry_cash", ReleasedPaymentRule: "roll_all"}}
	today := date.MustNew(2026, 1, 21)
	if _, _, err := b.CashPlans(today); err == nil {
		t.Fatal("calendar spending silently relabelled")
	}
	b.Funding.SpentPeriodStart = "2026-01-15"
	cp, _, err := b.CashPlans(today)
	if err != nil || cp.Spending.Spent.Minor() != 400 {
		t.Fatalf("explicit cycle statement lost: %v", err)
	}
}

func TestBudgetConfirmedReleaseAppliesNextPeriodAndKeepsOverrideIntent(t *testing.T) {
	b := budgetPolicyFixture()
	before, after, fixed, replacement := int64(100), int64(0), int64(100), int64(700)
	b.Policies = []BudgetPolicy{{Version: 8, EffectiveFrom: "2026-01-01", MonthlyMinor: 1000, CarryRule: "carry_cash", ReleasedPaymentRule: "release_all", Growth: &BudgetPolicyGrowth{EveryMonths: 1, StartsOn: "2026-02-01", FixedMinor: &fixed}, Adjustments: []BudgetPolicyAdjustment{{Month: "2026-02", ReplacementMinor: &replacement}}}}
	fact := BudgetReleaseFact{PolicyVersion: 8, SourceID: "verified-source", On: "2026-01-20", BeforeMinor: &before, AfterMinor: &after}
	b.Releases = []BudgetReleaseFact{fact, fact}
	cp, fallback, err := b.CashPlans(date.MustNew(2026, 1, 21))
	if err != nil {
		t.Fatal(err)
	}
	// Jan is unchanged. February replacement is literal. March is 1200-100;
	// without growth March is 1000-100. Duplicate source cannot release twice.
	for i, want := range []int64{1000, 700, 1100} {
		if cp.Spending.Changes[i].Limit.Minor() != want {
			t.Fatalf("change %d", i)
		}
	}
	if fallback.Spending.Changes[2].Limit.Minor() != 900 || len(cp.Spending.ReleaseSources) != 1 {
		t.Fatal("fallback or provenance lost")
	}
	b.Releases[0].BeforeMinor = nil
	b.Releases = b.Releases[:1]
	if _, _, err := b.CashPlans(date.MustNew(2026, 1, 21)); err == nil {
		t.Fatal("unproven release accepted")
	}
}

func TestBudgetUserCycleGrowthIsNotAppliedAtCalendarStart(t *testing.T) {
	b := budgetPolicyFixture()
	b.Funding.SpentMinor = 0
	fixed := int64(100)
	b.Policies = []BudgetPolicy{{Version: 8, EffectiveFrom: "2026-01-15", CycleDay: 15, MonthlyMinor: 1000, CarryRule: "carry_cash", ReleasedPaymentRule: "roll_all", Growth: &BudgetPolicyGrowth{EveryMonths: 1, StartsOn: "2026-02-15", FixedMinor: &fixed}}}
	cp, _, err := b.CashPlans(date.MustNew(2026, 2, 1))
	if err != nil {
		t.Fatal(err)
	}
	if cp.Spending.Changes[0].Limit.Minor() != 1000 || cp.Spending.Changes[1].On.String() != "2026-02-15" || cp.Spending.Changes[1].Limit.Minor() != 1100 {
		t.Fatal("cycle growth shifted early")
	}
}

func TestBudgetMonthlyScenarioPreservesFuturePoliciesAndGrowthPhase(t *testing.T) {
	b := budgetPolicyFixture()
	growth, replacement := int64(100), int64(700)
	b.Policies = []BudgetPolicy{{Version: 8, EffectiveFrom: "2026-01-01", MonthlyMinor: 1000, CarryRule: "carry_cash", ReleasedPaymentRule: "roll_all", Growth: &BudgetPolicyGrowth{EveryMonths: 2, StartsOn: "2026-02-01", FixedMinor: &growth}, Adjustments: []BudgetPolicyAdjustment{{Month: "2026-03", ReplacementMinor: &replacement}, {Month: "2026-05", ReplacementMinor: &replacement}}}, {Version: 9, EffectiveFrom: "2026-06-01", MonthlyMinor: 3000, CarryRule: "no_carry", ReleasedPaymentRule: "roll_all"}}
	p, err := b.PolicyForMonthlyChange(date.MustNew(2026, 3, 15), 2000)
	if err != nil {
		t.Fatal(err)
	}
	if p.Growth.StartsOn != "2026-04-01" || len(p.Adjustments) != 1 || p.Adjustments[0].Month != "2026-05" {
		t.Fatal("future intent lost or current override retained")
	}
	p.Version = 10
	b.Policies = append(b.Policies, p)
	cp, _, err := b.CashPlans(date.MustNew(2026, 3, 15))
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []int64{2000, 2100, 700, 3000} {
		if cp.Spending.Changes[i].Limit.Minor() != want {
			t.Fatalf("period %d", i)
		}
	}
	if b.Policies[0].Growth.StartsOn != "2026-02-01" {
		t.Fatal("historical rule mutated")
	}
}

func TestBudgetCycleStatementSurvivesCalendarBoundary(t *testing.T) {
	b := budgetPolicyFixture()
	b.OpeningAsOf = date.MustNew(2026, 1, 20)
	b.Funding.CashThrough = "2026-01-20"
	b.Funding.SpentPeriodStart = "2026-01-15"
	b.Policies = []BudgetPolicy{{Version: 8, EffectiveFrom: "2026-01-15", CycleDay: 15, MonthlyMinor: 1000, CarryRule: "carry_cash", ReleasedPaymentRule: "roll_all"}}
	cp, _, err := b.CashPlans(date.MustNew(2026, 2, 3))
	if err != nil {
		t.Fatal(err)
	}
	if cp.OpeningCash.Minor() != 700 || cp.Spending.Spent.Minor() != 400 || cp.CashThrough.String() != "2026-01-20" {
		t.Fatal("calendar boundary erased cycle statement")
	}
}
