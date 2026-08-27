// Package telegramclient is the only outbound path to Telegram.
//
// One place, because Telegram's limits are per bot rather than per process: a
// second sender somewhere else in the codebase would not know about this one's
// budget, and the two together would exceed a limit neither was breaking alone.
package telegramclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/internal/obs"
)

// ErrRetryable marks a failure worth trying again: a timeout, a 5xx, or a
// rate limit. Anything else -- a blocked bot, a deleted chat -- will fail
// identically forever, and retrying it only delays the queue behind it.
var ErrRetryable = errors.New("telegram: retryable")

// Client sends messages.
type Client struct {
	token string
	http  *http.Client
	base  string
}

// New builds a client. The timeout is deliberately short: a send that has not
// completed in ten seconds has almost certainly already been delivered, and
// waiting longer only widens the window in which the lease expires and someone
// else sends it again.
func New(token string) *Client {
	return &Client{
		token: token,
		base:  "https://api.telegram.org",
		http:  &http.Client{Timeout: 10 * time.Second},
	}
}

// WithBase points the client at a different host, for tests.
func (c *Client) WithBase(u string) *Client { c.base = u; return c }

// SendMessage delivers text to a chat.
//
// Delivery is at-least-once and cannot be made exactly-once: the gap between
// Telegram accepting a message and Marum recording that it did is unclosable.
// Message text is therefore written so that arriving twice reads correctly.
func (c *Client) SendMessage(ctx context.Context, chatID int64, text string, markup any) error {
	ctx, span := obs.ComponentSender.CallService(ctx, "telegram", "sendMessage")
	defer span.End()

	body := map[string]any{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "HTML",
		// A preview turns a plain URL into a card that pushes the numbers off
		// the screen on a phone.
		"link_preview_options": map[string]any{"is_disabled": true},
	}
	if markup != nil {
		body["reply_markup"] = markup
	}
	return c.call(ctx, "sendMessage", body)
}

// AnswerCallbackQuery acknowledges a button press. Telegram shows a loading
// spinner on the button until this arrives, so it is not optional.
func (c *Client) AnswerCallbackQuery(ctx context.Context, id string) error {
	ctx, span := obs.ComponentSender.CallService(ctx, "telegram", "answerCallbackQuery")
	defer span.End()
	return c.call(ctx, "answerCallbackQuery", map[string]any{"callback_query_id": id})
}

func (c *Client) call(ctx context.Context, method string, body map[string]any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/bot%s/%s", c.base, c.token, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		// A transport error is ambiguous: the message may or may not have been
		// delivered. Retryable is the safe reading, because a duplicate reminder
		// is a nuisance and a missing one is a missed payment.
		return fmt.Errorf("%w: %w", ErrRetryable, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))

	switch {
	case resp.StatusCode == http.StatusOK:
		return nil
	case resp.StatusCode == http.StatusTooManyRequests, resp.StatusCode >= 500:
		return fmt.Errorf("%w: %s: %s", ErrRetryable, resp.Status, describe(raw))
	default:
		// 400 and 403 are permanent: a blocked bot stays blocked, and a chat
		// that no longer exists will not come back.
		return fmt.Errorf("telegram: %s: %s", resp.Status, describe(raw))
	}
}

// describe pulls Telegram's own explanation out of an error body without
// echoing the whole payload, which would put message text into a log.
func describe(raw []byte) string {
	var r struct {
		Description string `json:"description"`
	}
	if err := json.Unmarshal(raw, &r); err == nil && r.Description != "" {
		return r.Description
	}
	return strings.TrimSpace(string(raw[:min(len(raw), 120)]))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// SetMyCommands publishes the command list for one language.
//
// Without this the chat offers no suggestions at all: a user has to know the
// command names and type them exactly. Telegram scopes the list by language
// code, so Armenian and English users each see their own -- and the default
// scope, sent with an empty language, is what everyone else gets.
func (c *Client) SetMyCommands(ctx context.Context, lang string, cmds []app.BotCommand) error {
	ctx, span := obs.ComponentSender.CallService(ctx, "telegram", "setMyCommands")
	defer span.End()

	body := map[string]any{"commands": cmds}
	if lang != "" {
		body["language_code"] = lang
	}
	return c.call(ctx, "setMyCommands", body)
}

// SetChatMenuButton replaces the chat's menu button with one that opens the
// Mini App.
//
// This is the difference between a form a user can find and one they cannot.
// An inline button only exists inside the message that carried it, and scrolls
// away; the menu button sits beside the message box permanently.
func (c *Client) SetChatMenuButton(ctx context.Context, text, url string) error {
	ctx, span := obs.ComponentSender.CallService(ctx, "telegram", "setChatMenuButton")
	defer span.End()

	return c.call(ctx, "setChatMenuButton", map[string]any{
		"menu_button": map[string]any{
			"type":    "web_app",
			"text":    text,
			"web_app": map[string]any{"url": url},
		},
	})
}

// SendChatAction shows "typing…" in the chat.
//
// It does not make the reply faster; it makes the wait legible. A bot that sits
// silent for two seconds reads as broken, and the same two seconds with a
// typing indicator reads as working. Telegram clears it after five seconds or
// when a message arrives, whichever comes first.
//
// Failures are ignored by the caller: an indicator that did not appear is not a
// reason to fail the command it was announcing.
func (c *Client) SendChatAction(ctx context.Context, chatID int64, action string) error {
	ctx, span := obs.ComponentSender.CallService(ctx, "telegram", "sendChatAction")
	defer span.End()
	return c.call(ctx, "sendChatAction", map[string]any{
		"chat_id": chatID,
		"action":  action,
	})
}
