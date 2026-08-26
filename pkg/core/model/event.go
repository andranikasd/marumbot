package model

import (
	"fmt"
	"sort"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// EventKind is what the borrower reported happened. These record facts, not
// Marum's interpretation of them: how a payment splits across buckets is a
// derived allocation, stored separately and superseded rather than rewritten.
//
// There is deliberately no generic "correction" kind. A wrong number is
// corrected by a new bank snapshot; a wrong entry is undone by a void.
type EventKind uint8

// The event kinds a borrower can report.
const (
	PaymentReported EventKind = iota
	PrepaymentReported
	BankFeeReported
	EntryVoided
	LoanClosedReported
)

var eventKindNames = map[EventKind]string{
	PaymentReported: "payment_reported", PrepaymentReported: "prepayment_reported",
	BankFeeReported: "bank_fee_reported", EntryVoided: "entry_voided",
	LoanClosedReported: "loan_closed_reported",
}

func (k EventKind) String() string {
	if n, ok := eventKindNames[k]; ok {
		return n
	}
	return "unknown"
}

// ParseEventKind reads a persisted event kind.
func ParseEventKind(s string) (EventKind, error) {
	for k, n := range eventKindNames {
		if n == s {
			return k, nil
		}
	}
	return 0, fmt.Errorf("%w: unknown event kind %q", ErrInvalid, s)
}

// Event is one immutable entry in a loan's ledger.
//
// It carries two independent orderings and they are never conflated:
//
//   - RecordedSeq is gapless per loan and records the order Marum learned
//     things. It survives clock skew and out-of-order entry.
//   - ValueDate is when the lender applies the transaction, and it — not
//     RecordedSeq — drives financial replay.
//
// A payment made on the 3rd and entered on the 7th accrues interest as of the
// 3rd. Conflating the two mis-states interest by four days.
type Event struct {
	ID          ID
	LoanID      ID
	ContractVer int
	RecordedSeq int64
	Kind        EventKind
	ValueDate   date.Date
	Amount      money.Amount

	// BankOrder is the lender's intra-day sequence where one is supplied. It
	// is a nullable integer, not the free-text bank reference: ordering by an
	// arbitrary reference string is not well defined.
	BankOrder    int
	HasBankOrder bool

	// VoidsEvent is set only on an EntryVoided event and names the single
	// earlier event it retracts.
	VoidsEvent ID

	// IntendedPrepayment distinguishes "I paid extra" from "I paid my
	// instalment", which is a different instruction to the lender and may be a
	// different instruction to the engine.
	IntendedPrepayment bool
}

// Validate rejects an event that cannot exist.
func (e Event) Validate() error {
	switch {
	case e.LoanID == "":
		return fmt.Errorf("%w: event has no loan", ErrInvalid)
	case e.RecordedSeq < 1:
		return fmt.Errorf("%w: recorded_seq must be positive", ErrInvalid)
	case e.ValueDate.IsZero():
		return fmt.Errorf("%w: event needs a value date", ErrInvalid)
	case e.Kind == EntryVoided && e.VoidsEvent == "":
		return fmt.Errorf("%w: a void must name the event it retracts", ErrInvalid)
	case e.Kind != EntryVoided && e.VoidsEvent != "":
		return fmt.Errorf("%w: only a void may reference another event", ErrInvalid)
	case e.Kind == LoanClosedReported || e.Kind == EntryVoided:
		return nil // carry no amount
	case e.Amount.Sign() <= 0:
		return fmt.Errorf("%w: %s must carry a positive amount", ErrInvalid, e.Kind)
	}
	return nil
}

// SortForReplay orders events the way money moves: by value date, then by the
// lender's intra-day sequence where supplied, then by recorded_seq as a
// deterministic tie-break. Events without a bank order sort after those with
// one on the same day.
//
// The sort is total and stable across runs, which is what lets replay produce
// a byte-identical result every time.
func SortForReplay(events []Event) {
	sort.SliceStable(events, func(i, j int) bool {
		a, b := events[i], events[j]
		if c := a.ValueDate.Compare(b.ValueDate); c != 0 {
			return c < 0
		}
		if a.HasBankOrder != b.HasBankOrder {
			return a.HasBankOrder // ordered entries first
		}
		if a.HasBankOrder && a.BankOrder != b.BankOrder {
			return a.BankOrder < b.BankOrder
		}
		return a.RecordedSeq < b.RecordedSeq
	})
}
