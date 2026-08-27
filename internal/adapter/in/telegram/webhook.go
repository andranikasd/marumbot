package telegram

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/propagation"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/internal/i18n"
	"github.com/andranikasd/marumbot/internal/identity"
	"github.com/andranikasd/marumbot/internal/obs"
)

// maxBody caps what the handler will read. Telegram updates are small; an
// unbounded read is a way to be knocked over by one large request.
const maxBody = 1 << 20 // 1 MiB

// Webhook is the inbound HTTP edge.
type Webhook struct {
	Inbox        app.InboxStore
	Users        app.UserStore
	Cipher       *identity.Cipher
	ServiceToken string
	Timezone     string
	Clock        app.Clock
	Log          *slog.Logger
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

	accepted, err := h.Inbox.Enqueue(ctx, app.InboundCommand{
		ID:           uuid.NewString(),
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
	}
	return nil
}

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
