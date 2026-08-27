package app

import (
	"context"
	"fmt"
	"html"
	"strings"

	"github.com/andranikasd/marumbot/internal/i18n"
	"github.com/andranikasd/marumbot/pkg/core/amortisation"
	"github.com/andranikasd/marumbot/pkg/core/model"
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

	cash := plan.Cash{Monthly: budget.Monthly, Day: budget.PayDay}
	if budget.PayDay > 0 {
		b.WriteString(i18n.T(l, "advice.payday", budget.PayDay) + "\n")
	}

	if compare {
		return w.compareGoals(ctx, chat, l, &b, positions, cash)
	}

	rep, err := plan.Search(positions, cash, goal)
	if err != nil {
		return fmt.Errorf("searching %s: %w", goal, err)
	}
	o := rep.Best

	b.WriteString("\n<b>" + i18n.T(l, goalKey(goal)) + "</b>\n")
	writeHeadline(&b, l, rep, positions, cash)

	b.WriteString("\n<b>" + i18n.T(l, "advice.this_month") + "</b>\n")
	writeActions(&b, l, o.Actions)
	b.WriteString(i18n.T(l, "advice.remaining", o.NextMonthOwed.String()) + "\n")
	if o.ClearedFirst != "" {
		b.WriteString(i18n.T(l, "advice.first_clear", html.EscapeString(o.ClearedFirst), o.ClearedMonth) + "\n")
	}

	b.WriteString("\n<b>" + i18n.T(l, "advice.how") + "</b>\n")
	writeSearchSummary(&b, l, rep)
	return w.Send.SendMessage(ctx, chat, b.String(), goalMenu(l))
}

// compareGoals answers every goal at once, with the minimum as the floor, so
// the reader sees what each choice costs in months, interest and monthly
// outflow rather than three reports that each look reasonable alone.
func (w *Worker) compareGoals(ctx context.Context, chat int64, l i18n.Locale, b *strings.Builder, positions []plan.Position, cash plan.Cash) error {
	b.WriteString("\n<b>" + i18n.T(l, "advice.compare") + "</b>\n")
	var first *plan.Report
	for _, g := range []plan.Goal{plan.PayLeastInterest, plan.FinishSoonest, plan.FreeUpMonthly} {
		rep, err := plan.Search(positions, cash, g)
		if err != nil {
			return fmt.Errorf("searching %s: %w", g, err)
		}
		if first == nil {
			first = &rep
		}
		o := rep.Best
		b.WriteString("\n" + i18n.T(l, goalKey(g)) + "\n")
		b.WriteString(i18n.T(l, "advice.row", o.Months, o.TotalInterest.String()) + "\n")
		if o.FinalMonthly.Cmp(o.PeakMonthly) < 0 && o.ReliefMonth > 0 {
			b.WriteString(i18n.T(l, "advice.row_relief", o.PeakMonthly.String(), o.FinalMonthly.String(), o.ReliefMonth) + "\n")
		} else {
			b.WriteString(i18n.T(l, "advice.row_flat", o.PeakMonthly.String()) + "\n")
		}
	}
	m := first.Minimum
	b.WriteString("\n" + i18n.T(l, "advice.minimum") + "\n")
	b.WriteString(i18n.T(l, "advice.row", m.Months, m.TotalInterest.String()) + "\n")
	if len(first.Ties) > 0 {
		b.WriteString("\n<i>" + i18n.T(l, "advice.ties_intro") + "</i>\n")
		for _, t := range first.Ties {
			b.WriteString("• " + i18n.T(l, tieKey(t)) + "\n")
		}
	}
	return w.Send.SendMessage(ctx, chat, b.String(), goalMenu(l))
}

