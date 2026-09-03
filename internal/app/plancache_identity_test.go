package app

import (
	"reflect"
	"strings"
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

func identityInput(t *testing.T) plan.Input {
	t.Helper()
	in := cacheInput(t)
	a := money.FromMinor(100, money.AMD)
	in.Cash.OpeningCash = a
	in.Cash.ReserveFloor = a
	in.Cash.MonthlyOverrides = map[string]money.Amount{"2026-02": a, "2026-03": a}
	in.Cash.Lumps = []plan.CashEvent{{On: in.ValuationDate, Amount: a}}
	in.Cash.Spending = &plan.SpendingPlan{
		Monthly: a, Spent: a,
		Overrides: map[string]money.Amount{"2026-02": a, "2026-03": a},
	}
	in.Loans[0].Contract.ScheduledPayment = a
	in.Loans[0].Contract.Prepayment = model.Prepayment{
		MinAmount: a,
		Charges:   []model.PrepaymentCharge{{Fixed: a, FreeAllowance: a, MinCharge: a, MaxCharge: a}},
	}
	second := in.Loans[0]
	second.ID = "b"
	second.Contract.LoanID = "b"
	in.Loans = append(in.Loans, second)
	return in
}

func TestSearchFingerprintIdentityMutations(t *testing.T) {
	amount := func() money.Amount { return money.FromMinor(101, money.AMD) }
	cases := []struct {
		name   string
		mutate func(*plan.Input)
	}{
		{"valuation date", func(in *plan.Input) { in.ValuationDate = date.AddDays(in.ValuationDate, 1) }},
		{"horizon", func(in *plan.Input) { in.Horizon++ }},
		{"monthly subunit", func(in *plan.Input) { in.Cash.Monthly = money.FromMinor(25_000_001, money.AMD) }},
		{"pay day", func(in *plan.Input) { in.Cash.PayDay++ }},
		{"opening subunit", func(in *plan.Input) { in.Cash.OpeningCash = amount() }},
		{"reserve subunit", func(in *plan.Input) { in.Cash.ReserveFloor = amount() }},
		{"override subunit", func(in *plan.Input) { in.Cash.MonthlyOverrides["2026-02"] = amount() }},
		{"override key", func(in *plan.Input) { delete(in.Cash.MonthlyOverrides, "2026-02") }},
		{"lump date", func(in *plan.Input) { in.Cash.Lumps[0].On = date.AddDays(in.ValuationDate, 1) }},
		{"lump subunit", func(in *plan.Input) { in.Cash.Lumps[0].Amount = amount() }},
		{"lump expected", func(in *plan.Input) { in.Cash.Lumps[0].Expected = true }},
		{"spending monthly subunit", func(in *plan.Input) { in.Cash.Spending.Monthly = amount() }},
		{"spending spent subunit", func(in *plan.Input) { in.Cash.Spending.Spent = amount() }},
		{"spending override subunit", func(in *plan.Input) { in.Cash.Spending.Overrides["2026-02"] = amount() }},
		{"spending override key", func(in *plan.Input) { delete(in.Cash.Spending.Overrides, "2026-02") }},
		{"spending absent", func(in *plan.Input) { in.Cash.Spending = nil }},
		{"loan order", func(in *plan.Input) { in.Loans[0], in.Loans[1] = in.Loans[1], in.Loans[0] }},
		{"loan id", func(in *plan.Input) { in.Loans[0].ID += "x" }},
		{"loan name", func(in *plan.Input) { in.Loans[0].Name += "x" }},
		{"balance subunit", func(in *plan.Input) { in.Loans[0].Balance = money.FromMinor(120_000_001, money.AMD) }},
		{"anchor date", func(in *plan.Input) { in.Loans[0].From = date.AddDays(in.ValuationDate, 1) }},
		{"excess", func(in *plan.Input) { in.Loans[0].Excess++ }},
		{"trust", func(in *plan.Input) { in.Loans[0].Trust += "x" }},
		{"excluded", func(in *plan.Input) { in.Loans[0].OptionalExcluded = true }},
	}
	g := plan.Goal{Kind: plan.LeastInterest}
	base := searchFingerprint(identityInput(t), g)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := identityInput(t)
			tc.mutate(&in)
			if searchFingerprint(in, g) == base {
				t.Fatal("mutation did not change fingerprint")
			}
		})
	}
}

