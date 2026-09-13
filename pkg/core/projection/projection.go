// Package projection models monthly payment capacity, never available cash.
// Required payments keep their bank dates; optional amounts occur at month-end.
package projection

import (
	"errors"
	"sort"

	"github.com/andranikasd/marumbot/pkg/core/amortisation"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// MaxMonths bounds a projection to fifty years.
const (
	MaxMonths = 600
	// MaxLoans bounds work for one borrower.
	MaxLoans = 100
	// MaxMinor keeps API amounts exactly representable as JavaScript integers.
	MaxMinor int64 = 9007199254740991
)

// ErrInvalid rejects inconsistent or unsafe source declarations.
var ErrInvalid = errors.New("projection: invalid source")

// Extra is the default commitment and whole-month replacements.
type Extra struct {
	Minor     int64            `json:"extra_minor"`
	Overrides map[string]int64 `json:"overrides"`
}

// Loan is one current source statement and its contractual terms.
type Loan struct {
	ID       string
	Name     string
	Contract model.Contract
	Balance  money.Amount
	AsOf     date.Date
	// Reason identifies the missing source; any reason restricts this loan to
	// its one known bank obligation, without inferring interest or a payoff.
	Reason           string
	OptionalExcluded bool
	// OpeningInterestKnown distinguishes a bank statement from an implicit zero.
	OpeningInterestKnown bool
	OpeningInterestMinor int64
}

// Payment separates a required instalment from an optional contribution.
type Payment struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Due      string `json:"due,omitempty"`
	Required int64  `json:"required_minor"`
	Extra    int64  `json:"extra_minor"`
	Reason   string `json:"reason,omitempty"`
}

// Month reports a conditional monthly outlay, never available cash.
type Month struct {
	Month            string    `json:"month"`
	Required         int64     `json:"required_minor"`
	Extra            int64     `json:"extra_minor"`
	RequestedExtra   int64     `json:"requested_extra_minor"`
	UnallocatedExtra int64     `json:"unallocated_extra_minor"`
	Total            int64     `json:"total_minor"`
	Loans            []Payment `json:"loans"`
}

// Result is one currency projection; incomplete results never claim a payoff.
type Result struct {
	Currency   string  `json:"currency"`
	Exponent   uint8   `json:"exponent"`
	StartMonth string  `json:"start_month"`
	Complete   bool    `json:"complete"`
	Reason     string  `json:"reason,omitempty"`
	Finish     string  `json:"finish,omitempty"`
	Months     []Month `json:"months"`
}

// MonthKey identifies a calendar month.
func MonthKey(d date.Date) string { return d.String()[:7] }

// ValidateExtra rejects malformed dates, negative amounts and unsafe integers.
func ValidateExtra(x Extra) error {
	if x.Minor < 0 || x.Minor > MaxMinor || len(x.Overrides) > MaxMonths {
		return ErrInvalid
	}
	for month, n := range x.Overrides {
		d, err := date.Parse(month + "-01")
		if err != nil || len(month) != 7 || MonthKey(d) != month || n < 0 || n > MaxMinor {
			return ErrInvalid
		}
	}
	return nil
}

type state struct {
	loan              Loan
	balance, interest int64
	from              date.Date
	dates             []date.Date
	index             int
	known             date.Date
}

