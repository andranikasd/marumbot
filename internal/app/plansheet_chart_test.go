package app

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/andranikasd/marumbot/pkg/core/amortisation"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

func chartWorker(t *testing.T) (*Worker, plan.Input) {
	t.Helper()
	l := shadowLoan(t)
	// 1,200,000 AMD / 24 months = 50,000 AMD required per month.
	l.Contract.NominalRate = 0
	l.Contract.Prepayment.Charges = []model.PrepaymentCharge{{PercentBP: 100}}
	f := &shadowFakes{
		loans:  []UserLoan{l},
		budget: Budget{Currency: "AMD", Monthly: money.FromMinor(25_000_000, money.AMD), Set: true, PayDay: 1, Funding: &BudgetFunding{MonthlyMinor: 30_000_000}},
	}
	w := shadowWorker(t, f)
	v := date.From(w.Clock.Now(), time.UTC)
	return w, plan.Input{
		ValuationDate: v, Cash: f.budget.CashPlan(v),
		Loans: []plan.Position{{
			ID: l.ID, Name: l.Name, Contract: l.Contract, Balance: l.Balance,
			From: l.AsOf, Excess: l.Excess, Trust: l.Trust, OptionalExcluded: l.OptionalExcluded,
		}},
	}
}

func assertChartRows(t *testing.T, got []SheetMonth, source plan.Result) {
	t.Helper()
	if len(got) != len(source.Timeline) {
		t.Fatalf("chart has %d rows, engine has %d", len(got), len(source.Timeline))
	}
	var interest, fees int64
	for i, row := range got {
		m := source.Timeline[i]
		if row.N != m.Month || row.On != m.On.String() || row.RequiredMinor != m.Required.Minor() ||
			row.ExtraMinor != m.Extra.Minor() || row.FeesMinor != m.Fees.Minor() ||
			row.InterestMinor != m.Interest.Minor() || row.OwedMinor != m.Owed.Minor() || row.Cleared != m.Cleared {
			t.Fatalf("month %d differs from its engine source: %+v", i, row)
		}
		if len(row.Loans) != len(m.Loans) {
			t.Fatalf("month %d lost loan rows", i)
		}
		var loanFees int64
		for j, l := range row.Loans {
			original := m.Loans[j]
			if l.ID != original.ID || l.Name != original.Name || l.PaidMinor != original.Paid.Minor() ||
				l.ExtraMinor != original.Extra.Minor() || l.FeesMinor != original.Fees.Minor() || l.OwedMinor != original.Owed.Minor() ||
				l.Cleared != original.Cleared || l.FreedMinor != original.Freed.Minor() {
				t.Fatalf("month %d loan %d differs from source: %+v", i, j, l)
			}
			loanFees += l.FeesMinor
		}
		if loanFees != row.FeesMinor {
			t.Fatalf("month %d per-loan fees do not match monthly fees", i)
		}
		interest += row.InterestMinor
		fees += row.FeesMinor
	}
	if interest != source.TotalInterest.Minor() || fees != source.TotalFees.Minor() {
		t.Fatal("chart sums differ from engine totals")
	}
}

