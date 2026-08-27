package plan

import (
	"fmt"

	"github.com/andranikasd/marumbot/pkg/core/allocation"
	"github.com/andranikasd/marumbot/pkg/core/amortisation"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/model"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// maxMonths bounds a simulation. Forty years of monthly instalments is 480; a
// run that passes this is a loan that never amortises, which is a finding
// rather than a reason to keep looping.
const maxMonths = 600

// Position is one loan as a simulation carries it.
type Position struct {
	ID       string
	Name     string
	Contract model.Contract
	Balance  money.Amount
	From     date.Date
	// Excess is what the lender does with money paid beyond the instalment.
	// Only ExcessReducePrincipal lets an early payment save interest; the
	// search credits early timing for no other rule.
	Excess allocation.ExcessRule
}

// Outcome is what following a goal actually produces.
//
// These are the four things a borrower asks and the engine can answer: how long
// it takes, what it costs, what is owed after the next payment, and how much of
// the monthly obligation disappears. Anything the engine cannot derive is left
// out rather than estimated.
type Outcome struct {
	Goal          Goal
	Months        int          // until every loan is clear
	TotalInterest money.Amount // over the whole run
	FirstTarget   string       // which loan the surplus goes to first, by name
	FirstExtra    money.Amount // how much it receives this month
	NextMonthOwed money.Amount // total still owed after this month's payments
	MonthlyFreed  money.Amount // required payment removed by the first payoff
	ClearedFirst  string       // the first loan to disappear, by name
	ClearedMonth  int          // when it does
}

// Simulate follows a goal month by month until every loan is clear.
//
// It is a simulation rather than a formula because the answer depends on the
// order things are paid off: clearing one loan frees its required payment,
// which changes the surplus available next month, which can change the target.
// No closed form survives that feedback.
//
// Interest is accrued by the same dated engine that produces a schedule, so a
// projection and a plan cannot disagree about what a month costs.
func Simulate(loans []Position, budget money.Amount, goal Goal) (Outcome, error) {
	if len(loans) == 0 {
		return Outcome{}, fmt.Errorf("plan: no loans")
	}
	cur := budget.Currency()
	out := Outcome{
		Goal:          goal,
		TotalInterest: money.Zero(cur),
		FirstExtra:    money.Zero(cur),
		NextMonthOwed: money.Zero(cur),
		MonthlyFreed:  money.Zero(cur),
	}

	live := make([]Position, len(loans))
	copy(live, loans)

	for month := 1; month <= maxMonths; month++ {
		// What each loan requires this month, and when.
		type step struct {
			idx      int
			due      date.Date
			interest money.Amount
			required money.Amount
		}
		var steps []step
		var pl []Loan

		for i := range live {
			if live[i].Balance.Sign() <= 0 {
				continue
			}
			s, err := amortisation.Build(live[i].Contract, live[i].Balance, live[i].From)
			if err != nil || len(s.Rows) == 0 {
				// A loan that cannot be projected is left alone rather than
				// guessed at; it keeps its balance and stops the run.
				return out, fmt.Errorf("plan: projecting %s: %w", live[i].ID, err)
			}
			r := s.Rows[0]
			steps = append(steps, step{i, r.Due, r.Interest, r.Payment})
			pl = append(pl, Loan{
				ID: live[i].ID, Balance: live[i].Balance,
				Rate: live[i].Contract.NominalRate, Required: r.Payment,
			})
		}
		if len(steps) == 0 {
			out.Months = month - 1
			return out, nil
		}

		p, err := Allocate(pl, budget, goal)
		if err != nil {
			// The budget no longer covers the minimums. That is an answer:
			// this plan reaches a month the borrower cannot afford.
			return out, err
		}

		extra := map[string]money.Amount{}
		for _, a := range p.Allocations {
			extra[a.LoanID] = a.Extra
		}
		if month == 1 {
			out.FirstTarget = nameOf(loans, p.Target)
			if e, ok := extra[p.Target]; ok {
				out.FirstExtra = e
			}
		}

		for _, st := range steps {
			l := &live[st.idx]
			pay := st.required
			if e, ok := extra[l.ID]; ok && e.Sign() > 0 {
				if pay, err = pay.Add(e); err != nil {
					return out, err
				}
			}
			owed, err := l.Balance.Add(st.interest)
			if err != nil {
				return out, err
			}
			if pay.Cmp(owed) > 0 {
				pay = owed
			}
			principal, err := pay.Sub(st.interest)
			if err != nil {
				return out, err
			}
			if l.Balance, err = l.Balance.Sub(principal); err != nil {
				return out, err
			}
			if out.TotalInterest, err = out.TotalInterest.Add(st.interest); err != nil {
				return out, err
			}
			l.From = st.due

			if l.Balance.Sign() <= 0 && out.ClearedFirst == "" {
				out.ClearedFirst = nameOf(loans, l.ID)
				out.ClearedMonth = month
				out.MonthlyFreed = st.required
			}
		}

		if month == 1 {
			total := money.Zero(cur)
			for i := range live {
				if live[i].Balance.Sign() > 0 {
					if total, err = total.Add(live[i].Balance); err != nil {
						return out, err
					}
				}
			}
			out.NextMonthOwed = total
		}
	}
	return out, fmt.Errorf("plan: still owing after %d months", maxMonths)
}

// CompareAll runs every goal over the same starting position, so the answers
// are comparable rather than merely each correct on its own.
func CompareAll(loans []Position, budget money.Amount) ([]Outcome, error) {
	goals := []Goal{PayLeastInterest, FinishSoonest, FreeUpMonthly}
	out := make([]Outcome, 0, len(goals))
	for _, g := range goals {
		o, err := Simulate(loans, budget, g)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, nil
}

func nameOf(loans []Position, id string) string {
	for _, l := range loans {
		if l.ID == id {
			return l.Name
		}
	}
	return ""
}
