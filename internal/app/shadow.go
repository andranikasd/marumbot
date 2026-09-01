package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

// Shadow mode (DK.1).
//
// Before anyone is asked to act on a recommendation, the engine's answers are
// computed for real accounts and stored, silently. Later bank cycles are then
// compared row-by-row against what the engine said at the time -- the first
// field gate on the road to v1.0.0. Nothing here sends a message or changes
// a plan; the borrower cannot tell shadow mode exists.

// ShadowRecommendation is one frozen answer: what the engine recommended for
// an account on a day, and the fingerprint of the inputs it saw.
type ShadowRecommendation struct {
	UserID      string
	ComputedOn  string // ISO date, the valuation date of the sheet
	Goal        string
	Engine      string
	Fingerprint string
	Sheet       []byte // the marshalled Sheet, exactly as a surface would render it
}

// ShadowStore persists shadow recommendations. Optional: a Worker without one
// simply runs no shadow walk.
type ShadowStore interface {
	// RecordShadow stores one recommendation. It reports false when a row for
	// this account, day and goal already exists -- the normal case for every
	// tick after the day's first.
	RecordShadow(ctx context.Context, r ShadowRecommendation) (bool, error)
}

// shadowWalkLimit caps how many accounts one walk touches.
const shadowWalkLimit = 500

// shadowEvery is how often the walk actually computes. The store dedups per
// day, so more often only helps accounts created since the last walk; every
// six hours catches those without paying for a plan search per tick.
const shadowEvery = 6 * time.Hour

// TickShadow computes and stores today's recommendation for every account
// that has enough state to plan. It is called from the scheduler tick and
// rate-limits itself the way TickReminders does; the per-day unique key in
// the store makes every run after the day's first a no-op per account.
func (w *Worker) TickShadow(ctx context.Context, users UserLister) (int, error) {
	if w.Shadow == nil || users == nil {
		return 0, nil
	}
	now := w.Clock.Now()
	last := w.lastShadow.Load()
	if last != 0 && now.Sub(time.Unix(0, last)) < shadowEvery {
		return 0, nil
	}
	if !w.lastShadow.CompareAndSwap(last, now.UnixNano()) {
		return 0, nil // another tick won the walk
	}
	ids, err := users.ActiveLoanUsers(ctx, shadowWalkLimit)
	if err != nil {
		return 0, fmt.Errorf("listing accounts for the shadow walk: %w", err)
	}
	today := date.From(w.Clock.Now(), time.UTC).String()
	recorded := 0
	for _, id := range ids {
		sh, err := w.PlanSheet(ctx, id, nil)
		if errors.Is(err, ErrNotFound) {
			continue // no live loans or no budget: nothing to shadow
		}
		if err != nil {
			// One account's broken plan must not silence the others' evidence.
			w.Log.WarnContext(ctx, "shadow: computing the sheet failed", "user", id, "error", err)
			continue
		}
		raw, err := json.Marshal(sh)
		if err != nil {
			w.Log.WarnContext(ctx, "shadow: marshalling the sheet failed", "user", id, "error", err)
			continue
		}
		sum := sha256.Sum256(raw)
		wrote, err := w.Shadow.RecordShadow(ctx, ShadowRecommendation{
			UserID:      id,
			ComputedOn:  today,
			Goal:        sh.Goal,
			Engine:      plan.EngineVersion,
			Fingerprint: hex.EncodeToString(sum[:]),
			Sheet:       raw,
		})
		if err != nil {
			w.Log.WarnContext(ctx, "shadow: storing failed", "user", id, "error", err)
			continue
		}
		if wrote {
			recorded++
		}
	}
	if recorded > 0 {
		// Counts only; never amounts or identifiers (I5).
		w.Log.InfoContext(ctx, "shadow recommendations recorded", "accounts", recorded)
	}
	return recorded, nil
}
