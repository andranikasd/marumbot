package app

import (
	"context"
	"fmt"
	"sort"

	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/money"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

// BudgetPolicy is an approved declaration. Version is assigned by persistence.
// It changes permission only: cash statements and spending facts are separate.
type BudgetPolicy struct {
	CarryMinimumMinor *int64 `json:"carry_minimum_minor,omitempty"`
	CarryUntil        string `json:"carry_until,omitempty"`
	RetainMinor       *int64 `json:"retain_minor,omitempty"`
	RetainPercentPPB  *int64 `json:"retain_percent_ppb,omitempty"`

	Version             int64                    `json:"version"`
	EffectiveFrom       string                   `json:"effective_from"`
	MonthlyMinor        int64                    `json:"monthly_minor"`
	CycleDay            int                      `json:"cycle_day"`
	CarryRule           string                   `json:"carry_rule"`
	ReleasedPaymentRule string                   `json:"released_payment_rule"`
	Growth              *BudgetPolicyGrowth      `json:"growth,omitempty"`
	Adjustments         []BudgetPolicyAdjustment `json:"adjustments"`
}

type BudgetPolicyGrowth struct {
	EveryMonths  int    `json:"every_months"`
	StartsOn     string `json:"starts_on"`
	EndsOn       string `json:"ends_on,omitempty"`
	FixedMinor   *int64 `json:"fixed_minor,omitempty"`
	PercentPPB   *int64 `json:"percent_ppb,omitempty"`
	MaximumMinor *int64 `json:"maximum_minor,omitempty"`
}

type BudgetPolicyAdjustment struct {
	Month            string `json:"month"`
	ReplacementMinor *int64 `json:"replacement_minor,omitempty"`
	DeltaMinor       *int64 `json:"delta_minor,omitempty"`
}

func (p BudgetPolicy) rules(cur money.Currency, noGrowth bool) (*plan.BudgetGrowth, []plan.BudgetAdjustment, error) {
	var growth *plan.BudgetGrowth
	if p.Growth != nil {
		g := p.Growth
		if (g.FixedMinor == nil) == (g.PercentPPB == nil) {
			return nil, nil, fmt.Errorf("budget: choose one growth operation")
		}
		start, err := date.Parse(g.StartsOn)
		if err != nil {
			return nil, nil, fmt.Errorf("budget: invalid growth start")
		}
		growth = &plan.BudgetGrowth{EveryMonths: g.EveryMonths, StartsOn: start}
		if g.EndsOn != "" {
			growth.EndsOn, err = date.Parse(g.EndsOn)
			if err != nil {
				return nil, nil, fmt.Errorf("budget: invalid growth end")
			}
		}
		if g.FixedMinor != nil {
			growth.Fixed = money.FromMinor(*g.FixedMinor, cur)
		}
		if g.PercentPPB != nil {
			growth.PercentPPB = *g.PercentPPB
		}
		if g.MaximumMinor != nil {
			growth.Maximum = money.FromMinor(*g.MaximumMinor, cur)
		}
	}
	if growth != nil && p.CycleDay > 1 {
		cycle := plan.SpendingPlan{CycleDay: p.CycleDay}
		if !cycle.PeriodStart(growth.StartsOn).Equal(growth.StartsOn) || (!growth.EndsOn.IsZero() && growth.EndsOn.Before(growth.StartsOn)) {
			return nil, nil, fmt.Errorf("budget: growth must start on a cycle boundary")
		}
		if !growth.EndsOn.IsZero() {
			growth.EndsOn = cycle.PeriodStart(growth.EndsOn).EndOfMonth()
		}
		growth.StartsOn = date.OnDayOfMonth(growth.StartsOn, 1)
	}
	adjustments := make([]plan.BudgetAdjustment, 0, len(p.Adjustments))
	for _, a := range p.Adjustments {
		item := plan.BudgetAdjustment{Month: a.Month}
		if a.ReplacementMinor != nil {
			n := money.FromMinor(*a.ReplacementMinor, cur)
			item.Replacement = &n
		}
		if a.DeltaMinor != nil {
			n := money.FromMinor(*a.DeltaMinor, cur)
			item.Delta = &n
		}
		adjustments = append(adjustments, item)
	}
	if noGrowth {
		growth = nil
	}
	return growth, adjustments, nil
}

// Validate refuses unimplemented carry/release behavior rather than storing a
// selection that would silently run as a different rule.
func (p BudgetPolicy) Validate(currency string) error {
	cur, err := money.Lookup(currency)
	if err != nil {
		return err
	}
	effective, err := date.Parse(p.EffectiveFrom)
	if err != nil || effective.Year() < 1 || effective.Year() > 9949 || p.MonthlyMinor < 0 || p.CycleDay < 0 || p.CycleDay > 31 || len(p.Adjustments) > 36 {
		return fmt.Errorf("budget: invalid policy")
	}
	switch p.CarryRule {
	case plan.CarryCash, plan.NoCarry:
		if p.CarryMinimumMinor != nil || p.CarryUntil != "" {
			return fmt.Errorf("budget: unexpected carry parameters")
		}
	case plan.BatchUntil:
		if p.CarryMinimumMinor == nil || *p.CarryMinimumMinor <= 0 || p.CarryUntil != "" {
			return fmt.Errorf("budget: carry threshold required")
		}
	case plan.CarryToDate:
		until, err := date.Parse(p.CarryUntil)
		if err != nil || until.Before(effective) || p.CarryMinimumMinor != nil {
			return fmt.Errorf("budget: carry date required")
		}
	default:
		return fmt.Errorf("budget: invalid carry rule")
	}
	switch p.ReleasedPaymentRule {
	case plan.RollAll, plan.ReleaseAll:
		if p.RetainMinor != nil || p.RetainPercentPPB != nil {
			return fmt.Errorf("budget: unexpected retained payment")
		}
	case plan.RollAmount:
		if p.RetainMinor == nil || *p.RetainMinor < 0 || p.RetainPercentPPB != nil {
			return fmt.Errorf("budget: retained amount required")
		}
	case plan.RollPercent:
		if p.RetainPercentPPB == nil || *p.RetainPercentPPB < 0 || *p.RetainPercentPPB > 1_000_000_000 || p.RetainMinor != nil {
			return fmt.Errorf("budget: retained percentage required")
		}
	default:
		return &plan.UnsupportedError{Feature: "released-payment goal requires a defined confirmed target"}
	}
	g, a, err := p.rules(cur, false)
	if err != nil {
		return err
	}
	if g != nil && p.Growth.StartsOn < effective.String() {
		return fmt.Errorf("budget: growth precedes policy")
	}
	_, err = plan.ExpandBudgetRules(money.FromMinor(p.MonthlyMinor, cur), effective, plan.DefaultHorizon, g, a)
	return err
}

// CashPlans returns matched growth and no-growth inputs. Neither version
// invents funding. Existing CashPlan callers get the growth input, and invalid
// declarations become a typed planner refusal through Spending.RuleError.
func (b Budget) CashPlans(valuation date.Date) (plan.CashPlan, plan.CashPlan, error) {
	legacy := b
	legacy.Policies = nil
	base := legacy.CashPlan(valuation)
	if len(b.Policies) == 0 {
		return base, legacy.CashPlan(valuation), nil
	}
	if b.Funding == nil {
		return base, base, &plan.UnsupportedError{Feature: "budget policies require explicit funding"}
	}
	// A user cycle can span two calendar months. Preserve a statement only
	// within that declared cycle; never manufacture a newer cash balance.
	restoreCash := func(cp *plan.CashPlan) {
		if !b.OpeningAsOf.IsZero() && !b.OpeningAsOf.After(valuation) && BudgetPeriodStart(b.Policies, b.OpeningAsOf).Equal(BudgetPeriodStart(b.Policies, valuation)) {
			cp.OpeningCash = b.Opening
			if through, err := date.Parse(b.Funding.CashThrough); err == nil {
				cp.CashThrough = through
			}
		}
	}
	restoreCash(&base)
	var err error
	base.Spending, err = b.policySpending(valuation, false, base.Spending.Spent)
	if err != nil {
		return base, base, err
	}
	fallback := legacy.CashPlan(valuation)
	restoreCash(&fallback)
	fallback.Spending, err = b.policySpending(valuation, true, fallback.Spending.Spent)
	return base, fallback, err
}

func (b Budget) policySpending(valuation date.Date, noGrowth bool, spent money.Amount) (*plan.SpendingPlan, error) {
	policies := append([]BudgetPolicy(nil), b.Policies...)
	sort.SliceStable(policies, func(i, j int) bool {
		if policies[i].EffectiveFrom == policies[j].EffectiveFrom {
			return policies[i].Version < policies[j].Version
		}
		return policies[i].EffectiveFrom < policies[j].EffectiveFrom
	})
	p := &plan.SpendingPlan{Monthly: b.Monthly, Spent: spent, CarryRule: plan.CarryCash, ConfirmedReleaseOnly: true}
	sources := map[string]bool{}
	for _, fact := range b.Releases {
		if fact.On <= valuation.String() && !sources[fact.SourceID] {
			sources[fact.SourceID] = true
			p.ReleaseSources = append(p.ReleaseSources, fact.SourceID)
		}
	}
	sort.Strings(p.ReleaseSources)
	for _, fact := range b.Releases {
		if fact.On > valuation.String() {
			continue
		}
		on, err := date.Parse(fact.On)
		if err != nil {
			return nil, err
		}
		source := plan.BudgetReleaseSource{PolicyVersion: fact.PolicyVersion, SourceID: fact.SourceID, PriorSourceID: fact.PriorSourceID, On: on}
		if fact.BeforeMinor != nil {
			n := money.FromMinor(*fact.BeforeMinor, b.Monthly.Currency())
			source.Before = &n
		}
		if fact.AfterMinor != nil {
			n := money.FromMinor(*fact.AfterMinor, b.Monthly.Currency())
			source.After = &n
		}
		p.ReleaseFacts = append(p.ReleaseFacts, source)
	}
	// The opening statement's spending belongs to its cycle, independently of
	// whether current cash was also stated that calendar month.
	for _, policy := range policies {
		if err := policy.Validate(b.Currency); err != nil {
			return nil, err
		}
		if policy.CycleDay != policies[0].CycleDay && (policy.CycleDay > 1 || policies[0].CycleDay > 1) {
			return nil, &plan.UnsupportedError{Feature: "changing an established budget cycle"}
		}
	}
	p.CycleDay = policies[0].CycleDay
	if p.CycleDay > 1 && policies[0].EffectiveFrom > valuation.String() {
		return nil, &plan.UnsupportedError{Feature: "future transition from calendar to user cycle"}
	}
	p.Spent = money.Zero(b.Monthly.Currency())
	if !b.OpeningAsOf.IsZero() && !b.OpeningAsOf.After(valuation) && p.PeriodStart(b.OpeningAsOf).Equal(p.PeriodStart(valuation)) {
		if p.CycleDay > 1 && b.Funding.SpentMinor > 0 && b.Funding.SpentPeriodStart != p.PeriodStart(b.OpeningAsOf).String() {
			return nil, &plan.UnsupportedError{Feature: "user-cycle spending statement required"}
		}
		p.Spent = money.FromMinor(b.Funding.SpentMinor, b.Monthly.Currency())
	}
	end := date.AddMonths(valuation, plan.DefaultHorizon-1).EndOfMonth()
	// Build the event calendar from cycle boundaries and exact activation dates.
	days := map[string]date.Date{valuation.String(): valuation}
	day := p.CycleDay
	if day == 0 {
		day = 1
	}
	for d := date.OnDayOfMonth(date.AddMonths(p.PeriodStart(valuation), 1), day); !d.After(end); d = date.OnDayOfMonth(date.AddMonths(d, 1), day) {
		days[d.String()] = d
	}
	for _, policy := range policies {
		d, _ := date.Parse(policy.EffectiveFrom)
		if d.After(valuation) && !d.After(end) {
			days[d.String()] = d
		}
	}
	ordered := make([]date.Date, 0, len(days))
	for _, d := range days {
		ordered = append(ordered, d)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Before(ordered[j]) })
	for _, d := range ordered {
		var active *BudgetPolicy
		for i := range policies {
			if policies[i].EffectiveFrom <= d.String() {
				active = &policies[i]
			}
		}
		limit := b.Monthly
		if active != nil {
			g, a, err := active.rules(b.Monthly.Currency(), noGrowth)
			if err != nil {
				return nil, err
			}
			limits, err := plan.ExpandBudgetRules(money.FromMinor(active.MonthlyMinor, b.Monthly.Currency()), p.PeriodStart(d), 1, g, nil)
			if err != nil {
				return nil, err
			}
			limit = limits[plan.MonthKey(p.PeriodStart(d))]
			limit, err = b.releasedLimit(*active, limit, d, valuation)
			if err != nil {
				return nil, err
			}
			adjusted, err := plan.ExpandBudgetRules(limit, p.PeriodStart(d), 1, nil, a)
			if err != nil {
				return nil, err
			}
			limit = adjusted[plan.MonthKey(p.PeriodStart(d))]
		} else if minor, ok := b.Overrides[plan.MonthKey(p.PeriodStart(d))]; ok {
			limit = money.FromMinor(minor, b.Monthly.Currency())
		}
		change := plan.SpendingChange{On: d, Limit: limit, CarryRule: plan.CarryCash}
		if active != nil {
			change.CarryRule = active.CarryRule
			if active.CarryMinimumMinor != nil {
				change.CarryMinimum = money.FromMinor(*active.CarryMinimumMinor, b.Monthly.Currency())
			}
			if active.CarryUntil != "" {
				change.CarryUntil, _ = date.Parse(active.CarryUntil)
			}
		}
		p.Changes = append(p.Changes, change)
	}
	return p, nil
}

