package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/propagation"

	"github.com/andranikasd/marumbot/internal/i18n"
	"github.com/andranikasd/marumbot/internal/obs"
	"github.com/andranikasd/marumbot/pkg/core/amortisation"
	"github.com/andranikasd/marumbot/pkg/core/date"
	"github.com/andranikasd/marumbot/pkg/core/money"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

// Command kinds, mirrored from the inbound adapter so this package does not
// import it: the app layer must not depend on an adapter, and a shared string
// literal repeated in two places is how the two drift apart silently.
const (
	KindStart    = "start"
	KindHelp     = "help"
	KindLoans    = "loans"
	KindAdd      = "add"
	KindBudget   = "budget"
	KindLanguage = "language"
	KindAdvice   = "advice"
	KindWorking  = "working"
	KindText     = "text"
	KindCallback = "callback"
	KindIgnore   = "ignore"
)

// Sender is the outbound Telegram surface the worker needs.
type Sender interface {
	SendMessage(ctx context.Context, chatID int64, text string, markup any) error
	SendChatAction(ctx context.Context, chatID int64, action string) error
	SetChatMenuButtonFor(ctx context.Context, chatID int64, text, url string) error
}

// ChatResolver turns an account back into the chat to reply to. The chat id is
// encrypted at rest, so recovering it is a deliberate act with a name rather
// than a field someone can read off a struct.
type ChatResolver interface {
	ChatID(ctx context.Context, userID string) (int64, error)
}

// Clock is time, injected. time.Now lives in exactly one adapter so that every
// schedule, lease and retry can be tested without sleeping.
type Clock interface{ Now() time.Time }

// Worker drains the command inbox.
type Worker struct {
	Inbox     InboxStore
	Users     UserStore
	Loans     LoanReader
	Editor    LoanEditor
	Budgets   BudgetStore
	Convos    ConversationStore
	Reminders ReminderStore
	Plans     PlanStore
	// plans caches the pure search by a fingerprint of its inputs; see
	// plancache.go. Zero value ready.
	plans searchCache
	// Shadow stores silent recommendations for the field gates; see shadow.go.
	// Optional: nil disables the walk.
	Shadow ShadowStore
	// Balances stores the borrower's statement of what is owed after a
	// payment; see paid.go. Optional: nil hides the paid flow.
	Balances BalanceRecorder
	// Contracts writes revised contract versions; see revise.go. Optional:
	// nil limits editing to name and note.
	Contracts ContractReviser
	// lastRemind is when TickReminders last did the work, as unix nanos. Ticks
	// arrive over HTTP, so a slow walk can overlap the next fire; the CAS both
	// prevents the race and makes the overlapping tick a no-op.
	lastRemind atomic.Int64
	reminding  atomic.Bool
	// lastShadow is the same gate for the shadow walk; see shadow.go.
	lastShadow atomic.Int64
	shadowing  atomic.Bool

	// DefaultCurrency is what a bare number means. AMD here; a user with a
	// dollar loan writes the code and it is honoured.
	DefaultCurrency money.Currency
	Chats           ChatResolver
	Send            Sender
	Clock           Clock
	Owner           string
	Log             *slog.Logger
	MiniApp         string // absolute URL of the Mini App, empty if not deployed
	// AppVersion stamps every Mini App URL the bot hands out, so a deploy is
	// a new URL and the Telegram webview cannot serve last week's app from
	// its cache. The webview caches by URL and honours little else.
	AppVersion string

	// Menus is the Telegram surface used to publish the command list. Optional:
	// without it the bot works and simply suggests nothing.
	Menus    MenuPublisher
	menusPub MenuPublication
}

// LeaseFor is how long a worker holds a command.
//
// Longer than any handler should take, short enough that a container killed
// mid-command frees the work within one scheduler tick rather than parking it
// until someone notices.
const LeaseFor = 2 * time.Minute

// HandleOne processes one named command: the one the webhook just recorded.
//
// The webhook must answer the message in front of it. Draining oldest-first
// there lets a single command that keeps failing hold the slot while every new
// message queues behind it -- a backlog indistinguishable, from the outside,
// from the bot being slow. That is exactly what happened: /loans failed on a
// scan error, retried five times each, and starved every other command.
func (w *Worker) HandleOne(ctx context.Context, id string) error {
	if w.Menus != nil {
		w.menusPub.Publish(ctx, w.Menus, w.miniURL(""), w.Log)
	}
	l, ok, err := w.Inbox.LeaseByID(ctx, id, w.Owner, w.Clock.Now().Add(LeaseFor))
	if err != nil {
		return fmt.Errorf("leasing command: %w", err)
	}
	if !ok {
		// Already leased or finished: the tick got there first. Ordinary race.
		return nil
	}
	w.handle(ctx, l)
	return nil
}

