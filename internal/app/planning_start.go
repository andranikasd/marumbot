package app

import (
	"context"
	"sort"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/money"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

type PlanningStartUpdate struct {
	Key             string `json:"idempotency_key"`
	ExpectedVersion int64  `json:"expected_version"`
	Month           string `json:"planning_start_month"`
}

type planningStartWriter interface {
	SetPlanningStart(context.Context, string, string, int64, string) (int64, error)
}

func (s BudgetCommands) SetPlanningStart(ctx context.Context, user string, in PlanningStartUpdate) (int64, error) {
	today, err := (PaymentService{Clock: s.Clock, Users: s.Users}).BusinessDate(ctx, user)
	if err != nil {
		return 0, err
	}
	return s.execute(ctx, user, in.Key, "planning_start", in, func(tx BudgetCommandTransaction) (int64, error) {
		start := today
		if in.Month != "" {
			var err error
			start, err = date.Parse(in.Month + "-01")
			if err != nil || plan.MonthKey(start) != in.Month || in.Month < plan.MonthKey(today) || in.Month > plan.MonthKey(date.AddMonths(today, 1)) {
				return 0, ErrPaymentInvalid
			}
		}
		b, err := tx.Budget(ctx, user)
		if err != nil {
			return 0, err
		}
		if !b.Set || b.Version != in.ExpectedVersion {
			return 0, ErrConflict
		}
		if b.Funding == nil {
			return 0, ErrFundingRequired
		}
		if start.After(today) {
			loans, err := tx.LoansForUser(ctx, user, plan.MaxLoans+1)
			if err != nil {
				return 0, err
			}
			if len(loans) > plan.MaxLoans {
				return 0, ErrPaymentInvalid
			}
			for _, loan := range loans {
				if loan.Balance.Sign() == 0 {
					continue
				}
				due, err := loan.NextInstalment()
				if err != nil {
					return 0, err
				}
				if !due.Due.IsZero() && due.Due.Before(start) {
					return 0, &plan.UnsupportedError{Feature: "required payments remain before planning start"}
				}
			}
		}
		writer, ok := tx.(planningStartWriter)
		if !ok {
			return 0, ErrPaymentInvalid
		}
		return writer.SetPlanningStart(ctx, user, b.Currency, in.ExpectedVersion, in.Month)
	})
}

// applyPlanningStart expresses the pause using existing dated permissions, so
// old manifests retain their encoding and neither cash nor actual spending is
// fabricated. Cycle boundaries remain intact even when start is mid-cycle.
func (b Budget) applyPlanningStart(cp plan.CashPlan, valuation date.Date) plan.CashPlan {
	if b.Funding == nil || b.Funding.PlanningStartMonth == "" || cp.Spending == nil {
		return cp
	}
	start, err := date.Parse(b.Funding.PlanningStartMonth + "-01")
	if err != nil {
		cp.Spending.RuleError = "invalid planning start month"
		return cp
	}
	if !start.After(valuation) {
		return cp
	}
	original := *cp.Spending
	p := original
	p.Changes = nil
	end := date.AddMonths(valuation, plan.DefaultHorizon).EndOfMonth()
	days := map[string]date.Date{valuation.String(): valuation, start.String(): start}
	day := p.CycleDay
	if day == 0 {
		day = 1
	}
	for d := date.OnDayOfMonth(date.AddMonths(p.PeriodStart(valuation), 1), day); !d.After(end); d = date.OnDayOfMonth(date.AddMonths(d, 1), day) {
		days[d.String()] = d
	}
	for _, c := range original.Changes {
		if !c.On.Before(valuation) {
			days[c.On.String()] = c.On
		}
	}
	ordered := make([]date.Date, 0, len(days))
	for _, d := range days {
		ordered = append(ordered, d)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Before(ordered[j]) })
	for _, on := range ordered {
		c := plan.SpendingChange{On: on, Limit: original.Monthly, CarryRule: original.CarryRule, CarryMinimum: original.CarryMinimum, CarryUntil: original.CarryUntil}
		if limit, ok := original.Overrides[plan.MonthKey(original.PeriodStart(on))]; ok {
			c.Limit = limit
		}
		for _, old := range original.Changes {
			if old.On.After(on) {
				break
			}
			c.Limit = old.Limit
			if old.CarryRule != "" {
				c.CarryRule = old.CarryRule
				c.CarryMinimum = old.CarryMinimum
				c.CarryUntil = old.CarryUntil
			}
		}
		// Preserve an existing over-spending refusal; pausing must not
		// manufacture a larger approved permission to conceal it.
		if on.Equal(valuation) && c.Limit.Minor() < original.Spent.Minor() {
			return cp
		}
		if on.Before(start) {
			c.Limit = money.Zero(original.Monthly.Currency())
			if original.PeriodStart(on).Equal(original.PeriodStart(valuation)) {
				c.Limit = money.FromMinor(original.Spent.Minor(), original.Monthly.Currency())
			}
		}
		p.Changes = append(p.Changes, c)
	}
	cp.Spending = &p
	return cp
}
