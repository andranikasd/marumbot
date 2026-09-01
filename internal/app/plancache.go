package app

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/andranikasd/marumbot/pkg/core/plan"
)

// The search cache.
//
// plan.Search is pure and deterministic: the same input and goal always
// produce the same report. It is also the most expensive computation in the
// system, and every open of the plan sheet runs it from scratch. Caching the
// report by a fingerprint of its inputs turns a repeat open into a lookup,
// while any change that could alter the answer — a payment, a new loan, a
// budget edit, the day rolling over — changes the fingerprint and misses.
//
// Only the pure computation is cached. The rows the fingerprint is built from
// are read fresh from the database on every call, so the cache can never
// serve a plan for loans the user no longer has.

// searchCacheMax bounds memory. A report is a few kilobytes; 256 of them is
// nothing, and one container serves far fewer concurrent users than that.
const searchCacheMax = 256

// searchCacheTTL is a backstop only: fingerprints already roll with the
// valuation date, so entries stop being reachable after midnight. The TTL
// exists so an unreachable entry is also eventually gone.
const searchCacheTTL = 26 * time.Hour

type searchCache struct {
	mu      sync.Mutex
	entries map[string]searchEntry
}

type searchEntry struct {
	rep     plan.Report
	addedAt time.Time
}

// fingerprint reduces everything the search reads to one key. %+v is stable
// here because Input and Goal are trees of value types — no pointers, no maps
// — and the engine's determinism tests hold exactly that shape still.
func searchFingerprint(in plan.Input, g plan.Goal) string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%s|%+v|%+v", plan.EngineVersion, in, g) // sha256 never errors
	return hex.EncodeToString(h.Sum(nil))
}

// search returns the cached report for this exact input, computing and
// remembering it on a miss.
func (c *searchCache) search(in plan.Input, g plan.Goal, now time.Time) (plan.Report, error) {
	key := searchFingerprint(in, g)

	c.mu.Lock()
	if e, ok := c.entries[key]; ok && now.Sub(e.addedAt) < searchCacheTTL {
		c.mu.Unlock()
		return e.rep, nil
	}
	c.mu.Unlock()

	// Compute outside the lock: a search can take seconds, and holding the
	// lock across it would serialise every user behind the slowest plan.
	rep, err := plan.Search(in, g)
	if err != nil {
		return plan.Report{}, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]searchEntry)
	}
	if len(c.entries) >= searchCacheMax {
		c.evictLocked(now)
	}
	c.entries[key] = searchEntry{rep: rep, addedAt: now}
	return rep, nil
}

// evictLocked drops expired entries, and if none were, the oldest one. Called
// with the lock held.
func (c *searchCache) evictLocked(now time.Time) {
	oldestKey := ""
	var oldestAt time.Time
	dropped := false
	for k, e := range c.entries {
		if now.Sub(e.addedAt) >= searchCacheTTL {
			delete(c.entries, k)
			dropped = true
			continue
		}
		if oldestKey == "" || e.addedAt.Before(oldestAt) {
			oldestKey, oldestAt = k, e.addedAt
		}
	}
	if !dropped && oldestKey != "" {
		delete(c.entries, oldestKey)
	}
}
