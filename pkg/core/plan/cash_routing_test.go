package plan

import (
	"errors"
	"reflect"
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

func routingFixture(t *testing.T) (Input, Policy) {
	t.Helper()
	in := comparisonInput(t)
	cur := money.MustLookup("USD")
	in.Cash = CashPlan{Monthly: money.FromMinor(400, cur)}
	for i := range in.Loans {
		l := &in.Loans[i]
		l.Contract.Currency = cur
		l.Contract.NominalRate = 0
		l.Contract.ScheduledPayment = money.FromMinor(200, cur)
		l.Contract.Rounding = money.Policy{Mode: money.HalfUp, Unit: 1}
		l.Balance = money.FromMinor(1000, cur)
	}
	return in, Policy{Name: "routing-golden", Order: []int{0, 1}, Timing: uniform(2, OnReceipt), Effect: effectVectors(in.Loans)[0]}
}

func routingAmount(in Input, n int64) money.Amount {
	return money.FromMinor(n, in.Cash.Monthly.Currency())
}

func routingExtras(actions []Action, on date.Date) map[string]int64 {
	out := map[string]int64{}
	for _, a := range actions {
		if a.Kind == Extra && a.On == on {
			out[a.LoanID] += a.Amount.Minor()
		}
	}
	return out
}

// Synthetic golden: two zero-interest principals of 1000 cents, fixed required
// payments of 200 each, 400 monthly funding, and one 300-cent routed receipt.
// Principal conservation and the exact requested split are hand-computed; no
// schedule/accrual primitive generates these expected figures.
func TestCashRoutingGoldenEarmarkAndSplit(t *testing.T) {
	for _, tt := range []struct {
		name  string
		route CashRouting
		want  map[string]int64
	}{
		{"earmark", CashRouting{LoanID: "b"}, map[string]int64{"b": 300}},
		{"split", CashRouting{Splits: []CashSplit{{LoanID: "b", Amount: money.FromMinor(200, money.MustLookup("USD"))}, {LoanID: "a", Amount: money.FromMinor(100, money.MustLookup("USD"))}}}, map[string]int64{"a": 100, "b": 200}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			in, pol := routingFixture(t)
			on := date.MustNew(2026, 1, 20)
			in.Cash.Lumps = []CashEvent{{ID: "bonus", On: on, Amount: routingAmount(in, 300), Routing: &tt.route}}
			got, actions, err := PaymentTimeline(in, pol)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(routingExtras(actions, on), tt.want) {
				t.Fatalf("allocation %v want %v", routingExtras(actions, on), tt.want)
			}
			if got.TotalPaid.Minor() != 2000 || got.TotalInterest.Sign() != 0 || got.TotalFees.Sign() != 0 {
				t.Fatal("zero-rate principal identity changed")
			}
			if got.Timeline[len(got.Timeline)-1].Cash.Minor() != 300 {
				t.Fatalf("closing cash = 2300 confirmed inflow - 2000 paid; got %s", got.Timeline[len(got.Timeline)-1].Cash)
			}
		})
	}
}

func TestCashRoutingGoldenDateAndThreshold(t *testing.T) {
	in, pol := routingFixture(t)
	first := date.MustNew(2026, 1, 20)
	second := date.MustNew(2026, 1, 25)
	until := date.MustNew(2026, 1, 28)
	route := &CashRouting{LoanID: "b", HoldUntil: until, HoldMinimum: routingAmount(in, 300)}
	in.Cash.Lumps = []CashEvent{{ID: "first", On: first, Amount: routingAmount(in, 100), Routing: route}, {ID: "second", On: second, Amount: routingAmount(in, 200), Routing: route}}
	_, actions, err := PaymentTimeline(in, pol)
	if err != nil {
		t.Fatal(err)
	}
	if len(routingExtras(actions, first)) != 0 || len(routingExtras(actions, second)) != 0 || routingExtras(actions, until)["b"] != 300 {
		t.Fatal("both date and aggregate threshold must pass before 300 is routed", actions)
	}
	// Reordering input events never changes the trace.
	in.Cash.Lumps[0], in.Cash.Lumps[1] = in.Cash.Lumps[1], in.Cash.Lumps[0]
	_, again, err := PaymentTimeline(in, pol)
	if err != nil || !reflect.DeepEqual(actions, again) {
		t.Fatal("input event order changed trace", err)
	}
}

