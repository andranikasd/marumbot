package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

func planActualFixture(t *testing.T) (PlanTimeline, PlanActualFact) {
	t.Helper()
	data, err := os.ReadFile("../../testdata/golden/inecobank-consumer-M26-029210.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Rows []struct {
			Due       string `json:"due"`
			Principal int64  `json:"principal_minor"`
			Interest  int64  `json:"interest_minor"`
		}
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	row := fixture.Rows[0]
	zero := int64(0)
	fact := PlanActualFact{ID: "fact", LoanID: "loan", TransactionDate: row.Due, ValueDate: row.Due, AmountMinor: row.Principal + row.Interest, Allocation: &PaymentAllocation{&row.Principal, &row.Interest, &zero}, RecordedAfterActivation: true}
	return PlanTimeline{Currency: "AMD", Exponent: 2, Payments: []PlanPayment{{On: row.Due, LoanID: "loan", Loan: "Fixture", Kind: "instalment", AmountMinor: row.Principal + row.Interest}}}, fact
}

func compareActualFixture(t *testing.T, timeline PlanTimeline, facts []PlanActualFact) PlanActualComparison {
	t.Helper()
	out, err := comparePlanActuals(ActualBaseline{PlanID: "plan"}, timeline, map[string]string{"loan": "Fixture"}, facts, "2026-09", date.MustNew(2026, 9, 3), date.MustNew(2026, 9, 30))
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestPlanActualsGoldenValuesAreNotVerifiedStatus(t *testing.T) {
	timeline, fact := planActualFixture(t)
	out := compareActualFixture(t, timeline, []PlanActualFact{fact})
	if len(out.Rows) != 1 {
		t.Fatal(out)
	}
	r := out.Rows[0]
	if *r.PostedMinor != "12507940" || *r.KnownPrincipalMinor != "5603410" || *r.KnownInterestMinor != "6904530" || *r.KnownFeeMinor != "0" || *r.AmountDeltaMinor != "0" || len(r.Causes) != 0 {
		t.Fatalf("golden: %+v", r)
	}
	var encoded map[string]any
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &encoded); err != nil {
		t.Fatal(err)
	}
	if _, exists := encoded["status"]; exists {
		t.Fatal("comparison manufactured a verification status")
	}
}

func TestPlanActualsEvidenceCausesAndMissingCoverage(t *testing.T) {
	for _, name := range []string{"amount", "date", "fee", "allocation", "missing", "schedule"} {
		t.Run(name, func(t *testing.T) {
			timeline, fact := planActualFixture(t)
			facts := []PlanActualFact{fact}
			switch name {
			case "amount":
				facts[0].AmountMinor++
				fees := int64(1)
				a := *fact.Allocation
				a.FeesMinor = &fees
				facts[0].Allocation = &a
			case "date":
				facts[0].ValueDate = "2026-09-25"
			case "fee":
				p := *fact.Allocation.PrincipalMinor - 1
				fee := int64(1)
				a := *fact.Allocation
				a.PrincipalMinor = &p
				a.FeesMinor = &fee
				facts[0].Allocation = &a
			case "allocation":
				facts[0].Allocation = nil
			case "missing":
				facts = nil
			case "schedule":
				timeline.Payments = nil
			}
			out := compareActualFixture(t, timeline, facts)
			r := out.Rows[0]
			found := false
			for _, cause := range r.Causes {
				if string(cause) == name {
					found = true
				}
			}
			if !found {
				t.Fatalf("missing cause %s: %+v", name, r)
			}
			if name == "missing" && (r.PostedMinor != nil || r.AmountDeltaMinor != nil || r.KnownInterestMinor != nil || r.FeeDeltaMinor != nil) {
				t.Fatal("absent fact became zero")
			}
			if name == "allocation" && (r.KnownInterestMinor != nil || r.KnownFeeMinor != nil || r.FeeDeltaMinor != nil || r.UnknownAllocationMinor != "12507940") {
				t.Fatal("missing split became zero")
			}
		})
	}
}

func TestPlanActualsActivationPendingAndFutureBoundaries(t *testing.T) {
	timeline, fact := planActualFixture(t)
	pre := fact
	pre.TransactionDate = "2026-09-02" // A later posting is not a new-plan transfer.
	sameDay := fact
	sameDay.TransactionDate = "2026-09-03"
	earlierRecorded := fact
	earlierRecorded.RecordedAfterActivation = false
	pending := fact
	pending.ValueDate = ""
	pending.Allocation = nil
	outside := fact
	outside.LoanID = "new-loan"
	out := compareActualFixture(t, timeline, []PlanActualFact{pre, sameDay, earlierRecorded, pending, outside})
	if out.ExcludedBeforeActivationCount != 3 || out.PendingCount != 1 || out.OutsideBaselineCount != 1 || out.Rows[0].PostedMinor != nil {
		t.Fatalf("boundary: %+v", out)
	}
	out, err := comparePlanActuals(ActualBaseline{}, timeline, map[string]string{"loan": "Fixture"}, []PlanActualFact{fact}, "2026-09", date.MustNew(2026, 9, 3), date.MustNew(2026, 9, 23))
	if err != nil || len(out.Rows) != 0 {
		t.Fatalf("future action declared missing: %+v %v", out, err)
	}
	out, err = comparePlanActuals(ActualBaseline{}, timeline, map[string]string{"loan": "Fixture"}, nil, "2026-08", date.MustNew(2026, 9, 3), date.MustNew(2026, 9, 30))
	if err != nil || !out.EmptyWindow {
		t.Fatal("preactivation month compared")
	}
}

