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

// storedInputHash pins the fingerprint of a fixed input.
//
// The round trip above renames with the code, so it stays green through a
// change that would strand every manifest already written. This does not:
// searchFingerprint hashes plan.Input's field and type names, so any rename
// inside that tree moves this hash, and a moved hash means ReplayManifest
// refuses stored plans with ErrConflict.
const storedInputHash = "9c17cb12b2198d19c5c7d9d88d3513b593574c3b146fbb77aa94149399d556b5"

func TestStoredManifestsSurviveTheCurrentShapeOfInput(t *testing.T) {
	got := searchFingerprint(cacheInput(t), plan.Goal{})
	if got != storedInputHash {
		t.Fatalf(`the shape of plan.Input changed: input hash is now
  %s
committed:
  %s

Every PlanManifest already stored carries the committed hash, so ReplayManifest
will refuse those plans with ErrConflict -- which reads like corruption, not
like a rename. See the note at the top of pkg/core/plan/input.go.

If the change is deliberate, decide what happens to the stored rows first, then
update this constant in the same reviewed change.`, got, storedInputHash)
	}
}