// Drain leases up to n commands and processes them. It returns the number
// handled, so a caller can decide whether to go round again.
func (w *Worker) Drain(ctx context.Context, n int) (int, error) {
	// Publish the command list the first time this process does any work. See
	// MenuPublication for why startup alone is not enough.
	if w.Menus != nil {
		w.menusPub.Publish(ctx, w.Menus, w.miniURL(""), w.Log)
	}

	leases, err := w.Inbox.Lease(ctx, w.Owner, n, w.Clock.Now().Add(LeaseFor))
	if err != nil {
		return 0, fmt.Errorf("leasing: %w", err)
	}
	for _, l := range leases {
		w.handle(ctx, l)
	}
	return len(leases), nil
}

// handle processes one command and settles its lease exactly once.
func (w *Worker) handle(ctx context.Context, l Lease) {
	// Rejoin the webhook's trace, so a reply is visibly caused by the message
	// that asked for it rather than appearing as unrelated work.
	if l.Command.TraceContext != "" {
		ctx = propagation.TraceContext{}.Extract(ctx,
			propagation.MapCarrier{"traceparent": l.Command.TraceContext})
	}
	ctx, span := obs.ComponentWorker.Enter(ctx, l.Command.Kind)
	defer span.End()

	err := w.apply(ctx, l.Command)
	if err == nil {
		if err := w.Inbox.Complete(ctx, l.Command.ID, l.Token); err != nil &&
			!errors.Is(err, ErrNotLeased) {
			w.Log.ErrorContext(ctx, "completing command failed", "error", err)
		}
		return
	}

	span.RecordError(err)
	dead := l.Command.Attempts >= MaxAttempts
	backoff := RetryAfter(l.Command.Attempts)
	// Telegram's 429 names its own wait; retrying on the generic schedule
	// before it elapses only re-hits the limit. Structural check, so app does
	// not import the adapter.
	var slow interface{ RetryAfter() time.Duration }
	if errors.As(err, &slow) && slow.RetryAfter() > backoff {
		backoff = slow.RetryAfter()
	}
	retryAt := w.Clock.Now().Add(backoff)
	if ferr := w.Inbox.Fail(ctx, l.Command.ID, l.Token, code(err), retryAt, dead); ferr != nil &&
		!errors.Is(ferr, ErrNotLeased) {
		w.Log.ErrorContext(ctx, "recording failure failed", "error", ferr)
	}
	w.Log.WarnContext(ctx, "command failed",
		"kind", l.Command.Kind, "attempts", l.Command.Attempts, "dead", dead, "error", err)
}

type textPayload struct {
	Text string `json:"text,omitempty"`
	Data string `json:"data,omitempty"`
	Arg  string `json:"arg,omitempty"`
}