// BudgetPolicyStore is the consumer-owned atomic declaration port.
type BudgetPolicyStore interface {
	BudgetStore
	AppendBudgetPolicy(ctx context.Context, userID, currency string, expectedVersion int64, policy BudgetPolicy) (int64, error)
}

// SaveBudgetPolicy validates a prospective timeline before the atomic append.
// A failed compare-and-swap requires a reload; no source facts are reset.
func SaveBudgetPolicy(ctx context.Context, store BudgetPolicyStore, userID, currency string, expectedVersion int64, today date.Date, policy BudgetPolicy) (int64, error) {
	if err := policy.Validate(currency); err != nil {
		return 0, err
	}
	on, _ := date.Parse(policy.EffectiveFrom)
	if on.Before(today) {
		return 0, fmt.Errorf("budget: cannot backdate permission")
	}
	b, err := store.Budget(ctx, userID)
	if err != nil {
		return 0, err
	}
	if !b.Set || b.Currency != currency || b.Version != expectedVersion {
		return 0, ErrConflict
	}
	// Existing calendar spending cannot be relabelled as user-cycle spending.
	if len(b.Policies) == 0 && policy.CycleDay > 1 && (b.Funding == nil || b.Funding.SpentMinor != 0 || !on.Equal(today) || !on.Equal((plan.SpendingPlan{CycleDay: policy.CycleDay}).PeriodStart(on))) {
		return 0, &plan.UnsupportedError{Feature: "user cycle requires an unspent cycle boundary"}
	}
	policy.Version = expectedVersion + 1
	b.Policies = append(append([]BudgetPolicy(nil), b.Policies...), policy)
	if _, _, err = b.CashPlans(today); err != nil {
		return 0, err
	}
	return store.AppendBudgetPolicy(ctx, userID, currency, expectedVersion, policy)
}