func TestPlanSheetChartSeriesMatchEngine(t *testing.T) {
	w, in := chartWorker(t)
	g := plan.Goal{Kind: plan.Fastest}
	sh, err := w.PlanSheet(context.Background(), "chart-user", &g)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := plan.Search(in, g)
	if err != nil {
		t.Fatal(err)
	}
	assertChartRows(t, sh.Months, rep.Best)
	assertChartRows(t, sh.MinimumMonths, rep.Minimum)
	if !sh.BaselineAvailable || len(sh.MinimumMonths) <= len(sh.Months) || rep.Best.TotalFees.Sign() <= 0 {
		t.Fatal("fixture must have distinct timelines and optimized fees")
	}
	// Verify the baseline independently of Search/minimum and the sheet
	// converter: follow required-only payments with the ORIGINAL funding.
	required, err := plan.Run(in, plan.Policy{
		RequiredOnly: true, Order: []int{0}, Timing: []plan.Timing{plan.OnDue},
		Effect: []model.PrepaymentEffect{model.PrepayReduceInstalment}, Rollover: plan.KeepFreed,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertChartRows(t, sh.MinimumMonths, required)
	l := in.Loans[0]
	schedule, err := amortisation.Build(l.Contract, l.Balance, l.From)
	if err != nil {
		t.Fatal(err)
	}
	// Explicit spending includes the valuation month before the first due.
	// Keep this real opening row instead of synthesizing a payment for it.
	opening := sh.MinimumMonths[0]
	if len(sh.MinimumMonths) != len(schedule.Rows)+1 || opening.On != in.ValuationDate.String() ||
		opening.RequiredMinor != 0 || opening.ExtraMinor != 0 || opening.OwedMinor != l.Balance.Minor() {
		t.Fatal("baseline lost the opening state or contractual payment rows")
	}
	for i, row := range sh.MinimumMonths[1:] {
		original := schedule.Rows[i]
		if row.RequiredMinor != original.Payment.Minor() || row.InterestMinor != original.Interest.Minor() ||
			row.OwedMinor != original.Closing.Minor() || row.ExtraMinor != 0 || row.FeesMinor != 0 {
			t.Fatalf("baseline row %d differs from independent amortization", i)
		}
	}
	if sh.Summary.BudgetMinor != 25_000_000 {
		t.Fatal("sheet replaced spending permission with funding")
	}
	wantSaved := rep.Minimum.Cost().Minor() - rep.Best.Cost().Minor()
	if wantSaved >= 0 {
		t.Fatal("fixture must preserve negative savings from accelerated payoff fees")
	}
	if sh.Summary.SavedMinor == nil || *sh.Summary.SavedMinor != wantSaved ||
		sh.Summary.SavedMonths == nil || *sh.Summary.SavedMonths != rep.Minimum.Months-rep.Best.Months {
		t.Fatal("savings do not match the actual engine results")
	}
	if sh.InputHash != searchFingerprint(in, plan.Goal{}) || sh.AsOf != in.ValuationDate.String() ||
		sh.EngineVersion != rep.Certificate.EngineVersion || sh.CurrencyExponent != 2 || sh.SettlementQuantum != 10 {
		t.Fatalf("incorrect provenance or units: %+v", sh)
	}
	c := sh.Certificate
	if c.Policies != rep.Certificate.Policies || c.FeasiblePolicies != rep.Certificate.FeasiblePolicies ||
		c.Strength != rep.Certificate.Strength || c.Truncation != rep.Certificate.Truncation ||
		c.LowerBoundMinor != nil || rep.Certificate.LowerBound != nil ||
		c.GapMinor != nil || rep.Certificate.Gap != nil {
		t.Fatalf("incorrect typed certificate: %+v", c)
	}
	// Fees have no solved admissible lower bound: preserve unknown as null,
	// not zero or the winning plan's interest. Other certificate amounts remain numbers.
	b, err := json.Marshal(sh)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Certificate map[string]json.RawMessage `json:"certificate"`
		Months      []SheetMonth               `json:"months"`
		Minimum     []SheetMonth               `json:"minimum_months"`
	}
	if err := json.Unmarshal(b, &wire); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"policies", "feasible_policies", "best_cost_minor"} {
		var n int64
		if len(wire.Certificate[key]) == 0 || json.Unmarshal(wire.Certificate[key], &n) != nil {
			t.Fatalf("certificate %s is not a number: %s", key, wire.Certificate[key])
		}
	}
	for _, key := range []string{"lower_bound_minor", "gap_minor"} {
		if string(wire.Certificate[key]) != "null" {
			t.Fatalf("unknown certificate %s must be null, got %s", key, wire.Certificate[key])
		}
	}
	if !reflect.DeepEqual(wire.Months, sh.Months) || !reflect.DeepEqual(wire.Minimum, sh.MinimumMonths) {
		t.Fatal("chart series changed during JSON serialization")
	}
}

func TestPlanSheetChartUnavailableBaselineIsNotZeroSavings(t *testing.T) {
	w, in := chartWorker(t)
	g := plan.Goal{Kind: plan.Fastest}
	rep, err := plan.Search(in, g)
	if err != nil {
		t.Fatal(err)
	}
	// Match Rank's swallowed Infeasible result without inventing any totals.
	rep.Minimum = plan.Result{}
	w.plans.entries = map[string]searchEntry{searchFingerprint(in, g): {rep: rep, addedAt: w.Clock.Now()}}
	sh, err := w.PlanSheet(context.Background(), "chart-user", &g)
	if err != nil {
		t.Fatal(err)
	}
	assertChartRows(t, sh.Months, rep.Best)
	if sh.BaselineAvailable || len(sh.MinimumMonths) != 0 || sh.Summary.SavedMinor != nil || sh.Summary.SavedMonths != nil {
		t.Fatal("unavailable baseline produced a savings comparison")
	}
	b, err := json.Marshal(sh)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Available bool                       `json:"baseline_available"`
		Minimum   json.RawMessage            `json:"minimum_months"`
		Summary   map[string]json.RawMessage `json:"summary"`
	}
	if err := json.Unmarshal(b, &wire); err != nil {
		t.Fatal(err)
	}
	if wire.Available || string(wire.Minimum) != "[]" || string(wire.Summary["saved_minor"]) != "null" ||
		string(wire.Summary["saved_months"]) != "null" {
		t.Fatalf("unavailable baseline JSON misrepresents savings: %s", b)
	}
}

