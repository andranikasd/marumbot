package model

import (
	"fmt"

	"github.com/andranikasd/marumbot/pkg/core/date"
)

// Reliability says how much weight Marum's own numbers deserve. It is computed
// by replay and it gates what the product may claim, so it is part of the
// state rather than a presentation concern.
type Reliability uint8

const (
	// Confirmed means anchored on a bank-confirmed snapshot, fresh, unambiguous.
	Confirmed Reliability = iota
	// Estimated means derived from an unconfirmed snapshot: usable, labelled.
	Estimated
	// Stale means confirmed once, but the anchor is past the freshness
	// threshold. Projections are indicative; savings figures are not shown.
	Stale
	// NeedsReconciliation means an event predates the anchor without a
	// coverage assertion, or the allocation policy is unknown. Marum does not
	// know what it does not know, so it asks for a bank balance.
	NeedsReconciliation
	// Unsupported means arrears, penalties or a contract shape the MVP refuses
	// to model. The ledger and reminders continue; projections stop.
	Unsupported
)

var reliabilityNames = map[Reliability]string{
	Confirmed: "confirmed", Estimated: "estimated", Stale: "stale",
	NeedsReconciliation: "needs_reconciliation", Unsupported: "unsupported",
}

func (r Reliability) String() string {
	if n, ok := reliabilityNames[r]; ok {
		return n
	}
	return "unknown"
}

// PlanTier maps reliability onto what the interface is allowed to show.
// Eligibility is graded rather than binary: a single confirmed-and-fresh gate
// in front of every calculation would refuse most users most of the time, and
// a tool that usually refuses is one people stop opening.
type PlanTier uint8

// What an interface is allowed to claim at each tier.
const (
	TierConfident  PlanTier = iota // plans, savings, debt-free dates
	TierIndicative                 // projections, labelled with the anchor's age
	TierBlocked                    // ledger and reminders only, with the reason
)

func (t PlanTier) String() string {
	switch t {
	case TierIndicative:
		return "indicative"
	case TierBlocked:
		return "blocked"
	}
	return "confident"
}

// Tier returns what may be shown for this reliability.
func (r Reliability) Tier() PlanTier {
	switch r {
	case Confirmed:
		return TierConfident
	case Estimated, Stale:
		return TierIndicative
	default:
		return TierBlocked
	}
}

// Reason is a machine-readable explanation the interface renders to the user.
// Replay returns reasons rather than prose so the same fact can be translated,
// alerted on, and counted.
type Reason struct {
	Code   string
	Detail string
}

// LoanState is the derived position of a loan: a cache, not a source of truth.
// Delete it and replay rebuilds it byte for byte from the anchor snapshot plus
// the active events.
type LoanState struct {
	LoanID   ID
	Position Buckets

	// Anchor identifies the snapshot replay started from, and BalanceAsOf the
	// date the position is stated at.
	AnchorSnapshot ID
	BalanceAsOf    date.Date

	// LastRecordedSeq is the causal watermark: every event up to this sequence
	// has been considered, including ones excluded from the arithmetic.
	LastRecordedSeq int64

	// EventSetHash covers the exact set of events that produced this position.
	// The nightly reconciliation recomputes it; a mismatch rebuilds the cache
	// and raises an alert, because it means the two disagree.
	EventSetHash [32]byte

	Reliability   Reliability
	Reasons       []Reason
	EngineVersion string
}

// Tier is what the interface may claim about this loan.
func (s LoanState) Tier() PlanTier { return s.Reliability.Tier() }

func (s LoanState) String() string {
	total, err := s.Position.TotalOwed()
	if err != nil {
		return fmt.Sprintf("loan %s: <invalid position>", s.LoanID)
	}
	return fmt.Sprintf("loan %s: %s owed as of %s (%s)",
		s.LoanID, total, s.BalanceAsOf, s.Reliability)
}