// apply is the whole conversation. Every branch ends in at most one message,
// because a command that sends two is a command that sends four when Telegram
// retries it.
func (w *Worker) apply(ctx context.Context, c InboundCommand) error {
	if c.Kind == KindIgnore {
		return nil
	}
	locale, _, err := w.Users.Locale(ctx, c.UserID)
	if err != nil {
		return fmt.Errorf("reading locale: %w", err)
	}
	l := i18n.Locale(locale)

	var p textPayload
	if len(c.Payload) > 0 {
		if err := json.Unmarshal(c.Payload, &p); err != nil {
			return fmt.Errorf("decoding command payload: %w", err)
		}
	}

	chat, err := w.Chats.ChatID(ctx, c.UserID)
	if err != nil {
		return fmt.Errorf("resolving chat: %w", err)
	}
	// A per-chat menu overrides Telegram's global default forever. Refresh it on
	// every interaction so even an account missed by a rollout sweep converges
	// to this build before the user opens the Mini App again.
	if w.MiniApp != "" {
		if err := w.Send.SetChatMenuButtonFor(ctx, chat, i18n.DashboardButton(l), w.miniURL("")); err != nil {
			w.Log.DebugContext(ctx, "menu button not refreshed", "error", err)
		}
	}

	// Show that the bot heard, before doing the work that produces a reply.
	// This does not make anything faster; it makes the wait legible, which is
	// the difference between a bot that seems slow and one that seems broken.
	// A failure here is ignored: an indicator that did not appear is no reason
	// to fail the command it was announcing.
	if err := w.Send.SendChatAction(ctx, chat, "typing"); err != nil {
		w.Log.DebugContext(ctx, "typing indicator failed", "error", err)
	}

	switch c.Kind {
	case KindStart:
		// The per-chat menu button, once set, overrides the bot-wide default
		// forever — including its URL. /start re-pins it, so the one command
		// everyone knows always leads to the current app. Best-effort: a
		// missing button is not a reason to greet nobody.
		return w.Send.SendMessage(ctx, chat, w.withTip(ctx, c.UserID, l, w.startText(l), stageNoLoans), w.mainMenu(l))

	case KindHelp:
		return w.Send.SendMessage(ctx, chat, w.withTip(ctx, c.UserID, l, w.helpText(l)), w.mainMenu(l))

	case KindAdd:
		if w.MiniApp == "" {
			// Say what is actually wrong. "Something went wrong" sent a user
			// hunting for a bug in the command when the cause was a variable
			// the container never received.
			w.Log.ErrorContext(ctx, "the mini app url is not configured; /add cannot offer a form")
			return w.Send.SendMessage(ctx, chat, i18n.T(l, "add.unavailable"), w.mainMenu(l))
		}
		return w.Send.SendMessage(ctx, chat, i18n.T(l, "add.open"), w.addMarkup(l))

	case KindLoans:
		return w.listLoans(ctx, c.UserID, chat, l)

	case KindAdvice:
		return w.advise(ctx, c.UserID, chat, l, plan.Goal{Kind: plan.LeastInterest}, false)

	case KindBudget:
		if strings.TrimSpace(p.Arg) != "" {
			taken, err := w.takeBudget(ctx, c.UserID, chat, l, p.Arg)
			if err != nil {
				return err
			}
			if taken {
				return nil
			}
			return w.Send.SendMessage(ctx, chat, i18n.T(l, "budget.not_a_number"), w.budgetMarkup(l))
		}
		return w.askBudget(ctx, c.UserID, chat, l)

	case KindLanguage:
		if want := i18n.Locale(strings.ToLower(strings.TrimSpace(p.Arg))); want.Valid() {
			return w.setLanguage(ctx, c.UserID, chat, want)
		}
		return w.Send.SendMessage(ctx, chat, i18n.T(l, "language.prompt"), languageMenu())

	case KindCallback:
		return w.callback(ctx, c.UserID, chat, p.Data)

	case KindText:
		// A reply-keyboard button sends its own label as a message, so this is
		// where a tap arrives. Matching runs across every locale, not just the
		// current one: a user who switches language still has the old keyboard
		// on screen until they tap something.
		if kind, ok := i18n.MatchButton(p.Text); ok {
			return w.apply(ctx, InboundCommand{UserID: c.UserID, Kind: kind, Payload: c.Payload})
		}
		// An answer to a question the bot asked. Checked before falling back to
		// help, because a reply that goes unheard is how a user decides the bot
		// is broken.
		if w.Convos != nil {
			state, err := w.Convos.State(ctx, c.UserID)
			switch {
			case err != nil:
				w.Log.WarnContext(ctx, "reading the conversation state failed", "error", err)
			case state == StateAwaitingBudget:
				taken, err := w.takeBudget(ctx, c.UserID, chat, l, p.Text)
				if err != nil {
					return err
				}
				if taken {
					return nil
				}
				return w.Send.SendMessage(ctx, chat, i18n.T(l, "budget.not_a_number"), w.mainMenu(l))
			case strings.HasPrefix(state, StateAwaitingBalance+":"):
				taken, err := w.takeBalance(ctx, c.UserID, chat, l, state, p.Text)
				if err != nil {
					return err
				}
				if taken {
					return nil
				}
				return w.Send.SendMessage(ctx, chat, i18n.T(l, "paid.not_a_number"), w.mainMenu(l))
			case state == StateAwaitingReliefCap:
				taken, err := w.takeReliefCap(ctx, c.UserID, chat, l, p.Text)
				if err != nil {
					return err
				}
				if taken {
					return nil
				}
				return w.Send.SendMessage(ctx, chat, i18n.T(l, "relief.not_a_number"), w.mainMenu(l))
			}
		}
		return w.Send.SendMessage(ctx, chat, w.helpText(l), w.mainMenu(l))
	}
	return fmt.Errorf("unknown command kind %q", c.Kind)
}

