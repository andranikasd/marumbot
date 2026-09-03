package plan_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/allocation"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

var amd = money.MustLookup("AMD")

func amt(major int64) money.Amount { return money.FromMinor(major*100, amd) }

var valuation = date.MustNew(2026, 1, 15)

func pos(id, name string, major, ratePct int64, years int) plan.Position {
	return plan.Position{
		ID: id, Name: name,
		Contract: model.Contract{
			LoanID: model.ID(id), Version: 1, Currency: amd, EffectiveFrom: valuation,
			NominalRate: money.RateFromPercent(ratePct, 0),
			DayCount:    money.Actual365, Type: model.Annuity,
			StartDate: valuation, MaturityDate: date.MustNew(2026+years, 1, 15),
			PaymentDay: 15, Rounding: money.DefaultPolicy(amd),
		},
		Balance: amt(major), From: valuation,
		Excess: allocation.ExcessReducePrincipal,
	}
}

func three() []plan.Position {
	return []plan.Position{
		pos("a", "Car", 1_200_000, 21, 3),
		pos("b", "Home", 4_000_000, 11, 10),
		pos("c", "Phone", 300_000, 26, 2),
	}
}

func input(loans []plan.Position, monthly int64, payDay int) plan.Input {
	return plan.Input{ValuationDate: valuation, Cash: plan.CashPlan{Monthly: amt(monthly), PayDay: payDay}, Loans: loans}
}

func avalanche(n int, timing plan.Timing) plan.Policy {
	return plan.Policy{
		Name: "avalanche", Order: []int{2, 0, 1}, Timing: uniformT(n, timing),
		Effect: uniformE(n, model.PrepayShortenTerm),
	}
}

func uniformT(n int, t plan.Timing) []plan.Timing {
	out := make([]plan.Timing, n)
	for i := range out {
		out[i] = t
	}
	return out
}

func uniformE(n int, e model.PrepaymentEffect) []model.PrepaymentEffect {
	out := make([]model.PrepaymentEffect, n)
	for i := range out {
		out[i] = e
	}
	return out
}

// --- simulator -------------------------------------------------------------

// Interest must accrue in every cycle of a run, and the cash identity must
// hold: what came in equals what was paid plus what is left.
func TestRunAccruesEveryMonthAndConservesCash(t *testing.T) {
	r, err := plan.Run(input(three(), 200_000, 1), avalanche(3, plan.OnReceipt))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Timeline) != r.Months || r.Months < 12 {
		t.Fatalf("timeline %d, months %d", len(r.Timeline), r.Months)
	}
	for _, m := range r.Timeline[:r.Months-1] {
		if m.Interest.Sign() <= 0 {
			t.Fatalf("cycle %d accrued %s", m.Month, m.Interest)
		}
	}
	if r.PayoffDate.IsZero() || r.TotalPaid.Sign() <= 0 {
		t.Fatalf("payoff %s paid %s", r.PayoffDate, r.TotalPaid)
	}
}

// With no optional payments the simulator must reproduce the schedule
// exactly: same instalment, same interest, same number of rows. This is the
// contract between layer 1 and layer 2.
func TestMinimumMatchesTheSchedule(t *testing.T) {
	ls := three()[:1]
	rep, err := plan.Search(input(ls, 60_000, 1), plan.Goal{Kind: plan.LeastInterest})
	if err != nil {
		t.Fatal(err)
	}
	m := rep.Minimum
	if m.Months != 36 {
		t.Fatalf("minimum on a 3-year loan ran %d cycles", m.Months)
	}
	if m.TotalFees.Sign() != 0 || m.Prepayments != 0 {
		t.Fatalf("minimum made %d prepayments, fees %s", m.Prepayments, m.TotalFees)
	}
}

