package app

import (
	"context"
	"errors"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/andranikasd/marumbot/internal/i18n"
	"github.com/andranikasd/marumbot/pkg/core/amortisation"
	"github.com/andranikasd/marumbot/pkg/core/date"
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
	loans, err := w.Loans.LoansForUser(ctx, userID, plan.MaxLoans+1)
	if err != nil {
		return fmt.Errorf("listing loans: %w", err)
	}
	if len(loans) > plan.MaxLoans {
		return w.Send.SendMessage(ctx, chat, i18n.T(l, "advice.refuse.too_many", plan.MaxLoans), w.mainMenu(l))
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

	in := plan.Input{
		ValuationDate: date.From(w.Clock.Now(), time.UTC),
		Cash:          plan.CashPlan{Monthly: budget.Monthly, PayDay: budget.PayDay},
		Loans:         positions,
	}
	u, err := plan.Explore(in)
	if err != nil {
		return w.refuse(ctx, chat, l, err)
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
	if budget.PayDay > 0 {
		b.WriteString(i18n.T(l, "advice.payday", budget.PayDay) + "\n")
	}
	if n := assumedTotal(u); n > 0 {
		b.WriteString("<i>" + i18n.T(l, "advice.assumed", n) + "</i>\n")
	}

	if compare {
		return w.compareGoals(ctx, chat, l, &b, u, required)
	}

	rep, err := u.Rank(goal)
	if err != nil {
		return w.refuse(ctx, chat, l, err)
	}
	o := rep.Best

	b.WriteString("\n<b>" + i18n.T(l, "advice.this_month") + "</b>\n")
	writeActions(&b, l, o.Actions)
	b.WriteString(i18n.T(l, "advice.remaining", bare(o.NextMonthOwed)) + "\n")

	b.WriteString("\n<b>" + i18n.T(l, "advice.result") + "</b>\n")
	writeResult(&b, l, rep, in, required)
	if o.FirstClear != "" {
		b.WriteString(i18n.T(l, "advice.first_clear", html.EscapeString(o.FirstClear), o.FirstClearOn.String()) + "\n")
	}

	b.WriteString("\n<b>" + i18n.T(l, "advice.how") + "</b>\n")
	writeSearchSummary(&b, l, rep)

	b.WriteString("\n<i>" + i18n.T(l, "advice.pick") + "</i>")
	return w.Send.SendMessage(ctx, chat, b.String(), goalMenu(l))
}

// refuse maps a typed engine refusal to a message that says what would have
// to change. Anything untyped is a fault, and is returned to be logged and
// retried rather than explained away.
func (w *Worker) refuse(ctx context.Context, chat int64, l i18n.Locale, err error) error {
	var inf *plan.InfeasibleError
	var un *plan.UnsupportedError
	var tr *plan.TruncatedError
	var mc *plan.MixedCurrencyError
	switch {
	case errors.As(err, &inf):
		return w.Send.SendMessage(ctx, chat,
			i18n.T(l, "advice.refuse.infeasible", inf.On.String(), bare(inf.Required), bare(inf.Shortfall)), w.budgetMarkup(l))
	case errors.As(err, &un):
		return w.Send.SendMessage(ctx, chat, i18n.T(l, "advice.refuse.unsupported", un.Feature), w.mainMenu(l))
	case errors.As(err, &tr):
		return w.Send.SendMessage(ctx, chat, i18n.T(l, "advice.refuse.too_many", tr.Max), w.mainMenu(l))
	case errors.As(err, &mc):
		return w.Send.SendMessage(ctx, chat, i18n.T(l, "advice.currency_mismatch", mc.Want, mc.Have), w.budgetMarkup(l))
	case errors.Is(err, plan.ErrHorizon):
		return w.Send.SendMessage(ctx, chat, i18n.T(l, "advice.refuse.horizon"), w.mainMenu(l))
	case errors.Is(err, plan.ErrInvariant):
		w.Log.ErrorContext(ctx, "engine invariant violated", "error", err)
		return w.Send.SendMessage(ctx, chat, i18n.T(l, "advice.refuse.calculation"), w.mainMenu(l))
	}
	return fmt.Errorf("planning: %w", err)
}

func assumedTotal(u *plan.Universe) int {
	n := 0
	if len(u.Results) > 0 {
		for _, k := range u.Results[0].Assumed {
			n += k
		}
	}
	return n
}

// compareGoals answers every goal at once over the same simulated policies,
// with the minimum as the floor.
func (w *Worker) compareGoals(ctx context.Context, chat int64, l i18n.Locale, b *strings.Builder, u *plan.Universe, required money.Amount) error {
	goals := []plan.Goal{{Kind: plan.LeastInterest}, {Kind: plan.Fastest}, {Kind: plan.FirstWin}}
	// Relief needs a target; for the comparison, use "get under half of
	// today's required total", which is the question most people mean.
	half := money.FromMinor(required.Minor()/2, required.Currency())
	goals = append(goals, plan.Goal{Kind: plan.Relief, Cap: half})
	var first *plan.Report
	for _, g := range goals {
		rep, err := u.Rank(g)
		if err != nil {
			return w.refuse(ctx, chat, l, err)
		}
		if first == nil {
			r := rep
			first = &r
		}
		name := i18n.T(l, goalKey(g))
		if g.Kind == plan.Relief {
			name += " ≤ " + bare(half)
		}
		writeRow(b, l, name, rep, required)
	}
	writeRow(b, l, i18n.T(l, "advice.minimum"), plan.Report{Goal: plan.Goal{}, Best: first.Minimum}, required)

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
func writeRow(b *strings.Builder, l i18n.Locale, name string, rep plan.Report, required money.Amount) {
	o := rep.Best
	b.WriteString("\n<b>" + name + "</b>\n")
	b.WriteString(i18n.T(l, "advice.row", o.PayoffDate.String(), bare(o.Cost())) + "\n")
	if rep.Goal.Kind == plan.Relief {
		if m := plan.ReliefMonth(rep.Goal, required, o); m < 1<<29 {
			b.WriteString(i18n.T(l, "advice.row_relief", bare(o.PeakRequired), bare(o.FinalRequired), m) + "\n")
		} else {
			b.WriteString(i18n.T(l, "advice.relief.none") + "\n")
		}
	} else {
		b.WriteString(i18n.T(l, "advice.row_flat", bare(o.PeakRequired)) + "\n")
	}
}

// writeResult answers the goal's own question.
func writeResult(b *strings.Builder, l i18n.Locale, rep plan.Report, in plan.Input, required money.Amount) {
	o, m := rep.Best, rep.Minimum
	b.WriteString(i18n.T(l, "advice.months_interest", o.PayoffDate.String(), o.Months, bare(o.TotalInterest)) + "\n")
	if o.TotalFees.Sign() > 0 {
		b.WriteString(i18n.T(l, "advice.fees", bare(o.TotalFees)) + "\n")
	}
	switch rep.Goal.Kind {
	case plan.Fastest:
		if len(rep.Ladder) > 1 {
			b.WriteString(i18n.T(l, "advice.ladder_intro") + "\n")
			for _, r := range rep.Ladder[1:] {
				b.WriteString(i18n.T(l, "advice.ladder", bare(r.Budget), r.Payoff.String(), bare(r.Interest)) + "\n")
			}
		}
		if o.Months >= 2 {
			by := date.AddMonths(in.ValuationDate, o.Months/2)
			if need, err := plan.BudgetFor(in, o.Policy, by); err == nil {
				b.WriteString(i18n.T(l, "advice.budget_for", by.String(), bare(need)) + "\n")
			}
		}
	case plan.Relief:
		if mo := plan.ReliefMonth(rep.Goal, required, o); mo < 1<<29 {
			b.WriteString(i18n.T(l, "advice.relief.head", bare(o.PeakRequired), bare(o.FinalRequired), mo) + "\n")
		} else {
			b.WriteString(i18n.T(l, "advice.relief.none") + "\n")
		}
		if d, err := o.Cost().Sub(rep.Avalanche.Cost()); err == nil && d.Sign() > 0 {
			b.WriteString(i18n.T(l, "advice.relief.vs_cheapest", bare(d), rep.Avalanche.PayoffDate.String()) + "\n")
		}
	default:
		if saved, err := m.Cost().Sub(o.Cost()); err == nil && saved.Sign() > 0 {
			b.WriteString(i18n.T(l, "advice.vs_minimum", bare(saved), m.Months-o.Months) + "\n")
		}
	}
}

// writeActions lists the first cycle as a dated checklist.
func writeActions(b *strings.Builder, l i18n.Locale, acts []plan.Action) {
	var extra int
	for _, a := range acts {
		name := html.EscapeString(a.Loan)
		switch {
		case a.Kind == plan.Extra && a.Saves.Sign() > 0:
			b.WriteString(i18n.T(l, "advice.step_early", a.On.String(), bare(a.Amount), name, bare(a.Saves)) + "\n")
			extra++
		case a.Kind == plan.Extra:
			b.WriteString(i18n.T(l, "advice.step_extra", a.On.String(), bare(a.Amount), name) + "\n")
			extra++
		default:
			b.WriteString(i18n.T(l, "advice.step_due", a.On.String(), bare(a.Amount), name) + "\n")
		}
		if a.Fee.Sign() > 0 {
			b.WriteString("   " + i18n.T(l, "advice.step_fee", bare(a.Fee)) + "\n")
		}
	}
	if extra == 0 {
		b.WriteString(i18n.T(l, "advice.no_surplus") + "\n")
	}
}

// writeSearchSummary says how the answer was found and what it beat, in
// the borrower's language: one sentence for the search strength, the
// comparisons that carry a number, and the caveats.
func writeSearchSummary(b *strings.Builder, l i18n.Locale, rep plan.Report) {
	c := rep.Certificate
	b.WriteString(i18n.T(l, strengthKey(c.Strength), c.Policies) + "\n")
	if rep.Goal.Kind != plan.Relief {
		if d, err := rep.Avalanche.Cost().Sub(rep.Best.Cost()); err == nil && d.Sign() > 0 {
			b.WriteString(i18n.T(l, "advice.vs_avalanche", bare(d)) + "\n")
		}
		if d, err := rep.Snowball.Cost().Sub(rep.Best.Cost()); err == nil && d.Sign() > 0 {
			b.WriteString(i18n.T(l, "advice.vs_snowball", bare(d)) + "\n")
		}
	}
	if rep.TimingSaving.Sign() > 0 {
		b.WriteString(i18n.T(l, "advice.timing", bare(rep.TimingSaving)) + "\n")
	}
	if e, ok := uniformEffectOf(rep.Best.Policy); ok {
		b.WriteString("<i>" + i18n.T(l, effectKey(e)) + "</i>\n")
	} else {
		b.WriteString("<i>" + i18n.T(l, "advice.effect.mixed") + "</i>\n")
	}
	for _, t := range rep.Ties {
		b.WriteString("• " + i18n.T(l, tieKey(t)) + "\n")
	}
	if unconfirmed(rep) {
		b.WriteString("• " + i18n.T(l, "advice.trust_caveat") + "\n")
	}
}

// unconfirmed reports whether any figure rests on what the borrower typed
// rather than on a bank-confirmed balance.
func unconfirmed(rep plan.Report) bool {
	for _, p := range rep.Certificate.Positions {
		if p.Trust != "bank_confirmed" && p.Trust != "imported_verified" {
			return true
		}
	}
	return false
}

func uniformEffectOf(p plan.Policy) (model.PrepaymentEffect, bool) {
	if len(p.Effect) == 0 {
		return model.PrepayBorrowerChooses, true
	}
	for _, e := range p.Effect[1:] {
		if e != p.Effect[0] {
			return 0, false
		}
	}
	return p.Effect[0], true
}

func strengthKey(s plan.Strength) string {
	switch s {
	case plan.ProvenOptimal:
		return "advice.strength.proven"
	case plan.ExhaustiveStaticOrder:
		return "advice.strength.exhaustive"
	case plan.NamedStrategiesOnly:
		return "advice.strength.named"
	default:
		return "advice.strength.bounded"
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
			Balance: ln.Balance, From: ln.AsOf, Excess: ln.Excess, Trust: ln.Trust,
		})
	}
	return out, owed, required, cur, nil
}

func goalKey(g plan.Goal) string {
	switch g.Kind {
	case plan.Fastest:
		return "goal.soonest"
	case plan.Relief:
		return "goal.relief"
	case plan.FirstWin:
		return "goal.first"
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
			{keyText: i18n.T(l, "goal.first"), keyCallback: "goal:first"},
		},
		{
			{keyText: i18n.T(l, "advice.compare"), keyCallback: "goal:compare"},
		},
	}}
}

// RequiredThisMonth sums the next instalment of every active loan, using the
// same projection the advice report uses so the two cannot disagree.
func (w *Worker) RequiredThisMonth(ctx context.Context, userID string) (money.Amount, money.Currency, error) {
	loans, err := w.Loans.LoansForUser(ctx, userID, plan.MaxLoans+1)
	if err != nil {
		return money.Amount{}, money.Currency{}, err
	}
	_, _, required, cur, err := w.positions(ctx, loans)
	return required, cur, err
}
