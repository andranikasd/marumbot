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
//
// The layout is fixed so the eye learns where to look: a header with the three
// figures that frame every plan, this month's steps as a dated checklist, the
// result, and only then the reasoning. The currency is named once in the
// header; every amount below it is bare, so a line reads as a number rather
// than as a sentence about a number.
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
		return w.Send.SendMessage(ctx, chat, i18n.T(l, "loans.none"), w.addMarkup(l))
	}

	budget, err := w.Budgets.Budget(ctx, userID)
	if err != nil {
		return fmt.Errorf("reading the budget: %w", err)
	}

	// The refusals come before the header: a report that opens with three
	// figures and then says it cannot continue reads as a broken report, not as
	// a request for one more fact.
	switch {
	case !budget.Set:
		return w.Send.SendMessage(ctx, chat, i18n.T(l, "advice.set_budget"), w.budgetMarkup(l))
	case budget.Currency != cur.Code:
		return w.Send.SendMessage(ctx, chat,
			i18n.T(l, "advice.currency_mismatch", budget.Currency, cur.Code), w.budgetMarkup(l))
	case budget.Monthly.Cmp(required) < 0:
		return w.Send.SendMessage(ctx, chat,
			i18n.T(l, "budget.too_low", bare(budget.Monthly), bare(required)), w.budgetMarkup(l))
	}

	var b strings.Builder
	b.WriteString("<b>" + i18n.T(l, "advice.title") + "</b>")
	if compare {
		b.WriteString(" · " + i18n.T(l, "advice.compare_title"))
	} else {
		b.WriteString(" · " + i18n.T(l, goalKey(goal)))
	}
	b.WriteString("\n<i>" + i18n.T(l, "advice.currency", cur.Code) + "</i>\n\n")
	figure(&b, l, "advice.owed", owed)
	figure(&b, l, "advice.required", required)
	figure(&b, l, "advice.budget", budget.Monthly)
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

	b.WriteString("\n<b>" + i18n.T(l, "advice.this_month") + "</b>\n")
	writeActions(&b, l, o.Actions)
	b.WriteString(i18n.T(l, "advice.remaining", bare(o.NextMonthOwed)) + "\n")

	b.WriteString("\n<b>" + i18n.T(l, "advice.result") + "</b>\n")
	writeResult(&b, l, rep, positions, cash)
	if o.ClearedFirst != "" {
		b.WriteString(i18n.T(l, "advice.first_clear", html.EscapeString(o.ClearedFirst), o.ClearedMonth) + "\n")
	}

	b.WriteString("\n<b>" + i18n.T(l, "advice.how") + "</b>\n")
	writeSearchSummary(&b, l, rep)

	b.WriteString("\n<i>" + i18n.T(l, "advice.pick") + "</i>")
	return w.Send.SendMessage(ctx, chat, b.String(), goalMenu(l))
}

// compareGoals answers every goal at once, with the minimum as the floor, so
// the reader sees what each choice costs in months, interest and monthly
// outflow rather than three reports that each look reasonable alone.
//
// Each goal is two lines: its name, then one monospace line of figures, so the
// four blocks scan as a table without a table.
func (w *Worker) compareGoals(ctx context.Context, chat int64, l i18n.Locale, b *strings.Builder, positions []plan.Position, cash plan.Cash) error {
	var first *plan.Report
	for _, g := range []plan.Goal{plan.PayLeastInterest, plan.FinishSoonest, plan.FreeUpMonthly} {
		rep, err := plan.Search(positions, cash, g)
		if err != nil {
			return fmt.Errorf("searching %s: %w", g, err)
		}
		if first == nil {
			first = &rep
		}
		writeRow(b, l, i18n.T(l, goalKey(g)), rep.Best)
	}
	writeRow(b, l, i18n.T(l, "advice.minimum"), first.Minimum)

	if len(first.Ties) > 0 {
		b.WriteString("\n<i>" + i18n.T(l, "advice.ties_intro") + "</i>\n")
		for _, t := range first.Ties {
			b.WriteString("• " + i18n.T(l, tieKey(t)) + "\n")
		}
	}
	b.WriteString("\n<i>" + i18n.T(l, "advice.compare_pick") + "</i>")
	return w.Send.SendMessage(ctx, chat, b.String(), goalMenu(l))
}

// writeRow is one block of the comparison: the name, then the figures.
func writeRow(b *strings.Builder, l i18n.Locale, name string, o plan.Result) {
	b.WriteString("\n<b>" + name + "</b>\n")
	b.WriteString(i18n.T(l, "advice.row", o.Months, bare(o.TotalInterest)) + "\n")
	if o.FinalMonthly.Cmp(o.PeakMonthly) < 0 && o.ReliefMonth > 0 {
		b.WriteString(i18n.T(l, "advice.row_relief", bare(o.PeakMonthly), bare(o.FinalMonthly), o.ReliefMonth) + "\n")
	} else {
		b.WriteString(i18n.T(l, "advice.row_flat", bare(o.PeakMonthly)) + "\n")
	}
}