func TestPlanSheetChartZeroSavingsAndCurrencyUnits(t *testing.T) {
	for _, code := range []string{"JPY", "KWD"} {
		t.Run(code, func(t *testing.T) {
			in := cacheInput(t)
			cur := money.MustLookup(code)
			in.Cash.Monthly = money.FromMinor(25_000_000, cur)
			in.Loans[0].Balance = money.FromMinor(120_000_000, cur)
			in.Loans[0].Contract.Currency = cur
			in.Loans[0].Contract.Rounding = money.DefaultPolicy(cur)
			in.Loans[0].Contract.NominalRate = 0
			g := plan.Goal{Kind: plan.Fastest}
			rep, err := plan.Search(in, g)
			if err != nil {
				t.Fatal(err)
			}
			sh, err := sheetFromReport(in, g, rep, in.Loans[0].Balance, in.Cash.Monthly, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !sh.BaselineAvailable || sh.Summary.SavedMinor == nil || *sh.Summary.SavedMinor != 0 ||
				sh.Summary.SavedMonths == nil || *sh.Summary.SavedMonths <= 0 {
				t.Fatal("zero interest savings must still expose the actual months saved")
			}
			b, err := json.Marshal(sh.Certificate)
			if err != nil {
				t.Fatal(err)
			}
			var wire map[string]json.RawMessage
			if err := json.Unmarshal(b, &wire); err != nil {
				t.Fatal(err)
			}
			if string(wire["lower_bound_minor"]) != "null" || string(wire["gap_minor"]) != "null" {
				t.Fatalf("unknown bounds must remain null: %s", b)
			}
			if sh.Currency != code || sh.CurrencyExponent != cur.Exponent || sh.SettlementQuantum != cur.SettlementUnit {
				t.Fatal("currency units were hardcoded")
			}
		})
	}
}

func TestPlanSheetChartIdentityIsNormalizedAndSharedAcrossGoals(t *testing.T) {
	w, in := chartWorker(t)
	// Move past one contractual due date, so raw and normalized balances
	// differ and a raw-input implementation cannot accidentally pass.
	w.Clock = &fixedClock{at: time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)}
	in.ValuationDate = date.MustNew(2026, 3, 1)
	normalized, assumed, err := plan.Normalize(in)
	if err != nil {
		t.Fatal(err)
	}
	want := searchFingerprint(normalized, plan.Goal{})
	if assumed[in.Loans[0].ID] != 1 || want == searchFingerprint(in, plan.Goal{}) {
		t.Fatal("fixture must normalize exactly one assumed payment")
	}
	for _, g := range []plan.Goal{
		{Kind: plan.LeastInterest},
		{Kind: plan.Fastest},
		{Kind: plan.FirstWin},
		{Kind: plan.Relief, Cap: money.FromMinor(25_000_000, money.AMD)},
	} {
		sh, err := w.PlanSheet(context.Background(), "chart-user", &g)
		if err != nil {
			t.Fatal(err)
		}
		if sh.InputHash != want || sh.Goal != g.Kind.String() || sh.CapMinor != g.Cap.Minor() {
			t.Fatalf("goal %s lost shared identity or active goal: %s", g, sh.InputHash)
		}
	}
	// A bank-confirmed anchor equivalent to the normalized snapshot must
	// identify the same inputs as the older anchor with an assumed payment.
	f := w.Loans.(*shadowFakes)
	f.loans[0].Balance = normalized.Loans[0].Balance
	f.loans[0].AsOf = normalized.Loans[0].From
	sh, err := w.PlanSheet(context.Background(), "chart-user", nil)
	if err != nil {
		t.Fatal(err)
	}
	if sh.InputHash != want {
		t.Fatal("equivalent normalized anchors have different identities")
	}
}
