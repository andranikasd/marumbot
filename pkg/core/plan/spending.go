package plan

import (
	"fmt"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

func (c CashPlan) validateFunding(valuation date.Date) error {
	if c.CashThrough.After(valuation) {
		return fmt.Errorf("plan: cash statement is in the future")
	}
	cur := c.Monthly.Currency()
	check := func(a money.Amount) error {
		if a.Currency().Code == "" && a.Sign() == 0 {
			return nil
		}
		if a.Currency().Code != cur.Code {
			return &MixedCurrencyError{Have: a.Currency().Code, Want: cur.Code}
		}
		if a.Sign() < 0 {
			return fmt.Errorf("plan: funding and spending amounts must be non-negative")
		}
		return nil
	}
	for _, a := range []money.Amount{c.Monthly, c.OpeningCash, c.ReserveFloor} {
		if err := check(a); err != nil {
			return err
		}
	}
	for _, l := range c.Lumps {
		if l.On.IsZero() || l.On.Before(valuation) {
			return fmt.Errorf("plan: cash event precedes valuation")
		}
		if err := check(l.Amount); err != nil {
			return err
		}
	}
	if c.Spending == nil {
		return nil
	}
	if c.PayDay == 0 {
		return &UnsupportedError{Feature: "explicit funding requires a known payday"}
	}
	for _, a := range []money.Amount{c.Spending.Monthly, c.Spending.Spent} {
		if err := check(a); err != nil {
			return err
		}
	}
	for key, a := range c.Spending.Overrides {
		d, err := date.Parse(key + "-01")
		if err != nil || MonthKey(d) != key {
			return fmt.Errorf("plan: invalid spending month %q", key)
		}
		if err := check(a); err != nil {
			return err
		}
	}
	return nil
}

// period opens spending permission without crediting any cash. Unused cash
// carries; unused permission expires. Funding events do not reset permission.
func (s *sim) period(on date.Date) {
	p := s.in.Cash.Spending
	limit := s.budget
	if v, ok := p.Overrides[MonthKey(on)]; ok {
		limit = v
	}
	s.periodLeft = limit
	if MonthKey(on) == MonthKey(s.in.ValuationDate) && p.Spent.Sign() > 0 {
		s.periodLeft = s.sub(s.periodLeft, p.Spent)
		if s.periodLeft.Sign() < 0 {
			s.err = &InfeasibleError{On: on, Required: p.Spent, Available: limit, Shortfall: s.sub(p.Spent, limit), Constraint: "spending_limit"}
			return
		}
	}
	s.nextPeriod = date.OnDayOfMonth(date.AddMonths(on, 1), 1)
	// A reservation was a policy intention, not spending. Reconsider it under
	// the new month's permission and current contractual obligations.
	for _, l := range s.loans {
		l.pending = money.Zero(s.cur)
	}
	if s.cycle > 0 {
		s.closeCycle()
	}
	s.openCycle(on)
	s.allocate(on)
}

// optionalPermission protects every remaining instalment in the current month
// and any optional payment already reserved. It never changes cash itself.
func (s *sim) optionalPermission(on date.Date) money.Amount {
	left := s.periodLeft
	for _, l := range s.loans {
		if l.closed {
			continue
		}
		if MonthKey(l.due) == MonthKey(on) {
			left = s.sub(left, l.required)
		}
		left = s.sub(left, l.pending)
	}
	return left
}

func (s *sim) optionalCash(on date.Date) money.Amount {
	surplus := s.sub(s.cash, s.reserved())
	if s.in.Cash.Spending != nil {
		permission := s.optionalPermission(on)
		if permission.Cmp(surplus) < 0 {
			surplus = permission
		}
	}
	return surplus
}