// writeResult is the block that answers the goal's own question: months and
// interest for every goal, then the comparison that goal is measured by.
func writeResult(b *strings.Builder, l i18n.Locale, rep plan.Report, positions []plan.Position, cash plan.Cash) {
	o, m := rep.Best, rep.Minimum
	b.WriteString(i18n.T(l, "advice.months_interest", o.Months, bare(o.TotalInterest)) + "\n")
	switch rep.Goal {
	case plan.FinishSoonest:
		if len(rep.Ladder) > 1 {
			b.WriteString(i18n.T(l, "advice.ladder_intro") + "\n")
			for _, r := range rep.Ladder[1:] {
				b.WriteString(i18n.T(l, "advice.ladder", bare(r.Budget), r.Months, bare(r.Interest)) + "\n")
			}
		}
		if target := o.Months / 2; target >= 1 {
			if need, err := plan.BudgetFor(positions, cash, o.Policy, target); err == nil {
				b.WriteString(i18n.T(l, "advice.budget_for", target, bare(need)) + "\n")
			}
		}
	case plan.FreeUpMonthly:
		if o.ReliefMonth > 0 {
			b.WriteString(i18n.T(l, "advice.relief.head", bare(o.PeakMonthly), bare(o.FinalMonthly), o.ReliefMonth) + "\n")
		} else {
			b.WriteString(i18n.T(l, "advice.relief.none") + "\n")
		}
		if d, err := o.TotalInterest.Sub(rep.Avalanche.TotalInterest); err == nil && d.Sign() > 0 {
			b.WriteString(i18n.T(l, "advice.relief.vs_cheapest", bare(d), rep.Avalanche.Months) + "\n")
		}
	default:
		if saved, err := m.TotalInterest.Sub(o.TotalInterest); err == nil && saved.Sign() > 0 {
			b.WriteString(i18n.T(l, "advice.vs_minimum", bare(saved), m.Months-o.Months) + "\n")
		}
	}
}

// writeActions lists the first month as a dated checklist: what to pay, when,
// to which loan, and what an early payment saves.
func writeActions(b *strings.Builder, l i18n.Locale, acts []plan.Action) {
	var extra int
	for _, a := range acts {
		name := html.EscapeString(a.Loan)
		switch {
		case a.Extra && a.Saves.Sign() > 0:
			b.WriteString(i18n.T(l, "advice.step_early", a.On.String(), bare(a.Amount), name, bare(a.Saves)) + "\n")
			extra++
		case a.Extra:
			b.WriteString(i18n.T(l, "advice.step_extra", a.On.String(), bare(a.Amount), name) + "\n")
			extra++
		default:
			b.WriteString(i18n.T(l, "advice.step_due", a.On.String(), bare(a.Amount), name) + "\n")
		}
	}
	if extra == 0 {
		b.WriteString(i18n.T(l, "advice.no_surplus") + "\n")
	}
}

// writeSearchSummary says how the answer was found and what it beat. Only the
// lines that carry a number or a caveat: the reader wants to trust the plan,
// not to audit the search.
func writeSearchSummary(b *strings.Builder, l i18n.Locale, rep plan.Report) {
	if rep.Exhaustive {
		b.WriteString(i18n.T(l, "advice.evaluated", rep.Evaluated) + "\n")
	} else {
		b.WriteString(i18n.T(l, "advice.evaluated_named", rep.Evaluated) + "\n")
	}
	if rep.Goal != plan.FreeUpMonthly {
		if d, err := rep.Avalanche.TotalInterest.Sub(rep.Best.TotalInterest); err == nil && d.Sign() > 0 {
			b.WriteString(i18n.T(l, "advice.vs_avalanche", bare(d)) + "\n")
		}
		if d, err := rep.Snowball.TotalInterest.Sub(rep.Best.TotalInterest); err == nil && d.Sign() > 0 {
			b.WriteString(i18n.T(l, "advice.vs_snowball", bare(d)) + "\n")
		}
	}
	if rep.TimingSaving.Sign() > 0 {
		b.WriteString(i18n.T(l, "advice.timing", bare(rep.TimingSaving)) + "\n")
	}
	b.WriteString("<i>" + i18n.T(l, effectKey(rep.Best.Policy.Effect)) + "</i>\n")
	for _, t := range rep.Ties {
		b.WriteString("• " + i18n.T(l, tieKey(t)) + "\n")
	}
}

// figure writes one header line: a label and a monospace amount.
func figure(b *strings.Builder, l i18n.Locale, key string, a money.Amount) {
	b.WriteString(i18n.T(l, key) + ": <code>" + bare(a) + "</code>\n")
}

// bare renders an amount without its currency code. The report names the
// currency once at the top; repeating it on every line buries the number.
func bare(a money.Amount) string {
	return strings.TrimSuffix(a.String(), " "+a.Currency().Code)
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
