package app

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
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

func TestSearchCacheCoalescesConcurrentMisses(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	c := searchCache{compute: func(context.Context, plan.Input, plan.Goal) (plan.Report, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return plan.Report{}, nil
	}}
	in, goal := cacheInput(t), plan.Goal{Kind: plan.LeastInterest}
	now := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.searchContext(context.Background(), in, goal, now); err != nil {
				t.Error(err)
			}
		}()
	}
	<-started
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("computed %d times", calls.Load())
	}
}

// Signals that the waiter reached a cancellation-aware blocking point.
type observedSearchContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (c *observedSearchContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })
	return c.Context.Done()
}

func TestSearchCacheCanceledLeaderAllowsWaiterRetry(t *testing.T) {
	started := make(chan struct{})
	var calls atomic.Int32
	c := searchCache{compute: func(ctx context.Context, _ plan.Input, _ plan.Goal) (plan.Report, error) {
		if calls.Add(1) == 1 {
			close(started)
			<-ctx.Done()
			return plan.Report{}, ctx.Err()
		}
		return plan.Report{}, nil
	}}
	in, goal := cacheInput(t), plan.Goal{Kind: plan.LeastInterest}
	now := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	leader := make(chan error, 1)
	go func() { _, err := c.searchContext(ctx, in, goal, now); leader <- err }()
	<-started
	waiterCtx := &observedSearchContext{Context: context.Background(), waiting: make(chan struct{})}
	waiter := make(chan error, 1)
	go func() { _, err := c.searchContext(waiterCtx, in, goal, now); waiter <- err }()
	<-waiterCtx.waiting
	cancel()
	if err := <-leader; !errors.Is(err, context.Canceled) {
		t.Fatalf("leader error = %v", err)
	}
	if err := <-waiter; err != nil {
		t.Fatalf("waiter error = %v", err)
	}
	if calls.Load() != 2 || len(c.entries) != 1 {
		t.Fatalf("calls=%d entries=%d", calls.Load(), len(c.entries))
	}
}

func TestSearchCacheWaitingMissCanCancelWithoutStarting(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	c := searchCache{compute: func(context.Context, plan.Input, plan.Goal) (plan.Report, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return plan.Report{}, nil
	}}
	in, goal := cacheInput(t), plan.Goal{Kind: plan.LeastInterest}
	now := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	leader := make(chan error, 1)
	go func() { _, err := c.searchContext(context.Background(), in, goal, now); leader <- err }()
	<-started
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	waiterCtx := &observedSearchContext{Context: ctx, waiting: make(chan struct{})}
	waiter := make(chan error, 1)
	go func() { _, err := c.searchContext(waiterCtx, in, plan.Goal{Kind: plan.Fastest}, now); waiter <- err }()
	<-waiterCtx.waiting
	cancel()
	err := <-waiter
	close(release)
	leaderErr := <-leader
	if !errors.Is(err, context.Canceled) || leaderErr != nil {
		t.Fatalf("waiter=%v leader=%v", err, leaderErr)
	}
	if calls.Load() != 1 {
		t.Fatalf("computed %d times", calls.Load())
	}
}

func TestSearchCacheEvictsToByteBudget(t *testing.T) {
	now := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	c := searchCache{
		entries: map[string]searchEntry{"large": {addedAt: now, bytes: searchCacheBytes}},
		bytes:   searchCacheBytes,
		compute: func(context.Context, plan.Input, plan.Goal) (plan.Report, error) { return plan.Report{}, nil },
	}
	if _, err := c.search(cacheInput(t), plan.Goal{Kind: plan.LeastInterest}, now); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.entries["large"]; ok {
		t.Fatal("byte budget failed to evict old entry")
	}
	if c.bytes <= 0 || c.bytes > searchCacheBytes {
		t.Fatalf("retained size estimate = %d", c.bytes)
	}
}
