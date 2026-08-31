package plan

import (
	"fmt"

	"github.com/andranikasd/marumbot/pkg/core/allocation"
	"github.com/andranikasd/marumbot/pkg/core/amortisation"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// ActionKind labels one payment in the first cycle.
type ActionKind uint8

const (
	// Instalment is a contractual required payment.
	Instalment ActionKind = iota
	// Extra is an optional payment the policy chose.
	Extra
)

func (k ActionKind) String() string {
	if k == Extra {
		return "extra"
	}
	return "instalment"
}

// Action is one dated payment the policy makes in the first cycle, in the
// order the borrower must make them.
type Action struct {
	On     date.Date
	LoanID string
	Loan   string
	Kind   ActionKind
	Amount money.Amount // cash out, fee included
	Fee    money.Amount
	// Saves is the interest this cycle that paying on this date rather than
	// on the due date avoids. Zero on the due date, and zero when the lender
	// does not credit early payment.
	Saves money.Amount
}

// MonthLoan is one loan's share of a cycle: what it was paid and where it
// ended, so a sheet can answer "whom do I pay, how much, in month seven".
type MonthLoan struct {
	ID      string
	Name    string
	Paid    money.Amount // everything handed to this loan this cycle, fees included
	Extra   money.Amount // the optional part, fees excluded
	Owed    money.Amount // balance after the cycle
	Cleared bool
}

// MonthState is one cycle of a run, for timelines and comparators.
type MonthState struct {
	Month    int
	On       date.Date    // the income date that opened the cycle
	Required money.Amount // contractual instalments paid this cycle
	Extra    money.Amount // optional payments, fees excluded
	Fees     money.Amount
	Interest money.Amount // settled this cycle
	Owed     money.Amount // balances at the end of the cycle
	Cash     money.Amount // carried into the next cycle
	Cleared  string       // a loan that reached zero this cycle, by name
	Loans    []MonthLoan  // per loan, in input order
}

// Result is what one policy produces over a whole run.
type Result struct {
	Policy        Policy
	PayoffDate    date.Date
	Months        int
	TotalInterest money.Amount
	TotalFees     money.Amount
	TotalPaid     money.Amount // required + extra + fees
	NextMonthOwed money.Amount // balances after the first cycle
	FirstClear    string
	FirstClearOn  date.Date
	FirstClearAt  int          // cycle
	FirstFreed    money.Amount // the instalment that loan no longer requires
	Actions       []Action
	Timeline      []MonthState
	// PeakRequired and FinalRequired bracket the contractual outflow: what
	// the borrower must pay in the heaviest cycle and in the last one. Extra
	// payments are excluded; relief is about obligations, not choices.
	PeakRequired  money.Amount
	FinalRequired money.Amount
	Prepayments   int
	// TimingCredited is true when at least one early payment was credited
	// by a lender that reduces principal on the day of payment.
	TimingCredited bool
	// Assumed is the number of instalments assumed paid to bring each loan
	// to the valuation date, when the anchor was older.
	Assumed map[string]int
}

// Cost is what the least-cost comparator minimises: interest plus fees.
func (r Result) Cost() money.Amount {
	c, _ := r.TotalInterest.Add(r.TotalFees)
	return c
}

// eventKind orders same-day events: money arrives, then instalments fall
// due, then optional payments are made from what is left.
type eventKind uint8

const (
	evLump eventKind = iota
	evIncome
	evDue
)

type loanState struct {
	pos    Position
	fp     string
	effect model.PrepaymentEffect
	timing Timing
	// balance is principal outstanding; accrued is interest since the last
	// instalment, in the rounded pieces the lender books it in; from is the
	// day accrual last ran to.
	balance money.Amount
	accrued money.Amount
	from    date.Date
	// carried is the level instalment under shorten_term, once known.
	carried money.Amount
	// due and required are the current obligation.
	due      date.Date
	required money.Amount
	// pending is optional money allocated at income and held for the due
	// date, because the policy or the lender pays it then.
	pending money.Amount
	// allowance is free prepayment allowance used, by contract year.
	allowance map[int]money.Amount
	closed    bool
	closedOn  date.Date
	// The cycle's running figures, reset when a cycle opens.
	cyclePaid    money.Amount
	cycleExtra   money.Amount
	cycleCleared bool
}

// obligation is the memoised next instalment for one loan state.
type obligation struct {
	due        date.Date
	required   money.Amount
	instalment money.Amount
}

type oblKey struct {
	fp      string
	balance int64
	from    date.Date
	effect  model.PrepaymentEffect
	carried int64
}

// cache memoises contract projections across policies. The key is every
// input the projection depends on; a false hit would corrupt money, so the
// contract itself is fingerprinted rather than identified by index.
type cache map[oblKey]obligation

// fingerprint is every contract term the projection reads.
func fingerprint(c model.Contract) string {
	return fmt.Sprintf("%s|%d|%s|%s|%s|%s|%s|%d|%v|%s|%d|%d|%s|%d",
		c.LoanID, c.Version, c.Currency.Code, c.NominalRate, c.DayCount, c.Type,
		c.StartDate, c.PaymentDay, c.HasScheduled, c.ScheduledPayment,
		c.Rounding.Mode, c.Rounding.Unit, c.MaturityDate, c.Prepayment.Effect)
}

// next projects one loan's next obligation from its current state.
//
// Under reduce_instalment the schedule is rebuilt from the balance to
// maturity, which is what a lender does when it re-issues the schedule
// after a prepayment. Under shorten_term the instalment fixed at the anchor
// is carried and the loan ends when the balance does. Declining-principal
// loans are always rebuilt: their principal part is a term of the contract.
func (m cache) next(ls *loanState) (obligation, error) {
	fixed := ls.effect == model.PrepayShortenTerm && ls.pos.Contract.Type == model.Annuity && ls.carried.Sign() > 0
	k := oblKey{fp: ls.fp, balance: ls.balance.Minor(), from: ls.from, effect: ls.effect}
	if fixed {
		k.carried = ls.carried.Minor()
	}
	if v, ok := m[k]; ok {
		return v, nil
	}
	var o obligation
	if fixed {
		dates, err := amortisation.RemainingDates(ls.pos.Contract, ls.from)
		if err != nil {
			// Past the last contractual date with a balance left: the next
			// monthly occurrence, rather than pretending it vanished.
			o.due = date.Occurrence(ls.from, ls.pos.Contract.PaymentDay, 1)
		} else {
			o.due = dates[0]
		}
		o.required, o.instalment = ls.carried, ls.carried
	} else {
		s, err := amortisation.Build(ls.pos.Contract, ls.balance, ls.from)
		if err != nil || len(s.Rows) == 0 {
			return o, fmt.Errorf("plan: projecting %s: %w", ls.pos.ID, err)
		}
		o = obligation{due: s.Rows[0].Due, required: s.Rows[0].Payment, instalment: s.Instalment}
	}
	m[k] = o
	return o, nil
}

// Run follows one policy on one dated timeline until every loan is clear.
func Run(in Input, pol Policy) (Result, error) {
	return run(in, pol, cache{})
}

type sim struct {
	in     Input
	pol    Policy
	cache  cache
	cur    money.Currency
	loans  []*loanState
	cash   money.Amount
	budget money.Amount
	res    Result
	// cycle bookkeeping
	cycle       int
	cycleIncome date.Date // the income date that opened the current cycle
	nextIncome  date.Date // zero when there is no pay day: income lands on the first due of each month
	incomeYM    int       // year*12+month of the last income, for the no-pay-day rule
	month       MonthState
	lumps       []CashEvent
	inflow      money.Amount // every credit to the pool, for the identity
	err         error
}

func run(in Input, pol Policy, c cache) (Result, error) {
	if err := in.Validate(); err != nil {
		return Result{}, err
	}
	n := len(in.Loans)
	if len(pol.Order) != n || len(pol.Timing) != n || len(pol.Effect) != n {
		return Result{}, fmt.Errorf("plan: policy covers %d/%d/%d of %d loans", len(pol.Order), len(pol.Timing), len(pol.Effect), n)
	}
	cur := in.Cash.Monthly.Currency()
	zero := money.Zero(cur)
	s := &sim{in: in, pol: pol, cache: c, cur: cur, cash: in.Cash.OpeningCash, budget: in.Cash.Monthly, inflow: zero}
	if s.cash.Currency().Code == "" {
		s.cash = zero
	}
	s.res = Result{
		Policy: pol, TotalInterest: zero, TotalFees: zero, TotalPaid: zero, NextMonthOwed: zero,
		FirstFreed: zero, PeakRequired: zero, FinalRequired: zero,
	}
	for i, p := range in.Loans {
		ls := &loanState{
			pos: p, fp: fingerprint(p.Contract), timing: pol.Timing[i],
			balance: p.Balance, accrued: zero, from: p.From, carried: zero, pending: zero,
			allowance: map[int]money.Amount{},
		}
		ls.effect = p.Contract.Prepayment.Effect
		if ls.effect == model.PrepayBorrowerChooses {
			ls.effect = pol.Effect[i]
		}
		if ls.effect == model.PrepayBorrowerChooses {
			ls.effect = model.PrepayShortenTerm
		}
		if p.Balance.Sign() <= 0 {
			ls.closed, ls.closedOn = true, p.From
		} else if err := s.refresh(ls); err != nil {
			return Result{}, err
		}
		s.loans = append(s.loans, ls)
	}
	s.lumps = append([]CashEvent(nil), in.Cash.Lumps...)
	s.nextIncome = s.firstIncome()

	horizon := date.AddMonths(in.ValuationDate, in.horizon())
	for !s.allClosed() {
		kind, on, idx := s.nextEvent()
		if on.After(horizon) {
			return Result{}, ErrHorizon
		}
		switch kind {
		case evLump:
			s.lump(idx)
		case evIncome:
			s.income(on)
		case evDue:
			s.due(s.loans[idx], on)
		}
		if s.err != nil {
			return Result{}, s.err
		}
	}
	if s.cycle > 0 {
		s.closeCycle()
	}
	if s.res.Months == 0 {
		s.res.Months = s.cycle
	}
	if err := s.conserve(); err != nil {
		return Result{}, err
	}
	return s.res, nil
}

// refresh recomputes a loan's obligation from its current state.
func (s *sim) refresh(ls *loanState) error {
	o, err := s.cache.next(ls)
	if err != nil {
		return err
	}
	if ls.carried.Sign() == 0 {
		ls.carried = o.instalment
	}
	ls.due, ls.required = o.due, o.required
	return nil
}

func (s *sim) allClosed() bool {
	for _, ls := range s.loans {
		if !ls.closed {
			return false
		}
	}
	return true
}

// firstIncome is the first pay day on or after the valuation date. With no
// pay day there is no income event: the month's money is deemed to arrive
// on the first instalment date of each calendar month, handled in due().
func (s *sim) firstIncome() date.Date {
	if s.in.Cash.PayDay == 0 {
		return date.Date{}
	}
	d := date.OnDayOfMonth(s.in.ValuationDate, s.in.Cash.PayDay)
	if d.Before(s.in.ValuationDate) {
		d = date.OnDayOfMonth(date.AddMonths(s.in.ValuationDate, 1), s.in.Cash.PayDay)
	}
	return d
}

func (s *sim) incomeAfter(d date.Date) date.Date {
	if s.in.Cash.PayDay == 0 {
		return date.Date{}
	}
	return date.OnDayOfMonth(date.AddMonths(d, 1), s.in.Cash.PayDay)
}

// nextEvent picks the earliest pending event; same-day order is lump, income,
// then dues by loan index.
func (s *sim) nextEvent() (eventKind, date.Date, int) {
	var (
		kind eventKind
		on   date.Date
		idx  = -1
		have bool
	)
	consider := func(k eventKind, d date.Date, i int) {
		if !have || d.Before(on) || (d.Equal(on) && k < kind) {
			kind, on, idx, have = k, d, i, true
		}
	}
	if !s.nextIncome.IsZero() {
		consider(evIncome, s.nextIncome, -1)
	}
	for i, l := range s.lumps {
		consider(evLump, l.On, i)
	}
	for i, ls := range s.loans {
		if !ls.closed {
			consider(evDue, ls.due, i)
		}
	}
	return kind, on, idx
}

func ym(d date.Date) int { return d.Year()*12 + int(d.Month()) }

func (s *sim) add(a, b money.Amount) money.Amount {
	if s.err != nil {
		return a
	}
	r, err := a.Add(b)
	if err != nil {
		s.err = err
	}
	return r
}

func (s *sim) sub(a, b money.Amount) money.Amount {
	if s.err != nil {
		return a
	}
	r, err := a.Sub(b)
	if err != nil {
		s.err = err
	}
	return r
}

func (s *sim) openCycle(on date.Date) {
	zero := money.Zero(s.cur)
	s.cycle++
	s.cycleIncome = on
	s.month = MonthState{Month: s.cycle, On: on, Required: zero, Extra: zero, Fees: zero, Interest: zero, Owed: zero, Cash: zero}
	for _, ls := range s.loans {
		ls.cyclePaid, ls.cycleExtra, ls.cycleCleared = zero, zero, false
	}
}

func (s *sim) closeCycle() {
	owed := money.Zero(s.cur)
	for _, ls := range s.loans {
		if !ls.closed {
			owed = s.add(owed, ls.balance)
		}
	}
	s.month.Owed, s.month.Cash = owed, s.cash
	for _, ls := range s.loans {
		if ls.cyclePaid.Sign() <= 0 && ls.closed && !ls.cycleCleared {
			continue // long since paid off; not a row in this month
		}
		s.month.Loans = append(s.month.Loans, MonthLoan{
			ID: ls.pos.ID, Name: ls.pos.Name,
			Paid: ls.cyclePaid, Extra: ls.cycleExtra,
			Owed: ls.balance, Cleared: ls.cycleCleared,
		})
	}
	s.res.Timeline = append(s.res.Timeline, s.month)
	if s.cycle == 1 {
		s.res.NextMonthOwed = owed
	}
	if s.month.Required.Cmp(s.res.PeakRequired) > 0 {
		s.res.PeakRequired = s.month.Required
	}
	if s.month.Required.Sign() > 0 {
		s.res.FinalRequired = s.month.Required
	}
}

func (s *sim) lump(i int) {
	l := s.lumps[i]
	s.lumps = append(s.lumps[:i], s.lumps[i+1:]...)
	if s.cycle == 0 {
		s.openCycle(l.On)
	}
	s.cash = s.add(s.cash, l.Amount)
	s.inflow = s.add(s.inflow, l.Amount)
	s.allocate(l.On)
}

// income credits the cycle's budget. Each income opens a cycle; the first
// due date before any income opens the first, so nothing is counted twice
// and no empty cycle precedes the money.
func (s *sim) income(on date.Date) {
	if s.cycle > 0 && !s.cycleIncome.Equal(on) {
		s.closeCycle()
	}
	if s.cycle == 0 || !s.cycleIncome.Equal(on) {
		s.openCycle(on)
	}
	s.cash = s.add(s.cash, s.budget)
	s.inflow = s.add(s.inflow, s.budget)
	s.nextIncome = s.incomeAfter(on)
	s.incomeYM = ym(on)
	s.allocate(on)
}

// reserved is cash that optional payments may not touch: the floor, every
// instalment falling due before the next income, and money already
// allocated to a due date.
func (s *sim) reserved() money.Amount {
	r := s.in.Cash.ReserveFloor
	if r.Currency().Code == "" {
		r = money.Zero(s.cur)
	}
	for _, ls := range s.loans {
		if ls.closed {
			continue
		}
		if (s.nextIncome.IsZero() && ym(ls.due) == s.incomeYM) || (!s.nextIncome.IsZero() && ls.due.Before(s.nextIncome)) {
			r = s.add(r, ls.required)
		}
		r = s.add(r, ls.pending)
	}
	return r
}

// allocate spends the surplus down the priority order. A loan paid on
// receipt by a lender that credits it is paid now; otherwise its share is
// held for its due date. Whatever no loan can absorb stays in the pool.
func (s *sim) allocate(on date.Date) {
	surplus := s.sub(s.cash, s.reserved())
	if s.err != nil || surplus.Sign() <= 0 {
		return
	}
	for _, idx := range s.pol.Order {
		ls := s.loans[idx]
		if ls.closed || surplus.Sign() <= 0 {
			continue
		}
		q, err := s.quote(ls, on, surplus)
		if err != nil {
			s.err = err
			return
		}
		if q.Outflow.Sign() <= 0 {
			continue
		}
		if s.pol.MinPrepay.Sign() > 0 && q.Principal.Cmp(s.pol.MinPrepay) < 0 && !q.Closes {
			continue // batch: carry the cash until it is worth a payment
		}
		if ls.timing == OnReceipt && ls.pos.Excess == allocation.ExcessReducePrincipal && on.Before(ls.due) {
			s.prepay(ls, on, q, true)
		} else {
			// Held to the due date. The full quote is reserved so the cash
			// cannot be promised twice; the amount is re-quoted then.
			ls.pending = s.add(ls.pending, q.Principal)
		}
		surplus = s.sub(surplus, q.Outflow)
	}
}

// quote is the lender's answer to "what would paying up to `available` on
// this date do". The payment is capped at what closes the loan.
func (s *sim) quote(ls *loanState, on date.Date, available money.Amount) (Quote, error) {
	zero := money.Zero(s.cur)
	if err := s.accrueTo(ls, on); err != nil {
		return Quote{}, err
	}
	c := ls.pos.Contract
	used := ls.allowance[contractYear(c, on)]
	if used.Currency().Code == "" {
		used = zero
	}
	// Closing the loan: balance, accrued interest, and the fee on the balance.
	closeFee, err := charge(c, on, ls.balance, used)
	if err != nil {
		return Quote{}, err
	}
	closeOut, err := ls.balance.Add(ls.accrued)
	if err != nil {
		return Quote{}, err
	}
	if closeOut, err = closeOut.Add(closeFee); err != nil {
		return Quote{}, err
	}
	if available.Cmp(closeOut) >= 0 {
		return Quote{Principal: ls.balance, Interest: ls.accrued, Fee: closeFee, Outflow: closeOut, Closes: true}, nil
	}
	if c.Prepayment.MinAmount.Sign() > 0 && available.Cmp(c.Prepayment.MinAmount) < 0 {
		return Quote{Principal: zero, Interest: zero, Fee: zero, Outflow: zero}, nil
	}
	// A partial payment credits principal; the fee is paid on top, so the
	// principal is what remains of `available` after its own fee. Solve by
	// one correction step, which is exact when the fee is proportional.
	principal := available
	fee, err := charge(c, on, principal, used)
	if err != nil {
		return Quote{}, err
	}
	if fee.Sign() > 0 {
		if principal, err = available.Sub(fee); err != nil {
			return Quote{}, err
		}
		if principal.Sign() <= 0 {
			return Quote{Principal: zero, Interest: zero, Fee: zero, Outflow: zero}, nil
		}
		if fee, err = charge(c, on, principal, used); err != nil {
			return Quote{}, err
		}
	}
	principal = money.Quantise(principal, c.Rounding)
	out, err := principal.Add(fee)
	if err != nil {
		return Quote{}, err
	}
	return Quote{Principal: principal, Interest: zero, Fee: fee, Outflow: out}, nil
}

// accrueTo books interest from the last accrual date to `on`, in the
// rounded piece the lender would book. With no intervening payment there is
// one piece per cycle and the result equals the schedule row exactly.
func (s *sim) accrueTo(ls *loanState, on date.Date) error {
	if !on.After(ls.from) || ls.balance.Sign() <= 0 {
		return nil
	}
	c := ls.pos.Contract
	i, err := money.Accrue(ls.balance, c.NominalRate, int64(date.DaysBetween(ls.from, on)), c.DayCount, c.Rounding)
	if err != nil {
		return err
	}
	if ls.accrued, err = ls.accrued.Add(i); err != nil {
		return err
	}
	ls.from = on
	return nil
}

// prepay applies a quoted optional payment.
func (s *sim) prepay(ls *loanState, on date.Date, q Quote, early bool) {
	c := ls.pos.Contract
	s.cash = s.sub(s.cash, q.Outflow)
	ls.cyclePaid = s.add(ls.cyclePaid, q.Outflow)
	ls.cycleExtra = s.add(ls.cycleExtra, s.sub(q.Outflow, q.Fee))
	s.res.TotalPaid = s.add(s.res.TotalPaid, q.Outflow)
	s.res.TotalFees = s.add(s.res.TotalFees, q.Fee)
	s.month.Fees = s.add(s.month.Fees, q.Fee)
	s.month.Extra = s.add(s.month.Extra, s.sub(q.Outflow, q.Fee))
	s.res.Prepayments++
	if early {
		s.res.TimingCredited = true
	}
	y := contractYear(c, on)
	used := ls.allowance[y]
	if used.Currency().Code == "" {
		used = money.Zero(s.cur)
	}
	ls.allowance[y] = s.add(used, q.Principal)

	saves := money.Zero(s.cur)
	if early && on.Before(ls.due) {
		saves, _ = money.Accrue(q.Principal, c.NominalRate, int64(date.DaysBetween(on, ls.due)), c.DayCount, c.Rounding)
	}
	s.action(ls, on, Extra, q.Outflow, q.Fee, saves)

	ls.balance = s.sub(ls.balance, q.Principal)
	if q.Closes {
		s.res.TotalInterest = s.add(s.res.TotalInterest, q.Interest)
		s.month.Interest = s.add(s.month.Interest, q.Interest)
		ls.accrued = money.Zero(s.cur)
		s.close(ls, on)
	}
}

// due settles a loan's instalment on its date, then any money held for it.
func (s *sim) due(ls *loanState, on date.Date) {
	if s.in.Cash.PayDay == 0 && ym(on) != s.incomeYM {
		s.income(on)
		if s.err != nil {
			return
		}
	}
	if s.cycle == 0 {
		s.openCycle(on) // a due before the first pay day: opening cash pays it
	}
	if err := s.accrueTo(ls, on); err != nil {
		s.err = err
		return
	}
	interest := ls.accrued
	owed := s.add(ls.balance, interest)
	required := ls.required
	// The last contractual date settles whatever is owed: a residue of one
	// rounding unit left by two accrual pieces must not outlive the loan.
	if _, err := amortisation.RemainingDates(ls.pos.Contract, on); err != nil {
		required = owed
	}
	if required.Cmp(owed) > 0 {
		required = owed
	}
	if s.err != nil {
		return
	}
	if s.cash.Cmp(required) < 0 {
		short, _ := required.Sub(s.cash)
		s.err = &InfeasibleError{On: on, LoanID: ls.pos.ID, Required: required, Available: s.cash, Shortfall: short}
		return
	}
	s.cash = s.sub(s.cash, required)
	ls.cyclePaid = s.add(ls.cyclePaid, required)
	principal := s.sub(required, interest)
	ls.balance = s.sub(ls.balance, principal)
	ls.accrued = money.Zero(s.cur)
	s.res.TotalInterest = s.add(s.res.TotalInterest, interest)
	s.res.TotalPaid = s.add(s.res.TotalPaid, required)
	s.month.Interest = s.add(s.month.Interest, interest)
	s.month.Required = s.add(s.month.Required, required)
	s.action(ls, on, Instalment, required, money.Zero(s.cur), money.Zero(s.cur))
	if s.err != nil {
		return
	}

	if ls.balance.Sign() <= 0 {
		s.close(ls, on)
		return
	}
	// Money held for this date, re-quoted against what is owed now.
	if ls.pending.Sign() > 0 {
		want := ls.pending
		ls.pending = money.Zero(s.cur)
		if want.Cmp(s.cash) > 0 {
			want = s.cash
		}
		q, err := s.quote(ls, on, want)
		if err != nil {
			s.err = err
			return
		}
		if q.Outflow.Sign() > 0 {
			s.prepay(ls, on, q, false)
			if ls.closed {
				return
			}
		}
	}
	if err := s.refresh(ls); err != nil {
		s.err = err
	}
}

// close retires a loan and frees its instalment.
func (s *sim) close(ls *loanState, on date.Date) {
	ls.closed, ls.closedOn = true, on
	ls.cycleCleared = true
	ls.balance = money.Zero(s.cur)
	freed := ls.carried
	if freed.Sign() == 0 {
		freed = ls.required
	}
	s.month.Cleared = ls.pos.Name
	if s.res.FirstClear == "" {
		s.res.FirstClear, s.res.FirstClearOn, s.res.FirstClearAt, s.res.FirstFreed = ls.pos.Name, on, s.cycle, freed
	}
	if s.pol.Rollover == KeepFreed {
		b := s.sub(s.budget, freed)
		if b.Sign() < 0 {
			b = money.Zero(s.cur)
		}
		s.budget = b
	}
	if on.After(s.res.PayoffDate) {
		s.res.PayoffDate = on
	}
	if s.allClosed() {
		s.res.Months = s.cycle
	}
}

// action records a payment while the first cycle is open.
func (s *sim) action(ls *loanState, on date.Date, kind ActionKind, amount, fee, saves money.Amount) {
	if s.cycle > 1 {
		return
	}
	s.res.Actions = append(s.res.Actions, Action{
		On: on, LoanID: ls.pos.ID, Loan: ls.pos.Name, Kind: kind, Amount: amount, Fee: fee, Saves: saves,
	})
}

// conserve checks the cash identity over the whole run:
// opening + income + lumps = required + extra + fees + closing.
func (s *sim) conserve() error {
	in := s.in.Cash.OpeningCash
	if in.Currency().Code == "" {
		in = money.Zero(s.cur)
	}
	var err error
	// Income is whatever budget applied in each cycle; replay it from the
	// timeline's own record rather than trusting the running total.
	paid := money.Zero(s.cur)
	for _, m := range s.res.Timeline {
		for _, a := range []money.Amount{m.Required, m.Extra, m.Fees} {
			if paid, err = paid.Add(a); err != nil {
				return err
			}
		}
	}
	if paid.Cmp(s.res.TotalPaid) != 0 {
		return fmt.Errorf("%w: timeline paid %s, ledger paid %s", ErrInvariant, paid, s.res.TotalPaid)
	}
	if s.cash.Sign() < 0 {
		return fmt.Errorf("%w: closing cash %s", ErrInvariant, s.cash)
	}
	credits, err := in.Add(s.inflow)
	if err != nil {
		return err
	}
	debits, err := s.res.TotalPaid.Add(s.cash)
	if err != nil {
		return err
	}
	if credits.Cmp(debits) != 0 {
		return fmt.Errorf("%w: opening %s + inflow %s != paid %s + closing %s", ErrInvariant, in, s.inflow, s.res.TotalPaid, s.cash)
	}
	return nil
}
