package app

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"strings"
	"time"

	"github.com/andranikasd/marumbot/internal/i18n"
	"github.com/andranikasd/marumbot/pkg/core/amortisation"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/money"
)

// The paid callback opens Quick Record when a Mini App is configured.
// An acknowledgement alone never creates a payment. The legacy chat-only
// fallback asks for a bank balance and records only that separate statement.

// askPaidBalance is the button's landing: remember which loan the answer
// settles, then ask for the bank's figure.
func (w *Worker) askPaidBalance(ctx context.Context, userID string, chat int64, l i18n.Locale, loanID string) error {
	if w.Editor == nil {
		return w.Send.SendMessage(ctx, chat, i18n.T(l, "error.generic"), w.mainMenu(l))
	}
	// Ownership first: a forged callback with someone else's loan id must
	// die here, not at the write.
	ln, err := w.Editor.LoanForUser(ctx, loanID, userID)
	if err != nil {
		return w.refuse(ctx, chat, l, err)
	}
	if w.MiniApp != "" {
		markup := map[string]any{keyInline: [][]map[string]any{{webAppButton(i18n.T(l, "payment.record"), w.miniURL("payment")+"&id="+url.QueryEscape(loanID))}}}
		return w.Send.SendMessage(ctx, chat, i18n.T(l, "payment.prompt", html.EscapeString(ln.Name)), markup)
	}
	if w.Convos == nil || w.Balances == nil {
		return w.Send.SendMessage(ctx, chat, i18n.T(l, "error.generic"), w.mainMenu(l))
	}
	if err := w.Convos.SetState(ctx, userID, StateAwaitingBalance+":"+loanID); err != nil {
		return fmt.Errorf("recording the question: %w", err)
	}
	text := "<b>" + i18n.T(l, "paid.title") + "</b>\n" + i18n.T(l, "paid.ask_balance", html.EscapeString(ln.Name))
	markup := map[string]any{keyInline: [][]map[string]any{{
		{keyText: i18n.T(l, "paid.skip_button"), keyCallback: "paidskip"},
	}}}
	return w.Send.SendMessage(ctx, chat, text, markup)
}

// skipPaidBalance dismisses the balance question without creating a fact.
func (w *Worker) skipPaidBalance(ctx context.Context, userID string, chat int64, l i18n.Locale) error {
	if w.Convos != nil {
		if err := w.Convos.ClearState(ctx, userID); err != nil {
			w.Log.WarnContext(ctx, "clearing the conversation state failed", "error", err)
		}
	}
	return w.Send.SendMessage(ctx, chat, i18n.T(l, "paid.skipped"), w.mainMenu(l))
}

// takeBalance interprets the typed figure. Returns false when the text is not
// an amount, so an unrelated message is not swallowed as a malformed answer.
func (w *Worker) takeBalance(ctx context.Context, userID string, chat int64, l i18n.Locale, state, text string) (bool, error) {
	loanID := strings.TrimPrefix(state, StateAwaitingBalance+":")
	if loanID == "" || w.Balances == nil || w.Editor == nil {
		return false, nil
	}
	ln, err := w.Editor.LoanForUser(ctx, loanID, userID)
	if err != nil {
		// The loan is gone -- archived since the question was asked. Clear the
		// question rather than trap the user in it.
		if w.Convos != nil {
			_ = w.Convos.ClearState(ctx, userID)
		}
		return true, w.refuse(ctx, chat, l, err)
	}

	// parseAmount refuses zero, rightly, everywhere else -- a zero budget is
	// nonsense. Here zero is the best possible answer: the loan is settled.
	minor, cur, ok := int64(0), ln.Contract.Currency, strings.TrimSpace(text) == "0"
	if !ok {
		minor, cur, ok = parseAmount(text, ln.Contract.Currency)
	}
	if !ok {
		return false, nil
	}
	if cur.Code != ln.Contract.Currency.Code {
		// A dollar figure cannot anchor a dram loan; say so instead of
		// storing a number that means something else.
		return true, w.Send.SendMessage(ctx, chat,
			i18n.T(l, "paid.wrong_currency", ln.Contract.Currency.Code), w.mainMenu(l))
	}
	// Zero is a real answer: the loan is settled. Anything at or above the
	// previous balance is likely a typo of intent -- a payment reduces what
	// is owed -- but the borrower knows fees exist, so it is stored as said.
	asOf := date.From(w.Clock.Now(), time.UTC)
	if err := w.Balances.RecordBalance(ctx, loanID, userID, minor, asOf.String()); err != nil {
		return true, fmt.Errorf("recording the balance: %w", err)
	}
	if w.Convos != nil {
		if err := w.Convos.ClearState(ctx, userID); err != nil {
			w.Log.WarnContext(ctx, "clearing the conversation state failed", "error", err)
		}
	}

	balance := money.FromMinor(minor, cur)
	if minor == 0 {
		return true, w.Send.SendMessage(ctx, chat,
			i18n.T(l, "paid.cleared", html.EscapeString(ln.Name)), w.mainMenu(l))
	}
	msg := "<b>" + i18n.T(l, "paid.updated") + "</b>\n<b>" + html.EscapeString(ln.Name) + "</b>\n"
	rows := [][2]string{{i18n.T(l, "fig.balance"), balance.String()}}
	// The next instalment from the new anchor, when it can be projected: the
	// confirmation shows the consequence, not just the receipt.
	if s, err := amortisation.Build(ln.Contract, balance, asOf); !ln.UnreconciledPayments && err == nil && len(s.Rows) > 0 {
		rows = append(rows, [2]string{i18n.T(l, "fig.next"), shortDate(l, s.Rows[0].Due, asOf) + "  " + bare(s.Rows[0].Payment)})
	}
	msg += strings.TrimRight(figures(rows), "\n")
	return true, w.Send.SendMessage(ctx, chat, w.withTip(ctx, userID, l, msg), w.mainMenu(l))
}

// paidMarkup is the reminder's keyboard: one button, the loan it is about.
func paidMarkup(l i18n.Locale, loanID string) any {
	return map[string]any{keyInline: [][]map[string]any{{
		{keyText: i18n.T(l, "reminder.paid_button"), keyCallback: "paid:" + loanID},
	}}}
}
