package plan

import (
	"fmt"
	"strings"

	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// Timing is when a loan's share of the surplus is paid.
type Timing uint8

const (
	// OnDue pays the extra together with the required instalment.
	OnDue Timing = iota
	// OnReceipt pays the extra on the day the money arrives, so the balance
	// accrues less for the rest of the cycle. Only lenders that reduce
	// principal on the day of payment give this any effect; for the others
	// the simulator holds the money until the due date and says so.
	OnReceipt
)

func (t Timing) String() string {
	if t == OnReceipt {
		return "on_receipt"
	}
	return "on_due"
}

// Rollover is what happens to a cleared loan's instalment.
type Rollover uint8

const (
	// RollFreed keeps the debt budget: the freed instalment goes to the next
	// loan and the monthly outflow stays the same until everything is clear.
	RollFreed Rollover = iota
	// KeepFreed reduces the debt budget by the freed instalment: the
	// borrower keeps the money and the outflow falls. This is what "pay less
	// per month" means, and it costs interest.
	KeepFreed
)

func (r Rollover) String() string {
	if r == KeepFreed {
		return "keep_freed"
	}
	return "roll_freed"
}

// Policy is one way of spending the surplus. Order is a priority over the
// loans by index; Timing and Effect are per loan, because one lender may
// credit an early payment while another holds it, and one contract may fix
// the prepayment effect while another leaves it to the borrower.
type Policy struct {
	// RequiredOnly forbids every optional action, including an early payoff.
	RequiredOnly bool
	Name         string // avalanche, snowball, minimum, or order
	Order        []int
	Timing       []Timing
	Effect       []model.PrepaymentEffect
	Rollover     Rollover
	// MinPrepay withholds an optional payment smaller than this, carrying
	// the cash to the next cycle. Zero means pay whatever is available. It
	// is how a fixed per-event fee is answered: batch, then pay.
	MinPrepay money.Amount
}

// ID is the canonical name of a policy, used as the final tie-break so a
// ranking is total and the same input always names the same winner.
func (p Policy) ID() string {
	var b strings.Builder
	b.WriteString(p.Name)
	if p.RequiredOnly {
		b.WriteString("/required-only")
	}
	fmt.Fprint(&b, p.Order)
	for _, t := range p.Timing {
		b.WriteString("/" + t.String())
	}
	for _, e := range p.Effect {
		b.WriteString("/" + e.String())
	}
	b.WriteString("/" + p.Rollover.String())
	if p.MinPrepay.Sign() > 0 {
		b.WriteString("/min=" + p.MinPrepay.String())
	}
	return b.String()
}

// String is the short human form: name, the timing and effect if uniform,
// the rollover if it keeps cash.
func (p Policy) String() string {
	s := p.Name
	if t, ok := uniformTiming(p.Timing); ok {
		s += "/" + t.String()
	} else {
		s += "/mixed_timing"
	}
	if e, ok := uniformEffect(p.Effect); ok && e != model.PrepayBorrowerChooses {
		s += "/" + e.String()
	} else if !ok {
		s += "/mixed_effect"
	}
	if p.Rollover == KeepFreed {
		s += "/" + p.Rollover.String()
	}
	if p.MinPrepay.Sign() > 0 {
		s += "/batch"
	}
	return s
}

func uniformTiming(ts []Timing) (Timing, bool) {
	if len(ts) == 0 {
		return OnDue, true
	}
	for _, t := range ts[1:] {
		if t != ts[0] {
			return 0, false
		}
	}
	return ts[0], true
}

func uniformEffect(es []model.PrepaymentEffect) (model.PrepaymentEffect, bool) {
	if len(es) == 0 {
		return model.PrepayBorrowerChooses, true
	}
	for _, e := range es[1:] {
		if e != es[0] {
			return 0, false
		}
	}
	return es[0], true
}

func uniform[T any](n int, v T) []T {
	out := make([]T, n)
	for i := range out {
		out[i] = v
	}
	return out
}

func identity(n int) []int {
	o := make([]int, n)
	for i := range o {
		o[i] = i
	}
	return o
}
