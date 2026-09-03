package app

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/allocation"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/money"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

func TestBudgetCashRoutingDeclarationRoundTrip(t *testing.T) {
	today := date.MustNew(2026, 9, 3)
	minimum := int64(5000)
	event := BudgetCashEvent{ID: "retained", On: today.String(), Minor: 5000, FromOpening: true, Routing: &BudgetCashRouting{
		Splits: []BudgetCashSplit{{LoanID: "a", Minor: 2000}, {LoanID: "b", Minor: 3000}}, HoldUntil: "2026-09-10", HoldMinimumMinor: &minimum,
	}}
	if err := ValidateBudgetCashEvents([]BudgetCashEvent{event}, "USD", today); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	var decoded BudgetCashEvent
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, event) {
		t.Fatal("stored routing declaration lost fields")
	}
	cur := money.MustLookup("USD")
	b := Budget{Currency: "USD", Monthly: money.FromMinor(10000, cur), Opening: money.FromMinor(8000, cur), OpeningAsOf: today, PayDay: 10, Funding: &BudgetFunding{MonthlyMinor: 0, CashThrough: today.String(), Events: []BudgetCashEvent{decoded}}}
	cash := b.CashPlan(today)
	if cash.Monthly.Sign() != 0 || cash.OpeningCash.Minor() != 8000 || len(cash.Lumps) != 1 {
		t.Fatal("retained cash was dropped or credited as income")
	}
	got := cash.Lumps[0]
	if got.ID != "retained" || !got.FromOpening || !got.On.Equal(today) || got.Amount.Minor() != 5000 || got.Routing.HoldMinimum.Minor() != 5000 || got.Routing.HoldUntil.String() != "2026-09-10" || len(got.Routing.Splits) != 2 || got.Routing.Splits[0].Amount.Minor() != 2000 || got.Routing.Splits[1].Amount.Minor() != 3000 {
		t.Fatalf("routing changed: %+v", got)
	}
	// The old declaration stays visible for editing, but cannot fund a fresh
	// plan tomorrow: no remaining earmark is inferred from an old receipt.
	tomorrow := date.MustNew(2026, 9, 4)
	if len(b.CashPlan(tomorrow).Lumps) != 1 {
		t.Fatal("stale earmark silently removed")
	}
	var unsupported *plan.UnsupportedError
	if !errors.As(b.validateCurrentCashRouting(tomorrow), &unsupported) {
		t.Fatal("stale routing must require a retained-cash declaration")
	}
}

func TestBudgetCashRoutingRejectsInvalidDeclarations(t *testing.T) {
	today := date.MustNew(2026, 9, 3)
	for name, event := range map[string]BudgetCashEvent{
		"split remainder":   {ID: "x", On: today.String(), Minor: 100, Routing: &BudgetCashRouting{Splits: []BudgetCashSplit{{LoanID: "a", Minor: 99}}}},
		"duplicate target":  {ID: "x", On: today.String(), Minor: 100, Routing: &BudgetCashRouting{Splits: []BudgetCashSplit{{LoanID: "a", Minor: 50}, {LoanID: "a", Minor: 50}}}},
		"expected retained": {ID: "x", On: today.String(), Minor: 100, Expected: true, FromOpening: true, Routing: &BudgetCashRouting{LoanID: "a"}},
		"empty route":       {ID: "x", On: today.String(), Minor: 100, Routing: &BudgetCashRouting{}},
		"missing identity":  {On: today.String(), Minor: 100, Routing: &BudgetCashRouting{LoanID: "a"}},
		"past hold":         {ID: "x", On: today.String(), Minor: 100, Routing: &BudgetCashRouting{LoanID: "a", HoldUntil: "2026-09-02"}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateBudgetCashEvents([]BudgetCashEvent{event}, "USD", today); err == nil {
				t.Fatal("accepted invalid cash restriction")
			}
		})
	}
	event := BudgetCashEvent{ID: "same", On: today.String(), Minor: 100}
	if err := ValidateBudgetCashEvents([]BudgetCashEvent{event, event}, "USD", today); err == nil {
		t.Fatal("duplicate identity accepted")
	}
}

func TestBudgetCashRoutingRequiresOwnedSupportedTarget(t *testing.T) {
	l := shadowLoan(t)
	l.Excess = allocation.ExcessReducePrincipal
	events := []BudgetCashEvent{{Routing: &BudgetCashRouting{LoanID: l.ID}}}
	if err := ValidateBudgetCashTargets(events, "AMD", []UserLoan{l}); err != nil {
		t.Fatal(err)
	}
	for _, loans := range [][]UserLoan{nil, {func() UserLoan { v := l; v.OptionalExcluded = true; return v }()}, {func() UserLoan { v := l; v.Excess = allocation.ExcessUnknown; return v }()}} {
		if err := ValidateBudgetCashTargets(events, "AMD", loans); err == nil {
			t.Fatal("unsafe target accepted")
		}
	}
}