func TestCashRoutingGoldenFromOpeningIsNotSecondReceipt(t *testing.T) {
	in, pol := routingFixture(t)
	in.Cash.OpeningCash = routingAmount(in, 300)
	in.Cash.Lumps = []CashEvent{{ID: "retained", On: in.ValuationDate, Amount: routingAmount(in, 300), FromOpening: true, Routing: &CashRouting{LoanID: "b"}}}
	got, actions, err := PaymentTimeline(in, pol)
	if err != nil {
		t.Fatal(err)
	}
	if routingExtras(actions, in.ValuationDate)["b"] != 300 || got.Timeline[len(got.Timeline)-1].Cash.Minor() != 300 {
		t.Fatal("opening earmark was spent elsewhere or counted twice")
	}
}

func TestCashRoutingNeverSpillsClosedTargetOrFundsRequired(t *testing.T) {
	in, pol := routingFixture(t)
	on := date.MustNew(2026, 1, 20)
	in.Loans[1].Balance = routingAmount(in, 100)
	in.Cash.Lumps = []CashEvent{{ID: "reserved", On: on, Amount: routingAmount(in, 300), Routing: &CashRouting{LoanID: "b"}}}
	got, actions, err := PaymentTimeline(in, pol)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(routingExtras(actions, on), map[string]int64{"b": 100}) {
		t.Fatal("payoff remainder spilled", actions)
	}
	if got.Timeline[len(got.Timeline)-1].Cash.Minor() < 200 {
		t.Fatal("unused earmark disappeared")
	}
	in, pol = routingFixture(t)
	in.Cash.Monthly = routingAmount(in, 0)
	in.Cash.Lumps = []CashEvent{{ID: "reserved", On: on, Amount: routingAmount(in, 300), Routing: &CashRouting{LoanID: "b", HoldUntil: date.MustNew(2026, 4, 1)}}}
	_, err = Run(in, pol)
	var inf *InfeasibleError
	if !errors.As(err, &inf) || inf.Available.Minor() != 0 {
		t.Fatalf("held money funded required payment: %v", err)
	}
}

func TestCashRoutingRespectsSpendingAndNoCarry(t *testing.T) {
	in, pol := routingFixture(t)
	in.ValuationDate = date.MustNew(2026, 1, 15)
	in.Cash.PayDay = 15
	in.Cash.Spending = &SpendingPlan{Monthly: routingAmount(in, 400), CarryRule: NoCarry}
	in.Cash.Lumps = []CashEvent{{ID: "reserved", On: date.MustNew(2026, 2, 20), Amount: routingAmount(in, 300), Routing: &CashRouting{LoanID: "b"}}}
	got, actions, err := PaymentTimeline(in, pol)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range actions {
		if a.Kind == Extra && a.On.Month() == 2 {
			t.Fatal("routing bypassed required-payment permission reserve")
		}
	}
	if got.TotalPaid.Minor() != 2000 {
		t.Fatal("principal mismatch")
	}
}

func TestCashRoutingExpectedMoneyIsNotFunding(t *testing.T) {
	in, pol := routingFixture(t)
	on := date.MustNew(2026, 1, 20)
	in.Cash.Lumps = []CashEvent{{ID: "expected", On: on, Amount: routingAmount(in, 300), Expected: true, Routing: &CashRouting{LoanID: "b"}}}
	_, actions, err := PaymentTimeline(in, pol)
	if err != nil {
		t.Fatal(err)
	}
	if len(routingExtras(actions, on)) != 0 {
		t.Fatal("uncertain earmark was spent")
	}
}

