package allocation

import (
	"errors"
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

func amd(major int64) money.Amount {
	a, err := money.FromMajor(major, money.AMD)
	if err != nil {
		panic(err)
	}
	return a
}

func position(principal, accrued, fees, penalties int64) model.Buckets {
	b := model.NewBuckets(money.AMD)
	b.Principal = amd(principal)
	b.AccruedInterest = amd(accrued)
	b.CurrentFees = amd(fees)
	b.Penalties = amd(penalties)
	return b
}

func known(excess ExcessRule) Policy {
	return Policy{
		Ref:    model.PolicyRef{Key: "test-consumer", Version: 1},
		Order:  StandardOrder,
		Excess: excess,
		Source: "synthetic fixture",
	}
}

// The invariant that matters most: a split accounts for every dram of the
// payment. Anything uninterpretable is recorded, never dropped.
func TestApply_MoneyIsConserved(t *testing.T) {
	cases := []struct {
		name    string
		pos     model.Buckets
		payment int64
		policy  Policy
	}{
		{"partial", position(1_000_000, 15_000, 2_000, 0), 10_000, known(ExcessReducePrincipal)},
		{"exact interest", position(1_000_000, 15_000, 2_000, 0), 17_000, known(ExcessReducePrincipal)},
		{"into principal", position(1_000_000, 15_000, 2_000, 0), 100_000, known(ExcessReducePrincipal)},
		{"overpayment", position(50_000, 1_000, 0, 0), 100_000, known(ExcessReducePrincipal)},
		{"held as advance", position(50_000, 1_000, 0, 0), 100_000, known(ExcessHoldAsAdvance)},
		{"needs request", position(50_000, 1_000, 0, 0), 100_000, known(ExcessRequiresBankRequest)},
		{"with penalties", position(1_000_000, 15_000, 2_000, 7_500), 20_000, known(ExcessReducePrincipal)},
		{"unknown policy", position(1_000_000, 15_000, 0, 0), 74_500, Unknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, split, err := Apply(tc.pos, amd(tc.payment), tc.policy)
			if err != nil && !errors.Is(err, ErrUnknownPolicy) {
				t.Fatalf("unexpected error: %v", err)
			}
			total, err := split.Total(money.AMD)
			if err != nil {
				t.Fatal(err)
			}
			if want := amd(tc.payment); total.Cmp(want) != 0 {
				t.Errorf("split totals %s, payment was %s", total, want)
			}
		})
	}
}

// Nothing may appear either: what leaves the buckets must equal what the split
// says was applied to them.
func TestApply_BucketsFallByExactlyWhatWasApplied(t *testing.T) {
	before := position(1_000_000, 15_000, 2_000, 7_500)
	after, split, err := Apply(before, amd(30_000), known(ExcessReducePrincipal))
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range split.Buckets() {
		fell, err := before.Get(b).Sub(after.Get(b))
		if err != nil {
			t.Fatal(err)
		}
		if applied := split.Applied[b]; fell.Cmp(applied) != 0 {
			t.Errorf("bucket %s fell by %s but the split says %s", b, fell, applied)
		}
	}
}

// The lender's order is the whole point of the package: penalties and fees are
// settled before principal, so a payment that looks like it reduces the debt
// may barely touch it.
func TestApply_SettlesInPolicyOrder(t *testing.T) {
	pos := position(1_000_000, 15_000, 2_000, 7_500)
	after, split, err := Apply(pos, amd(20_000), known(ExcessReducePrincipal))
	if err != nil {
		t.Fatal(err)
	}
	if got := split.Applied[model.Penalties]; got.Cmp(amd(7_500)) != 0 {
		t.Errorf("penalties settled %s, want 7500", got)
	}
	if got := split.Applied[model.CurrentFees]; got.Cmp(amd(2_000)) != 0 {
		t.Errorf("fees settled %s, want 2000", got)
	}
	if got := split.Applied[model.AccruedInterest]; got.Cmp(amd(10_500)) != 0 {
		t.Errorf("interest settled %s, want 10500", got)
	}
	if _, touched := split.Applied[model.Principal]; touched {
		t.Error("a 20,000 payment against 24,500 of charges must not reach principal")
	}
	if after.Principal.Cmp(amd(1_000_000)) != 0 {
		t.Errorf("principal moved to %s; it should be untouched", after.Principal)
	}
}