func TestSearchFingerprintFullContract(t *testing.T) {
	a := money.FromMinor(101, money.AMD)
	cases := []struct {
		name   string
		mutate func(*model.Contract)
	}{
		{"loan id", func(c *model.Contract) { c.LoanID += "x" }},
		{"version", func(c *model.Contract) { c.Version++ }},
		{"currency code", func(c *model.Contract) { c.Currency.Code += "x" }},
		{"currency exponent", func(c *model.Contract) { c.Currency.Exponent++ }},
		{"currency settlement", func(c *model.Contract) { c.Currency.SettlementUnit++ }},
		{"currency name", func(c *model.Contract) { c.Currency.Name += "x" }},
		{"effective from", func(c *model.Contract) { c.EffectiveFrom = date.AddDays(c.EffectiveFrom, 1) }},
		{"effective through", func(c *model.Contract) { c.EffectiveThru = c.MaturityDate }},
		{"rate", func(c *model.Contract) { c.NominalRate++ }},
		{"day count", func(c *model.Contract) { c.DayCount++ }},
		{"type", func(c *model.Contract) { c.Type++ }},
		{"start date", func(c *model.Contract) { c.StartDate = date.AddDays(c.StartDate, 1) }},
		{"maturity date", func(c *model.Contract) { c.MaturityDate = date.AddDays(c.MaturityDate, 1) }},
		{"payment day", func(c *model.Contract) { c.PaymentDay++ }},
		{"scheduled subunit", func(c *model.Contract) { c.ScheduledPayment = a }},
		{"has scheduled", func(c *model.Contract) { c.HasScheduled = true }},
		{"rounding mode", func(c *model.Contract) { c.Rounding.Mode++ }},
		{"rounding unit", func(c *model.Contract) { c.Rounding.Unit++ }},
		{"prepayment effect", func(c *model.Contract) { c.Prepayment.Effect++ }},
		{"prepayment fee", func(c *model.Contract) { c.Prepayment.FeeBP++ }},
		{"minimum subunit", func(c *model.Contract) { c.Prepayment.MinAmount = a }},
		{"charge from year", func(c *model.Contract) { c.Prepayment.Charges[0].FromYear++ }},
		{"charge through year", func(c *model.Contract) { c.Prepayment.Charges[0].ThroughYear++ }},
		{"charge percent", func(c *model.Contract) { c.Prepayment.Charges[0].PercentBP++ }},
		{"fixed subunit", func(c *model.Contract) { c.Prepayment.Charges[0].Fixed = a }},
		{"allowance subunit", func(c *model.Contract) { c.Prepayment.Charges[0].FreeAllowance = a }},
		{"minimum charge subunit", func(c *model.Contract) { c.Prepayment.Charges[0].MinCharge = a }},
		{"maximum charge subunit", func(c *model.Contract) { c.Prepayment.Charges[0].MaxCharge = a }},
		{"policy key", func(c *model.Contract) { c.AllocationPolicy.Key = "lender" }},
		{"policy version", func(c *model.Contract) { c.AllocationPolicy.Version++ }},
	}
	g := plan.Goal{Kind: plan.LeastInterest}
	base := searchFingerprint(identityInput(t), g)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := identityInput(t)
			tc.mutate(&in.Loans[0].Contract)
			if searchFingerprint(in, g) == base {
				t.Fatal("contract mutation did not change fingerprint")
			}
		})
	}
}

func TestSearchFingerprintGoalRawValues(t *testing.T) {
	in := identityInput(t)
	g := plan.Goal{Kind: plan.LeastInterest, Cap: money.FromMinor(100, money.AMD), Free: money.FromMinor(100, money.AMD)}
	base := searchFingerprint(in, g)
	for _, field := range []string{"cap", "free"} {
		t.Run(field, func(t *testing.T) {
			changed := g
			if field == "cap" {
				changed.Cap = money.FromMinor(101, money.AMD)
			} else {
				changed.Free = money.FromMinor(101, money.AMD)
			}
			if g.String() != changed.String() {
				t.Fatal("regression fixture must retain identical display representation")
			}
			if searchFingerprint(in, changed) == base {
				t.Fatal("goal subunit mutation did not change fingerprint")
			}
		})
	}
}

func TestSearchFingerprintAmountCurrency(t *testing.T) {
	in := identityInput(t)
	g := plan.Goal{}
	base := searchFingerprint(in, g)
	for _, field := range []string{"code", "exponent", "settlement", "name"} {
		t.Run(field, func(t *testing.T) {
			changed := identityInput(t)
			cur := changed.Cash.Spending.Monthly.Currency()
			switch field {
			case "code":
				cur.Code += "x"
			case "exponent":
				cur.Exponent++
			case "settlement":
				cur.SettlementUnit++
			case "name":
				cur.Name += "x"
			}
			changed.Cash.Spending.Monthly = money.FromMinor(100, cur)
			if field == "name" && changed.Cash.Spending.Monthly.String() != in.Cash.Spending.Monthly.String() {
				t.Fatal("currency name fixture must retain identical display representation")
			}
			if searchFingerprint(changed, g) == base {
				t.Fatal("embedded currency mutation did not change fingerprint")
			}
		})
	}
}

func TestSearchFingerprintCanonicalAllocation(t *testing.T) {
	first, second := identityInput(t), identityInput(t)
	if first.Cash.Spending == second.Cash.Spending {
		t.Fatal("fixture must allocate independent spending pointers")
	}
	second.Cash.MonthlyOverrides = make(map[string]money.Amount)
	second.Cash.Spending.Overrides = make(map[string]money.Amount)
	for _, k := range []string{"2026-03", "2026-02"} {
		second.Cash.MonthlyOverrides[k] = first.Cash.MonthlyOverrides[k]
		second.Cash.Spending.Overrides[k] = first.Cash.Spending.Overrides[k]
	}
	g := plan.Goal{Kind: plan.LeastInterest}
	base := searchFingerprint(first, g)
	for range 20 {
		if searchFingerprint(second, g) != base {
			t.Fatal("pointer allocation or map insertion changed fingerprint")
		}
	}
	first.Cash.Spending.Spent = money.FromMinor(101, money.AMD)
	if searchFingerprint(first, g) == base {
		t.Fatal("in-place pointed value mutation did not change fingerprint")
	}
}

func TestSearchFingerprintRejectsUnsupportedKinds(t *testing.T) {
	for _, v := range []any{make(chan int), func() {}, map[int]string{1: "x"}} {
		t.Run(reflect.TypeOf(v).String(), func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("unsupported kind did not panic")
				}
			}()
			var b strings.Builder
			fingerprintValue(&b, reflect.ValueOf(v))
		})
	}
}
