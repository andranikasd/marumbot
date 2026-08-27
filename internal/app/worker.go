package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/propagation"

	"github.com/andranikasd/marumbot/internal/i18n"
	"github.com/andranikasd/marumbot/internal/obs"
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
	KindText     = "text"
	KindCallback = "callback"
	KindIgnore   = "ignore"
)

// Sender is the outbound Telegram surface the worker needs.
type Sender interface {
	SendMessage(ctx context.Context, chatID int64, text string, markup any) error
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
	Inbox   InboxStore
	Users   UserStore
	Loans   LoanReader
	Chats   ChatResolver
	Send    Sender
	Clock   Clock
	Owner   string
	Log     *slog.Logger
	MiniApp string // absolute URL of the loan form, empty if not deployed

	// Menus is the Telegram surface used to publish the command list. Optional:
	// without it the bot works and simply suggests nothing.
	Menus     MenuPublisher
	menusOnce sync.Once
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
		PublishOnce(ctx, &w.menusOnce, w.Menus, w.MiniApp)
	}
	l, ok, err := w.Inbox.LeaseByID(ctx, id, w.Owner, w.Clock.Now().Add(LeaseFor))
	if err != nil {
		return fmt.Errorf("leasing %s: %w", id, err)
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
	// PublishOnce for why startup alone is not enough.
	if w.Menus != nil {
		PublishOnce(ctx, &w.menusOnce, w.Menus, w.MiniApp)
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
	retryAt := w.Clock.Now().Add(RetryAfter(l.Command.Attempts))
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
		_ = json.Unmarshal(c.Payload, &p)
	}

	chat, err := w.Chats.ChatID(ctx, c.UserID)
	if err != nil {
		return fmt.Errorf("resolving chat: %w", err)
	}

	switch c.Kind {
	case KindStart:
		return w.Send.SendMessage(ctx, chat,
			i18n.T(l, "start.greeting")+"\n\n"+i18n.T(l, "start.no_ai"), w.mainMenu(l))

	case KindHelp:
		return w.Send.SendMessage(ctx, chat, w.helpText(l), w.mainMenu(l))

	case KindAdd:
		if w.MiniApp == "" {
			// Say what is actually wrong. "Something went wrong" sent a user
			// hunting for a bug in the command when the cause was a variable
			// the container never received.
			w.Log.ErrorContext(ctx, "the mini app url is not configured; /add cannot offer a form")
			return w.Send.SendMessage(ctx, chat, i18n.T(l, "add.unavailable"), w.mainMenu(l))
		}
		return w.Send.SendMessage(ctx, chat, i18n.T(l, "add.open"), w.addButton(l))

	case KindLoans:
		return w.listLoans(ctx, c.UserID, chat, l)

	case KindBudget:
		return w.Send.SendMessage(ctx, chat, i18n.T(l, "budget.prompt"), w.mainMenu(l))

	case KindLanguage:
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
		return w.Send.SendMessage(ctx, chat, w.helpText(l), w.mainMenu(l))
	}
	return fmt.Errorf("unknown command kind %q", c.Kind)
}

// listLoans renders the borrower's loans.
//
// Each figure carries how it was established. A balance the borrower typed is
// shown as indicative, because only a lender-confirmed one resets drift -- and
// a planner that presents a guess with the same confidence as a bank statement
// is the thing this product exists not to be.
func (w *Worker) listLoans(ctx context.Context, userID string, chat int64, l i18n.Locale) error {
	loans, err := w.Loans.LoansForUser(ctx, userID, 25)
	if err != nil {
		return fmt.Errorf("listing loans: %w", err)
	}
	if len(loans) == 0 {
		return w.Send.SendMessage(ctx, chat, i18n.T(l, "loans.none"), w.mainMenu(l))
	}

	var b strings.Builder
	b.WriteString("<b>" + i18n.T(l, "loans.title") + "</b>\n")
	for _, ln := range loans {
		b.WriteString("\n<b>" + html.EscapeString(ln.Name) + "</b>\n")
		b.WriteString(i18n.T(l, "loan.balance", ln.Balance.String()) + "\n")
		b.WriteString(i18n.T(l, "loan.rate", ln.Rate) + "\n")
		if ln.Trust == "user_entered" {
			b.WriteString("<i>" + i18n.T(l, "reliability.stale", ln.MaturityDate) + "</i>\n")
		}
	}
	return w.Send.SendMessage(ctx, chat, b.String(), w.mainMenu(l))
}

func (w *Worker) callback(ctx context.Context, userID string, chat int64, data string) error {
	if len(data) > 5 && data[:5] == "lang:" {
		want := i18n.Locale(data[5:])
		if !want.Valid() {
			return nil
		}
		if err := w.Users.SetLocale(ctx, userID, string(want)); err != nil {
			return fmt.Errorf("setting locale: %w", err)
		}
		// Redraw the keyboard in the new language. Without this the buttons stay
		// in the old one until the user finds another reason to receive a
		// keyboard, which makes the switch look like it did not work.
		return w.Send.SendMessage(ctx, chat, i18n.T(want, "language.set"), w.mainMenu(want))
	}
	return nil
}

func (w *Worker) helpText(l i18n.Locale) string {
	return i18n.T(l, "help.title") + "\n\n" +
		i18n.T(l, "help.add") + "\n" +
		i18n.T(l, "help.loans") + "\n" +
		i18n.T(l, "help.budget") + "\n" +
		i18n.T(l, "help.language") + "\n" +
		i18n.T(l, "help.help")
}

// mainMenu is the persistent keyboard under the message box.
//
// A reply keyboard rather than an inline one, because an inline keyboard lives
// inside the message that carried it and scrolls out of reach; this stays put.
// A borrower on a phone should never have to remember a command name to see
// what they owe.
//
// The first button opens the loan form directly. web_app works on a reply
// keyboard as well as an inline one, which is what lets the form be one tap
// from anywhere in the conversation rather than buried behind /add.
func (w *Worker) mainMenu(l i18n.Locale) any {
	rows := [][]map[string]any{}
	if w.MiniApp != "" {
		rows = append(rows, []map[string]any{webAppButton(i18n.Button(l, KindAdd), w.MiniApp)})
	}
	rows = append(rows,
		[]map[string]any{button(i18n.Button(l, KindLoans)), button(i18n.Button(l, KindBudget))},
		[]map[string]any{button(i18n.Button(l, KindLanguage)), button(i18n.Button(l, KindHelp))},
	)
	return map[string]any{
		"keyboard":                rows,
		"resize_keyboard":         true,
		"is_persistent":           true,
		"input_field_placeholder": i18n.T(l, "kb.placeholder"),
	}
}

func (w *Worker) addButton(l i18n.Locale) any {
	return map[string]any{"inline_keyboard": [][]map[string]any{{
		webAppButton(i18n.T(l, "add.button"), w.MiniApp),
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

// keyText is Telegram's JSON field, not a command kind. Naming it keeps the two
// apart -- they happen to be the same string and mean entirely different things.
const keyText = "text"

func languageMenu() any {
	row := []map[string]any{}
	for _, l := range i18n.Supported() {
		row = append(row, map[string]any{
			keyText:         l.Name(),
			"callback_data": "lang:" + string(l),
		})
	}
	return map[string]any{"inline_keyboard": [][]map[string]any{row}}
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