// A budget that cannot meet a required instalment must fail with the date
// and the exact shortfall, never a partial result.
func TestInfeasibleIsTypedWithDateAndShortfall(t *testing.T) {
	_, err := plan.Run(input(three(), 50_000, 1), avalanche(3, plan.OnDue))
	var inf *plan.InfeasibleError
	if !errors.As(err, &inf) {
		t.Fatalf("got %v, want InfeasibleError", err)
	}
	if inf.On.IsZero() || inf.Shortfall.Sign() <= 0 || inf.LoanID == "" {
		t.Fatalf("incomplete refusal: %+v", inf)
	}
}

// Refusals by name: mixed currency, too many loans, anchor after valuation.
func TestInputRefusals(t *testing.T) {
	usd := money.MustLookup("USD")
	mixed := three()
	mixed[0].Contract.Currency = usd
	mixed[0].Balance = money.FromMinor(100, usd)
	var mc *plan.MixedCurrencyError
	if _, err := plan.Search(input(mixed, 200_000, 1), plan.Goal{}); !errors.As(err, &mc) {
		t.Fatalf("mixed currency: %v", err)
	}
	var many []plan.Position
	for i := 0; i <= plan.MaxLoans; i++ {
		many = append(many, pos(fmt.Sprint("l", i), "L", 100_000, 10, 2))
	}
	var tr *plan.TruncatedError
	if _, err := plan.Search(input(many, 900_000, 1), plan.Goal{}); !errors.As(err, &tr) {
		t.Fatalf("too many: %v", err)
	}
	future := three()
	future[1].From = date.MustNew(2027, 1, 15)
	var un *plan.UnsupportedError
	if _, err := plan.Search(input(future, 200_000, 1), plan.Goal{}); !errors.As(err, &un) {
		t.Fatalf("future anchor: %v", err)
	}
}

// A loan anchored before the valuation date is advanced, and the report
// says how many instalments were assumed.
func TestNormalizeAdvancesOldAnchors(t *testing.T) {
	ls := three()
	in := input(ls, 200_000, 1)
	in.ValuationDate = date.MustNew(2026, 4, 20) // three instalments have fallen due
	rep, err := plan.Search(in, plan.Goal{Kind: plan.LeastInterest})
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range ls {
		if rep.Certificate.AssumedPayments[l.ID] != 3 {
			t.Errorf("%s: assumed %d instalments, want 3", l.ID, rep.Certificate.AssumedPayments[l.ID])
		}
	}
}

// Reserve floor and opening cash are honoured: the floor is never spent on
// an optional payment, and opening cash is.
func TestReserveFloorIsNeverSpent(t *testing.T) {
	in := input(three(), 200_000, 1)
	in.Cash.OpeningCash = amt(500_000)
	in.Cash.ReserveFloor = amt(400_000)
	r, err := plan.Run(in, avalanche(3, plan.OnReceipt))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range r.Timeline {
		if m.Cash.Cmp(amt(400_000)) < 0 {
			t.Fatalf("cycle %d closed with %s, below the floor", m.Month, m.Cash)
		}
	}
}

// --- properties (each scoped to its preconditions) ---------------------------

// P1: under immediate principal credit, no fees, same effect, a fixed
// non-negative rate, paying on receipt costs no more than paying on due.
func TestP1TimingMonotone(t *testing.T) {
	in := input(three(), 250_000, 1)
	due, err := plan.Run(in, avalanche(3, plan.OnDue))
	if err != nil {
		t.Fatal(err)
	}
	early, err := plan.Run(in, avalanche(3, plan.OnReceipt))
	if err != nil {
		t.Fatal(err)
	}
	if early.Cost().Cmp(due.Cost()) > 0 {
		t.Fatalf("on receipt %s dearer than on due %s", early.Cost(), due.Cost())
	}
	if early.Cost().Cmp(due.Cost()) == 0 {
		t.Fatalf("early payment saved nothing; timing not credited")
	}
	if !early.TimingCredited {
		t.Fatal("early payment not credited on a reduce_principal lender")
	}
}