// PermissionOn returns the normalized permission in force on a business date.
// It is the approved limit, not remaining cash or remaining permission.
func (b Budget) PermissionOn(on date.Date) (money.Amount, error) {
	if len(b.Policies) == 0 {
		if n, ok := b.Overrides[plan.MonthKey(on)]; ok {
			return money.FromMinor(n, b.Monthly.Currency()), nil
		}
		return b.Monthly, nil
	}
	cp, _, err := b.CashPlans(on)
	if err != nil {
		return money.Amount{}, err
	}
	return cp.Spending.Changes[0].Limit, nil
}

// BudgetPeriodStart selects the policy effective on the statement date.
func BudgetPeriodStart(policies []BudgetPolicy, on date.Date) date.Date {
	var active *BudgetPolicy
	for i := range policies {
		p := &policies[i]
		if p.EffectiveFrom <= on.String() && (active == nil || p.EffectiveFrom > active.EffectiveFrom || (p.EffectiveFrom == active.EffectiveFrom && p.Version > active.Version)) {
			active = p
		}
	}
	day := 1
	if active != nil {
		day = active.CycleDay
	}
	return (plan.SpendingPlan{CycleDay: day}).PeriodStart(on)
}

// BudgetReleaseFact is a pair of verified source statements, selected by the
// store only when captured after the policy was declared. No derived balance
// or release amount is persisted.
type BudgetReleaseFact struct {
	PolicyVersion int64
	SourceID      string
	PriorSourceID string
	On            string
	BeforeMinor   *int64
	AfterMinor    *int64
}

