package app

import (
	"testing"
	"time"

	"github.com/andranikasd/marumbot/pkg/core/allocation"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

func cacheInput(t *testing.T) plan.Input {
	t.Helper()
	amd := money.MustLookup("AMD")
	v := date.MustNew(2026, 1, 15)
	return plan.Input{
		ValuationDate: v,
		Cash:          plan.CashPlan{Monthly: money.FromMinor(25_000_000, amd), PayDay: 1},
		Loans: []plan.Position{{
			ID: "a", Name: "Car",
			Contract: model.Contract{
				LoanID: "a", Version: 1, Currency: amd, EffectiveFrom: v,
				NominalRate: money.RateFromPercent(21, 0), DayCount: money.Actual365,
				Type: model.Annuity, StartDate: v, MaturityDate: date.MustNew(2029, 1, 15),
				PaymentDay: 15, Rounding: money.DefaultPolicy(amd),
			},
			Balance: money.FromMinor(120_000_000, amd), From: v,
			Excess: allocation.ExcessReducePrincipal, Trust: "user_entered",
		}},
	}
}

// The fingerprint must move with anything that can move the answer, and hold
// still when nothing did — that is the entire correctness argument for the
// cache.
func TestSearchFingerprint(t *testing.T) {
	g := plan.Goal{Kind: plan.LeastInterest}
	base := searchFingerprint(cacheInput(t), g)

	if searchFingerprint(cacheInput(t), g) != base {
		t.Fatal("the same input produced two fingerprints")
	}

	moved := cacheInput(t)
	moved.Loans[0].Balance = money.FromMinor(119_999_900, money.MustLookup("AMD"))
	if searchFingerprint(moved, g) == base {
		t.Error("a payment did not change the fingerprint")
	}

	later := cacheInput(t)
	later.ValuationDate = date.MustNew(2026, 1, 16)
	if searchFingerprint(later, g) == base {
		t.Error("the day rolling over did not change the fingerprint")
	}

	if searchFingerprint(cacheInput(t), plan.Goal{Kind: plan.Fastest}) == base {
		t.Error("a different goal did not change the fingerprint")
	}
}

func TestSearchCacheHitsAndExpires(t *testing.T) {
	var c searchCache
	in := cacheInput(t)
	g := plan.Goal{Kind: plan.LeastInterest}
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	first, err := c.search(in, g, now)
	if err != nil {
		t.Fatalf("first search: %v", err)
	}
	if len(c.entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(c.entries))
	}

	again, err := c.search(in, g, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("second search: %v", err)
	}
	if len(c.entries) != 1 {
		t.Fatalf("a hit added an entry: %d", len(c.entries))
	}
	if first.Best.Months != again.Best.Months ||
		first.Best.TotalInterest.Minor() != again.Best.TotalInterest.Minor() {
		t.Error("the cached report differs from the computed one")
	}

	// Past the TTL the entry is recomputed, not served.
	if _, err := c.search(in, g, now.Add(searchCacheTTL+time.Minute)); err != nil {
		t.Fatalf("post-TTL search: %v", err)
	}
	if len(c.entries) != 1 {
		t.Fatalf("expected the expired entry replaced, got %d entries", len(c.entries))
	}
	if e := c.entries[searchFingerprint(in, g)]; !e.addedAt.After(now) {
		t.Error("the expired entry was not replaced")
	}
}

func TestSearchCacheEviction(t *testing.T) {
	var c searchCache
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	c.entries = make(map[string]searchEntry, searchCacheMax)

	// Fill to the cap with synthetic entries: one stale, the rest fresh.
	c.entries["stale"] = searchEntry{addedAt: now.Add(-searchCacheTTL - time.Hour)}
	for i := 0; len(c.entries) < searchCacheMax; i++ {
		c.entries[string(rune('a'+i%26))+string(rune('0'+i/26))] = searchEntry{addedAt: now}
	}

	if _, err := c.search(cacheInput(t), plan.Goal{Kind: plan.LeastInterest}, now); err != nil {
		t.Fatalf("search at the cap: %v", err)
	}
	if _, ok := c.entries["stale"]; ok {
		t.Error("the stale entry survived eviction")
	}
	if len(c.entries) > searchCacheMax {
		t.Errorf("cache grew past its cap: %d", len(c.entries))
	}
}
