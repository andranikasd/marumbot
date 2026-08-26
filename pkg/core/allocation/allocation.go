// Package allocation decides where a payment goes.
//
// Which bucket a payment settles first — penalty, fee, overdue interest,
// current instalment — is a fact about the lender, not a convention. Two banks
// facing the same payment can produce different balances, and an excess above
// the instalment may reduce principal, sit as an advance instalment, or do
// nothing at all until the borrower files a written request.
//
// So a policy is data, versioned and sourced from a real contract, and the
// default policy is the one that admits it does not know.
package allocation

import (
	"errors"
	"fmt"

	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// ExcessRule says what a lender does with money paid beyond what is currently
// owed.
type ExcessRule uint8

const (
	// ExcessUnknown is the honest default: Marum records the payment but does
	// not claim to know the resulting balance, and asks for a bank figure.
	ExcessUnknown ExcessRule = iota
	// ExcessReducePrincipal applies the surplus to principal immediately.
	ExcessReducePrincipal
	// ExcessHoldAsAdvance parks the surplus against future instalments. The
	// principal does not fall, so no interest saving may be shown.
	ExcessHoldAsAdvance
	// ExcessRequiresBankRequest means nothing happens until the borrower files
	// a request. The plan shows the operational step and treats the
	// prepayment as pending.
	ExcessRequiresBankRequest
)

var excessNames = map[ExcessRule]string{
	ExcessUnknown: "unknown", ExcessReducePrincipal: "reduce_principal",
	ExcessHoldAsAdvance: "hold_as_advance", ExcessRequiresBankRequest: "requires_bank_request",
}

func (e ExcessRule) String() string {
	if n, ok := excessNames[e]; ok {
		return n
	}
	return "unknown"
}

// ParseExcessRule reads a persisted rule.
func ParseExcessRule(s string) (ExcessRule, error) {
	for r, n := range excessNames {
		if n == s {
			return r, nil
		}
	}
	return ExcessUnknown, fmt.Errorf("unknown excess rule %q", s)
}

// Policy is a versioned description of one lender-product's behaviour.
type Policy struct {
	Ref    model.PolicyRef
	Order  []model.Bucket
	Excess ExcessRule
	// Source is the contract clause or schedule this was derived from. A
	// policy with no source is a guess, and guesses do not belong here.
	Source string
}

// ErrUnknownPolicy is returned when a payment cannot be interpreted because
// the lender's behaviour has not been established. It is not a failure: it is
// the engine declining to invent a balance.
var ErrUnknownPolicy = errors.New("allocation policy not established for this lender")

// Unknown is the default policy. It settles nothing and forces reconciliation
// against a bank-reported figure.
var Unknown = Policy{Excess: ExcessUnknown, Source: "no contract studied"}

// IsKnown reports whether the policy can be used to derive a balance.
func (p Policy) IsKnown() bool { return !p.Ref.IsZero() && len(p.Order) > 0 }

// StandardOrder is the sequence most consumer contracts describe: the lender
// takes what is overdue and priced first, and principal last. It is a starting
// point for a real policy, never a substitute for reading the contract.
var StandardOrder = []model.Bucket{
	model.Penalties,
	model.OverdueFees,
	model.UnpaidInterest,
	model.CurrentFees,
	model.AccruedInterest,
	model.OverduePrincipal,
	model.Principal,
}

// Split records how one payment was interpreted. It is a derived result: it is
// stored beside the immutable payment fact, superseded when an earlier event
// changes it, and never written back over the fact itself.
type Split struct {
	Applied        map[model.Bucket]money.Amount
	ExtraToAdvance money.Amount // parked as a future instalment
	Pending        money.Amount // awaiting a bank request; changes nothing yet
	Unapplied      money.Amount // could not be interpreted
	Confident      bool         // false when the policy is unknown or partial
}

// Total returns everything the split accounts for, which must equal the
// payment it came from. Replay asserts this on every event.
func (s Split) Total(cur money.Currency) (money.Amount, error) {
	sum := money.Zero(cur)
	var err error
	for _, b := range order(s.Applied) {
		if sum, err = sum.Add(s.Applied[b]); err != nil {
			return money.Amount{}, err
		}
	}
	for _, a := range []money.Amount{s.ExtraToAdvance, s.Pending, s.Unapplied} {
		if a.IsZero() {
			continue
		}
		if sum, err = sum.Add(a); err != nil {
			return money.Amount{}, err
		}
	}
	return sum, nil
}

// Apply settles payment against pos under the policy, returning the resulting
// position and how the money was accounted for.
//
// Money is conserved: the split always totals exactly the payment, with
// anything uninterpretable recorded as Unapplied rather than quietly dropped.
func Apply(pos model.Buckets, payment money.Amount, p Policy) (model.Buckets, Split, error) {
	cur := pos.Currency()
	if payment.Currency().Code != cur.Code {
		return pos, Split{}, fmt.Errorf("payment is %s, loan is %s", payment.Currency().Code, cur.Code)
	}
	if payment.Sign() <= 0 {
		return pos, Split{}, fmt.Errorf("payment must be positive, got %s", payment)
	}

	split := Split{Applied: map[model.Bucket]money.Amount{}, Confident: p.IsKnown()}
	zero := money.Zero(cur)
	split.ExtraToAdvance, split.Pending, split.Unapplied = zero, zero, zero

	if !p.IsKnown() {
		// Nothing is settled. The fact is recorded, the balance is not touched,
		// and the loan is marked as needing a bank-confirmed figure.
		split.Unapplied = payment
		return pos, split, ErrUnknownPolicy
	}

	remaining := payment
	for _, bucket := range p.Order {
		if remaining.Sign() <= 0 {
			break
		}
		owed := pos.Get(bucket)
		if owed.Sign() <= 0 {
			continue
		}
		take := money.Min(owed, remaining)
		left, err := owed.Sub(take)
		if err != nil {
			return pos, split, err
		}
		pos = pos.With(bucket, left)
		if remaining, err = remaining.Sub(take); err != nil {
			return pos, split, err
		}
		split.Applied[bucket] = take
	}

	if remaining.Sign() <= 0 {
		return pos, split, nil
	}

	// Everything owed is settled and money is left over.
	switch p.Excess {
	case ExcessReducePrincipal:
		// Principal is already zero here, so a surplus beyond a fully repaid
		// loan is an overpayment the lender holds, not negative principal.
		split.ExtraToAdvance = remaining
		credit, err := pos.AdvanceCredit.Add(remaining)
		if err != nil {
			return pos, split, err
		}
		pos.AdvanceCredit = credit
	case ExcessHoldAsAdvance:
		split.ExtraToAdvance = remaining
		credit, err := pos.AdvanceCredit.Add(remaining)
		if err != nil {
			return pos, split, err
		}
		pos.AdvanceCredit = credit
	case ExcessRequiresBankRequest:
		// The money left the borrower but the lender has not applied it. Say
		// so rather than showing a principal reduction that has not happened.
		split.Pending = remaining
		split.Confident = false
	default:
		split.Unapplied = remaining
		split.Confident = false
	}
	return pos, split, nil
}

// order returns the buckets of a split in a stable sequence, so that summing
// or rendering a split never depends on map iteration order.
func order(m map[model.Bucket]money.Amount) []model.Bucket {
	all := []model.Bucket{
		model.Penalties, model.OverdueFees, model.CurrentFees, model.UnpaidInterest,
		model.AccruedInterest, model.OverduePrincipal, model.Principal, model.AdvanceCredit,
	}
	out := make([]model.Bucket, 0, len(m))
	for _, b := range all {
		if _, ok := m[b]; ok {
			out = append(out, b)
		}
	}
	return out
}

// Buckets returns the split's buckets in stable order, for callers that render
// or persist it.
func (s Split) Buckets() []model.Bucket { return order(s.Applied) }