// P2: the best found is never worse than the named strategies it contains.
func TestP2CandidateContainment(t *testing.T) {
	rep, err := plan.Search(input(three(), 250_000, 1), plan.Goal{Kind: plan.LeastInterest})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Best.Cost().Cmp(rep.Avalanche.Cost()) > 0 || rep.Best.Cost().Cmp(rep.Snowball.Cost()) > 0 {
		t.Fatalf("best %s, avalanche %s, snowball %s", rep.Best.Cost(), rep.Avalanche.Cost(), rep.Snowball.Cost())
	}
	if rep.Best.Cost().Cmp(rep.Minimum.Cost()) >= 0 {
		t.Fatalf("best %s not cheaper than minimum %s", rep.Best.Cost(), rep.Minimum.Cost())
	}
}

// P3: with unused cash carried and no fee-bearing payment forced, a larger
// budget cannot worsen the objective — rung by rung.
func TestP3BudgetMonotone(t *testing.T) {
	rep, err := plan.Search(input(three(), 250_000, 1), plan.Goal{Kind: plan.Fastest})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Ladder) < 3 {
		t.Fatalf("ladder has %d rungs", len(rep.Ladder))
	}
	for i := 1; i < len(rep.Ladder); i++ {
		a, b := rep.Ladder[i-1], rep.Ladder[i]
		if b.Payoff.After(a.Payoff) || b.Interest.Cmp(a.Interest) > 0 {
			t.Fatalf("rung %s (%s, %s) worse than %s (%s, %s)", b.Budget, b.Payoff, b.Interest, a.Budget, a.Payoff, a.Interest)
		}
	}
}

// P4: the former three-loan inverse search has no monotonicity proof.
// Successful bisection and a local boundary alone cannot prove global minimality.
// The supported-domain minimum is checked independently in inverse_domain_test.go.
func TestP4BudgetForRefusesUnprovenAllocation(t *testing.T) {
	in := input(three(), 250_000, 1)
	_, err := plan.BudgetFor(in, avalanche(3, plan.OnReceipt), date.MustNew(2027, 1, 15))
	var refusal *plan.NonMonotoneError
	if !errors.As(err, &refusal) {
		t.Fatalf("unproven inverse result: %v", err)
	}
}

// P5: a fee-free lump sum weakly reduces interest and the payoff date.
func TestP5LumpSumWeaklyHelps(t *testing.T) {
	in := input(three(), 200_000, 1)
	base, err := plan.Run(in, avalanche(3, plan.OnReceipt))
	if err != nil {
		t.Fatal(err)
	}
	in.Cash.Lumps = []plan.CashEvent{{On: date.MustNew(2026, 3, 10), Amount: amt(1_000_000)}}
	lump, err := plan.Run(in, avalanche(3, plan.OnReceipt))
	if err != nil {
		t.Fatal(err)
	}
	if lump.PayoffDate.After(base.PayoffDate) || lump.Cost().Cmp(base.Cost()) > 0 {
		t.Fatalf("lump: %s %s vs base %s %s", lump.PayoffDate, lump.Cost(), base.PayoffDate, base.Cost())
	}
}

// P7: replay segmentation — advancing to a later valuation date by
// Normalize, then running, equals the tail of running from the start.
func TestP7ReplaySegmentation(t *testing.T) {
	in := input(three()[:1], 60_000, 0) // minimum only: no optional payments
	full, err := plan.Run(in, plan.Policy{
		Name: "min", Order: []int{0}, Timing: uniformT(1, plan.OnDue),
		Effect: uniformE(1, model.PrepayReduceInstalment), MinPrepay: amt(1_000_000_000),
	})
	if err != nil {
		t.Fatal(err)
	}
	// Three months on: the most Normalize will advance on faith. Beyond
	// MaxAssumedInstalments it refuses with a StaleBalanceError instead,
	// which TestNormalizeRefusesAStaleAnchor pins down.
	later := in
	later.ValuationDate = date.MustNew(2026, 5, 1)
	norm, assumed, err := plan.Normalize(later)
	if err != nil {
		t.Fatal(err)
	}
	if assumed["a"] != 3 {
		t.Fatalf("assumed %d, want 3", assumed["a"])
	}
	if norm.Loans[0].Balance.Cmp(full.Timeline[2].Owed) != 0 {
		t.Fatalf("balance after 3 instalments: normalized %s, full run %s", norm.Loans[0].Balance, full.Timeline[2].Owed)
	}
}

