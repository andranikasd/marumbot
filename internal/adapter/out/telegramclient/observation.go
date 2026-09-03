package telegramclient

import (
	"context"
	"errors"
	"time"
)

// CallObservation contains only bounded labels and timing, never payloads,
// tokens, URLs, chat IDs or Telegram error descriptions.
type CallObservation struct {
	Method      string
	Outcome     string        // success, error, canceled or rate_limited
	Duration    time.Duration // includes pacing, cooldown and response-body reads
	RateLimited bool          // true only when this call received HTTP 429
}

// WithObserver installs a metrics callback before the client is used. The
// callback runs once per call, synchronously, and must be fast and concurrency
// safe. Nil disables observation. No instruments or goroutines are created here.
func (c *Client) WithObserver(observe func(context.Context, CallObservation)) *Client {
	c.observe = observe
	return c
}

func (c *Client) observeCall(ctx context.Context, method string, start time.Time, err error) {
	switch method {
	case "sendMessage", "answerCallbackQuery", "setMyCommands", "setMyName",
		"setMyShortDescription", "setMyDescription", "setChatMenuButton", "sendChatAction", "getUpdates":
	default:
		method = "other"
	}
	observation := CallObservation{Method: method, Outcome: "success", Duration: c.pace.clock.Now().Sub(start)}
	var limited *TooManyError
	switch {
	case errors.As(err, &limited):
		observation.Outcome = "rate_limited"
		observation.RateLimited = true
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		observation.Outcome = "canceled"
	case err != nil:
		observation.Outcome = "error"
	}
	c.observe(ctx, observation)
}
