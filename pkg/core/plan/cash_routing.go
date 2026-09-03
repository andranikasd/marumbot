package plan

import (
	"fmt"
	"sort"

	"github.com/andranikasd/marumbot/pkg/core/allocation"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// CashRouting constrains optional use of one confirmed receipt. LoanID earmarks
// all of it; Splits assigns exact outflow amounts (including fees) to unique
// loans and must sum to the receipt. Both empty releases to the ordinary pool
// after the hold. HoldUntil and HoldMinimum are AND gates. Matching unreleased
// buckets (each split destination separately) accumulate toward the cash
// threshold; once released, leftovers never
// re-lock. A closed target's remaining cash never spills to another loan.
type CashRouting struct {
	LoanID      string
	Splits      []CashSplit
	HoldUntil   date.Date
	HoldMinimum money.Amount
}

// CashSplit is an exact, positive, settlement-aligned outflow allocation.
type CashSplit struct {
	LoanID string
	Amount money.Amount
}

type cashBucket struct {
	target    string
	remaining money.Amount
	until     date.Date
	minimum   money.Amount
	released  bool
	wake      date.Date
}

func (in Input) validateCashRouting() error {
	fail := func(reason string) error { return &UnsupportedError{Feature: "cash routing: " + reason} }
	cur := in.Cash.Monthly.Currency()
	seen := map[string]bool{}
	opening := money.Zero(cur)
	hasRouting := false
	for _, event := range in.Cash.Lumps {
		if event.Routing != nil || event.FromOpening {
			hasRouting = true
		}
	}
	if !hasRouting {
		return nil
	}
	if err := validateStrategyIDs(in); err != nil {
		return err
	}
	target := func(id string, amount money.Amount) error {
		for _, l := range in.Loans {
			if l.ID == id {
				if l.Balance.Sign() <= 0 || l.OptionalExcluded {
					return fail("target is closed or excluded")
				}
				if l.Excess != allocation.ExcessReducePrincipal {
					return fail("target requires verified immediate principal credit")
				}
				unit := l.Contract.Rounding.Unit
				if unit < 1 {
					unit = cur.SettlementUnit
				}
				if unit < 1 {
					unit = 1
				}
				if amount.Minor()%unit != 0 {
					return fail("target amount is not settlement aligned")
				}
				return nil
			}
		}
		return fail("unknown target loan ID")
	}
	for _, event := range in.Cash.Lumps {
		if event.ID != "" {
			if seen[event.ID] {
				return fail("duplicate event ID")
			}
			seen[event.ID] = true
		}
		r := event.Routing
		if r != nil && (event.Amount.Currency() != cur || event.Amount.Sign() <= 0) {
			return fail("positive event amount in funding currency required")
		}
		if event.FromOpening {
			if r == nil || event.Expected || !event.On.Equal(in.ValuationDate) {
				return fail("retained cash must be confirmed, routed and dated at valuation")
			}
			var err error
			opening, err = opening.Add(event.Amount)
			if err != nil {
				return err
			}
		}
		if r == nil {
			continue
		}
		if event.ID == "" {
			return fail("routed event requires a stable ID")
		}
		if event.Amount.Currency() != cur || event.Amount.Sign() <= 0 {
			return fail("positive event amount in funding currency required")
		}
		if !r.HoldUntil.IsZero() && (!budgetRuleDateValid(r.HoldUntil) || r.HoldUntil.Before(event.On)) {
			return fail("hold date precedes receipt or is invalid")
		}
		if r.HoldMinimum.Sign() < 0 || (r.HoldMinimum.Currency().Code != "" && r.HoldMinimum.Currency() != cur) || (r.HoldMinimum.Sign() > 0 && r.HoldMinimum.Currency() != cur) {
			return fail("invalid hold threshold")
		}
		if r.LoanID != "" && len(r.Splits) > 0 {
			return fail("loan earmark and split are mutually exclusive")
		}
		if r.LoanID == "" && len(r.Splits) == 0 && r.HoldUntil.IsZero() && r.HoldMinimum.Sign() == 0 {
			return fail("empty routing instruction")
		}
		if r.LoanID != "" {
			if err := target(r.LoanID, event.Amount); err != nil {
				return err
			}
		}
		sum := money.Zero(cur)
		ids := map[string]bool{}
		for _, part := range r.Splits {
			if part.LoanID == "" || ids[part.LoanID] || part.Amount.Currency() != cur || part.Amount.Sign() <= 0 {
				return fail("split needs unique targets and positive same-currency amounts")
			}
			ids[part.LoanID] = true
			if err := target(part.LoanID, part.Amount); err != nil {
				return err
			}
			var err error
			sum, err = sum.Add(part.Amount)
			if err != nil {
				return err
			}
		}
		if len(r.Splits) > 0 && sum.Cmp(event.Amount) != 0 {
			return fail("split amounts must exactly equal the event amount")
		}
	}
	available := in.Cash.OpeningCash
	if available.Currency().Code == "" {
		available = money.Zero(cur)
	}
	if opening.Cmp(available) > 0 {
		return fail("retained allocations exceed opening cash")
	}
	return nil
}

func (s *sim) restrictedCash() money.Amount {
	total := money.Zero(s.cur)
	for _, b := range s.cashBuckets {
		total = s.add(total, b.remaining)
	}
	return total
}

func (s *sim) addRouted(event CashEvent) {
	r := event.Routing
	parts := r.Splits
	if len(parts) == 0 {
		parts = []CashSplit{{LoanID: r.LoanID, Amount: event.Amount}}
	}
	for _, part := range parts {
		merged := false
		for i := range s.cashBuckets {
			b := &s.cashBuckets[i]
			if !b.released && b.target == part.LoanID && b.until == r.HoldUntil && b.minimum.Minor() == r.HoldMinimum.Minor() {
				b.remaining = s.add(b.remaining, part.Amount)
				merged = true
				break
			}
		}
		if !merged {
			s.cashBuckets = append(s.cashBuckets, cashBucket{target: part.LoanID, remaining: part.Amount, until: r.HoldUntil, minimum: r.HoldMinimum, wake: event.On})
		}
	}
	// Destination order never inherits input/map order or the search priority.
	sort.SliceStable(s.cashBuckets, func(i, j int) bool {
		a, b := s.cashBuckets[i], s.cashBuckets[j]
		if a.target != b.target {
			return a.target < b.target
		}
		if a.until != b.until {
			return a.until.Before(b.until)
		}
		if a.minimum.Minor() != b.minimum.Minor() {
			return a.minimum.Minor() < b.minimum.Minor()
		}
		return !a.released && b.released
	})
}

// routeCash spends only the isolated bucket. Temporarily making its balance
// visible to optionalCash reuses the existing reserve and permission guards;
// only the quoted outflow is transferred for prepay, so free cash is unchanged.
// dueLoan allows on-due routing immediately after that obligation was settled.
func (s *sim) routeCash(on date.Date, dueLoan *loanState) {
	if s.err != nil {
		return
	}
	for i := range s.cashBuckets {
		b := &s.cashBuckets[i]
		if !b.wake.IsZero() && !b.wake.After(on) {
			b.wake = date.Date{}
		}
		if b.remaining.Sign() <= 0 {
			continue
		}
		if on.Before(b.until) {
			b.wake = b.until
			continue
		}
		if !b.released {
			if b.remaining.Minor() < b.minimum.Minor() {
				continue
			}
			b.released = true
		}
		if b.target == "" {
			s.cash = s.add(s.cash, b.remaining)
			b.remaining = money.Zero(s.cur)
			continue
		}
		if s.pol.RequiredOnly {
			continue
		}
		var loan *loanState
		for _, l := range s.loans {
			if l.pos.ID == b.target {
				loan = l
				break
			}
		}
		if loan == nil || loan.closed {
			continue
		}
		early := loan != dueLoan && loan.timing == OnReceipt && on.Before(loan.due)
		if !early && loan != dueLoan && on.Before(loan.due) {
			continue
		}
		s.cash = s.add(s.cash, b.remaining)
		available := s.optionalCash(on)
		s.cash = s.sub(s.cash, b.remaining)
		if s.err != nil {
			return
		}
		if available.Cmp(b.remaining) > 0 {
			available = b.remaining
		}
		if available.Sign() <= 0 {
			continue
		}
		q, err := s.quote(loan, on, available)
		if err != nil {
			s.err = err
			return
		}
		if q.Outflow.Sign() <= 0 {
			continue
		}
		if s.in.Cash.Spending != nil && s.carryRule == BatchUntil && q.Principal.Cmp(s.carryMinimum) < 0 && !q.Closes {
			continue
		}
		if s.pol.MinPrepay.Sign() > 0 && q.Principal.Cmp(s.pol.MinPrepay) < 0 && !q.Closes {
			continue
		}
		b.remaining = s.sub(b.remaining, q.Outflow)
		s.cash = s.add(s.cash, q.Outflow)
		s.prepay(loan, on, q, early)
		if dueLoan == loan && !loan.closed {
			if err := s.refresh(loan); err != nil {
				s.err = err
				return
			}
		}
	}
}

// receiveRoutedDate aggregates ALL confirmed same-date receipts before deciding
// allocations, including ordinary cash on that date. Pure legacy runs retain
// their established event ordering.
func (s *sim) receiveRoutedDate(on date.Date) {
	if s.cycle == 0 {
		s.openCycle(on)
	}
	remaining := make([]CashEvent, 0, len(s.lumps))
	for _, event := range s.lumps {
		if event.On != on {
			remaining = append(remaining, event)
			continue
		}
		s.inflow = s.add(s.inflow, event.Amount)
		if event.Routing == nil {
			s.cash = s.add(s.cash, event.Amount)
		} else {
			s.addRouted(event)
		}
	}
	s.lumps = remaining
	s.allocate(on)
}

func (s *sim) checkRestrictedCash() error {
	for _, b := range s.cashBuckets {
		if b.remaining.Sign() < 0 {
			return fmt.Errorf("%w: negative routed balance", ErrInvariant)
		}
	}
	return nil
}
