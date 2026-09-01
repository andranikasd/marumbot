package telegram

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/propagation"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/internal/i18n"
	"github.com/andranikasd/marumbot/internal/identity"
	"github.com/andranikasd/marumbot/internal/obs"
)

// HandleFunc processes one command by id.
type HandleFunc func(ctx context.Context, id string) error

// maxBody caps what the handler will read. Telegram updates are small; an
// unbounded read is a way to be knocked over by one large request.
const maxBody = 1 << 20 // 1 MiB

// Webhook is the inbound HTTP edge.
type Webhook struct {
	Inbox        app.InboxStore
	Users        app.UserStore
	Cipher       *identity.Cipher
	ServiceToken string
	// WebhookSecret is Telegram's secret_token, echoed by Telegram in
	// X-Telegram-Bot-Api-Secret-Token. The Worker checks it at the edge, but a
	// deployment without the Worker still needs the check here: defence in
	// depth costs one compare.
	WebhookSecret string
	Timezone      string
	Clock         app.Clock
	// Handle processes the command that was just recorded, before the webhook
	// answers. See accept for why this is synchronous, and why it is the one
	// command rather than whatever is oldest.
	Handle HandleFunc
	Log    *slog.Logger
}

// Handler returns the route Cloudflare's Worker forwards to.
//
// The Worker has already checked Telegram's secret token before this is
// reached; the service token checked here proves the request came from the
// Worker rather than from someone who found the container. Two different
// claims, so two different checks.
func (h *Webhook) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := obs.ComponentWebhook.Enter(r.Context(), "update")
		defer span.End()

		if h.ServiceToken != "" {
			got := r.Header.Get("X-Marum-Service-Token")
			if subtle.ConstantTimeCompare([]byte(got), []byte(h.ServiceToken)) != 1 {
				// No detail in the response: an attacker learns nothing from a
				// 401 and everything from "wrong token".
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		}
		if h.WebhookSecret != "" {
			got := r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
			if subtle.ConstantTimeCompare([]byte(got), []byte(h.WebhookSecret)) != 1 {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		}

		var u Update
		if err := json.NewDecoder(io.LimitReader(r.Body, maxBody)).Decode(&u); err != nil {
			// Telegram will not send malformed JSON, so this is either a bug or
			// somebody probing. Answering 200 stops a retry loop over something
			// that will never parse.
			h.Log.WarnContext(ctx, "undecodable update", "error", err)
			w.WriteHeader(http.StatusOK)
			return
		}

		if err := h.accept(ctx, u); err != nil {
			// 500 makes Telegram retry, which is what we want: the update is
			// not yet recorded anywhere, so dropping it would lose it.
			h.Log.ErrorContext(ctx, "recording update failed", "error", err, "update_id", u.UpdateID)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
}

// accept resolves the sender and records the command. It deliberately does no
// work beyond that: the reply is the worker's job, and doing it here would put
// a Telegram round trip inside the window Telegram is timing.
func (h *Webhook) accept(ctx context.Context, u Update) error {
	n, ok := Normalise(u)
	if !ok {
		return nil // nothing addressed to us
	}

	acct, err := h.resolve(ctx, n)
	if err != nil {
		return err
	}

	// Carry the trace across the queue, so the worker's span joins the webhook's
	// rather than starting an orphan an hour later.
	carrier := propagation.MapCarrier{}
	otelPropagator.Inject(ctx, carrier)

	id := uuid.NewString()
	accepted, err := h.Inbox.Enqueue(ctx, app.InboundCommand{
		ID:           id,
		UpdateID:     u.UpdateID,
		UserID:       acct.ID,
		Kind:         n.Kind,
		Payload:      n.Payload,
		TraceContext: carrier["traceparent"],
	})
	if err != nil {
		return err
	}
	if !accepted {
		// A repeat. Telegram retries until acknowledged, so this is ordinary
		// traffic and the only correct response is to do nothing again.
		h.Log.DebugContext(ctx, "duplicate update ignored", "update_id", u.UpdateID)
		return nil
	}

	// Answer now, not on the next tick.
	//
	// The inbox exists so a crash cannot lose an update, and that property comes
	// from writing the row BEFORE doing the work -- not from deferring the work
	// to a scheduler. Draining here keeps the durability and removes the wait: a
	// person who types /start gets a reply in the time it takes to send one,
	// rather than whenever the cron next fires.
	//
	// A failure here is not returned. The command is already recorded, so the
	// tick will retry it; turning a send failure into a 500 would make Telegram
	// redeliver an update that is safely stored, which is how one slow reply
	// becomes four.
	if h.Handle != nil {
		handleCtx, cancel := context.WithTimeout(ctx, drainBudget)
		defer cancel()
		if err := h.Handle(handleCtx, id); err != nil {
			h.Log.WarnContext(ctx, "immediate handling failed; the tick will retry",
				"error", err, "update_id", u.UpdateID)
		}
	}
	return nil
}

// drainBudget bounds the inline work. Telegram gives a webhook far longer than
// this, but a reply that takes ten seconds is already a bad reply, and the tick
// is a better place to be slow than the request path.
const drainBudget = 10 * time.Second

func (h *Webhook) resolve(ctx context.Context, n Normalised) (app.Account, error) {
	ctx, span := obs.ComponentWebhook.Call(ctx, obs.ComponentStore, "upsert_user")
	defer span.End()

	userSealed, err := h.Cipher.Seal(n.UserID)
	if err != nil {
		return app.Account{}, err
	}
	chatSealed, err := h.Cipher.Seal(n.ChatID)
	if err != nil {
		return app.Account{}, err
	}
	tz := h.Timezone
	if tz == "" {
		tz = "Asia/Yerevan"
	}
	return h.Users.UpsertByTelegram(ctx, app.UpsertUser{
		UserTag:    h.Cipher.Tag(n.UserID),
		UserSealed: userSealed,
		ChatTag:    h.Cipher.Tag(n.ChatID),
		ChatSealed: chatSealed,
		KeyVersion: identity.KeyVersion,
		NewID:      uuid.NewString(),
		Locale:     string(i18n.Parse(n.Language)),
		Timezone:   tz,
		TrialEnds:  h.Clock.Now().Add(app.TrialPeriod),
	})
}

var otelPropagator = propagation.TraceContext{}