func (b Budget) releasedLimit(policy BudgetPolicy, limit money.Amount, on, valuation date.Date) (money.Amount, error) {
	if policy.ReleasedPaymentRule == plan.RollAll {
		return limit, nil
	}
	cur := b.Monthly.Currency()
	retained := money.Zero(cur)
	if policy.RetainMinor != nil {
		retained = money.FromMinor(*policy.RetainMinor, cur)
	}
	percent := int64(0)
	if policy.RetainPercentPPB != nil {
		percent = *policy.RetainPercentPPB
	}
	seen := map[string]bool{}
	for _, fact := range b.Releases {
		if fact.PolicyVersion != policy.Version || seen[fact.SourceID] {
			continue
		}
		seen[fact.SourceID] = true
		confirmed, err := date.Parse(fact.On)
		if err != nil {
			return money.Amount{}, err
		}
		if confirmed.After(valuation) {
			continue
		}
		// A confirmed change affects the next period, never the already-spent one.
		next := date.OnDayOfMonth(date.AddMonths((plan.SpendingPlan{CycleDay: policy.CycleDay}).PeriodStart(confirmed), 1), max(1, policy.CycleDay))
		if next.After(on) {
			continue
		}
		if fact.BeforeMinor == nil || fact.AfterMinor == nil {
			return money.Amount{}, &plan.UnsupportedError{Feature: "confirmed prior and new instalments required for release"}
		}
		reduction, err := plan.ReleasedReduction(money.FromMinor(*fact.BeforeMinor, cur), money.FromMinor(*fact.AfterMinor, cur), policy.ReleasedPaymentRule, retained, percent, true)
		if err != nil {
			return money.Amount{}, err
		}
		limit, err = limit.Sub(reduction)
		if err != nil {
			return money.Amount{}, err
		}
		if limit.Sign() < 0 {
			limit = money.Zero(cur)
		}
	}
	return limit, nil
}

