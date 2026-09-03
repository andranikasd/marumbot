package plan

import (
	"fmt"
	"math"

	"github.com/andranikasd/marumbot/pkg/core/allocation"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// ExhaustiveDynamicDomain proves only the printed reduced domain and horizon.
// It is deliberately distinct from unrestricted ProvenOptimal.
const ExhaustiveDynamicDomain Strength = "exhaustive_dynamic_domain"

// DynamicRequest opts into a reduced, exact-quantum dynamic search. Only up to
// three synchronized, fixed-instalment, fee-free, immediate-credit annuities
// with fresh anchors and legacy constant cash are supported. Horizon is 1..24;
// MaxStates caps ALL expansion work (including split enumeration), 1..100000.
// Zero values use 12 months and 10000 expansions. Quantum must equal every
// contract's settlement unit; zero selects that unit. No clock or I/O is used.
type DynamicRequest struct {
	Input              Input
	Horizon, MaxStates int
	Quantum            int64
}

// DynamicStep records a complete due-date action, in input loan order. Required
// obligations are paid first; Extras may split or be zero (cash is held).
type DynamicStep struct {
	On               date.Date
	Required, Extras []money.Amount
	Interest         money.Amount
	Cash             money.Amount
}

// DynamicReport returns an incumbent only when a complete payoff was found.
// No incumbent is not proof of infeasibility when Complete is false. Complete
// means the finite action tree was exhausted, not that arbitrary future dates,
// request delays, effects, fees or spending permissions were explored.
type DynamicReport struct {
	SharedInputHash string
	Steps           []DynamicStep
	Cost            *money.Amount
	Payoff          date.Date
	Complete        bool
	Certificate     Certificate
}

type dynamicKey struct {
	Event    int
	Cash     int64
	Balances [3]int64
}
type dynamicTail struct {
	found bool
	cost  int64
	steps []DynamicStep
}
type dynamicSearch struct {
	in                   Input
	horizon, limit, work int
	states               int
	quantum              int64
	truncated            bool
	memo                 map[dynamicKey]dynamicTail
}

// SearchDynamic minimizes total interest among plans that settle by the finite
// horizon, breaking equal cost by earliest payoff. Full state for this domain
// is event index, carried cash and all balances; immutable fixed schedules,
// terms and funding are scoped to this search. No uncertain dominance pruning
// is used. Unsupported full-simulator features fail before exploration.
func SearchDynamic(req DynamicRequest) (DynamicReport, error) {
	var out DynamicReport
	if err := req.Input.Validate(); err != nil {
		return out, err
	}
	if err := validateStrategyIDs(req.Input); err != nil {
		return out, err
	}
	if req.Horizon == 0 {
		req.Horizon = req.Input.Horizon
		if req.Horizon == 0 {
			req.Horizon = 12
		}
	}
	if req.Input.Horizon > 0 && req.Horizon > req.Input.Horizon {
		return out, &UnsupportedError{Feature: "dynamic horizon exceeds input horizon"}
	}
	if req.MaxStates == 0 {
		req.MaxStates = 10000
	}
	if req.Horizon < 1 || req.Horizon > 24 || req.MaxStates < 1 || req.MaxStates > 100000 {
		return out, &UnsupportedError{Feature: "dynamic limits require horizon 1..24 and expansions 1..100000"}
	}
	if err := dynamicDomain(req.Input); err != nil {
		return out, err
	}
	for _, l := range req.Input.Loans {
		if !l.Contract.MaturityDate.After(date.Occurrence(req.Input.ValuationDate, l.Contract.PaymentDay, req.Horizon)) {
			return out, &UnsupportedError{Feature: "dynamic horizon must precede contractual maturity"}
		}
	}
	unit := req.Input.Loans[0].Contract.Rounding.Unit
	if req.Quantum == 0 {
		req.Quantum = unit
	}
	if req.Quantum != unit {
		return out, &UnsupportedError{Feature: "dynamic quantum must equal contract settlement unit"}
	}
	s := dynamicSearch{in: req.Input, horizon: req.Horizon, limit: req.MaxStates, quantum: req.Quantum, memo: map[dynamicKey]dynamicTail{}}
	key := dynamicKey{Cash: req.Input.Cash.OpeningCash.Minor()}
	for i, l := range req.Input.Loans {
		key.Balances[i] = l.Balance.Minor()
	}
	tail, err := s.solve(key)
	if err != nil {
		return out, err
	}
	out.SharedInputHash = inputHash(req.Input)
	out.Complete = !s.truncated
	c := Certificate{
		Strength: BoundedHeuristic, DynamicStates: s.states, DynamicExpansions: s.work, Quantum: s.quantum, EngineVersion: EngineVersion,
		Eligibility: fmt.Sprintf("at most three fixed-instalment fee-free annuities; synchronized due-date funding; split/hold at settlement quantum; least interest among payoffs within %d months; no other action dates or effects", s.horizon),
	}
	if s.truncated {
		c.Truncation = fmt.Sprintf("dynamic expansion cap %d reached; lower bound and gap unknown", s.limit)
	} else {
		c.Strength = ExhaustiveDynamicDomain
	}
	for _, l := range req.Input.Loans {
		c.Fingerprints = append(c.Fingerprints, fingerprint(l.Contract))
		c.Positions = append(c.Positions, CertifiedPosition{ID: l.ID, Trust: l.Trust})
	}
	if tail.found {
		amount := money.FromMinor(tail.cost, req.Input.Cash.Monthly.Currency())
		out.Cost = &amount
		out.Steps = tail.steps
		c.BestCost = amount
		if len(tail.steps) > 0 {
			out.Payoff = tail.steps[len(tail.steps)-1].On
		}
		if out.Complete {
			lb := amount
			gap := money.Zero(amount.Currency())
			c.LowerBound = &lb
			c.Gap = &gap
		}
	}
	for event := 1; event <= s.horizon; event++ {
		c.CandidateDates = append(c.CandidateDates, date.Occurrence(req.Input.ValuationDate, req.Input.Loans[0].Contract.PaymentDay, event))
	}
	out.Certificate = c
	return out, nil
}

func dynamicDomain(in Input) error {
	refuse := func(reason string) error { return &UnsupportedError{Feature: "dynamic domain: " + reason} }
	if len(in.Loans) > 3 {
		return refuse("at most three loans")
	}
	cash := in.Cash
	if cash.Spending != nil || len(cash.MonthlyOverrides) > 0 || len(cash.Lumps) > 0 || cash.ReserveFloor.Sign() != 0 || !cash.CashThrough.IsZero() {
		return refuse("constant legacy funding without reserves, events or spending permissions required")
	}
	first := in.Loans[0].Contract
	if first.PaymentDay < 1 || first.PaymentDay > 28 || in.ValuationDate.Day() != first.PaymentDay {
		return refuse("valuation and common payment day must match, in 1..28")
	}
	if cash.PayDay != 0 {
		return refuse("unknown payday (funding on first due) required; explicit payday may fund valuation day")
	}
	if first.Rounding.Unit <= 0 {
		return refuse("explicit positive settlement unit required")
	}
	if cash.Monthly.Minor()%first.Rounding.Unit != 0 || cash.OpeningCash.Minor()%first.Rounding.Unit != 0 {
		return refuse("cash must be quantum aligned")
	}
	for _, l := range in.Loans {
		c := l.Contract
		if !l.From.Equal(in.ValuationDate) || l.Balance.Sign() <= 0 || l.OptionalExcluded {
			return refuse("positive live loans with fresh anchors, no exclusions")
		}
		if c.Type != model.Annuity || c.Prepayment.Effect != model.PrepayShortenTerm || !c.HasScheduled || c.ScheduledPayment.Sign() <= 0 {
			return refuse("supplied fixed instalments and shorten-term effects required")
		}
		if c.ScheduledPayment.Currency() != cash.Monthly.Currency() {
			return refuse("instalment currency differs from funding")
		}
		if c.PaymentDay != first.PaymentDay || !c.NotBeforeDue.IsZero() || !c.EffectiveThru.IsZero() {
			return refuse("synchronized dates without deferred dues or rate windows required")
		}
		if c.DayCount != money.Actual365 && c.DayCount != money.Actual360 {
			return refuse("actual/365 or actual/360 required")
		}
		if c.Rounding != first.Rounding || l.Balance.Minor()%first.Rounding.Unit != 0 || c.ScheduledPayment.Minor()%first.Rounding.Unit != 0 {
			return refuse("common rounding and quantum-aligned principal/instalments required")
		}
		if l.Excess != allocation.ExcessReducePrincipal || len(c.Prepayment.Charges) > 0 || c.Prepayment.FeeBP != 0 || c.Prepayment.MinAmount.Sign() != 0 {
			return refuse("immediate principal credit without fees or thresholds required")
		}
	}
	return nil
}

func (s *dynamicSearch) tick() bool {
	if s.work >= s.limit {
		s.truncated = true
		return false
	}
	s.work++
	return true
}

func dynamicAdd(a, b int64) (int64, error) {
	if b > 0 && a > math.MaxInt64-b {
		return 0, money.ErrOverflow
	}
	return a + b, nil
}

func (s *dynamicSearch) solve(key dynamicKey) (dynamicTail, error) {
	empty := true
	for _, v := range key.Balances {
		if v > 0 {
			empty = false
		}
	}
	if empty {
		return dynamicTail{found: true}, nil
	}
	if key.Event == s.horizon {
		return dynamicTail{}, nil
	}
	if saved, ok := s.memo[key]; ok {
		return saved, nil
	}
	if !s.tick() {
		return dynamicTail{}, nil
	}
	s.states++
	cur := s.in.Cash.Monthly.Currency()
	day := s.in.Loans[0].Contract.PaymentDay
	from := s.in.ValuationDate
	if key.Event > 0 {
		from = date.Occurrence(s.in.ValuationDate, day, key.Event)
	}
	on := date.Occurrence(s.in.ValuationDate, day, key.Event+1)
	cash, err := dynamicAdd(key.Cash, s.in.Cash.Monthly.Minor())
	if err != nil {
		return dynamicTail{}, err
	}
	next := dynamicKey{Event: key.Event + 1}
	step := DynamicStep{On: on, Required: make([]money.Amount, len(s.in.Loans)), Extras: make([]money.Amount, len(s.in.Loans))}
	interest := int64(0)
	for i, l := range s.in.Loans {
		accrued, e := money.Accrue(money.FromMinor(key.Balances[i], cur), l.Contract.NominalRate, int64(date.DaysBetween(from, on)), l.Contract.DayCount, l.Contract.Rounding)
		if e != nil {
			return dynamicTail{}, e
		}
		interest, e = dynamicAdd(interest, accrued.Minor())
		if e != nil {
			return dynamicTail{}, e
		}
		owed, e := dynamicAdd(key.Balances[i], accrued.Minor())
		if e != nil {
			return dynamicTail{}, e
		}
		required := min(owed, l.Contract.ScheduledPayment.Minor())
		if required > cash {
			return dynamicTail{}, nil
		}
		cash -= required
		next.Balances[i] = owed - required
		step.Required[i] = money.FromMinor(required, cur)
	}
	step.Interest = money.FromMinor(interest, cur)
	best := dynamicTail{}
	var split func(int, int64) error
	split = func(i int, left int64) error {
		if !s.tick() {
			return nil
		}
		if i == len(s.in.Loans) {
			next.Cash = left
			tail, e := s.solve(next)
			if e != nil {
				return e
			}
			if !tail.found {
				return nil
			}
			cost, e := dynamicAdd(interest, tail.cost)
			if e != nil {
				return e
			}
			if !best.found || cost < best.cost || (cost == best.cost && len(tail.steps)+1 < len(best.steps)) {
				cp := step
				cp.Required = append([]money.Amount(nil), step.Required...)
				cp.Extras = append([]money.Amount(nil), step.Extras...)
				cp.Cash = money.FromMinor(left, cur)
				best = dynamicTail{found: true, cost: cost, steps: append([]DynamicStep{cp}, tail.steps...)}
			}
			return nil
		}
		balance := next.Balances[i]
		top := min(balance, left)
		for amount := top; amount >= 0; amount -= s.quantum {
			next.Balances[i] = balance - amount
			step.Extras[i] = money.FromMinor(amount, cur)
			if e := split(i+1, left-amount); e != nil {
				return e
			}
			if s.truncated {
				break
			}
		}
		next.Balances[i] = balance
		return nil
	}
	if err = split(0, cash); err != nil {
		return dynamicTail{}, err
	}
	if !s.truncated {
		s.memo[key] = best
	}
	return best, nil
}