// listLoans renders the borrower's loans, each with the instalment the engine
// derives from its contract.
//
// The payment is projected here rather than stored. A schedule is a function of
// the terms and the anchor; keeping one would create a second source of truth
// that can disagree with replay.
//
// Every figure carries how its balance was established. A balance the borrower
// typed is marked as their own figure, because only a lender-confirmed one
// resets drift -- and a planner that shows a guess with the same confidence as
// a bank statement is the thing this product exists not to be.
func (w *Worker) listLoans(ctx context.Context, userID string, chat int64, l i18n.Locale) error {
	loans, err := w.Loans.LoansForUser(ctx, userID, plan.MaxLoans+1)
	if err != nil {
		return fmt.Errorf("listing loans: %w", err)
	}
	if len(loans) == 0 {
		return w.Send.SendMessage(ctx, chat, i18n.T(l, "loans.none"), w.addMarkup(l))
	}

	var b strings.Builder
	today := date.From(w.Clock.Now(), time.UTC)
	b.WriteString("<b>" + i18n.T(l, "loans.title") + "</b>\n")

	// The totals the plan is built on, as one aligned block, so the list and
	// the plan agree and the eye finds the three figures in one place.
	if _, owed, required, cur, err := w.positions(ctx, loans); err == nil && owed.Sign() > 0 {
		rows := [][2]string{
			{i18n.T(l, "fig.owed"), bare(owed)},
			{i18n.T(l, "fig.required"), bare(required)},
		}
		if due, pay, ok := w.nextInstalment(loans); ok {
			rows = append(rows, [2]string{i18n.T(l, "fig.next"), shortDate(l, due, today) + "  " + bare(pay)})
		}
		b.WriteString(figures(rows))
		b.WriteString("<i>" + cur.Code + "</i>\n")
	}

	working := [][]map[string]any{}
	for _, ln := range loans {
		b.WriteString("\n<b>" + html.EscapeString(ln.Name) + "</b>\n")
		if ln.Description != "" {
			b.WriteString("<i>" + html.EscapeString(ln.Description) + "</i>\n")
		}
		if s, err := amortisation.Build(ln.Contract, ln.Balance, ln.AsOf); err == nil && len(s.Rows) > 0 {
			b.WriteString(i18n.T(l, "loan.line", bare(ln.Balance), percent(ln.Contract.NominalRate), shortDate(l, s.Rows[0].Due, today)) + "\n")
		} else {
			// Say the schedule is unavailable rather than omitting the line: a
			// silently missing number reads as a number of zero.
			if err != nil {
				w.Log.WarnContext(ctx, "projecting a loan failed", "error", err)
			}
			b.WriteString(i18n.T(l, "loan.balance", bare(ln.Balance), percent(ln.Contract.NominalRate)) + " · " + i18n.T(l, "loan.no_schedule") + "\n")
		}
		if !ln.Confirmed() {
			b.WriteString("<i>" + i18n.T(l, "reliability.yours", shortDate(l, ln.AsOf, today)) + "</i>\n")
		}
		working = append(working, []map[string]any{{
			keyText:     i18n.T(l, "working.button", ln.Name),
			keyCallback: "work:" + ln.ID,
		}})
	}

	// The keyboard: the one next step alone on top, the way to change things
	// second, the arithmetic behind each loan last.
	rows := [][]map[string]any{{{keyText: i18n.T(l, "btn.plan"), keyCallback: "goal:cheapest"}}}
	if w.MiniApp != "" {
		rows = append(rows, []map[string]any{webAppButton(i18n.T(l, "btn.manage"), w.miniURL("manage"))})
	}
	rows = append(rows, working...)
	return w.Send.SendMessage(ctx, chat, w.withTip(ctx, userID, l, b.String()), map[string]any{keyInline: rows})
}

