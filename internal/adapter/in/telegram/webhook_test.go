package telegram

import (
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/internal/identity"
)

type webhookInbox struct{ accepted bool }

func (f webhookInbox) Enqueue(context.Context, app.InboundCommand) (bool, error) {
	return f.accepted, nil
}

func (webhookInbox) Lease(context.Context, string, int, time.Time) ([]app.Lease, error) {
	return nil, nil
}

func (webhookInbox) LeaseByID(context.Context, string, string, time.Time) (app.Lease, bool, error) {
	return app.Lease{}, false, nil
}
func (webhookInbox) Complete(context.Context, string, string) error { return nil }
func (webhookInbox) Fail(context.Context, string, string, string, time.Time, bool) error {
	return nil
}

type webhookUsers struct{}

func (webhookUsers) UpsertByTelegram(context.Context, app.UpsertUser) (app.Account, error) {
	return app.Account{ID: "user-id"}, nil
}

func (webhookUsers) Locale(context.Context, string) (string, string, error) {
	return "hy", "Asia/Yerevan", nil
}
func (webhookUsers) ByTelegramTag(context.Context, string) (string, error) { return "user-id", nil }
func (webhookUsers) SetLocale(context.Context, string, string) error       { return nil }

type webhookClock struct{}

func (webhookClock) Now() time.Time { return time.Unix(1_700_000_000, 0) }

type callbackRecorder struct {
	id    string
	calls int
}

func (r *callbackRecorder) AnswerCallbackQuery(_ context.Context, id string) error {
	r.id = id
	r.calls++
	return nil
}

func TestWebhookAcknowledgesCallbackAfterDurableEnqueue(t *testing.T) {
	t.Parallel()

	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	cipher, err := identity.New(key)
	if err != nil {
		t.Fatal(err)
	}
	callbacks := &callbackRecorder{}
	h := &Webhook{
		Inbox: webhookInbox{accepted: true}, Users: webhookUsers{}, Cipher: cipher,
		Clock: webhookClock{}, Callbacks: callbacks,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	body := `{"update_id":12,"callback_query":{"id":"callback-1","data":"goal:cheapest",` +
		`"from":{"id":7},"message":{"chat":{"id":7,"type":"private"}}}}`
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.Handler().ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if callbacks.calls != 1 || callbacks.id != "callback-1" {
		t.Fatalf("callback acknowledgements = %d for %q", callbacks.calls, callbacks.id)
	}
}