func TestPlanActualsAggregationDoesNotPairFacts(t *testing.T) {
	timeline, fact := planActualFixture(t)
	first := fact
	first.AmountMinor = 5603410
	first.Allocation = nil
	second := fact
	second.AmountMinor = 6904530
	second.Allocation = nil
	out := compareActualFixture(t, timeline, []PlanActualFact{first, second})
	r := out.Rows[0]
	if r.PostedCount != 2 || r.PlannedCount != 1 || *r.AmountDeltaMinor != "0" || !reflect.DeepEqual(r.Causes, []VarianceCause{VarianceAllocation}) {
		t.Fatalf("invented pairing: %+v", r)
	}
	// Aggregation stays exact beyond the browser integer boundary.
	timeline.Payments[0].AmountMinor = 9007199254740991
	first.AmountMinor = 9007199254740991
	second.AmountMinor = 1
	r = compareActualFixture(t, timeline, []PlanActualFact{first, second}).Rows[0]
	if *r.PostedMinor != "9007199254740992" || *r.AmountDeltaMinor != "1" {
		t.Fatal("aggregate lost precision")
	}
}

type planActualHistoryFake struct {
	PlanHistoryStore
	baseline ActualBaseline
	manifest PlanManifest
	owner    string
	facts    []PlanActualFact
}

func (h *planActualHistoryFake) ActiveActualBaselines(_ context.Context, user string) ([]ActualBaseline, error) {
	if user != h.owner {
		return []ActualBaseline{}, nil
	}
	return []ActualBaseline{h.baseline}, nil
}

func (h *planActualHistoryFake) PlanVersion(_ context.Context, user, id string) (PlanVersion, error) {
	if user != h.owner || id != h.baseline.PlanID {
		return PlanVersion{}, ErrNotFound
	}
	return PlanVersion{ID: id, Manifest: h.manifest}, nil
}

func (h *planActualHistoryFake) PlanActualFacts(_ context.Context, user string, _ ActualBaseline, _ string) ([]PlanActualFact, error) {
	if user != h.owner {
		return nil, ErrNotFound
	}
	return h.facts, nil
}

type planActualClock struct{}

func (planActualClock) Now() time.Time { return time.Date(2026, 2, 28, 12, 0, 0, 0, time.UTC) }

func TestPlanActualsWorkerReplaysPinnedHistoricalManifest(t *testing.T) {
	in := cacheInput(t)
	g := plan.Goal{Kind: plan.LeastInterest}
	report, err := plan.Search(in, g)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := manifestFor(in, g, report, 1)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Sources = `{"timezone":"Asia/Yerevan"}`
	h := &planActualHistoryFake{owner: "owner", baseline: ActualBaseline{PlanID: "historical", Currency: "AMD", ActivatedAt: time.Date(2026, 1, 15, 21, 0, 0, 0, time.UTC)}, manifest: manifest}
	w := Worker{History: h, Clock: planActualClock{}}
	out, err := w.ActivePlanActuals(t.Context(), "owner", "2026-02")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].PlanID != "historical" || out[0].InputHash != manifest.InputHash || out[0].ActivatedOn != "2026-01-16" || len(out[0].Rows) == 0 {
		t.Fatalf("baseline: %+v", out)
	}
	before, _ := json.Marshal(h.manifest)
	if _, err := w.ActivePlanActuals(t.Context(), "foreign", "2026-02"); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(h.manifest)
	if string(before) != string(after) {
		t.Fatal("active plan changed")
	}
	h.manifest.Engine = "unavailable"
	if _, err := w.ActivePlanActuals(t.Context(), "owner", "2026-02"); !errors.Is(err, ErrHistoricalEngine) {
		t.Fatalf("old engine replaced: %v", err)
	}
	h.manifest = manifest
	h.manifest.InputHash = "tampered"
	if _, err := w.ActivePlanActuals(t.Context(), "owner", "2026-02"); !errors.Is(err, ErrConflict) {
		t.Fatalf("history not validated: %v", err)
	}
	h.manifest = manifest
	h.manifest.Sources = ""
	if _, err := w.ActivePlanActuals(t.Context(), "owner", "2026-02"); !errors.Is(err, ErrPlanActualsCoverage) {
		t.Fatal("activation timezone guessed")
	}
}
