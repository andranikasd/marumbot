package model

import (
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

func amd(major int64) money.Amount {
	a, err := money.FromMajor(major, money.AMD)
	if err != nil {
		panic(err)
	}
	return a
}

func d(s string) date.Date {
	v, err := date.Parse(s)
	if err != nil {
		panic(err)
	}
	return v
}

func TestBuckets_TotalOwedNetsAdvanceCredit(t *testing.T) {
	b := NewBuckets(money.AMD)
	b.Principal = amd(1_000_000)
	b.AccruedInterest = amd(15_000)
	b.CurrentFees = amd(2_000)
	b.AdvanceCredit = amd(50_000)

	got, err := b.TotalOwed()
	if err != nil {
		t.Fatal(err)
	}
	// A credit the lender is holding reduces what is owed; it is not a debt.
	if want := amd(967_000); got.Cmp(want) != 0 {
		t.Errorf("TotalOwed = %s, want %s", got, want)
	}
}

func TestBuckets_HasArrears(t *testing.T) {
	clean := NewBuckets(money.AMD)
	clean.Principal = amd(1_000_000)
	if clean.HasArrears() {
		t.Error("a current loan has no arrears")
	}
	for name, mutate := range map[string]func(*Buckets){
		"penalties":         func(b *Buckets) { b.Penalties = amd(1) },
		"overdue principal": func(b *Buckets) { b.OverduePrincipal = amd(1) },
		"overdue fees":      func(b *Buckets) { b.OverdueFees = amd(1) },
		"unpaid interest":   func(b *Buckets) { b.UnpaidInterest = amd(1) },
	} {
		b := clean
		mutate(&b)
		if !b.HasArrears() {
			t.Errorf("%s should count as arrears", name)
		}
	}
}

func TestBuckets_ValidateRejectsNegativeAndMixedCurrency(t *testing.T) {
	b := NewBuckets(money.AMD)
	b.Principal = money.FromMinor(-1, money.AMD)
	if err := b.Validate(); err == nil {
		t.Error("a negative bucket must be rejected")
	}
	b = NewBuckets(money.AMD)
	b.CurrentFees = money.FromMinor(100, money.MustLookup("USD"))
	if err := b.Validate(); err == nil {
		t.Error("a mixed-currency position must be rejected")
	}
}

func TestBuckets_GetWithRoundTrip(t *testing.T) {
	b := NewBuckets(money.AMD)
	for _, k := range []Bucket{
		Penalties, OverdueFees, CurrentFees, UnpaidInterest,
		AccruedInterest, OverduePrincipal, Principal, AdvanceCredit,
	} {
		b = b.With(k, amd(int64(k)+1))
	}
	for _, k := range []Bucket{
		Penalties, OverdueFees, CurrentFees, UnpaidInterest,
		AccruedInterest, OverduePrincipal, Principal, AdvanceCredit,
	} {
		if got, want := b.Get(k), amd(int64(k)+1); got.Cmp(want) != 0 {
			t.Errorf("bucket %s: got %s, want %s", k, got, want)
		}
	}
}

// Replay order is value date, then the lender's intra-day sequence, then
// recorded_seq. Recorded order is deliberately NOT the financial order: a
// payment entered late still accrues from the day it was made.
func TestSortForReplay_ValueDateBeatsRecordedOrder(t *testing.T) {
	events := []Event{
		{ID: "late-entry", RecordedSeq: 9, ValueDate: d("2026-03-01")},
		{ID: "early-entry", RecordedSeq: 1, ValueDate: d("2026-03-05")},
	}
	SortForReplay(events)
	if events[0].ID != "late-entry" {
		t.Errorf("value date must drive replay order, got %s first", events[0].ID)
	}
}

func TestSortForReplay_BankOrderBreaksSameDayTies(t *testing.T) {
	events := []Event{
		{ID: "b", RecordedSeq: 1, ValueDate: d("2026-03-01"), BankOrder: 2, HasBankOrder: true},
		{ID: "a", RecordedSeq: 2, ValueDate: d("2026-03-01"), BankOrder: 1, HasBankOrder: true},
		{ID: "c", RecordedSeq: 3, ValueDate: d("2026-03-01")},
	}
	SortForReplay(events)
	got := []string{string(events[0].ID), string(events[1].ID), string(events[2].ID)}
	want := []string{"a", "b", "c"} // ordered entries first, unordered last
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

// The sort must be total: the same input in any starting arrangement must
// produce the same output, or replay is not reproducible.
func TestSortForReplay_IsDeterministic(t *testing.T) {
	base := []Event{
		{ID: "a", RecordedSeq: 3, ValueDate: d("2026-01-10")},
		{ID: "b", RecordedSeq: 1, ValueDate: d("2026-01-10")},
		{ID: "c", RecordedSeq: 2, ValueDate: d("2026-01-05")},
		{ID: "e", RecordedSeq: 5, ValueDate: d("2026-01-10"), BankOrder: 7, HasBankOrder: true},
	}
	first := append([]Event(nil), base...)
	SortForReplay(first)
	for i := range base { // rotate the input and re-sort
		rotated := append(append([]Event(nil), base[i:]...), base[:i]...)
		SortForReplay(rotated)
		for j := range rotated {
			if rotated[j].ID != first[j].ID {
				t.Fatalf("rotation %d produced a different order at %d", i, j)
			}
		}
	}
}

func TestEvent_Validate(t *testing.T) {
	ok := Event{LoanID: "l1", RecordedSeq: 1, Kind: PaymentReported, ValueDate: d("2026-01-01"), Amount: amd(1000)}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid event rejected: %v", err)
	}
	bad := map[string]Event{
		"no loan":            {RecordedSeq: 1, Kind: PaymentReported, ValueDate: d("2026-01-01"), Amount: amd(1)},
		"zero seq":           {LoanID: "l1", Kind: PaymentReported, ValueDate: d("2026-01-01"), Amount: amd(1)},
		"no value date":      {LoanID: "l1", RecordedSeq: 1, Kind: PaymentReported, Amount: amd(1)},
		"zero amount":        {LoanID: "l1", RecordedSeq: 1, Kind: PaymentReported, ValueDate: d("2026-01-01")},
		"void without ref":   {LoanID: "l1", RecordedSeq: 1, Kind: EntryVoided, ValueDate: d("2026-01-01")},
		"payment with a ref": {LoanID: "l1", RecordedSeq: 1, Kind: PaymentReported, ValueDate: d("2026-01-01"), Amount: amd(1), VoidsEvent: "x"},
	}
	for name, e := range bad {
		if err := e.Validate(); err == nil {
			t.Errorf("%s: should be rejected", name)
		}
	}
}

func TestContract_CoversDate(t *testing.T) {
	c := Contract{EffectiveFrom: d("2026-01-01"), EffectiveThru: d("2026-06-30")}
	cases := map[string]bool{
		"2025-12-31": false, "2026-01-01": true, "2026-03-15": true,
		"2026-06-30": true, "2026-07-01": false,
	}
	for s, want := range cases {
		if got := c.CoversDate(d(s)); got != want {
			t.Errorf("CoversDate(%s) = %v, want %v", s, got, want)
		}
	}
	open := Contract{EffectiveFrom: d("2026-01-01")}
	if !open.CoversDate(d("2099-01-01")) {
		t.Error("an open-ended contract version covers any later date")
	}
}

func TestContract_Validate(t *testing.T) {
	base := Contract{
		LoanID: "l1", Version: 1, Currency: money.AMD,
		EffectiveFrom: d("2026-01-01"), NominalRate: money.RateFromPercent(18, 0),
		DayCount: money.Actual365, Type: Annuity,
		StartDate: d("2026-01-01"), MaturityDate: d("2029-01-01"),
		PaymentDay: 5, Rounding: money.Policy{Mode: money.HalfUp, Unit: 100},
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid contract rejected: %v", err)
	}
	mutations := map[string]func(*Contract){
		"maturity before start": func(c *Contract) { c.MaturityDate = d("2025-01-01") },
		"payment day 0":         func(c *Contract) { c.PaymentDay = 0 },
		"payment day 32":        func(c *Contract) { c.PaymentDay = 32 },
		"negative rate":         func(c *Contract) { c.NominalRate = -1 },
		"declining principal":   func(c *Contract) { c.Type = DecliningPrincipal },
		"zero rounding unit":    func(c *Contract) { c.Rounding.Unit = 0 },
		"scheduled but zero":    func(c *Contract) { c.HasScheduled = true },
	}
	for name, mutate := range mutations {
		c := base
		mutate(&c)
		if err := c.Validate(); err == nil {
			t.Errorf("%s: should be rejected", name)
		}
	}
}

// An absent scheduled payment is not an instalment of zero: it means the
// engine must solve for it, and the two must not be confusable.
func TestContract_AbsentScheduledPaymentIsNotZero(t *testing.T) {
	c := Contract{HasScheduled: false}
	if c.HasScheduled {
		t.Fatal("unset contracts must report no scheduled payment")
	}
	c.ScheduledPayment = amd(74_500)
	c.HasScheduled = true
	if !c.HasScheduled || c.ScheduledPayment.IsZero() {
		t.Error("a supplied instalment must survive round-trip")
	}
}

func TestReliability_Tiers(t *testing.T) {
	cases := map[Reliability]PlanTier{
		Confirmed:           TierConfident,
		Estimated:           TierIndicative,
		Stale:               TierIndicative,
		NeedsReconciliation: TierBlocked,
		Unsupported:         TierBlocked,
	}
	for r, want := range cases {
		if got := r.Tier(); got != want {
			t.Errorf("%s: tier = %s, want %s", r, got, want)
		}
	}
}

func TestPolicyRef_ZeroMeansUnknown(t *testing.T) {
	var p PolicyRef
	if !p.IsZero() || p.String() != "unknown/v0" {
		t.Errorf("the zero policy must read as unknown, got %q", p.String())
	}
	p = PolicyRef{Key: "ineco-consumer", Version: 2}
	if p.IsZero() || p.String() != "ineco-consumer/v2" {
		t.Errorf("got %q", p.String())
	}
}

func TestSnapshot_Validate(t *testing.T) {
	pos := NewBuckets(money.AMD)
	pos.Principal = amd(1_000_000)
	s := Snapshot{LoanID: "l1", AsOf: d("2026-01-31"), Trust: BankConfirmed, Position: pos}
	if err := s.Validate(); err != nil {
		t.Fatalf("valid snapshot rejected: %v", err)
	}
	s.AsOf = date.Date{}
	if err := s.Validate(); err == nil {
		t.Error("a snapshot without an as-of date must be rejected")
	}
}

func TestParseRoundTrips(t *testing.T) {
	for k := range eventKindNames {
		got, err := ParseEventKind(k.String())
		if err != nil || got != k {
			t.Errorf("event kind %s did not round-trip: %v %v", k, got, err)
		}
	}
	for b := range bucketNames {
		got, err := ParseBucket(b.String())
		if err != nil || got != b {
			t.Errorf("bucket %s did not round-trip: %v %v", b, got, err)
		}
	}
	if _, err := ParseEventKind("nonsense"); err == nil {
		t.Error("unknown event kind must be rejected")
	}
}
