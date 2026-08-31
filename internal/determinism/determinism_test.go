// Package determinism_test pins the engine's output across machines and
// architectures. The canonical hash of a fixed report is committed; CI runs
// this on amd64 and arm64, and a differing hash means a platform-dependent
// code path reached money.
package determinism_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/allocation"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

func fixedInput(t *testing.T) plan.Input {
	t.Helper()
	amd, err := money.Lookup("AMD")
	if err != nil {
		t.Fatal(err)
	}
	v := date.MustNew(2026, 1, 15)
	pos := func(id, name string, major, ratePct int64, years int) plan.Position {
		return plan.Position{
			ID: id, Name: name,
			Contract: model.Contract{
				LoanID: model.ID(id), Version: 1, Currency: amd, EffectiveFrom: v,
				NominalRate: money.RateFromPercent(ratePct, 0), DayCount: money.Actual365,
				Type: model.Annuity, StartDate: v, MaturityDate: date.MustNew(2026+years, 1, 15),
				PaymentDay: 15, Rounding: money.DefaultPolicy(amd),
			},
			Balance: money.FromMinor(major*100, amd), From: v,
			Excess: allocation.ExcessReducePrincipal, Trust: "bank_confirmed",
		}
	}
	return plan.Input{
		ValuationDate: v,
		Cash:          plan.CashPlan{Monthly: money.FromMinor(25_000_000, amd), PayDay: 1},
		Loans: []plan.Position{
			pos("a", "Car", 1_200_000, 21, 3),
			pos("b", "Home", 4_000_000, 11, 10),
			pos("c", "Phone", 300_000, 26, 2),
		},
	}
}

// canonical is the part of a report that must be identical everywhere: the
// winning policy, every figure a borrower sees, and the certificate claim.
func canonical(rep plan.Report) map[string]any {
	b := rep.Best
	var acts []map[string]any
	for _, a := range b.Actions {
		acts = append(acts, map[string]any{
			"on": a.On.String(), "loan": a.LoanID, "kind": a.Kind.String(),
			"amount": a.Amount.Minor(), "fee": a.Fee.Minor(), "saves": a.Saves.Minor(),
		})
	}
	return map[string]any{
		"engine":   plan.EngineVersion,
		"policy":   b.Policy.ID(),
		"payoff":   b.PayoffDate.String(),
		"months":   b.Months,
		"interest": b.TotalInterest.Minor(),
		"fees":     b.TotalFees.Minor(),
		"paid":     b.TotalPaid.Minor(),
		"strength": string(rep.Certificate.Strength),
		"minimum":  rep.Minimum.TotalInterest.Minor(),
		"actions":  acts,
	}
}

func TestReportHashMatchesTheCommittedGolden(t *testing.T) {
	rep, err := plan.Search(fixedInput(t), plan.Goal{Kind: plan.LeastInterest})
	if err != nil {
		t.Fatal(err)
	}
	blob, err := json.Marshal(canonical(rep))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(blob)
	got := hex.EncodeToString(sum[:])

	p := filepath.Join("..", "..", "testdata", "plan-report.sha256")
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("golden hash missing: %v\ncurrent hash: %s\ncanonical: %s", err, got, blob)
	}
	if strings.TrimSpace(string(want)) != got {
		t.Fatalf("report hash changed: %s, golden %s\nThe engine's answer moved. If deliberate, update testdata/plan-report.sha256 in the same reviewed change.\ncanonical: %s",
			got, strings.TrimSpace(string(want)), blob)
	}
	// Twice in one process: map order or hidden state would show here.
	rep2, err := plan.Search(fixedInput(t), plan.Goal{Kind: plan.LeastInterest})
	if err != nil {
		t.Fatal(err)
	}
	blob2, _ := json.Marshal(canonical(rep2))
	if string(blob) != string(blob2) {
		t.Fatal("two runs in one process disagree")
	}
}
