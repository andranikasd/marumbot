package app

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/andranikasd/marumbot/internal/i18n"
	"github.com/andranikasd/marumbot/pkg/core/money"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

// askBudget requests a monthly figure, and remembers that it asked.
//
// Both routes are offered: a form, and simply typing the number. The form is
// better on a phone; the reply is better when the form does not open, which is
// not a hypothetical -- a user typed 100000 at this prompt, nothing was
// listening, and the bot answered with its help text.
func (w *Worker) askBudget(ctx context.Context, userID string, chat int64, l i18n.Locale) error {
	if err := w.Convos.SetState(ctx, userID, StateAwaitingBudget); err != nil {
		return fmt.Errorf("recording the question: %w", err)
	}
	text := i18n.T(l, "budget.prompt_or_type")
	// The figure the answer has to clear, so it is chosen against a fact.
	if req, _, err := w.RequiredThisMonth(ctx, userID); err == nil && req.Sign() > 0 {
		text = i18n.T(l, "budget.required_hint", req.String()) + "\n\n" + text
	}
	return w.Send.SendMessage(ctx, chat, text, w.budgetMarkup(l))
}

// takeBudget interprets a reply to that question.
//
// Returns false when the text is not a number, so an ordinary message during a
// pending question is still treated as an ordinary message rather than
// swallowed as a malformed answer.
//
// The confirmation says what the figure means: the surplus over this month's
// instalments and a button to the plan, or a warning that it does not cover
// them. A bare "set" leaves the reader to find out from the next report.
func (w *Worker) takeBudget(ctx context.Context, userID string, chat int64, l i18n.Locale, text string) (bool, error) {
	minor, cur, ok := parseAmount(text, w.DefaultCurrency)
	if !ok {
		return false, nil
	}
	if err := w.Budgets.SetBudget(ctx, userID, cur.Code, minor, 0); err != nil {
		return true, fmt.Errorf("recording the budget: %w", err)
	}
	if err := w.Convos.ClearState(ctx, userID); err != nil {
		w.Log.WarnContext(ctx, "clearing the conversation state failed", "error", err)
	}

	monthly := money.FromMinor(minor, cur)
	req, reqCur, err := w.RequiredThisMonth(ctx, userID)
	if err != nil || req.Sign() <= 0 || reqCur.Code != cur.Code {
		return true, w.Send.SendMessage(ctx, chat, i18n.T(l, "budget.set", monthly.String()), w.mainMenu(l))
	}
	if monthly.Cmp(req) < 0 {
		return true, w.Send.SendMessage(ctx, chat,
			i18n.T(l, "budget.set_low", monthly.String(), req.String()), w.budgetMarkup(l))
	}
	msg := i18n.T(l, "budget.set", monthly.String())
	if surplus, err := monthly.Sub(req); err == nil && surplus.Sign() > 0 {
		msg += "\n" + i18n.T(l, "budget.set_surplus", surplus.String())
	}
	return true, w.Send.SendMessage(ctx, chat, w.withTip(ctx, userID, l, msg), planButton(l))
}

// planButton is the one tap from "budget saved" to "what do I do".
func planButton(l i18n.Locale) any {
	return map[string]any{keyInline: [][]map[string]any{{
		{keyText: i18n.T(l, "btn.plan"), keyCallback: "goal:cheapest"},
	}}}
}

// parseAmount reads a money figure the way a person writes one.
//
// "100000", "100 000", "100,000", "100000 AMD" and "100000֏" all mean the same
// thing, and refusing any of them makes the bot look pedantic about something
// it could simply have understood. A trailing currency code overrides the
// default, because a user with a dollar loan will write one.
func parseAmount(s string, def money.Currency) (int64, money.Currency, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, def, false
	}
	cur := def
	upper := strings.ToUpper(s)
	for _, code := range money.Codes() {
		if strings.HasSuffix(upper, code) {
			if c, err := money.Lookup(code); err == nil {
				cur = c
				s = strings.TrimSpace(s[:len(s)-len(code)])
			}
			break
		}
	}
	s = strings.TrimSuffix(strings.TrimSpace(s), "֏")

	var digits, frac strings.Builder
	seenSep := false
	for _, r := range s {
		switch {
		case unicode.IsDigit(r):
			if seenSep {
				frac.WriteRune(r)
			} else {
				digits.WriteRune(r)
			}
		case r == '.' || r == ',':
			// A comma is a thousands separator here far more often than a
			// decimal point, so it only counts as one when exactly two digits
			// follow -- which is what a decimal actually looks like.
			if !seenSep && looksDecimal(s, r) {
				seenSep = true
			}
		case r == ' ' || r == ' ' || r == '\'':
			// Thousands separators people actually type.
		default:
			return 0, def, false
		}
	}
	if digits.Len() == 0 {
		return 0, def, false
	}

	whole, err := parseInt64(digits.String())
	if err != nil {
		return 0, def, false
	}
	scale := int64(1)
	for i := uint8(0); i < cur.Exponent; i++ {
		scale *= 10
	}
	minor := whole * scale
	if f := frac.String(); f != "" {
		f = (f + "0000000000")[:cur.Exponent]
		if cur.Exponent > 0 {
			cents, err := parseInt64(f)
			if err != nil {
				return 0, def, false
			}
			minor += cents
		}
	}
	if minor <= 0 {
		return 0, def, false
	}
	return minor, cur, true
}

// looksDecimal reports whether the separator at hand is a decimal point rather
// than a thousands mark: exactly one or two digits follow it, and nothing else.
func looksDecimal(s string, sep rune) bool {
	i := strings.LastIndexFunc(s, func(r rune) bool { return r == sep })
	if i < 0 {
		return false
	}
	tail := s[i+len(string(sep)):]
	if tail == "" || len(tail) > 2 {
		return false
	}
	for _, r := range tail {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func parseInt64(s string) (int64, error) {
	var n int64
	for _, r := range s {
		d := int64(r - '0')
		if n > (1<<62)/10 {
			return 0, fmt.Errorf("amount too large")
		}
		n = n*10 + d
	}
	return n, nil
}

// askReliefCap asks what "pay less per month" should mean. A relief plan
// needs a target: the required monthly total the borrower wants to get
// under. Without one, "relief" would rank a one-dram reduction above a real
// one a month later.
func (w *Worker) askReliefCap(ctx context.Context, userID string, chat int64, l i18n.Locale) error {
	required, cur, err := w.RequiredThisMonth(ctx, userID)
	if err != nil {
		return err
	}
	if err := w.Convos.SetState(ctx, userID, StateAwaitingReliefCap); err != nil {
		return fmt.Errorf("recording the question: %w", err)
	}
	return w.Send.SendMessage(ctx, chat, i18n.T(l, "relief.prompt", required.String(), cur.Code), w.mainMenu(l))
}

// takeReliefCap interprets the reply and runs the relief plan.
func (w *Worker) takeReliefCap(ctx context.Context, userID string, chat int64, l i18n.Locale, text string) (bool, error) {
	minor, cur, ok := parseAmount(text, w.DefaultCurrency)
	if !ok {
		return false, nil
	}
	if err := w.Convos.ClearState(ctx, userID); err != nil {
		w.Log.WarnContext(ctx, "clearing the conversation state failed", "error", err)
	}
	return true, w.advise(ctx, userID, chat, l, plan.Goal{Kind: plan.Relief, Cap: money.FromMinor(minor, cur)}, false)
}