// P8: reordering the input loans cannot change the answer.
func TestP8PermutationSymmetry(t *testing.T) {
	a := three()
	b := []plan.Position{a[2], a[0], a[1]}
	ra, err := plan.Search(input(a, 250_000, 1), plan.Goal{Kind: plan.LeastInterest})
	if err != nil {
		t.Fatal(err)
	}
	rb, err := plan.Search(input(b, 250_000, 1), plan.Goal{Kind: plan.LeastInterest})
	if err != nil {
		t.Fatal(err)
	}
	if ra.Best.Cost().Cmp(rb.Best.Cost()) != 0 || !ra.Best.PayoffDate.Equal(rb.Best.PayoffDate) || ra.Best.FirstClear != rb.Best.FirstClear {
		t.Fatalf("order changed the answer: %s/%s vs %s/%s", ra.Best.Cost(), ra.Best.PayoffDate, rb.Best.Cost(), rb.Best.PayoffDate)
	}
}

// --- goals and certificate ---------------------------------------------------

func TestGoalsAreDifferentQuestions(t *testing.T) {
	u, err := plan.Explore(input(three(), 250_000, 1))
	if err != nil {
		t.Fatal(err)
	}
	cheap, err := u.Rank(plan.Goal{Kind: plan.LeastInterest})
	if err != nil {
		t.Fatal(err)
	}
	relief, err := u.Rank(plan.Goal{Kind: plan.Relief, Cap: amt(60_000)})
	if err != nil {
		t.Fatal(err)
	}
	rm := plan.ReliefMonth(relief.Goal, mustRequired(t, u), relief.Best)
	if rm > 1<<29 {
		t.Fatal("relief never reached")
	}
	if relief.Best.Cost().Cmp(cheap.Best.Cost()) <= 0 {
		t.Fatalf("relief %s not dearer than cheapest %s", relief.Best.Cost(), cheap.Best.Cost())
	}
	if relief.Best.FinalRequired.Cmp(amt(60_000)) > 0 {
		t.Fatalf("relief ends requiring %s, above the cap", relief.Best.FinalRequired)
	}
	if relief.Best.Policy.Rollover != plan.KeepFreed || cheap.Best.Policy.Rollover != plan.RollFreed {
		t.Fatalf("rollovers: relief %s cheap %s", relief.Best.Policy.Rollover, cheap.Best.Policy.Rollover)
	}
	if _, err := u.Rank(plan.Goal{Kind: plan.Relief}); err == nil {
		t.Fatal("relief without a target was accepted")
	}
}

func mustRequired(t *testing.T, u *plan.Universe) money.Amount {
	t.Helper()
	total := money.Zero(amd)
	for _, m := range u.Results[0].Timeline[:1] {
		total = m.Required
	}
	return total
}