// An unknown policy must change nothing and say so. This is the safe
// degradation the design requires: record the fact, refuse the interpretation.
func TestApply_UnknownPolicyChangesNothing(t *testing.T) {
	before := position(1_000_000, 15_000, 2_000, 0)
	after, split, err := Apply(before, amd(74_500), Unknown)
	if !errors.Is(err, ErrUnknownPolicy) {
		t.Fatalf("want ErrUnknownPolicy, got %v", err)
	}
	if after != before {
		t.Error("an uninterpretable payment must not move any bucket")
	}
	if split.Confident {
		t.Error("an unknown policy cannot produce a confident split")
	}
	if split.Unapplied.Cmp(amd(74_500)) != 0 {
		t.Errorf("the whole payment should be unapplied, got %s", split.Unapplied)
	}
}

// Excess handling is where lenders differ most, and where claiming the wrong
// behaviour invents an interest saving that did not happen.
func TestApply_ExcessRules(t *testing.T) {
	pos := position(50_000, 1_000, 0, 0) // 51,000 owed
	t.Run("hold as advance does not reduce principal", func(t *testing.T) {
		after, split, err := Apply(pos, amd(100_000), known(ExcessHoldAsAdvance))
		if err != nil {
			t.Fatal(err)
		}
		if split.ExtraToAdvance.Cmp(amd(49_000)) != 0 {
			t.Errorf("advance credit = %s, want 49000", split.ExtraToAdvance)
		}
		if after.AdvanceCredit.Cmp(amd(49_000)) != 0 {
			t.Errorf("position advance credit = %s", after.AdvanceCredit)
		}
	})
	t.Run("requires request changes nothing and is not confident", func(t *testing.T) {
		after, split, err := Apply(pos, amd(100_000), known(ExcessRequiresBankRequest))
		if err != nil {
			t.Fatal(err)
		}
		if split.Pending.Cmp(amd(49_000)) != 0 {
			t.Errorf("pending = %s, want 49000", split.Pending)
		}
		if !after.AdvanceCredit.IsZero() {
			t.Error("a prepayment awaiting a bank request must not create credit")
		}
		if split.Confident {
			t.Error("a pending prepayment is not a confident interpretation")
		}
	})
}

// Paying exactly what is owed must close the loan and leave nothing over.
func TestApply_ExactPayoffClosesTheLoan(t *testing.T) {
	pos := position(50_000, 1_000, 500, 0)
	after, split, err := Apply(pos, amd(51_500), known(ExcessReducePrincipal))
	if err != nil {
		t.Fatal(err)
	}
	if !after.IsClosed() {
		total, _ := after.TotalOwed()
		t.Errorf("loan should be closed, %s still owed", total)
	}
	if !split.ExtraToAdvance.IsZero() || !split.Pending.IsZero() || !split.Unapplied.IsZero() {
		t.Error("an exact payoff leaves no surplus")
	}
}

func TestApply_RejectsBadInput(t *testing.T) {
	pos := position(1_000, 0, 0, 0)
	if _, _, err := Apply(pos, amd(0), known(ExcessReducePrincipal)); err == nil {
		t.Error("a zero payment must be rejected")
	}
	if _, _, err := Apply(pos, money.FromMinor(-1, money.AMD), known(ExcessReducePrincipal)); err == nil {
		t.Error("a negative payment must be rejected")
	}
	usd, _ := money.FromMajor(100, money.MustLookup("USD"))
	if _, _, err := Apply(pos, usd, known(ExcessReducePrincipal)); err == nil {
		t.Error("a payment in the wrong currency must be rejected")
	}
}

func TestApply_IsDeterministic(t *testing.T) {
	pos := position(1_000_000, 15_000, 2_000, 7_500)
	first, firstSplit, err := Apply(pos, amd(60_000), known(ExcessReducePrincipal))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		got, gotSplit, err := Apply(pos, amd(60_000), known(ExcessReducePrincipal))
		if err != nil {
			t.Fatal(err)
		}
		if got != first {
			t.Fatal("the same inputs produced a different position")
		}
		for b, v := range firstSplit.Applied {
			if gotSplit.Applied[b].Cmp(v) != 0 {
				t.Fatalf("bucket %s differed between runs", b)
			}
		}
	}
}

func TestExcessRule_RoundTrip(t *testing.T) {
	for r := range excessNames {
		got, err := ParseExcessRule(r.String())
		if err != nil || got != r {
			t.Errorf("%s did not round-trip: %v %v", r, got, err)
		}
	}
	if _, err := ParseExcessRule("nonsense"); err == nil {
		t.Error("an unknown rule must be rejected")
	}
}