// nextInstalment finds the earliest projected instalment across the loans,
// for the summary block. Loans that cannot be projected are skipped; the
// figure is a fact about the loans that can.
func (w *Worker) nextInstalment(loans []UserLoan) (date.Date, money.Amount, bool) {
	var due date.Date
	var pay money.Amount
	found := false
	for _, ln := range loans {
		if ln.Balance.Sign() <= 0 {
			continue
		}
		s, err := amortisation.Build(ln.Contract, ln.Balance, ln.AsOf)
		if err != nil || len(s.Rows) == 0 {
			continue
		}
		if !found || s.Rows[0].Due.Before(due) {
			due, pay, found = s.Rows[0].Due, s.Rows[0].Payment, true
		}
	}
	return due, pay, found
}

// percent renders a rate the way a borrower reads it. The engine holds parts
// per billion of a year, and the column stores the fraction, so 14 per cent is
// 0.140000000 -- which rendered directly said "Rate: 0.140000000%".
func percent(r money.Rate) string {
	hundredths := (int64(r)*10000 + 500_000_000) / 1_000_000_000
	whole, frac := hundredths/100, hundredths%100
	if frac == 0 {
		return strconv.FormatInt(whole, 10)
	}
	if frac%10 == 0 {
		return fmt.Sprintf("%d.%d", whole, frac/10)
	}
	return fmt.Sprintf("%d.%02d", whole, frac)
}

func (w *Worker) callback(ctx context.Context, userID string, chat int64, data string) error {
	// The reader changing the question rather than accepting the answer.
	// "Show me how you got that." The only answer that settles a doubt about
	// arithmetic is the arithmetic.
	if strings.HasPrefix(data, "work:") {
		locale, _, err := w.Users.Locale(ctx, userID)
		if err != nil {
			return fmt.Errorf("reading locale: %w", err)
		}
		return w.showWorking(ctx, userID, chat, i18n.Locale(locale), data[5:])
	}

	if strings.HasPrefix(data, "paid:") {
		locale, _, err := w.Users.Locale(ctx, userID)
		if err != nil {
			return fmt.Errorf("reading locale: %w", err)
		}
		return w.askPaidBalance(ctx, userID, chat, i18n.Locale(locale), data[5:])
	}
	if data == "paidskip" {
		locale, _, err := w.Users.Locale(ctx, userID)
		if err != nil {
			return fmt.Errorf("reading locale: %w", err)
		}
		return w.skipPaidBalance(ctx, userID, chat, i18n.Locale(locale))
	}
	if strings.HasPrefix(data, "approve:") {
		locale, _, err := w.Users.Locale(ctx, userID)
		if err != nil {
			return fmt.Errorf("reading locale: %w", err)
		}
		return w.approvePlan(ctx, userID, chat, i18n.Locale(locale), GoalFromToken(data[8:]))
	}
	if strings.HasPrefix(data, "why:") {
		locale, _, err := w.Users.Locale(ctx, userID)
		if err != nil {
			return fmt.Errorf("reading locale: %w", err)
		}
		return w.explainPlan(ctx, userID, chat, i18n.Locale(locale), GoalFromToken(data[4:]))
	}
	if strings.HasPrefix(data, "goal:") {
		locale, _, err := w.Users.Locale(ctx, userID)
		if err != nil {
			return fmt.Errorf("reading locale: %w", err)
		}
		l := i18n.Locale(locale)
		switch data[5:] {
		case "soonest":
			return w.advise(ctx, userID, chat, l, plan.Goal{Kind: plan.Fastest}, false)
		case "relief":
			return w.askReliefCap(ctx, userID, chat, l)
		case "first":
			return w.advise(ctx, userID, chat, l, plan.Goal{Kind: plan.FirstWin}, false)
		case "compare":
			return w.advise(ctx, userID, chat, l, plan.Goal{Kind: plan.LeastInterest}, true)
		default:
			return w.advise(ctx, userID, chat, l, plan.Goal{Kind: plan.LeastInterest}, false)
		}
	}
	if len(data) > 5 && data[:5] == "lang:" {
		want := i18n.Locale(data[5:])
		if !want.Valid() {
			return nil
		}
		return w.setLanguage(ctx, userID, chat, want)
	}
	return nil
}

