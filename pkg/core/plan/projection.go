package plan

import (
	"fmt"

	"github.com/andranikasd/marumbot/pkg/core/amortisation"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// The simulator's only door onto contract arithmetic.
//
// Projections are memoised twice over: a contract's instalment dates do not
// depend on the balance at all, and one loan's next obligation depends on
// exactly the terms in oblKey. A search simulates thousands of policies over
// the same few loans, so both memos are the difference between projecting a
// loan once and projecting it once per candidate.

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
//
// calendars memoises the other half: a contract's instalment dates do not
// depend on the balance or the anchor at all, so every policy in a search
// shares one calendar per loan rather than rebuilding it per projection.
type cache struct {
	obligations map[oblKey]obligation
	calendars   map[string]amortisation.Calendar
}

func newCache() *cache { return &cache{} }

// calendar returns the memoised calendar for one loan's contract. ls.fp is
// already the contract fingerprint the obligation key is built from, so a hit
// here is a hit on exactly the same terms the dates depend on.
func (m *cache) calendar(ls *loanState) (amortisation.Calendar, error) {
	if cal, ok := m.calendars[ls.fp]; ok {
		return cal, nil
	}
	cal, err := amortisation.NewCalendar(ls.pos.Contract)
	if err != nil {
		return amortisation.Calendar{}, err
	}
	if m.calendars == nil {
		m.calendars = map[string]amortisation.Calendar{}
	}
	m.calendars[ls.fp] = cal
	return cal, nil
}

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
func (m *cache) next(ls *loanState) (obligation, error) {
	fixed := ls.effect == model.PrepayShortenTerm && ls.pos.Contract.Type == model.Annuity && ls.carried.Sign() > 0
	k := oblKey{fp: ls.fp, balance: ls.balance.Minor(), from: ls.from, effect: ls.effect}
	if fixed {
		k.carried = ls.carried.Minor()
	}
	if v, ok := m.obligations[k]; ok {
		return v, nil
	}
	var o obligation
	cal, err := m.calendar(ls)
	if err != nil {
		return o, fmt.Errorf("plan: projecting %s: %w", ls.pos.ID, err)
	}
	if fixed {
		dates, err := cal.Dates(ls.from)
		if err != nil {
			// Past the last contractual date with a balance left: the next
			// monthly occurrence, rather than pretending it vanished.
			o.due = date.Occurrence(ls.from, ls.pos.Contract.PaymentDay, 1)
		} else {
			o.due = dates[0]
		}
		o.required, o.instalment = ls.carried, ls.carried
	} else {
		next, err := cal.Next(ls.balance, ls.from)
		if err != nil {
			return o, fmt.Errorf("plan: projecting %s: %w", ls.pos.ID, err)
		}
		o = obligation{due: next.Due, required: next.Payment, instalment: next.Instalment}
	}
	if m.obligations == nil {
		m.obligations = map[oblKey]obligation{}
	}
	m.obligations[k] = o
	return o, nil
}
