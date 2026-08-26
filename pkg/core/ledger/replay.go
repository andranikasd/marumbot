// Package ledger rebuilds a loan's position from facts.
//
// Replay is the definition of what Marum believes: the latest trusted bank
// snapshot, plus every active event after it, interpreted under the contract
// version and allocation policy in force on each event's value date. The
// cached loan_state is only ever a memo of this function, and a nightly job
// recomputes it to prove the two still agree.
package ledger

import (
	"errors"
	"fmt"

	"github.com/andranikasd/marumbot/pkg/core/allocation"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// Input is everything replay needs. It is a value: no clock, no store, no
// network. AsOf is a parameter precisely so the same inputs always produce the
// same output.
type Input struct {
	Contracts []model.Contract
	Anchor    model.Snapshot
	Events    []model.Event

	// Covered names events the user confirmed were already included in the
	// anchor balance. Such an event stays in history but is not applied again.
	Covered map[model.ID]bool

	// Policies maps a contract's allocation reference to the policy itself.
	// A missing entry is treated as unknown, which is safe rather than fatal.
	Policies map[model.PolicyRef]allocation.Policy

	AsOf          date.Date
	FreshnessDays int // beyond this, a confirmed anchor is stale; 0 means 35
	EngineVersion string
}

// Result is the derived state plus the working that produced it. The splits
// are what get persisted as versioned allocation results.
type Result struct {
	State  model.LoanState
	Splits []AppliedEvent
}

// AppliedEvent records how one event was interpreted during this replay.
type AppliedEvent struct {
	Event    model.Event
	Split    allocation.Split
	Interest money.Amount // accrued between the previous cursor and this event
	Skipped  string       // non-empty when the event was excluded, with the reason
}

var ErrNoContract = errors.New("no contract version covers the date")

const defaultFreshnessDays = 35

// Replay derives the current position. It never returns a partial state: on a
// structural error it returns the error and no state, because a half-computed
// balance is worse than none.
func Replay(in Input) (Result, error) {
	if err := in.Anchor.Validate(); err != nil {
		return Result{}, fmt.Errorf("anchor snapshot: %w", err)
	}
	if len(in.Contracts) == 0 {
		return Result{}, fmt.Errorf("%w: loan has no contract versions", ErrNoContract)
	}
	if in.AsOf.IsZero() {
		return Result{}, errors.New("replay needs an as-of date")
	}
	if in.AsOf.Before(in.Anchor.AsOf) {
		return Result{}, fmt.Errorf("as-of %s precedes the anchor %s", in.AsOf, in.Anchor.AsOf)
	}

	var reasons []model.Reason
	pos := in.Anchor.Position

	// Partition the events before touching any arithmetic, so the reasons for
	// exclusion are decided once and recorded.
	active, skipped, ambiguous, watermark := partition(in)
	model.SortForReplay(active)

	applied := make([]AppliedEvent, 0, len(active)+len(skipped))
	applied = append(applied, skipped...)

	cursor := in.Anchor.AsOf
	policyUnknown := false

	for _, ev := range active {
		contract, err := contractFor(in.Contracts, ev.ValueDate)
		if err != nil {
			return Result{}, fmt.Errorf("event %s on %s: %w", ev.ID, ev.ValueDate, err)
		}

		interest, err := accrue(pos, contract, cursor, ev.ValueDate)
		if err != nil {
			return Result{}, fmt.Errorf("accruing to %s: %w", ev.ValueDate, err)
		}
		if pos, err = addInterest(pos, interest); err != nil {
			return Result{}, err
		}
		cursor = ev.ValueDate

		policy := in.Policies[contract.AllocationPolicy]
		next, split, err := applyEvent(pos, ev, policy)
		if err != nil && !errors.Is(err, allocation.ErrUnknownPolicy) {
			return Result{}, fmt.Errorf("event %s: %w", ev.ID, err)
		}
		if errors.Is(err, allocation.ErrUnknownPolicy) {
			policyUnknown = true
		}
		pos = next
		applied = append(applied, AppliedEvent{Event: ev, Split: split, Interest: interest})
	}

	// Accrue the tail: from the last event to the date we are stating at.
	contract, err := contractFor(in.Contracts, in.AsOf)
	if err != nil {
		return Result{}, fmt.Errorf("as-of %s: %w", in.AsOf, err)
	}
	tail, err := accrue(pos, contract, cursor, in.AsOf)
	if err != nil {
		return Result{}, fmt.Errorf("accruing to %s: %w", in.AsOf, err)
	}
	if pos, err = addInterest(pos, tail); err != nil {
		return Result{}, err
	}

	if err := pos.Validate(); err != nil {
		return Result{}, fmt.Errorf("replay produced an impossible position: %w", err)
	}

	reasons = append(reasons, ambiguous...)
	if policyUnknown {
		reasons = append(reasons, model.Reason{
			Code:   "allocation_policy_unknown",
			Detail: "this lender's payment allocation has not been established, so the balance is not derived",
		})
	}

	state := model.LoanState{
		LoanID:          in.Anchor.LoanID,
		Position:        pos,
		AnchorSnapshot:  in.Anchor.ID,
		BalanceAsOf:     in.AsOf,
		LastRecordedSeq: watermark,
		EventSetHash:    hashEvents(in.Anchor, active),
		EngineVersion:   in.EngineVersion,
	}
	state.Reliability, state.Reasons = grade(in, pos, reasons, policyUnknown)

	return Result{State: state, Splits: applied}, nil
}

// partition decides which events take part in the arithmetic. Voided events
// and their void markers leave history intact but drop out of the sum, and an
// event that predates the anchor without a coverage assertion is ambiguous:
// applying it might double-count, ignoring it might lose a payment, so replay
// refuses to guess and says so.
func partition(in Input) (active []model.Event, skipped []AppliedEvent, reasons []model.Reason, watermark int64) {
	voided := map[model.ID]bool{}
	for _, e := range in.Events {
		if e.Kind == model.EntryVoided && e.VoidsEvent != "" {
			voided[e.VoidsEvent] = true
		}
	}
	for _, e := range in.Events {
		if e.RecordedSeq > watermark {
			watermark = e.RecordedSeq
		}
		switch {
		case e.Kind == model.EntryVoided:
			skipped = append(skipped, AppliedEvent{Event: e, Skipped: "void marker"})
		case voided[e.ID]:
			skipped = append(skipped, AppliedEvent{Event: e, Skipped: "voided by a later entry"})
		case in.Covered[e.ID]:
			skipped = append(skipped, AppliedEvent{Event: e, Skipped: "already included in the anchor balance"})
		case e.ValueDate.Before(in.Anchor.AsOf):
			skipped = append(skipped, AppliedEvent{Event: e, Skipped: "predates the anchor without confirmation"})
			reasons = append(reasons, model.Reason{
				Code:   "ambiguous_pre_anchor_event",
				Detail: fmt.Sprintf("an entry dated %s predates the confirmed balance of %s", e.ValueDate, in.Anchor.AsOf),
			})
		default:
			active = append(active, e)
		}
	}
	return active, skipped, reasons, watermark
}