func (w *Worker) setLanguage(ctx context.Context, userID string, chat int64, want i18n.Locale) error {
	if err := w.Users.SetLocale(ctx, userID, string(want)); err != nil {
		return fmt.Errorf("setting locale: %w", err)
	}
	// The global Mini App button cannot be localised, so keep a per-chat one
	// in step with both command-based and button-based language changes.
	if w.MiniApp != "" {
		if err := w.Send.SetChatMenuButtonFor(ctx, chat, i18n.DashboardButton(want), w.miniURL("")); err != nil {
			w.Log.DebugContext(ctx, "menu button not localised", "error", err)
		}
	}
	// Telegram keeps the old reply keyboard until a new one is sent.
	return w.Send.SendMessage(ctx, chat, i18n.T(want, "language.set"), w.mainMenu(want))
}

// showWorking prints the arithmetic behind a loan's next instalments.
//
// Three rows, not the whole schedule: enough to check the method against a
// statement, few enough to read on a phone. Someone who can reproduce three
// rows can reproduce sixty.
func (w *Worker) showWorking(ctx context.Context, userID string, chat int64, l i18n.Locale, loanID string) error {
	ln, err := w.Editor.LoanForUser(ctx, loanID, userID)
	if err != nil {
		return w.Send.SendMessage(ctx, chat, i18n.T(l, "error.generic"), w.mainMenu(l))
	}
	s, err := amortisation.Build(ln.Contract, ln.Balance, ln.AsOf)
	if err != nil || len(s.Rows) == 0 {
		return w.Send.SendMessage(ctx, chat, i18n.T(l, "loan.no_schedule"), w.mainMenu(l))
	}

	var b strings.Builder
	b.WriteString("<b>" + html.EscapeString(ln.Name) + "</b>\n")
	b.WriteString("<i>" + i18n.T(l, "working.intro") + "</i>\n")
	for i, r := range s.Rows {
		if i == 3 {
			break
		}
		b.WriteString("\n<pre>" + html.EscapeString(
			amortisation.Explain(r, ln.Contract.NominalRate, ln.Contract.DayCount, ln.Contract.Rounding)) + "</pre>")
	}
	b.WriteString("\n" + i18n.T(l, "working.check"))
	return w.Send.SendMessage(ctx, chat, b.String(), w.mainMenu(l))
}

// startText is a titled card: what Marum is, the three steps, the one next
// action, and a way to switch language. The language hint is written in the
// other language on purpose: it is the one line a reader who landed in the
// wrong locale needs to be able to read.
func (w *Worker) startText(l i18n.Locale) string {
	return "<b>" + i18n.T(l, "start.title") + "</b>\n" +
		i18n.T(l, "start.greeting") + "\n\n" +
		i18n.T(l, "start.steps") + "\n\n" +
		i18n.T(l, "start.next") + "\n" +
		i18n.T(l, "start.language") + "\n\n" +
		"<i>" + i18n.T(l, "start.no_ai") + "</i>\n" +
		"<i>" + i18n.T(l, "start.reminders") + "</i>"
}

// helpText explains the three goals in plain words before it lists commands:
// a borrower who understands what "least interest" trades against "finish
// soonest" can choose; one who only knows the command names cannot.
func (w *Worker) helpText(l i18n.Locale) string {
	return "<b>" + i18n.T(l, "help.title") + "</b>\n\n" +
		i18n.T(l, "help.intro") + "\n\n" +
		i18n.T(l, "help.goals") + "\n" +
		i18n.T(l, "help.goal.cheapest") + "\n" +
		i18n.T(l, "help.goal.soonest") + "\n" +
		i18n.T(l, "help.goal.relief") + "\n" +
		i18n.T(l, "help.goal.first") + "\n" +
		i18n.T(l, "help.compare") + "\n\n" +
		i18n.T(l, "help.commands") + "\n" +
		i18n.T(l, "help.add") + "\n" +
		i18n.T(l, "help.advice") + "\n" +
		i18n.T(l, "help.loans") + "\n" +
		i18n.T(l, "help.budget") + "\n" +
		i18n.T(l, "help.language") + "\n" +
		i18n.T(l, "help.help") + "\n\n" +
		i18n.T(l, "help.reminders") + "\n\n" +
		"<i>" + i18n.T(l, "start.no_ai") + "</i>"
}

