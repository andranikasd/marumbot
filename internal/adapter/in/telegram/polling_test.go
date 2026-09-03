package telegram

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/internal/identity"
)

type pollSource struct {
	rows   []json.RawMessage
	err    error
	offset int64
}

func (p *pollSource) PollUpdates(_ context.Context, offset int64) ([]json.RawMessage, error) {
	p.offset = offset
	return p.rows, p.err
}

type pollInbox struct {
	webhookInbox
	fail  bool
	calls int
}

func (p *pollInbox) Enqueue(context.Context, app.InboundCommand) (bool, error) {
	p.calls++
	if p.fail {
		return false, errors.New("database unavailable")
	}
	return true, nil
}

func TestPollingAcknowledgesOnlyDurableUpdates(t *testing.T) {
	cipher, err := identity.New(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	inbox := &pollInbox{fail: true}
	h := &Webhook{Inbox: inbox, Users: webhookUsers{}, Cipher: cipher, Clock: webhookClock{}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	source := &pollSource{rows: []json.RawMessage{json.RawMessage(`{"update_id":42,"message":{"from":{"id":1},"chat":{"id":1,"type":"private"},"text":"/start"}}`)}}
	next, err := h.pollOnce(context.Background(), source, 42)
	if err == nil || next != 42 {
		t.Fatal("failed enqueue must not acknowledge update", next, err)
	}
	inbox.fail = false
	handled := 0
	h.Handle = func(context.Context, string) error { handled++; return errors.New("send failed") }
	next, err = h.pollOnce(context.Background(), source, next)
	if err != nil || next != 43 || handled != 1 {
		t.Fatal("durable update advances even if reply needs retry", next, err, handled)
	}
	next, err = h.pollOnce(context.Background(), source, next)
	if err != nil || next != 43 || inbox.calls != 2 {
		t.Fatal("older update must not be re-enqueued")
	}
}

func TestPollingTransportFailureKeepsOffset(t *testing.T) {
	h := &Webhook{}
	source := &pollSource{err: errors.New("offline")}
	next, err := h.pollOnce(context.Background(), source, 123)
	if err == nil || next != 123 || source.offset != 123 {
		t.Fatal(next, err)
	}
}
