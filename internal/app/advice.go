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

// advise answers "what should I do about this".
//
// It is the only place the whole engine is used at once: each loan is projected
// to find what it contractually requires this month, and the planner decides
// where any surplus should go. Everything it says is derived here and now from
// the contracts and the anchors -- nothing is stored, so nothing can be stale
// in a way the user cannot see.
func (w *Worker) advise(ctx context.Context, userID string, chat int64, l i18n.Locale) error {
	loans, err := w.Loans.LoansForUser(ctx, userID, 25)
	if err != nil {
		return fmt.Errorf("listing loans: %w", err)
	}
	if len(loans) == 0 {
		return w.Send.SendMessage(ctx, chat, i18n.T(l, "loans.none"), w.mainMenu(l))
	}

	// Group by currency. A budget in dram cannot be allocated across a loan in
	// dollars without an exchange rate, and there is no validated source for
	// one, so each currency is planned on its own.
	byCurrency := map[string][]plan.Loan{}
	totals := map[string]money.Amount{}
	required := map[string]money.Amount{}
	names := map[string]string{}

	for _, ln := range loans {
		if ln.Balance.Sign() <= 0 {
			continue
		}
		code := ln.Contract.Currency.Code
		names[ln.ID] = ln.Name

		s, err := amortisation.Build(ln.Contract, ln.Balance, ln.AsOf)
		if err != nil || len(s.Rows) == 0 {
			w.Log.WarnContext(ctx, "cannot project a loan for advice", "loan", ln.ID, "error", err)
			continue
		}
		if _, ok := totals[code]; !ok {
			totals[code] = money.Zero(ln.Contract.Currency)
			required[code] = money.Zero(ln.Contract.Currency)
		}
		if totals[code], err = totals[code].Add(ln.Balance); err != nil {
			return err
		}
		if required[code], err = required[code].Add(s.Rows[0].Payment); err != nil {
			return err
		}
		byCurrency[code] = append(byCurrency[code], plan.Loan{
			ID: ln.ID, Balance: ln.Balance,
			Rate: ln.Contract.NominalRate, Required: s.Rows[0].Payment,
		})
	}
	if len(byCurrency) == 0 {
		return w.Send.SendMessage(ctx, chat, i18n.T(l, "advice.nothing"), w.mainMenu(l))
	}

	budget, err := w.Budgets.Budget(ctx, userID)
	if err != nil {
		return fmt.Errorf("reading the budget: %w", err)
	}

	var b strings.Builder
	b.WriteString("<b>" + i18n.T(l, "advice.title") + "</b>\n")

	for code, ls := range byCurrency {
		b.WriteString("\n" + i18n.T(l, "advice.owed", totals[code].String()) + "\n")
		b.WriteString(i18n.T(l, "advice.required", required[code].String()) + "\n")

		if !budget.Set || budget.Currency != code {
			// Without a budget there is nothing to allocate, and inventing one
			// would be advice about a number the user never gave.
			b.WriteString("\n" + i18n.T(l, "advice.set_budget") + "\n")
			continue
		}

		p, err := plan.Allocate(ls, budget.Monthly, plan.PayLeastInterest)
		if err != nil {
			// The common case is a budget below the contractual minimums, which
			// is a real answer rather than a failure: it says the plan cannot
			// be met without arrears.
			b.WriteString("\n" + i18n.T(l, "budget.too_low", required[code].String()) + "\n")
			continue
		}
		b.WriteString("\n" + i18n.T(l, "advice.budget", budget.Monthly.String()) + "\n")
		if p.Target == "" {
			b.WriteString(i18n.T(l, "advice.no_surplus") + "\n")
			continue
		}
		b.WriteString(i18n.T(l, "advice.target",
			html.EscapeString(names[p.Target]), p.Surplus.String()) + "\n")
		b.WriteString("<i>" + i18n.T(l, "advice.why") + "</i>\n")
	}

	return w.Send.SendMessage(ctx, chat, b.String(), w.mainMenu(l))
}