func TestCertificateStrengths(t *testing.T) {
	rep, err := plan.Search(input(three(), 250_000, 1), plan.Goal{Kind: plan.LeastInterest})
	if err != nil {
		t.Fatal(err)
	}
	c := rep.Certificate
	if c.Strength != plan.ProvenOptimal {
		t.Fatalf("vanilla three-loan case is %s (%s), want proven", c.Strength, c.Truncation)
	}
	if c.Eligibility == "" || c.Policies == 0 || c.EngineVersion == "" || len(c.Fingerprints) != 3 {
		t.Fatalf("certificate incomplete: %+v", c)
	}
	// Six free-choice loans: exhaustive orders cap exceeded → bounded.
	var six []plan.Position
	for i := 0; i < 6; i++ {
		six = append(six, pos(fmt.Sprint("l", i), "L", 500_000+int64(i)*100_000, 10+int64(i), 3))
	}
	rep, err = plan.Search(input(six, 500_000, 1), plan.Goal{Kind: plan.LeastInterest})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Certificate.Strength == plan.ProvenOptimal || rep.Certificate.Truncation == "" {
		t.Fatalf("six loans claimed %s with no truncation note", rep.Certificate.Strength)
	}
	// Fees present → bounded heuristic with a lower bound and gap.
	fee := three()
	fee[1].Contract.Prepayment.Charges = []model.PrepaymentCharge{{ThroughYear: 3, PercentBP: 60}}
	rep, err = plan.Search(input(fee, 250_000, 1), plan.Goal{Kind: plan.LeastInterest})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Certificate.Strength != plan.BoundedHeuristic || rep.Certificate.LowerBound == nil || rep.Certificate.Gap == nil {
		t.Fatalf("fee case: %s lb=%v gap=%v", rep.Certificate.Strength, rep.Certificate.LowerBound, rep.Certificate.Gap)
	}
}

// Fees change decisions: a fixed per-event fee makes batching win, and a
// percentage fee can make prepaying a loan lose to prepaying another.
func TestFeesChangeTheDecision(t *testing.T) {
	ls := three()
	ls[2].Contract.Prepayment.Charges = []model.PrepaymentCharge{{Fixed: amt(5_000)}}
	rep, err := plan.Search(input(ls, 150_000, 1), plan.Goal{Kind: plan.LeastInterest})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Best.TotalFees.Sign() < 0 {
		t.Fatal("negative fees")
	}
	noFee := three()
	free, err := plan.Search(input(noFee, 150_000, 1), plan.Goal{Kind: plan.LeastInterest})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Best.Prepayments >= free.Best.Prepayments && rep.Best.TotalFees.Sign() > 0 {
		t.Fatalf("with a fixed fee the best plan made %d prepayments (fees %s); fee-free made %d",
			rep.Best.Prepayments, rep.Best.TotalFees, free.Best.Prepayments)
	}
	// A 20% "fee" on the highest-rate loan must push the surplus elsewhere.
	pct := three()
	pct[2].Contract.Prepayment.Charges = []model.PrepaymentCharge{{PercentBP: 2000}}
	rep, err = plan.Search(input(pct, 250_000, 1), plan.Goal{Kind: plan.LeastInterest})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Best.Policy.Order[0] == 2 {
		t.Fatalf("surplus still goes first to the fee-bearing loan: %v", rep.Best.Policy.Order)
	}
}

// Per-loan effect vectors: with two free-choice loans the certificate must
// report four vectors, and the best may mix them.
func TestEffectVectorsArePerLoan(t *testing.T) {
	rep, err := plan.Search(input(three()[:2], 200_000, 1), plan.Goal{Kind: plan.LeastInterest})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Certificate.EffectVectors != 4 || rep.Certificate.TimingVectors != 2 {
		t.Fatalf("vectors: effects %d timings %d", rep.Certificate.EffectVectors, rep.Certificate.TimingVectors)
	}
}

func TestDeterministic(t *testing.T) {
	in := input(three(), 250_000, 5)
	for _, g := range []plan.Goal{{Kind: plan.LeastInterest}, {Kind: plan.Fastest}, {Kind: plan.FirstWin}, {Kind: plan.Relief, Free: amt(40_000)}} {
		a, err := plan.Search(in, g)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := plan.Search(in, g)
		if a.Best.Policy.ID() != b.Best.Policy.ID() || a.Best.Cost().Cmp(b.Best.Cost()) != 0 {
			t.Errorf("%s: %s vs %s", g, a.Best.Policy.ID(), b.Best.Policy.ID())
		}
	}
}