// Build uses a stable highest-rate-first recommendation among supported loans.
// It makes no claim of optimality across bank policies. Unallocated extra stays
// explicit, never carries forward, and never gets turned into a cash receipt.
func Build(today date.Date, cur money.Currency, loans []Loan, extra Extra) (Result, error) {
	out := Result{Currency: cur.Code, Exponent: cur.Exponent, Complete: true, Months: []Month{}}
	if today.IsZero() || len(loans) > MaxLoans || ValidateExtra(extra) != nil {
		return out, ErrInvalid
	}
	states := make([]state, 0, len(loans))
	seen := map[string]bool{}
	start := MonthKey(today)
	earliest := ""
	for _, l := range loans {
		if l.ID == "" || seen[l.ID] || (l.Balance.Currency().Code != cur.Code || l.Contract.Currency.Code != cur.Code) || l.Balance.Minor() < 0 || l.Balance.Minor() > MaxMinor || l.AsOf.IsZero() || l.AsOf.After(today) {
			return out, ErrInvalid
		}
		seen[l.ID] = true
		if l.OpeningInterestMinor < 0 || l.OpeningInterestMinor > MaxMinor || (l.OpeningInterestMinor > 0 && (!l.OpeningInterestKnown || l.Balance.Sign() == 0)) {
			return out, ErrInvalid
		}
		if l.Balance.Sign() == 0 {
			continue
		}
		s := state{loan: l, balance: l.Balance.Minor(), from: l.AsOf, known: l.Contract.NotBeforeDue}
		if l.OpeningInterestKnown {
			s.interest = l.OpeningInterestMinor
		}
		if s.loan.Reason == "" && l.Contract.NominalRate > 0 && !l.OpeningInterestKnown && !l.AsOf.Equal(l.Contract.StartDate) {
			s.loan.Reason = "accrued_interest_needed"
		}
		if s.known.IsZero() && l.Reason == "" {
			dates, err := amortisation.RemainingDates(l.Contract, l.AsOf)
			if err == nil {
				s.known = dates[0]
			}
		}
		if !s.known.IsZero() && s.known.Before(today) {
			s.loan.Reason = "overdue_payment"
		}
		if s.loan.Reason == "" {
			c := l.Contract
			if c.Validate() != nil || !c.HasScheduled || c.Type != model.Annuity || c.Prepayment.Effect == model.PrepayReduceInstalment || c.Prepayment.FeeBP != 0 || len(c.Prepayment.Charges) != 0 || c.Prepayment.MinAmount.Sign() != 0 || (c.DayCount != money.Actual365 && c.DayCount != money.Actual360) {
				s.loan.Reason = "unsupported_terms"
			}
		}
		if s.loan.Reason == "" {
			var err error
			s.dates, err = amortisation.RemainingDates(l.Contract, l.AsOf)
			if err != nil || len(s.dates) == 0 || s.known.IsZero() || !s.dates[0].Equal(s.known) {
				s.loan.Reason = "bank_payment_needed"
			}
		}

		if s.loan.Reason == "" {
			first := s
			if err := first.accrue(s.dates[0]); err != nil {
				return out, err
			}
			owed, err := add(first.balance, first.interest)
			if err != nil {
				return out, err
			}
			required := l.Contract.ScheduledPayment.Minor()
			if required > owed || required <= first.interest || (len(s.dates) == 1 && required != owed) {
				s.loan.Reason = "bank_payment_mismatch"
			}
		}
		if s.loan.Reason != "" {
			out.Complete = false
			if out.Reason == "" {
				out.Reason = s.loan.Reason
			}
		}
		if s.known.IsZero() {
			earliest = start
		} else if earliest == "" || MonthKey(s.known) < earliest {
			earliest = MonthKey(s.known)
		}
		states = append(states, s)
	}
	if earliest > start {
		start = earliest
	}
	out.StartMonth = start
	sort.Slice(states, func(i, j int) bool {
		a, b := states[i].loan, states[j].loan
		if a.Contract.NominalRate != b.Contract.NominalRate {
			return a.Contract.NominalRate > b.Contract.NominalRate
		}
		if a.Balance.Minor() != b.Balance.Minor() {
			return a.Balance.Minor() < b.Balance.Minor()
		}
		return a.ID < b.ID
	})
	begin, _ := date.Parse(start + "-01")
	for n := 0; n < MaxMonths && len(states) > 0; n++ {
		on := date.AddMonths(begin, n).EndOfMonth()
		month := MonthKey(on)
		requested := extra.Minor
		if v, ok := extra.Overrides[month]; ok {
			requested = v
		}
		m := Month{Month: month, RequestedExtra: requested, Loans: []Payment{}}
		for i := range states {
			s := &states[i]
			if s.balance == 0 {
				continue
			}
			p := Payment{ID: s.loan.ID, Name: s.loan.Name, Reason: s.loan.Reason}
			if s.loan.Reason != "" {
				// A past due amount remains visible in the current month; never move it
				// to a future start or assume all subsequent instalments are known.
				if n == 0 && (s.known.IsZero() || MonthKey(s.known) <= month) || (!s.known.IsZero() && MonthKey(s.known) == month) {
					if s.loan.Contract.HasScheduled {
						p.Required = s.loan.Contract.ScheduledPayment.Minor()
					}
					if !s.known.IsZero() {
						p.Due = s.known.String()
					}
				} else {
					continue
				}
			} else if s.index < len(s.dates) && !s.dates[s.index].After(on) {
				due := s.dates[s.index]
				if err := s.accrue(due); err != nil {
					return out, err
				}
				owed, err := add(s.balance, s.interest)
				if err != nil {
					return out, err
				}
				required := s.loan.Contract.ScheduledPayment.Minor()
				if required > owed || s.index == len(s.dates)-1 {
					required = owed
				}
				if required <= s.interest {
					return out, ErrInvalid
				}
				s.balance -= required - s.interest
				s.interest = 0
				s.index++
				p.Due = due.String()
				p.Required = required
			}
			var err error
			m.Required, err = add(m.Required, p.Required)
			if err != nil {
				return out, err
			}
			m.Loans = append(m.Loans, p)
		}
		left := requested
		for i := range states {
			s := &states[i]
			if s.balance == 0 || s.loan.Reason != "" || s.loan.OptionalExcluded || left == 0 {
				continue
			}
			if err := s.accrue(on); err != nil {
				return out, err
			}
			owed, err := add(s.balance, s.interest)
			if err != nil {
				return out, err
			}
			paid := left
			if paid >= owed {
				paid = owed
				s.balance = 0
				s.interest = 0
			} else {
				if paid > s.balance {
					paid = s.balance
				}
				paid = (paid / s.loan.Contract.Rounding.Unit) * s.loan.Contract.Rounding.Unit
				s.balance -= paid
			}
			// A principal-only prepayment cannot leave interest on a zero principal.
			// Retain one settlement unit unless the full settlement is affordable.
			if s.balance == 0 && s.interest > 0 {
				unit := s.loan.Contract.Rounding.Unit
				if unit > paid {
					unit = paid
				}
				paid -= unit
				s.balance = unit
			}
			left -= paid
			for j := range m.Loans {
				if m.Loans[j].ID == s.loan.ID {
					m.Loans[j].Extra += paid
					break
				}
			}
		}
		m.Extra = requested - left
		m.UnallocatedExtra = left
		var err error
		m.Total, err = add(m.Required, m.Extra)
		if err != nil {
			return out, err
		}
		out.Months = append(out.Months, m)
		alive := false
		for _, s := range states {
			if s.balance > 0 && (s.loan.Reason == "" || (!s.known.IsZero() && s.known.After(on))) {
				alive = true
			}
		}
		if !alive {
			if out.Complete {
				out.Finish = month
			}
			break
		}
	}
	if out.Complete && len(states) > 0 && out.Finish == "" {
		out.Complete = false
		out.Reason = "projection_horizon"
	}
	return out, nil
}

func add(a, b int64) (int64, error) {
	if a < 0 || b < 0 || a > MaxMinor-b {
		return 0, ErrInvalid
	}
	return a + b, nil
}

func (s *state) accrue(on date.Date) error {
	if on.Before(s.from) {
		return ErrInvalid
	}
	interest, err := money.Accrue(money.FromMinor(s.balance, s.loan.Balance.Currency()), s.loan.Contract.NominalRate, int64(date.DaysBetween(s.from, on)), s.loan.Contract.DayCount, s.loan.Contract.Rounding)
	if err != nil {
		return err
	}
	s.interest, err = add(s.interest, interest.Minor())
	s.from = on
	return err
}
