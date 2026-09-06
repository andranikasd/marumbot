package app

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"reflect"
	"sort"
	"strconv"
	"sync"
	"time"

	"go.opentelemetry.io/otel"

	"github.com/andranikasd/marumbot/internal/obs"
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

var planSearchMetrics = obs.NewPlanSearchMetrics(otel.Meter("github.com/andranikasd/marumbot"))

type searchCache struct {
	mu      sync.Mutex
	entries map[string]searchEntry
	metrics *obs.PlanSearchMetrics // optional override, set before use
}

type searchEntry struct {
	rep     plan.Report
	addedAt time.Time
}

// searchFingerprint encodes raw values, never display strings or addresses.
// The schema prefix versions the encoding independently of the engine.
//
// It walks by reflection and writes each field's name and each type's name, so
// the fingerprint moves when the shape of plan.Input moves, not only when its
// values do. That is deliberate for the cache -- a restructured input is a
// different question -- but it is also why ReplayManifest refuses a stored
// manifest after any rename inside plan.Input. See the note at the top of
// pkg/core/plan/input.go before renaming anything there.
//
// The encoding streams straight into the hash. Building the whole encoded
// tree as one string first produced the same digest and threw away tens of
// kilobytes per call -- a cost paid on cache hits too, where it was the only
// cost.
func searchFingerprint(in plan.Input, g plan.Goal) string {
	b := newFingerprintHash()
	fingerprintToken(b, "marum/search-fingerprint/v2")
	fingerprintToken(b, plan.EngineVersion)
	fingerprintValue(b, reflect.ValueOf(in))
	fingerprintValue(b, reflect.ValueOf(g))
	return hex.EncodeToString(b.sum())
}

// fingerprintHash is the encoding sink: one reused block that is handed to the
// hash whenever it fills. The digest is the same one a single concatenated
// string would produce; the string itself is what is no longer built.
type fingerprintHash struct {
	h   hash.Hash
	buf []byte
}

// blockSize is a compromise between calls into the hash and the size of the
// one buffer a fingerprint allocates.
const fingerprintBlock = 8192

func newFingerprintHash() *fingerprintHash {
	return &fingerprintHash{h: sha256.New(), buf: make([]byte, 0, fingerprintBlock+64)}
}

// Length framing prevents collisions between adjacent strings or containers.
func fingerprintToken(b *fingerprintHash, s string) {
	b.buf = strconv.AppendInt(b.buf, int64(len(s)), 10)
	b.buf = append(b.buf, ':')
	b.buf = append(b.buf, s...)
	if len(b.buf) >= fingerprintBlock {
		b.h.Write(b.buf)
		b.buf = b.buf[:0]
	}
}

func (b *fingerprintHash) sum() []byte {
	b.h.Write(b.buf)
	b.buf = b.buf[:0]
	return b.h.Sum(nil)
}

// Input and Goal are acyclic value trees. Walk every field, including private
// money minor units, full currency metadata and date components, without
// Interface or Stringer calls. New unsupported kinds are programmer errors.
func fingerprintValue(b *fingerprintHash, v reflect.Value) {
	t := v.Type()
	fingerprintToken(b, v.Kind().String())
	fingerprintToken(b, t.PkgPath())
	fingerprintToken(b, t.String())
	switch v.Kind() {
	case reflect.Bool:
		fingerprintToken(b, strconv.FormatBool(v.Bool()))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		fingerprintToken(b, strconv.FormatInt(v.Int(), 10))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		fingerprintToken(b, strconv.FormatUint(v.Uint(), 10))
	case reflect.String:
		fingerprintToken(b, v.String())
	case reflect.Struct:
		fingerprintToken(b, strconv.Itoa(v.NumField()))
		for i := 0; i < v.NumField(); i++ {
			fingerprintToken(b, t.Field(i).Name)
			fingerprintValue(b, v.Field(i))
		}
	case reflect.Pointer:
		if v.IsNil() {
			fingerprintToken(b, "nil")
			return
		}
		fingerprintToken(b, "value")
		fingerprintValue(b, v.Elem())
	case reflect.Slice, reflect.Array:
		if v.Kind() == reflect.Slice && v.IsNil() {
			fingerprintToken(b, "nil")
			return
		}
		fingerprintToken(b, strconv.Itoa(v.Len()))
		for i := 0; i < v.Len(); i++ {
			fingerprintValue(b, v.Index(i))
		}
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			panic("search fingerprint: unsupported map key kind " + t.Key().Kind().String())
		}
		if v.IsNil() {
			fingerprintToken(b, "nil")
			return
		}
		keys := v.MapKeys()
		sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
		fingerprintToken(b, strconv.Itoa(len(keys)))
		for _, k := range keys {
			fingerprintValue(b, k)
			fingerprintValue(b, v.MapIndex(k))
		}
	default:
		panic("search fingerprint: unsupported kind " + v.Kind().String())
	}
}

// search returns the cached report for this exact input, computing and
// remembering it on a miss.
func (c *searchCache) search(in plan.Input, g plan.Goal, now time.Time) (plan.Report, error) {
	metrics := c.metrics
	if metrics == nil {
		metrics = planSearchMetrics
	}
	key := searchFingerprint(in, g)

	c.mu.Lock()
	if e, ok := c.entries[key]; ok && now.Sub(e.addedAt) < searchCacheTTL {
		c.mu.Unlock()
		metrics.CacheLookup(true)
		return e.rep, nil
	}
	c.mu.Unlock()
	metrics.CacheLookup(false)

	// Compute outside the lock: a search can take seconds, and holding the
	// lock across it would serialise every user behind the slowest plan.
	finish := metrics.StartSearch()
	rep, err := plan.Search(in, g)
	finish(err == nil)
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
//
// It scans the whole map, which is a full pass per insert once the cache is
// full. That is deliberate: the map holds at most searchCacheMax entries, so
// the pass is a few hundred iterations against a search that takes hundreds
// of milliseconds. An eviction order kept alongside the map would save
// nothing measurable and would add a second structure that can drift out of
// step with the first.
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
