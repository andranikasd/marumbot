package plan

import (
	"errors"
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/money"
)

func tinyDynamicInput(t *testing.T) Input {
	t.Helper()
	in := comparisonInput(t)
	cur := in.Cash.Monthly.Currency()
	for i := range in.Loans {
		l := &in.Loans[i]
		l.Contract.Rounding = money.Policy{Mode: money.HalfUp, Unit: 1}
		l.Balance = money.FromMinor(int64(20+i*10), cur)
		l.Contract.ScheduledPayment = money.FromMinor(5, cur)
		l.Contract.NominalRate = money.RateFromPercent(int64(120-i*60), 0)
	}
	in.Cash.Monthly = money.FromMinor(20, cur)
	return in
}

func TestDynamicKnownSingleLoanCost(t *testing.T) {
	in := tinyDynamicInput(t)
	in.Loans = in.Loans[:1]
	in.Cash.Monthly = money.FromMinor(10, in.Cash.Monthly.Currency())
	rep, err := SearchDynamic(DynamicRequest{Input: in, Horizon: 4, MaxStates: 100000})
	if err != nil {
		t.Fatal(err)
	}
	// Independent hand calculation: Jan15-Feb15 20*1.2*31/365 = 2;
	// balance 12, Feb15-Mar15 interest 1; balance 3, final interest 0.
	if !rep.Complete || rep.Cost == nil || rep.Cost.Minor() != 3 || len(rep.Steps) != 3 {
		t.Fatalf("cost=%v complete=%v steps=%d", rep.Cost, rep.Complete, len(rep.Steps))
	}
	if rep.Certificate.LowerBound == nil || rep.Certificate.Gap == nil || rep.Certificate.Gap.Sign() != 0 || rep.Certificate.Strength != ExhaustiveDynamicDomain {
		t.Fatal("missing finite-domain proof")
	}
}

func TestDynamicCapIsDeterministicAndUnknownBounds(t *testing.T) {
	req := DynamicRequest{Input: tinyDynamicInput(t), Horizon: 6, MaxStates: 50}
	a, err := SearchDynamic(req)
	if err != nil {
		t.Fatal(err)
	}
	b, err := SearchDynamic(req)
	if err != nil {
		t.Fatal(err)
	}
	if a.Complete || a.Certificate.DynamicExpansions > 50 || a.Certificate.LowerBound != nil || a.Certificate.Gap != nil || a.Certificate.Truncation == "" {
		t.Fatalf("dishonest cap: %+v", a.Certificate)
	}
	if a.Certificate.DynamicExpansions != b.Certificate.DynamicExpansions || a.SharedInputHash != b.SharedInputHash {
		t.Fatal("nondeterministic cap")
	}
	if (a.Cost == nil) != (b.Cost == nil) || (a.Cost != nil && *a.Cost != *b.Cost) {
		t.Fatal("nondeterministic incumbent")
	}
}

func TestDynamicRefusesUnsupportedFullState(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Input)
	}{
		{"spending", func(in *Input) { in.Cash.Spending = &SpendingPlan{Monthly: in.Cash.Monthly} }},
		{"fees", func(in *Input) { in.Loans[0].Contract.Prepayment.FeeBP = 10 }},
		{"payday", func(in *Input) { in.Cash.PayDay = 15 }},
		{"exclusion", func(in *Input) { in.Loans[0].OptionalExcluded = true }},
		{"different_due", func(in *Input) { in.Loans[0].Contract.PaymentDay = 20 }},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			in := tinyDynamicInput(t)
			tt.edit(&in)
			_, err := SearchDynamic(DynamicRequest{Input: in})
			var u *UnsupportedError
			if !errors.As(err, &u) {
				t.Fatalf("want typed refusal, got %v", err)
			}
		})
	}
}
