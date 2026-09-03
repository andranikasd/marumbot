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
	if c.Spending.RuleError != "" {
		return &UnsupportedError{Feature: c.Spending.RuleError}
	}
	if c.Spending.CycleDay < 0 || c.Spending.CycleDay > 31 {
		return fmt.Errorf("plan: invalid spending cycle")
	}
	if err := validateCarry(c.Spending.CarryRule, c.Spending.CarryMinimum, c.Spending.CarryUntil, cur); err != nil {
		return err
	}
	for i, change := range c.Spending.Changes {
		if err := validateCarry(change.CarryRule, change.CarryMinimum, change.CarryUntil, cur); err != nil {
			return err
		}
		if !budgetRuleDateValid(change.On) || (i > 0 && !change.On.After(c.Spending.Changes[i-1].On)) {
			return fmt.Errorf("plan: spending changes must be strictly dated")
		}
		if change.Limit.Currency() != cur {
			return fmt.Errorf("plan: invalid spending change currency")
		}
		if err := check(change.Limit); err != nil {
			return err
		}
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

// PeriodStart returns the start of the monthly permission cycle containing on.
func (p SpendingPlan) PeriodStart(on date.Date) date.Date {
	day := p.CycleDay
	if day == 0 {
		day = 1
	}
	start := date.OnDayOfMonth(on, day)
	if start.After(on) {
		start = date.OnDayOfMonth(date.AddMonths(on, -1), day)
	}
	return start
}

func (p SpendingPlan) periodEnd(on date.Date) date.Date {
	day := p.CycleDay
	if day == 0 {
		day = 1
	}
	return date.OnDayOfMonth(date.AddMonths(p.PeriodStart(on), 1), day)
}

// period changes permission without crediting cash or restoring spent permission.
func (s *sim) period(on date.Date) {
	p := s.in.Cash.Spending
	start := p.PeriodStart(on)
	limit := s.budget
	rule, minimum, until := p.CarryRule, p.CarryMinimum, p.CarryUntil
	if v, ok := p.Overrides[MonthKey(start)]; ok {
		limit = v
	}
	for _, change := range p.Changes {
		if change.On.After(on) {
			break
		}
		limit = change.Limit
		if change.CarryRule != "" {
			rule, minimum, until = change.CarryRule, change.CarryMinimum, change.CarryUntil
		}
	}
	samePeriod := !s.periodStart.IsZero() && s.periodStart.Equal(start)
	if !samePeriod {
		s.periodStart = start
		s.periodSpent = money.Zero(s.cur)
		if start.Equal(p.PeriodStart(s.in.ValuationDate)) && p.Spent.Sign() > 0 {
			s.periodSpent = p.Spent
		}
	}
	s.periodLeft = s.sub(limit, s.periodSpent)
	if s.periodLeft.Sign() < 0 {
		s.err = &InfeasibleError{On: on, Required: s.periodSpent, Available: limit, Shortfall: s.sub(s.periodSpent, limit), Constraint: "spending_limit"}
		return
	}
	s.nextPeriod = p.periodEnd(on)
	for _, change := range p.Changes {
		if change.On.After(on) && change.On.Before(s.nextPeriod) {
			s.nextPeriod = change.On
			break
		}
	}
	// Reconsider reservations without returning actual spending to permission.
	for _, l := range s.loans {
		l.pending = money.Zero(s.cur)
	}
	if !samePeriod {
		if s.cycle > 0 && s.carryRule == NoCarry {
			// Retain the reserve and contractual amounts needed before the next
			// funding date. Expired optional reservations cannot trap household cash.
			transfer := s.sub(s.cash, s.reserved())
			if transfer.Sign() > 0 {
				s.cash = s.sub(s.cash, transfer)
				s.res.HouseholdCash = s.add(s.res.HouseholdCash, transfer)
				s.month.HouseholdCash = transfer
			}
		}
		if s.cycle > 0 {
			s.closeCycle()
		}
		s.openCycle(on)
	}
	s.carryRule, s.carryMinimum, s.carryUntil = rule, minimum, until
	if rule == CarryToDate && until.After(on) && until.Before(s.nextPeriod) {
		s.nextPeriod = until
	}
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
		if !l.due.Before(on) && l.due.Before(s.in.Cash.Spending.periodEnd(on)) {
			left = s.sub(left, l.required)
		}
		left = s.sub(left, l.pending)
	}
	return left
}

func (s *sim) optionalCash(on date.Date) money.Amount {
	if s.in.Cash.Spending != nil && s.carryRule == CarryToDate && on.Before(s.carryUntil) {
		return money.Zero(s.cur)
	}
	surplus := s.sub(s.cash, s.reserved())
	if s.in.Cash.Spending != nil {
		permission := s.optionalPermission(on)
		if permission.Cmp(surplus) < 0 {
			surplus = permission
		}
	}
	return surplus
}

func validateCarry(rule string, minimum money.Amount, until date.Date, cur money.Currency) error {
	switch rule {
	case "", CarryCash, NoCarry:
		if minimum.Sign() != 0 || !until.IsZero() {
			return fmt.Errorf("plan: unexpected carry parameters")
		}
	case BatchUntil:
		if minimum.Currency() != cur || minimum.Sign() <= 0 || !until.IsZero() {
			return fmt.Errorf("plan: invalid carry threshold")
		}
	case CarryToDate:
		if !budgetRuleDateValid(until) || minimum.Sign() != 0 {
			return fmt.Errorf("plan: invalid carry date")
		}
	default:
		return fmt.Errorf("plan: invalid carry rule")
	}
	return nil
}

// spendPermission records actual outflow, including fees, independently of
// reporting cycles. Only a new spending period may reset this ledger.
func (s *sim) spendPermission(outflow money.Amount) {
	s.periodSpent = s.add(s.periodSpent, outflow)
	s.periodLeft = s.sub(s.periodLeft, outflow)
}
