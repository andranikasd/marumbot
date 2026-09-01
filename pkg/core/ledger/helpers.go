package ledger

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	"github.com/andranikasd/marumbot/pkg/core/allocation"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// contractFor returns the version in force on d. When a revision starts the
// day its predecessor ends -- the shape every revision writes -- both cover
// that one date, so the version with the latest effective_from wins: the new
// terms govern from their own first day, deterministically, regardless of
// slice order. If none covers the date, replay stops rather than picking one.
func contractFor(versions []model.Contract, d date.Date) (model.Contract, error) {
	var best model.Contract
	found := false
	for _, c := range versions {
		if !c.CoversDate(d) {
			continue
		}
		if !found || c.EffectiveFrom.After(best.EffectiveFrom) {
			best, found = c, true
		}
	}
	if !found {
		return model.Contract{}, fmt.Errorf("%w: %s", ErrNoContract, d)
	}
	return best, nil
}

// accrueAcross accrues interest over (from, to], splitting the interval at
// every contract-version boundary inside it, so a rate or day-count change
// mid-interval charges each day under the terms actually in force on it.
// Accruing the whole span under one version -- whichever the interval
// happened to end in -- was wrong by the difference between the versions,
// and loan revisions make that interval an ordinary occurrence.
//
// Each segment is rounded like a segment always was: the per-event rounding
// this ledger has done since the first version, applied at version edges too.
func accrueAcross(pos model.Buckets, versions []model.Contract, from, to date.Date) (money.Amount, error) {
	total := money.Zero(pos.Currency())
	segStart := from
	for segStart.Before(to) {
		// The version covering the day after segStart governs the segment,
		// which runs to the end of that version's own effective range (its
		// last covered day) or to `to`, whichever comes first. A revision
		// closes its predecessor on the day it starts, so the boundary day's
		// single day of interest accrues under the old terms and the new
		// terms take over from the next day -- one rule, applied every time.
		c, err := contractFor(versions, date.AddDays(segStart, 1))
		if err != nil {
			return money.Amount{}, err
		}
		segEnd := to
		if !c.EffectiveThru.IsZero() && c.EffectiveThru.Before(to) {
			segEnd = c.EffectiveThru
		}
		if !segEnd.After(segStart) {
			// A zero-length or inverted segment means the version table is
			// malformed; stopping beats looping forever.
			return money.Amount{}, fmt.Errorf("%w: version %d covers no days after %s",
				ErrNoContract, c.Version, segStart)
		}
		part, err := accrue(pos, c, segStart, segEnd)
		if err != nil {
			return money.Amount{}, err
		}
		if total, err = total.Add(part); err != nil {
			return money.Amount{}, err
		}
		segStart = segEnd
	}
	return total, nil
}

// accrue returns the interest earned on the outstanding principal between two
// dates. Interest accrues on principal and overdue principal; it does not
// compound onto unpaid interest, because compounding is a contract term the
// MVP does not model and inventing it would overstate every balance.
func accrue(pos model.Buckets, c model.Contract, from, to date.Date) (money.Amount, error) {
	cur := pos.Currency()
	days := date.DaysBetween(from, to)
	if days <= 0 {
		return money.Zero(cur), nil
	}
	if c.DayCount == money.Thirty360 {
		days = date.Days30360(from, to)
		if days <= 0 {
			return money.Zero(cur), nil
		}
	}
	base, err := pos.Principal.Add(pos.OverduePrincipal)
	if err != nil {
		return money.Amount{}, err
	}
	if base.Sign() <= 0 {
		return money.Zero(cur), nil
	}
	return money.Accrue(base, c.NominalRate, int64(days), c.DayCount, c.Rounding)
}

func addInterest(pos model.Buckets, interest money.Amount) (model.Buckets, error) {
	if interest.Sign() == 0 {
		return pos, nil
	}
	sum, err := pos.AccruedInterest.Add(interest)
	if err != nil {
		return pos, err
	}
	pos.AccruedInterest = sum
	return pos, nil
}