// mainMenu is the persistent keyboard under the message box.
//
// A reply keyboard rather than an inline one, because an inline keyboard lives
// inside the message that carried it and scrolls out of reach; this stays put.
// A borrower on a phone should never have to remember a command name to see
// what they owe.
//
// Four buttons, two rows: open the app, ask what to do, see the loans, set
// the budget. Language and help live in the "/" command menu Telegram
// already shows; a keyboard that lists everything lists nothing.
//
// The first button opens the Mini App. Contextual Add loan buttons still
// open the form directly; the persistent entry point must describe the
// whole product rather than one workflow inside it.
func (w *Worker) mainMenu(l i18n.Locale) any {
	rows := [][]map[string]any{}
	if w.MiniApp != "" {
		rows = append(rows, []map[string]any{
			webAppButton(i18n.DashboardButton(l), w.miniURL("")),
			button(i18n.Button(l, KindAdvice)),
		})
	} else {
		rows = append(rows, []map[string]any{button(i18n.Button(l, KindAdvice))})
	}
	rows = append(rows,
		[]map[string]any{button(i18n.Button(l, KindLoans)), button(i18n.Button(l, KindBudget))},
	)
	return map[string]any{
		"keyboard":                rows,
		"resize_keyboard":         true,
		"is_persistent":           true,
		"input_field_placeholder": i18n.T(l, "kb.placeholder"),
	}
}

// addMarkup is the one-tap way out of "no loans": the form, if there is one,
// else the ordinary keyboard.
func (w *Worker) addMarkup(l i18n.Locale) any {
	if w.MiniApp == "" {
		return w.mainMenu(l)
	}
	return map[string]any{keyInline: [][]map[string]any{{
		webAppButton(i18n.T(l, "add.button"), w.miniURL("")),
	}}}
}

// budgetMarkup is the same for "no budget" and "budget too low": the budget
// screen, opened on the field that needs changing.
func (w *Worker) budgetMarkup(l i18n.Locale) any {
	if w.MiniApp == "" {
		return w.mainMenu(l)
	}
	return map[string]any{keyInline: [][]map[string]any{{
		webAppButton(i18n.T(l, "budget.button"), w.miniURL("budget")),
	}}}
}

// button is a plain keyboard button. Tapping it sends its own label as a
// message, which is why every label has to map back to a command.
func button(label string) map[string]any {
	return map[string]any{keyText: label}
}

// webAppButton opens the Mini App directly, without a round trip through the
// bot to fetch a link.
func webAppButton(label, url string) map[string]any {
	return map[string]any{keyText: label, "web_app": map[string]any{"url": url}}
}

// Telegram's JSON field names. Named because keyText in particular collides
// with a command kind: they are the same string and mean entirely different
// things, and a bare literal cannot say which one is meant.
const (
	keyText     = "text"
	keyInline   = "inline_keyboard"
	keyCallback = "callback_data"
)

func languageMenu() any {
	row := []map[string]any{}
	for _, l := range i18n.Supported() {
		row = append(row, map[string]any{
			keyText:     l.Name(),
			keyCallback: "lang:" + string(l),
		})
	}
	return map[string]any{keyInline: [][]map[string]any{row}}
}

// code reduces an error to a short stable label for the inbox row. The message
// itself goes to the log; a free-form string in a column becomes a de facto
// enum that nothing can query.
func code(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, ErrNotLeased):
		return "lease_lost"
	default:
		return "error"
	}
}

// GoalFromToken decodes the callback form of a goal. Unknown tokens fall
// back to least interest: a stale button must answer something sensible.
func GoalFromToken(tok string) plan.Goal {
	switch {
	case tok == "soonest":
		return plan.Goal{Kind: plan.Fastest}
	case tok == "first":
		return plan.Goal{Kind: plan.FirstWin}
	case strings.HasPrefix(tok, "relief:"):
		if minor, err := parseInt64(tok[7:]); err == nil && minor > 0 {
			return plan.Goal{Kind: plan.Relief, Cap: money.FromMinor(minor, money.MustLookup("AMD"))}
		}
		return plan.Goal{Kind: plan.LeastInterest}
	default:
		return plan.Goal{Kind: plan.LeastInterest}
	}
}

// miniURL is the one place a Mini App URL is built: the base, the build
// version, and the screen. Everything the bot sends goes through it, so a
// deploy changes every URL at once.
func (w *Worker) miniURL(screen string) string {
	u := w.MiniApp + "?v=" + url.QueryEscape(w.AppVersion)
	if screen != "" {
		u += "&screen=" + screen
	}
	return u
}
