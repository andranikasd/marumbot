package app

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/money"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

func scenarioFixture(t *testing.T, explicit bool) PlanScenario {
	t.Helper()
	in := cacheInput(t)
	b := Budget{Set: true, Version: 7, Currency: "AMD", Monthly: in.Cash.Monthly, PayDay: 1}
	if explicit {
		b.Funding = &BudgetFunding{MonthlyMinor: in.Cash.Monthly.Minor()}
		b.Opening = in.Cash.Monthly
		b.OpeningAsOf = in.ValuationDate
	}
	in.Cash = b.CashPlan(in.ValuationDate)
	g := plan.Goal{Kind: plan.LeastInterest}
	r, err := plan.Search(in, g)
	if err != nil {
		t.Fatal(err)
	}
	m, err := manifestFor(in, g, r, b.Version)
	if err != nil {
		t.Fatal(err)
	}
	m.Sources = "source"
	return PlanScenario{Original: m, Budget: b}
}

func TestScenarioCloneReplayAndIsolation(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		s := scenarioFixture(t, explicit)
		before, _ := json.Marshal(s)
		monthly := int64(30_000_000)
		reserve := int64(500)
		payday := 1
		s.Changes = ScenarioChanges{MonthlyMinor: &monthly, ReserveMinor: &reserve, PayDay: &payday}
		if explicit {
			s.Changes.OneTimeCash = &BudgetCashEvent{On: "2026-02-01", Minor: 100000}
		}
		m, sh, err := scenarioCalculation(s, true)
		if err != nil {
			t.Fatal(err)
		}
		s.Policy = m.Policy
		s.ResultHash = m.ResultHash
		_, again, err := scenarioCalculation(s, false)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(sh, again) {
			t.Fatal("replay differs")
		}
		s.Changes = ScenarioChanges{}
		s.Policy = plan.Policy{}
		s.ResultHash = ""
		after, _ := json.Marshal(s)
		if string(before) != string(after) {
			t.Fatal("original mutated")
		}
		if s.Budget.Monthly.Minor() != 25_000_000 {
			t.Fatal("budget changed")
		}
	}
}

func TestScenarioTypedRefusals(t *testing.T) {
	cases := []struct {
		name     string
		explicit bool
		changes  ScenarioChanges
	}{
		{"retroactive", false, ScenarioChanges{MonthlyMinor: scenarioInt(30000000), EffectiveFrom: "2026-01-14"}},
		{"unfunded future", false, ScenarioChanges{MonthlyMinor: scenarioInt(30000000), EffectiveFrom: "2026-02-01"}},
		{"unfunded cash", false, ScenarioChanges{OneTimeCash: &BudgetCashEvent{On: "2026-02-01", Minor: 100}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := scenarioFixture(t, c.explicit)
			s.Changes = c.changes
			_, _, err := scenarioCalculation(s, true)
			var unsupported *plan.UnsupportedError
			if !errors.As(err, &unsupported) {
				t.Fatalf("want typed refusal, got %v", err)
			}
		})
	}
}

func TestScenarioFutureBudgetPreservesFunding(t *testing.T) {
	s := scenarioFixture(t, true)
	s.Changes = ScenarioChanges{MonthlyMinor: scenarioInt(30000000), EffectiveFrom: "2026-02-01"}
	m, _, err := scenarioCalculation(s, true)
	if err != nil {
		t.Fatal(err)
	}
	if m.Input.Cash.Monthly != s.Original.Input.Cash.Monthly {
		t.Fatal("invented cash from permission")
	}
	if len(m.Input.Cash.Spending.Changes) == 0 {
		t.Fatal("lost exact effective date")
	}
	s.Policy = m.Policy
	s.ResultHash = m.ResultHash
	if _, _, err = scenarioCalculation(s, false); err != nil {
		t.Fatal(err)
	}
	s.ResultHash = "tampered"
	if _, _, err = scenarioCalculation(s, false); !errors.Is(err, ErrConflict) {
		t.Fatal("tampered result accepted", err)
	}
}

