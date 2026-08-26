package ledger

import (
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/allocation"
	"github.com/andranikasd/marumbot/pkg/core/date"
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

func d(s string) date.Date {
	v, err := date.Parse(s)
	if err != nil {
		panic(err)
	}
	return v
}

var testPolicy = model.PolicyRef{Key: "test-consumer", Version: 1}

func contract() model.Contract {
	return model.Contract{
		LoanID: "loan-1", Version: 1, Currency: money.AMD,
		EffectiveFrom: d("2020-01-01"),
		NominalRate:   money.RateFromPercent(18, 0),
		DayCount:      money.Actual365, Type: model.Annuity,
		StartDate: d("2020-01-01"), MaturityDate: d("2030-01-01"),
		PaymentDay: 5, Rounding: money.DefaultAMDPolicy,
		AllocationPolicy: testPolicy,
	}
}

func anchor(trust model.Trust, asOf string, principal int64) model.Snapshot {
	pos := model.NewBuckets(money.AMD)
	pos.Principal = amd(principal)
	return model.Snapshot{
		ID: "snap-1", LoanID: "loan-1", AsOf: d(asOf), Trust: trust, Position: pos,
	}
}

func baseInput(events ...model.Event) Input {
	return Input{
		Contracts: []model.Contract{contract()},
		Anchor:    anchor(model.BankConfirmed, "2026-01-31", 1_000_000),
		Events:    events,
		Policies: map[model.PolicyRef]allocation.Policy{
			testPolicy: {Ref: testPolicy, Order: allocation.StandardOrder,
				Excess: allocation.ExcessReducePrincipal, Source: "fixture"},
		},
		AsOf:          d("2026-02-28"),
		EngineVersion: "test",
	}
}

func payment(id string, seq int64, valueDate string, major int64) model.Event {
	return model.Event{
		ID: model.ID(id), LoanID: "loan-1", RecordedSeq: seq,
		Kind: model.PaymentReported, ValueDate: d(valueDate), Amount: amd(major),
	}
}

// Interest accrues between the anchor and the as-of date even with no events.
func TestReplay_AccruesFromTheAnchor(t *testing.T) {
	res, err := Replay(baseInput())
	if err != nil {
		t.Fatal(err)
	}
	// 100,000,000 minor x 18% x 28 days / 365 = 1,380,821.9 -> 1,380,800
	if got := res.State.Position.AccruedInterest; got.Minor() != 1_380_800 {
		t.Errorf("accrued interest = %s (%d minor), want 1380800 minor", got, got.Minor())
	}
	if res.State.Position.Principal.Cmp(amd(1_000_000)) != 0 {
		t.Error("principal must not move without a payment")
	}
	if res.State.Reliability != model.Confirmed {
		t.Errorf("reliability = %s, want confirmed", res.State.Reliability)
	}
}

// The headline invariant: the same inputs always produce the same state and
// the same hash.
func TestReplay_IsDeterministic(t *testing.T) {
	in := baseInput(
		payment("e1", 1, "2026-02-05", 30_000),
		payment("e2", 2, "2026-02-20", 25_000),
	)
	first, err := Replay(in)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 25; i++ {
		got, err := Replay(in)
		if err != nil {
			t.Fatal(err)
		}
		if got.State.Position != first.State.Position {
			t.Fatal("position differed between runs")
		}
		if got.State.EventSetHash != first.State.EventSetHash {
			t.Fatal("event set hash differed between runs")
		}
	}
}

// Replay must not depend on the order events arrive in, only on their value
// dates and sequences.
func TestReplay_InputOrderDoesNotMatter(t *testing.T) {
	a := payment("e1", 1, "2026-02-05", 30_000)
	b := payment("e2", 2, "2026-02-20", 25_000)
	forward, err := Replay(baseInput(a, b))
	if err != nil {
		t.Fatal(err)
	}
	reversed, err := Replay(baseInput(b, a))
	if err != nil {
		t.Fatal(err)
	}
	if forward.State.Position != reversed.State.Position {
		t.Error("shuffling the input changed the derived position")
	}
	if forward.State.EventSetHash != reversed.State.EventSetHash {
		t.Error("shuffling the input changed the event set hash")
	}
}

// A void removes its target from the arithmetic but leaves both in history,
// and everything after it is recalculated.
func TestReplay_VoidExcludesTheTargetAndRecalculates(t *testing.T) {
	p1 := payment("e1", 1, "2026-02-05", 30_000)
	p2 := payment("e2", 2, "2026-02-10", 40_000)
	withBoth, err := Replay(baseInput(p1, p2))
	if err != nil {
		t.Fatal(err)
	}
	void := model.Event{
		ID: "e3", LoanID: "loan-1", RecordedSeq: 3, Kind: model.EntryVoided,
		ValueDate: d("2026-02-12"), VoidsEvent: "e1",
	}
	withVoid, err := Replay(baseInput(p1, p2, void))
	if err != nil {
		t.Fatal(err)
	}
	if withVoid.State.Position == withBoth.State.Position {
		t.Fatal("voiding a payment must change the position")
	}
	onlyP2, err := Replay(baseInput(p2))
	if err != nil {
		t.Fatal(err)
	}
	if withVoid.State.Position.Principal.Cmp(onlyP2.State.Position.Principal) != 0 {
		t.Errorf("after voiding e1 the principal should match a ledger that never had it: %s vs %s",
			withVoid.State.Position.Principal, onlyP2.State.Position.Principal)
	}
	// Both rows survive for audit.
	var seen int
	for _, s := range withVoid.Splits {
		if s.Skipped != "" && (s.Event.ID == "e1" || s.Event.ID == "e3") {
			seen++
		}
	}
	if seen != 2 {
		t.Errorf("the voided event and its marker must both be retained, saw %d", seen)
	}
}

// An event dated before the anchor is the double-counting trap: applying it
// might count a payment twice, ignoring it might lose one. Replay refuses to
// guess.
func TestReplay_PreAnchorEventForcesReconciliation(t *testing.T) {
	late := payment("e1", 1, "2026-01-15", 30_000) // before the 31 Jan anchor
	res, err := Replay(baseInput(late))
	if err != nil {
		t.Fatal(err)
	}
	if res.State.Reliability != model.NeedsReconciliation {
		t.Errorf("reliability = %s, want needs_reconciliation", res.State.Reliability)
	}
	if res.State.Tier() != model.TierBlocked {
		t.Error("an ambiguous ledger must not produce a plan")
	}
	if res.State.Position.Principal.Cmp(amd(1_000_000)) != 0 {
		t.Error("an ambiguous event must not be applied")
	}
}

// Confirming that the anchor already includes the event resolves the ambiguity
// without applying it twice.
func TestReplay_CoverageResolvesTheAmbiguity(t *testing.T) {
	late := payment("e1", 1, "2026-01-15", 30_000)
	in := baseInput(late)
	in.Covered = map[model.ID]bool{"e1": true}
	res, err := Replay(in)
	if err != nil {
		t.Fatal(err)
	}
	if res.State.Reliability != model.Confirmed {
		t.Errorf("reliability = %s, want confirmed", res.State.Reliability)
	}
	if res.State.Position.Principal.Cmp(amd(1_000_000)) != 0 {
		t.Error("a covered event must not be applied again")
	}
}

func TestReplay_GradesReliability(t *testing.T) {
	t.Run("unconfirmed anchor is estimated", func(t *testing.T) {
		in := baseInput()
		in.Anchor = anchor(model.UserEntered, "2026-01-31", 1_000_000)
		res, err := Replay(in)
		if err != nil {
			t.Fatal(err)
		}
		if res.State.Reliability != model.Estimated {
			t.Errorf("got %s, want estimated", res.State.Reliability)
		}
		if res.State.Tier() != model.TierIndicative {
			t.Error("an estimated loan may still show an indicative projection")
		}
	})
	t.Run("old confirmed anchor is stale", func(t *testing.T) {
		in := baseInput()
		in.AsOf = d("2026-04-30") // 89 days after the anchor
		res, err := Replay(in)
		if err != nil {
			t.Fatal(err)
		}
		if res.State.Reliability != model.Stale {
			t.Errorf("got %s, want stale", res.State.Reliability)
		}
	})
	t.Run("arrears are unsupported", func(t *testing.T) {
		in := baseInput()
		a := anchor(model.BankConfirmed, "2026-01-31", 1_000_000)
		a.Position.Penalties = amd(5_000)
		in.Anchor = a
		res, err := Replay(in)
		if err != nil {
			t.Fatal(err)
		}
		if res.State.Reliability != model.Unsupported {
			t.Errorf("got %s, want unsupported", res.State.Reliability)
		}
	})
	t.Run("unknown policy needs reconciliation", func(t *testing.T) {
		in := baseInput(payment("e1", 1, "2026-02-05", 30_000))
		in.Policies = map[model.PolicyRef]allocation.Policy{}
		res, err := Replay(in)
		if err != nil {
			t.Fatal(err)
		}
		if res.State.Reliability != model.NeedsReconciliation {
			t.Errorf("got %s, want needs_reconciliation", res.State.Reliability)
		}
		if res.State.Position.Principal.Cmp(amd(1_000_000)) != 0 {
			t.Error("an uninterpretable payment must not move the balance")
		}
	})
}

// The hash covers the facts that change the arithmetic, and nothing else.
func TestReplay_HashTracksTheEventSet(t *testing.T) {
	one, _ := Replay(baseInput(payment("e1", 1, "2026-02-05", 30_000)))
	two, _ := Replay(baseInput(payment("e1", 1, "2026-02-05", 30_000), payment("e2", 2, "2026-02-06", 1_000)))
	if one.State.EventSetHash == two.State.EventSetHash {
		t.Error("adding an event must change the hash")
	}
	changed, _ := Replay(baseInput(payment("e1", 1, "2026-02-05", 31_000)))
	if one.State.EventSetHash == changed.State.EventSetHash {
		t.Error("changing an amount must change the hash")
	}
}

// A closure reported by the lender outranks our arithmetic.
func TestReplay_LoanClosedZeroesThePosition(t *testing.T) {
	closed := model.Event{
		ID: "e9", LoanID: "loan-1", RecordedSeq: 9, Kind: model.LoanClosedReported,
		ValueDate: d("2026-02-15"),
	}
	res, err := Replay(baseInput(closed))
	if err != nil {
		t.Fatal(err)
	}
	if !res.State.Position.IsClosed() {
		total, _ := res.State.Position.TotalOwed()
		t.Errorf("loan should read as closed, %s outstanding", total)
	}
}

func TestReplay_RejectsStructuralProblems(t *testing.T) {
	t.Run("no contract covers the date", func(t *testing.T) {
		in := baseInput(payment("e1", 1, "2026-02-05", 1_000))
		c := contract()
		c.EffectiveFrom = d("2027-01-01")
		in.Contracts = []model.Contract{c}
		if _, err := Replay(in); err == nil {
			t.Error("replay must stop rather than pick a contract version")
		}
	})
	t.Run("as-of precedes the anchor", func(t *testing.T) {
		in := baseInput()
		in.AsOf = d("2026-01-01")
		if _, err := Replay(in); err == nil {
			t.Error("stating a balance before its anchor is nonsense and must be refused")
		}
	})
	t.Run("no contracts at all", func(t *testing.T) {
		in := baseInput()
		in.Contracts = nil
		if _, err := Replay(in); err == nil {
			t.Error("a loan with no contract cannot be replayed")
		}
	})
}

// Replaying twice from the same facts must be byte-identical, which is what
// the nightly reconciliation depends on.
func TestReplay_RebuildMatchesCache(t *testing.T) {
	in := baseInput(
		payment("e1", 1, "2026-02-05", 30_000),
		payment("e2", 2, "2026-02-12", 15_000),
		model.Event{ID: "e3", LoanID: "loan-1", RecordedSeq: 3, Kind: model.BankFeeReported,
			ValueDate: d("2026-02-15"), Amount: amd(1_500)},
	)
	cached, err := Replay(in)
	if err != nil {
		t.Fatal(err)
	}
	rebuilt, err := Replay(in)
	if err != nil {
		t.Fatal(err)
	}
	if cached.State.Position != rebuilt.State.Position {
		t.Error("a rebuild produced a different position")
	}
	if cached.State.EventSetHash != rebuilt.State.EventSetHash {
		t.Error("a rebuild produced a different event set hash")
	}
	if cached.State.Reliability != rebuilt.State.Reliability ||
		cached.State.BalanceAsOf != rebuilt.State.BalanceAsOf ||
		cached.State.LastRecordedSeq != rebuilt.State.LastRecordedSeq {
		t.Error("a rebuild disagreed with the cache")
	}
}
