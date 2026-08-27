// Package telegram is the inbound edge: it authenticates a webhook call,
// normalises the update into a command, and records it.
//
// It does not act on the update. Telegram retries until it gets an answer, so a
// handler that does the work inline turns a slow database into duplicate
// deliveries and a crash into a lost message. Persisting first and working later
// makes both survivable, and is why the inbox exists.
package telegram

import (
	"encoding/json"
	"strings"
)

// Update is the subset of Telegram's update object Marum reads.
//
// Deliberately partial. Telegram's schema is large and grows, and decoding
// fields nothing uses would mean storing data with no purpose -- which for a
// financial application is a liability rather than an asset.
type Update struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		MessageID int64  `json:"message_id"`
		From      *User  `json:"from"`
		Chat      *Chat  `json:"chat"`
		Date      int64  `json:"date"`
		Text      string `json:"text"`
	} `json:"message"`
	CallbackQuery *struct {
		ID      string `json:"id"`
		From    *User  `json:"from"`
		Data    string `json:"data"`
		Message *struct {
			Chat *Chat `json:"chat"`
		} `json:"message"`
	} `json:"callback_query"`
}

// User is a Telegram account.
type User struct {
	ID           int64  `json:"id"`
	IsBot        bool   `json:"is_bot"`
	LanguageCode string `json:"language_code"`
}

// Chat is where a message was sent.
type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

// Command kinds. These are stored, so they are part of the schema: renaming one
// orphans every row that used it.
const (
	KindStart    = "start"
	KindHelp     = "help"
	KindLoans    = "loans"
	KindAdd      = "add"
	KindBudget   = "budget"
	KindLanguage = "language"
	KindAdvice   = "advice"   // what should I do, given everything on file
	KindText     = "text"     // free text, meaningful only inside a conversation
	KindCallback = "callback" // an inline button
	KindIgnore   = "ignore"   // understood, and deliberately not acted on
)

// Normalised is an update reduced to what the worker needs.
type Normalised struct {
	Kind     string
	UserID   int64
	ChatID   int64
	Language string
	Payload  []byte
}

// payload is what gets stored. It carries the text and any button data, and
// nothing that identifies the sender: the identifiers live encrypted in
// identities, and copying them into a jsonb column would undo that.
type payload struct {
	Text      string `json:"text,omitempty"`
	Data      string `json:"data,omitempty"`
	MessageID int64  `json:"message_id,omitempty"`
	Arg       string `json:"arg,omitempty"`
}

// Normalise reduces an update to a command, or reports that there is nothing to
// do. Anything Marum does not understand becomes KindIgnore rather than an
// error: an unrecognised update is Telegram doing its job, not a fault.
func Normalise(u Update) (Normalised, bool) {
	switch {
	case u.CallbackQuery != nil && u.CallbackQuery.From != nil:
		var chat int64
		if m := u.CallbackQuery.Message; m != nil && m.Chat != nil {
			chat = m.Chat.ID
		}
		p, _ := json.Marshal(payload{Data: u.CallbackQuery.Data})
		return Normalised{
			Kind: KindCallback, UserID: u.CallbackQuery.From.ID, ChatID: chat,
			Language: u.CallbackQuery.From.LanguageCode, Payload: p,
		}, true

	case u.Message != nil && u.Message.From != nil && u.Message.Chat != nil:
		m := u.Message
		if m.From.IsBot {
			// Bots talking to bots is not a conversation Marum takes part in.
			return Normalised{}, false
		}
		kind, arg := classify(m.Text)
		p, _ := json.Marshal(payload{Text: m.Text, MessageID: m.MessageID, Arg: arg})
		return Normalised{
			Kind: kind, UserID: m.From.ID, ChatID: m.Chat.ID,
			Language: m.From.LanguageCode, Payload: p,
		}, true
	}
	return Normalised{}, false
}

// classify maps message text to a command kind.
//
// Telegram appends @botname to commands sent in groups, so /help@marum_dev_bot
// has to resolve to the same thing as /help.
func classify(text string) (kind, arg string) {
	t := strings.TrimSpace(text)
	if !strings.HasPrefix(t, "/") {
		if t == "" {
			return KindIgnore, ""
		}
		return KindText, t
	}
	head, rest, _ := strings.Cut(t, " ")
	head = strings.ToLower(strings.TrimPrefix(head, "/"))
	if at := strings.IndexByte(head, '@'); at >= 0 {
		head = head[:at]
	}
	arg = strings.TrimSpace(rest)

	switch head {
	case "start":
		return KindStart, arg
	case "help":
		return KindHelp, arg
	case "loans":
		return KindLoans, arg
	case "add":
		return KindAdd, arg
	case "budget":
		return KindBudget, arg
	case "advice", "plan":
		return KindAdvice, arg
	case "language", "lang":
		return KindLanguage, arg
	default:
		return KindIgnore, arg
	}
}
