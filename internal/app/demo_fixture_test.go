package app

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/plan"
)

func TestThreeLoanDemoMatchesCurrentEngine(t *testing.T) {
	raw, err := os.ReadFile("../../docs/design/v3/three-loans-data.json")
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		Interest int64  `json:"interest"`
		Payoff   string `json:"payoff"`
		Engine   string `json:"engine"`
		Strength string `json:"strength"`
	}
	if err = json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	report, _ := fixedReport(t, plan.Goal{Kind: plan.LeastInterest})
	if saved.Interest != report.Best.TotalInterest.Minor() || saved.Payoff != report.Best.PayoffDate.String() {
		t.Fatalf("demo answer changed: interest=%d payoff=%s", report.Best.TotalInterest.Minor(), report.Best.PayoffDate.String())
	}
	if saved.Engine != plan.EngineVersion || saved.Strength != string(report.Certificate.Strength) {
		t.Fatalf("demo metadata: engine=%s strength=%s", plan.EngineVersion, report.Certificate.Strength)
	}
}