// applyEvent settles one reported fact against the position.
func applyEvent(pos model.Buckets, ev model.Event, p allocation.Policy) (model.Buckets, allocation.Split, error) {
	switch ev.Kind {
	case model.PaymentReported, model.PrepaymentReported:
		return allocation.Apply(pos, ev.Amount, p)

	case model.BankFeeReported:
		// A fee the lender charged. It is a fact the borrower read off a
		// statement, not something Marum derived from a fee schedule.
		fees, err := pos.CurrentFees.Add(ev.Amount)
		if err != nil {
			return pos, allocation.Split{}, err
		}
		pos.CurrentFees = fees
		return pos, allocation.Split{
			Applied:   map[model.Bucket]money.Amount{model.CurrentFees: ev.Amount},
			Confident: true,
		}, nil

	case model.LoanClosedReported:
		// The lender says the loan is settled. That outranks our arithmetic,
		// and any residue we were carrying was our error, not a debt.
		cur := pos.Currency()
		return model.NewBuckets(cur), allocation.Split{Confident: true}, nil

	default:
		return pos, allocation.Split{}, fmt.Errorf("event kind %s cannot be applied", ev.Kind)
	}
}

// hashEvents fingerprints the exact set of facts that produced a position.
// The nightly reconciliation recomputes it; a mismatch means the cache and the
// ledger disagree and the cache is rebuilt.
//
// Only fields that change the arithmetic are hashed. Recording timestamps are
// excluded on purpose: re-entering the same payment later must not look like a
// different ledger.
func hashEvents(anchor model.Snapshot, events []model.Event) [32]byte {
	h := sha256.New()
	h.Write([]byte(anchor.ID))
	h.Write([]byte(anchor.AsOf.String()))
	var buf [8]byte
	for _, e := range events {
		h.Write([]byte(e.ID))
		binary.BigEndian.PutUint64(buf[:], uint64(e.RecordedSeq))
		h.Write(buf[:])
		h.Write([]byte{byte(e.Kind)})
		h.Write([]byte(e.ValueDate.String()))
		binary.BigEndian.PutUint64(buf[:], uint64(e.Amount.Minor()))
		h.Write(buf[:])
		h.Write([]byte(e.Amount.Currency().Code))
	}
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

// grade decides how much weight the derived position deserves. The most severe
// condition wins: a loan with arrears is unsupported even if its anchor was
// confirmed this morning.
func grade(in Input, pos model.Buckets, reasons []model.Reason, policyUnknown bool) (model.Reliability, []model.Reason) {
	if pos.HasArrears() || in.Anchor.Position.HasArrears() {
		return model.Unsupported, append(reasons, model.Reason{
			Code:   "arrears_present",
			Detail: "this loan carries penalties, overdue principal or unpaid interest, which the engine does not plan around",
		})
	}
	for _, r := range reasons {
		if r.Code == "ambiguous_pre_anchor_event" {
			return model.NeedsReconciliation, reasons
		}
	}
	if policyUnknown {
		return model.NeedsReconciliation, reasons
	}
	if in.Anchor.Trust != model.BankConfirmed && in.Anchor.Trust != model.ImportedVerified {
		return model.Estimated, append(reasons, model.Reason{
			Code:   "anchor_not_confirmed",
			Detail: "the opening balance was entered but not confirmed against the lender",
		})
	}
	limit := in.FreshnessDays
	if limit <= 0 {
		limit = defaultFreshnessDays
	}
	if age := date.DaysBetween(in.Anchor.AsOf, in.AsOf); age > limit {
		return model.Stale, append(reasons, model.Reason{
			Code:   "anchor_stale",
			Detail: fmt.Sprintf("the confirmed balance is %d days old, beyond the %d-day threshold", age, limit),
		})
	}
	return model.Confirmed, reasons
}