func TestCashRoutingValidation(t *testing.T) {
	for _, tt := range []struct {
		name string
		edit func(*Input)
	}{
		{"missing ID", func(in *Input) { in.Cash.Lumps[0].ID = "" }},
		{"unknown target", func(in *Input) { in.Cash.Lumps[0].Routing.LoanID = "unknown" }},
		{"excluded target", func(in *Input) { in.Loans[1].OptionalExcluded = true }},
		{"wrong sum", func(in *Input) {
			in.Cash.Lumps[0].Routing = &CashRouting{Splits: []CashSplit{{LoanID: "b", Amount: routingAmount(*in, 100)}}}
		}},
		{"opening excess", func(in *Input) { in.Cash.Lumps[0].FromOpening = true }},
		{"mixed currency", func(in *Input) {
			in.Cash.Lumps[0].FromOpening = true
			in.Cash.Lumps[0].Amount = money.FromMinor(300, money.MustLookup("AMD"))
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			in, pol := routingFixture(t)
			in.Cash.Lumps = []CashEvent{{ID: "bad", On: in.ValuationDate, Amount: routingAmount(in, 300), Routing: &CashRouting{LoanID: "b"}}}
			tt.edit(&in)
			_, err := Run(in, pol)
			var unsupported *UnsupportedError
			if !errors.As(err, &unsupported) {
				t.Fatalf("want typed refusal, got %v", err)
			}
		})
	}
}

func TestCashRoutingSameDateAggregatesBeforeFeeQuote(t *testing.T) {
	in, pol := routingFixture(t)
	on := date.MustNew(2026, 1, 20)
	in.Loans[1].Contract.Prepayment.Charges = []model.PrepaymentCharge{{Fixed: routingAmount(in, 10)}}
	route := &CashRouting{LoanID: "b", HoldMinimum: routingAmount(in, 300)}
	in.Cash.Lumps = []CashEvent{{ID: "one", On: on, Amount: routingAmount(in, 100), Routing: route}, {ID: "two", On: on, Amount: routingAmount(in, 200), Routing: route}}
	_, actions, err := PaymentTimeline(in, pol)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, a := range actions {
		if a.On == on && a.Kind == Extra {
			count++
			if a.LoanID != "b" || a.Amount.Minor() != 300 || a.Fee.Minor() != 10 {
				t.Fatal("golden: 290 principal + one 10 fee", a)
			}
		}
	}
	if count != 1 {
		t.Fatal("same-date receipt aggregation charged multiple fees")
	}
	in.Cash.Lumps[0], in.Cash.Lumps[1] = in.Cash.Lumps[1], in.Cash.Lumps[0]
	_, again, err := PaymentTimeline(in, pol)
	if err != nil || !reflect.DeepEqual(actions, again) {
		t.Fatal("same-date ordering changed fees", err)
	}
}

func TestCashRoutingHeldPoolUsesPolicyOnlyAfterRelease(t *testing.T) {
	in, pol := routingFixture(t)
	on := date.MustNew(2026, 1, 20)
	until := date.MustNew(2026, 1, 28)
	in.Cash.Lumps = []CashEvent{{ID: "hold", On: on, Amount: routingAmount(in, 300), Routing: &CashRouting{HoldUntil: until}}}
	_, actions, err := PaymentTimeline(in, pol)
	if err != nil {
		t.Fatal(err)
	}
	if len(routingExtras(actions, on)) != 0 || !reflect.DeepEqual(routingExtras(actions, until), map[string]int64{"a": 300}) {
		t.Fatal("held pool released before date or ignored policy")
	}
}

func TestCashRoutingOnDueDoesNotClaimEarlyCredit(t *testing.T) {
	in, pol := routingFixture(t)
	pol.Timing = uniform(2, OnDue)
	on := date.MustNew(2026, 1, 20)
	due := date.MustNew(2026, 2, 15)
	in.Cash.Lumps = []CashEvent{{ID: "due", On: on, Amount: routingAmount(in, 300), Routing: &CashRouting{LoanID: "b"}}}
	r, actions, err := PaymentTimeline(in, pol)
	if err != nil {
		t.Fatal(err)
	}
	if len(routingExtras(actions, on)) != 0 || routingExtras(actions, due)["b"] != 300 || r.TimingCredited {
		t.Fatal("due-date routing claimed early credit", actions)
	}
}
