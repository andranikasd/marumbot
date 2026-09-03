package app

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/plan"
)

func TestPlanManifestRoundTripAndReplay(t *testing.T) {
	in := cacheInput(t)
	goal := plan.Goal{Kind: plan.LeastInterest}
	report, err := plan.Search(in, goal)
	if err != nil {
		t.Fatal(err)
	}
	m, err := manifestFor(in, goal, report, 7)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var decoded PlanManifest
	if err = json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(m, decoded) {
		t.Fatal("source input changed on serialization")
	}
	r, err := ReplayManifest(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if r.TotalInterest.Cmp(report.Best.TotalInterest) != 0 {
		t.Fatal("historical answer changed")
	}
	decoded.Engine = "unavailable/old"
	if _, err = ReplayManifest(decoded); !errors.Is(err, ErrHistoricalEngine) {
		t.Fatal("old engine silently replaced")
	}
	decoded = m
	decoded.InputHash = "changed"
	if _, err = ReplayManifest(decoded); !errors.Is(err, ErrConflict) {
		t.Fatal("tampered inputs accepted")
	}
	decoded = m
	decoded.ResultHash = "changed"
	if _, err = ReplayManifest(decoded); !errors.Is(err, ErrConflict) {
		t.Fatal("changed result accepted")
	}
}
