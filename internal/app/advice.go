package app

import (
	"context"
	"fmt"
	"html"
	"strings"

	"github.com/andranikasd/marumbot/internal/i18n"
	"github.com/andranikasd/marumbot/pkg/core/amortisation"
	"github.com/andranikasd/marumbot/pkg/core/money"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

// advise produces the report a borrower actually wants: what to do, why, what
// it costs, and what is left afterwards.
//
// Everything is derived here and now from the contracts and the anchors.
// Nothing is stored, so nothing can be stale in a way the reader cannot see.
func (w *Worker) advise(ctx context.Context, userID string, chat int64, l i18n.Locale, goal plan.Goal, compare bool) error {
	loans, err := w.Loans.LoansForUser(ctx, userID, 25)
	if err != nil {
		return fmt.Errorf("listing loans: %w", err)
	}

	positions, owed, required, cur, err := w.positions(ctx, loans)
	if err != nil {
		return err
	}
	if len(positions) == 0 {
		return w.Send.SendMessage(ctx, chat, i18n.T(l, "loans.none"), w.mainMenu(l))
	}

	var b strings.Builder
	b.WriteString("<b>" + i18n.T(l, "advice.title") + "</b>\n")
	b.WriteString(i18n.T(l, "advice.owed", owed.String()) + "\n")
	b.WriteString(i18n.T(l, "advice.required", required.String()) + "\n")

	budget, err := w.Budgets.Budget(ctx, userID)
	if err != nil {
		return fmt.Errorf("reading the budget: %w", err)
	}
	if !budget.Set || budget.Currency != cur.Code {
		b.WriteString("\n" + i18n.T(l, "advice.set_budget"))
		return w.Send.SendMessage(ctx, chat, b.String(), w.mainMenu(l))
	}
	b.WriteString(i18n.T(l, "advice.budget", budget.Monthly.String()) + "\n")

	if budget.Monthly.Cmp(required) < 0 {
		b.WriteString("\n" + i18n.T(l, "budget.too_low", required.String()))
		return w.Send.SendMessage(ctx, chat, b.String(), w.mainMenu(l))
	}

	if compare {
		outs, err := plan.CompareAll(positions, budget.Monthly)
		if err != nil {
			return fmt.Errorf("comparing plans: %w", err)
		}
		b.WriteString("\n<b>" + i18n.T(l, "advice.compare") + "</b>\n")
		for _, o := range outs {
			b.WriteString("\n" + i18n.T(l, goalKey(o.Goal)) + "\n")
			b.WriteString(i18n.T(l, "advice.months", o.Months) + "\n")
			b.WriteString(i18n.T(l, "advice.interest", o.TotalInterest.String()) + "\n")
			if o.ClearedFirst != "" {
				b.WriteString(i18n.T(l, "advice.first_clear",
					html.EscapeString(o.ClearedFirst), o.ClearedMonth) + "\n")
			}
		}
		b.WriteString("\n<i>" + i18n.T(l, "advice.why") + "</i>")
		return w.Send.SendMessage(ctx, chat, b.String(), goalMenu(l))
	}

	o, err := plan.Simulate(positions, budget.Monthly, goal)
	if err != nil {
		return fmt.Errorf("simulating %s: %w", goal, err)
	}

	b.WriteString("\n<b>" + i18n.T(l, goalKey(goal)) + "</b>\n")
	if o.FirstTarget != "" && o.FirstExtra.Sign() > 0 {
		b.WriteString(i18n.T(l, "advice.do",
			o.FirstExtra.String(), html.EscapeString(o.FirstTarget)) + "\n")
	} else {
		b.WriteString(i18n.T(l, "advice.no_surplus") + "\n")
	}
	b.WriteString(i18n.T(l, "advice.remaining", o.NextMonthOwed.String()) + "\n")
	b.WriteString(i18n.T(l, "advice.months", o.Months) + "\n")
	b.WriteString(i18n.T(l, "advice.interest", o.TotalInterest.String()) + "\n")
	if o.ClearedFirst != "" {
		b.WriteString(i18n.T(l, "advice.first_clear",
			html.EscapeString(o.ClearedFirst), o.ClearedMonth) + "\n")
		if o.MonthlyFreed.Sign() > 0 {
			b.WriteString(i18n.T(l, "advice.frees", o.MonthlyFreed.String()) + "\n")
		}
	}
	b.WriteString("\n<i>" + i18n.T(l, "advice.why") + "</i>")
	return w.Send.SendMessage(ctx, chat, b.String(), goalMenu(l))
}

// positions turns stored loans into what the simulator needs, and totals what
// is owed and what this month contractually requires.
//
// Loans are grouped by currency only in the sense that a mismatch is refused: a
// dram budget cannot be allocated across a dollar loan without an exchange
// rate, and there is no validated source for one.
func (w *Worker) positions(ctx context.Context, loans []UserLoan) ([]plan.Position, money.Amount, money.Amount, money.Currency, error) {
	var (
		out      []plan.Position
		cur      money.Currency
		owed     money.Amount
		required money.Amount
		started  bool
	)
	for _, ln := range loans {
		if ln.Balance.Sign() <= 0 {
			continue
		}
		if !started {
			cur = ln.Contract.Currency
			owed, required = money.Zero(cur), money.Zero(cur)
			started = true
		}
		if ln.Contract.Currency.Code != cur.Code {
			w.Log.WarnContext(ctx, "skipping a loan in another currency",
				"loan", ln.ID, "currency", ln.Contract.Currency.Code)
			continue
		}
		s, err := amortisation.Build(ln.Contract, ln.Balance, ln.AsOf)
		if err != nil || len(s.Rows) == 0 {
			w.Log.WarnContext(ctx, "cannot project a loan", "loan", ln.ID, "error", err)
			continue
		}
		if owed, err = owed.Add(ln.Balance); err != nil {
			return nil, owed, required, cur, err
		}
		if required, err = required.Add(s.Rows[0].Payment); err != nil {
			return nil, owed, required, cur, err
		}
		out = append(out, plan.Position{
			ID: ln.ID, Name: ln.Name, Contract: ln.Contract,
			Balance: ln.Balance, From: ln.AsOf,
		})
	}
	return out, owed, required, cur, nil
}

func goalKey(g plan.Goal) string {
	switch g {
	case plan.FinishSoonest:
		return "goal.soonest"
	case plan.FreeUpMonthly:
		return "goal.relief"
	default:
		return "goal.cheapest"
	}
}

// goalMenu lets the reader change the question rather than accept the answer.
func goalMenu(l i18n.Locale) any {
	return map[string]any{keyInline: [][]map[string]any{
		{
			{keyText: i18n.T(l, "goal.cheapest"), keyCallback: "goal:cheapest"},
			{keyText: i18n.T(l, "goal.soonest"), keyCallback: "goal:soonest"},
		},
		{
			{keyText: i18n.T(l, "goal.relief"), keyCallback: "goal:relief"},
			{keyText: i18n.T(l, "advice.compare"), keyCallback: "goal:compare"},
		},
	}}
}

// RequiredThisMonth sums the next instalment of every active loan, using the
// same projection the advice report uses so the two cannot disagree.
func (w *Worker) RequiredThisMonth(ctx context.Context, userID string) (money.Amount, money.Currency, error) {
	loans, err := w.Loans.LoansForUser(ctx, userID, 25)
	if err != nil {
		return money.Amount{}, money.Currency{}, err
	}
	_, _, required, cur, err := w.positions(ctx, loans)
	return required, cur, err
}