// PolicyForMonthlyChange prepares an explicit recurring-total replacement.
// The active declaration supplies carry/release rules and future adjustments;
// existing future-effective declarations stay in Budget.Policies untouched.
func (b Budget) PolicyForMonthlyChange(on date.Date, minor int64) (BudgetPolicy, error) {
	p := BudgetPolicy{EffectiveFrom: on.String(), MonthlyMinor: minor, CarryRule: plan.CarryCash, ReleasedPaymentRule: plan.RollAll}
	var active *BudgetPolicy
	for i := range b.Policies {
		candidate := &b.Policies[i]
		if candidate.EffectiveFrom <= on.String() && (active == nil || candidate.EffectiveFrom > active.EffectiveFrom || (candidate.EffectiveFrom == active.EffectiveFrom && candidate.Version > active.Version)) {
			active = candidate
		}
	}
	if active != nil {
		if err := active.Validate(b.Currency); err != nil {
			return BudgetPolicy{}, err
		}
		p = *active
	}
	p.Version = 0
	p.EffectiveFrom = on.String()
	p.MonthlyMinor = minor
	period := (plan.SpendingPlan{CycleDay: p.CycleDay}).PeriodStart(on)
	p.Adjustments = nil
	if active != nil {
		for _, a := range active.Adjustments {
			if a.Month > plan.MonthKey(period) {
				p.Adjustments = append(p.Adjustments, a)
			}
		}
	}
	if active == nil {
		keys := make([]string, 0, len(b.Overrides))
		for month := range b.Overrides {
			if month > plan.MonthKey(period) {
				keys = append(keys, month)
			}
		}
		sort.Strings(keys)
		for _, month := range keys {
			minor := b.Overrides[month]
			p.Adjustments = append(p.Adjustments, BudgetPolicyAdjustment{Month: month, ReplacementMinor: &minor})
		}
	}
	if p.Growth != nil {
		g := *p.Growth
		next, err := date.Parse(g.StartsOn)
		if err != nil || g.EveryMonths < 1 {
			return BudgetPolicy{}, fmt.Errorf("budget: invalid growth recurrence")
		}
		origin := next
		for !next.After(on) {
			remaining := (9949-next.Year())*12 + 12 - int(next.Month())
			if g.EveryMonths > remaining {
				return BudgetPolicy{}, fmt.Errorf("budget: next growth exceeds supported dates")
			}
			months := (next.Year()-origin.Year())*12 + int(next.Month()-origin.Month())
			next = date.OnDayOfMonth(date.AddMonths(origin, months+g.EveryMonths), max(1, p.CycleDay))
		}
		if g.EndsOn != "" && g.EndsOn < next.String() {
			p.Growth = nil
		} else {
			g.StartsOn = next.String()
			p.Growth = &g
		}
	}
	if p.CarryRule == plan.CarryToDate && p.CarryUntil < on.String() {
		p.CarryUntil = on.String()
	}
	return p, p.Validate(b.Currency)
}

// WithReleaseFacts restores the original verified facts for a saved scenario.
// It does not re-query current statements or reinterpret them as new approvals.
func (b Budget) WithReleaseFacts(spending *plan.SpendingPlan) Budget {
	b.Releases = nil
	if spending == nil {
		return b
	}
	for _, source := range spending.ReleaseFacts {
		fact := BudgetReleaseFact{PolicyVersion: source.PolicyVersion, SourceID: source.SourceID, PriorSourceID: source.PriorSourceID, On: source.On.String()}
		if source.Before != nil {
			n := source.Before.Minor()
			fact.BeforeMinor = &n
		}
		if source.After != nil {
			n := source.After.Minor()
			fact.AfterMinor = &n
		}
		b.Releases = append(b.Releases, fact)
	}
	return b
}