func TestScenarioCashAndMapsAreCloned(t *testing.T) {
	s := scenarioFixture(t, true)
	s.Budget.Overrides = map[string]int64{"2026-02": 30000000}
	s.Budget.Funding.Events = []BudgetCashEvent{{On: "2026-02-01", Minor: 1}}
	b, err := scenarioBudget(s)
	if err != nil {
		t.Fatal(err)
	}
	b.Overrides["2026-02"] = 1
	b.Funding.Events[0].Minor = 9
	if s.Budget.Overrides["2026-02"] != 30000000 || s.Budget.Funding.Events[0].Minor != 1 {
		t.Fatal("clone aliases source")
	}
	s.Budget.Monthly = money.FromMinor(1, s.Budget.Monthly.Currency())
	_, _, err = scenarioCalculation(s, true)
	var unsupported *plan.UnsupportedError
	if !errors.As(err, &unsupported) {
		t.Fatal("mismatched source accepted", err)
	}
}
func scenarioInt(v int64) *int64 { return &v }

func TestScenarioExpectedCashIsExplicitPreviewAssumption(t *testing.T) {
	s := scenarioFixture(t, true)
	s.Changes.OneTimeCash = &BudgetCashEvent{On: "2026-02-01", Minor: 1000000, Expected: true}
	m, _, err := scenarioCalculation(s, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Input.Cash.Lumps) != 1 || m.Input.Cash.Lumps[0].Expected {
		t.Fatal("what-if cash was silently excluded")
	}
	if !s.Changes.OneTimeCash.Expected || len(s.Budget.Funding.Events) != 0 {
		t.Fatal("source declaration changed")
	}
	s.Policy = m.Policy
	s.ResultHash = m.ResultHash
	if _, _, err = scenarioCalculation(s, false); err != nil {
		t.Fatal("assumption does not replay", err)
	}
}

func TestScenarioPolicyEditChangesEffectivePermissionAndKeepsFuture(t *testing.T) {
	s := scenarioFixture(t, true)
	growth := int64(1000000)
	replacement := int64(35000000)
	s.Budget.Policies = []BudgetPolicy{
		{Version: 5, EffectiveFrom: "2026-01-01", MonthlyMinor: 25000000, CarryRule: "carry_cash", ReleasedPaymentRule: "roll_all", Growth: &BudgetPolicyGrowth{EveryMonths: 1, StartsOn: "2026-01-01", FixedMinor: &growth}, Adjustments: []BudgetPolicyAdjustment{{Month: "2026-01", ReplacementMinor: &replacement}}},
		{Version: 6, EffectiveFrom: "2026-03-01", MonthlyMinor: 40000000, CarryRule: "carry_cash", ReleasedPaymentRule: "roll_all"},
	}
	s.Original.Input.Cash = s.Budget.CashPlan(s.Original.Input.ValuationDate)
	r, err := plan.Search(s.Original.Input, s.Original.Goal)
	if err != nil {
		t.Fatal(err)
	}
	s.Original, err = manifestFor(s.Original.Input, s.Original.Goal, r, s.Budget.Version)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(s.Budget)
	s.Changes = ScenarioChanges{MonthlyMinor: scenarioInt(30000000)}
	b, err := scenarioBudget(s)
	if err != nil {
		t.Fatal(err)
	}
	current, err := b.PermissionOn(s.Original.Input.ValuationDate)
	if err != nil || current.Minor() != 30000000 {
		t.Fatal("current replacement hid new total", current, err)
	}
	if len(b.Policies) != 3 || b.Policies[2].Version != 8 {
		t.Fatal("did not append exact next policy")
	}
	if b.Policies[2].Growth.StartsOn != "2026-02-01" {
		t.Fatal("growth replayed before change")
	}
	future, err := b.PermissionOn(date.MustNew(2026, 3, 1))
	if err != nil || future.Minor() != 40000000 {
		t.Fatal("future policy lost", future, err)
	}
	after, _ := json.Marshal(s.Budget)
	if string(before) != string(after) {
		t.Fatal("original policies mutated")
	}
	m, _, err := scenarioCalculation(s, true)
	if err != nil {
		t.Fatal(err)
	}
	s.Policy = m.Policy
	s.ResultHash = m.ResultHash
	if _, _, err = scenarioCalculation(s, false); err != nil {
		t.Fatal(err)
	}
}
