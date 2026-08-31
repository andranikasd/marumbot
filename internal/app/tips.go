package app

import (
	"context"

	"github.com/andranikasd/marumbot/internal/i18n"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

// The journey. Every reply can end with one tip: the next thing worth doing,
// decided from what the account actually has, never from what it last did.
// One tip, not a list — a user follows a single nudge and ignores a lecture.
//
// The stages are ordered by what unlocks what: no loan means nothing can be
// planned; no budget means no plan; no payday means the plan cannot price
// early payment; an unconfirmed balance means every number carries a caveat.
// Past all four, the tips rotate through things worth knowing, keyed to the
// day so a returning user sees a different one without the bot storing
// anything.

type journeyStage uint8

const (
	stageNoLoans journeyStage = iota
	stageNoBudget
	stageNoPayday
	stageUnconfirmed
	stageCruising
)

// stage answers "what is this account missing". Errors degrade to the
// cruising stage: a tip is decoration, and a failed read must never break
// the message it decorates.
func (w *Worker) stage(ctx context.Context, userID string) journeyStage {
	loans, err := w.Loans.LoansForUser(ctx, userID, plan.MaxLoans+1)
	if err != nil {
		w.Log.WarnContext(ctx, "tip: listing loans failed", "error", err)
		return stageCruising
	}
	live := 0
	unconfirmed := false
	for _, l := range loans {
		if l.Balance.Sign() > 0 {
			live++
			if !l.Confirmed() {
				unconfirmed = true
			}
		}
	}
	if live == 0 {
		return stageNoLoans
	}
	budget, err := w.Budgets.Budget(ctx, userID)
	if err != nil {
		w.Log.WarnContext(ctx, "tip: reading the budget failed", "error", err)
		return stageCruising
	}
	switch {
	case !budget.Set:
		return stageNoBudget
	case budget.PayDay == 0:
		return stageNoPayday
	case unconfirmed:
		return stageUnconfirmed
	default:
		return stageCruising
	}
}

// proTips are what a fully set-up account is told, one per day, in a fixed
// order so the same day shows the same tip on every device.
var proTips = []string{
	"tip.pro.early",
	"tip.pro.compare",
	"tip.pro.first_win",
	"tip.pro.working",
	"tip.pro.update_balance",
	"tip.pro.relief",
}

// tip returns the next-step line for an account, ready to append to a
// message. The second value is false when the surrounding message already
// says the same thing and the tip should be omitted.
func (w *Worker) tip(ctx context.Context, userID string, l i18n.Locale) (string, journeyStage) {
	s := w.stage(ctx, userID)
	switch s {
	case stageNoLoans:
		return i18n.T(l, "tip.add"), s
	case stageNoBudget:
		return i18n.T(l, "tip.budget"), s
	case stageNoPayday:
		return i18n.T(l, "tip.payday"), s
	case stageUnconfirmed:
		return i18n.T(l, "tip.confirm"), s
	default:
		day := w.Clock.Now().YearDay()
		return i18n.T(l, proTips[day%len(proTips)]), s
	}
}

// withTip appends the journey tip to a message, unless the stage's own
// surface is the message being sent — a "set your budget" reply must not end
// with a tip to set the budget.
func (w *Worker) withTip(ctx context.Context, userID string, l i18n.Locale, text string, skip ...journeyStage) string {
	t, s := w.tip(ctx, userID, l)
	for _, sk := range skip {
		if s == sk {
			return text
		}
	}
	return text + "\n\n💡 <i>" + t + "</i>"
}

// OnLoanFiledMessage is the chat's answer to a loan created in the Mini App.
// The form closes and the conversation would otherwise go silent at the one
// moment the user has done something — so the bot confirms it, names the
// loan, and points at the next step.
func (w *Worker) OnLoanFiledMessage(ctx context.Context, userID string) {
	locale, _, err := w.Users.Locale(ctx, userID)
	if err != nil {
		w.Log.WarnContext(ctx, "filed message: locale", "error", err)
		return
	}
	l := i18n.Locale(locale)
	chat, err := w.Chats.ChatID(ctx, userID)
	if err != nil {
		w.Log.WarnContext(ctx, "filed message: chat", "error", err)
		return
	}
	text := w.withTip(ctx, userID, l, i18n.T(l, "add.saved_chat"))
	if err := w.Send.SendMessage(ctx, chat, text, w.mainMenu(l)); err != nil {
		w.Log.WarnContext(ctx, "filed message: send", "error", err)
	}
}