// writeHeadline is the one paragraph that answers the goal's own question.
func writeHeadline(b *strings.Builder, l i18n.Locale, rep plan.Report, positions []plan.Position, cash plan.Cash) {
	o, m := rep.Best, rep.Minimum
	switch rep.Goal {
	case plan.FinishSoonest:
		b.WriteString(i18n.T(l, "advice.soonest.head", o.Months, o.TotalInterest.String()) + "\n")
		for _, r := range rep.Ladder[1:] {
			b.WriteString(i18n.T(l, "advice.ladder", r.Budget.String(), r.Months, r.Interest.String()) + "\n")
		}
		if target := o.Months / 2; target >= 1 {
			if need, err := plan.BudgetFor(positions, cash, o.Policy, target); err == nil {
				b.WriteString(i18n.T(l, "advice.budget_for", target, need.String()) + "\n")
			}
		}
	case plan.FreeUpMonthly:
		if o.ReliefMonth > 0 {
			b.WriteString(i18n.T(l, "advice.relief.head", o.PeakMonthly.String(), o.FinalMonthly.String(), o.ReliefMonth) + "\n")
		} else {
			b.WriteString(i18n.T(l, "advice.relief.none") + "\n")
		}
		b.WriteString(i18n.T(l, "advice.relief.cost", o.Months, o.TotalInterest.String()) + "\n")
		if d, err := o.TotalInterest.Sub(rep.Avalanche.TotalInterest); err == nil && d.Sign() > 0 {
			b.WriteString(i18n.T(l, "advice.relief.vs_cheapest", d.String(), rep.Avalanche.Months) + "\n")
		}
	default:
		b.WriteString(i18n.T(l, "advice.cheapest.head", o.Months, o.TotalInterest.String()) + "\n")
		if saved, err := m.TotalInterest.Sub(o.TotalInterest); err == nil && saved.Sign() > 0 {
			b.WriteString(i18n.T(l, "advice.vs_minimum", saved.String(), m.Months-o.Months) + "\n")
		}
	}
}

// writeActions lists the first month as dated payments: what to pay, when, to
// which loan, and what an early payment saves.
func writeActions(b *strings.Builder, l i18n.Locale, acts []plan.Action) {
	var extra int
	for _, a := range acts {
		name := html.EscapeString(a.Loan)
		switch {
		case a.Extra && a.Saves.Sign() > 0:
			b.WriteString(i18n.T(l, "advice.step_early", a.On.String(), a.Amount.String(), name, a.Saves.String()) + "\n")
			extra++
		case a.Extra:
			b.WriteString(i18n.T(l, "advice.step_extra", a.On.String(), a.Amount.String(), name) + "\n")
			extra++
		default:
			b.WriteString(i18n.T(l, "advice.step_due", a.On.String(), a.Amount.String(), name) + "\n")
		}
	}
	if extra == 0 {
		b.WriteString(i18n.T(l, "advice.no_surplus") + "\n")
	}
}

// writeSearchSummary says how the answer was found and what it beat.
func writeSearchSummary(b *strings.Builder, l i18n.Locale, rep plan.Report) {
	if rep.Exhaustive {
		b.WriteString(i18n.T(l, "advice.evaluated", rep.Evaluated) + "\n")
	} else {
		b.WriteString(i18n.T(l, "advice.evaluated_named", rep.Evaluated) + "\n")
	}
	if rep.Goal != plan.FreeUpMonthly {
		if d, err := rep.Avalanche.TotalInterest.Sub(rep.Best.TotalInterest); err == nil && d.Sign() > 0 {
			b.WriteString(i18n.T(l, "advice.vs_avalanche", d.String()) + "\n")
		}
		if d, err := rep.Snowball.TotalInterest.Sub(rep.Best.TotalInterest); err == nil && d.Sign() > 0 {
			b.WriteString(i18n.T(l, "advice.vs_snowball", d.String()) + "\n")
		}
	}
	if rep.TimingSaving.Sign() > 0 {
		b.WriteString(i18n.T(l, "advice.timing", rep.TimingSaving.String()) + "\n")
	}
	b.WriteString(i18n.T(l, effectKey(rep.Best.Policy.Effect)) + "\n")
	for _, t := range rep.Ties {
		b.WriteString("• " + i18n.T(l, tieKey(t)) + "\n")
	}
}

func effectKey(e model.PrepaymentEffect) string {
	if e == model.PrepayReduceInstalment {
		return "advice.effect.reduce"
	}
	return "advice.effect.shorten"
}

// tieKey maps the engine's reasons to catalogue keys. The engine speaks
// English so its tests read; the borrower reads their own language.
func tieKey(reason string) string {
	switch {
	case strings.HasPrefix(reason, "one loan"):
		return "advice.tie.one_loan"
	case strings.HasPrefix(reason, "no surplus"):
		return "advice.tie.no_surplus"
	case strings.HasPrefix(reason, "the highest rate"):
		return "advice.tie.same_order"
	case strings.HasPrefix(reason, "no lender"):
		return "advice.rule_unknown"
	default:
		return "advice.set_payday"
	}
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
			Balance: ln.Balance, From: ln.AsOf, Excess: ln.Excess,
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
