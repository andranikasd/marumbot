package obs

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/andranikasd/marumbot/pkg/core/money"
)

func capture(t *testing.T, f func(*slog.Logger)) string {
	t.Helper()
	var buf bytes.Buffer
	log := slog.New(newRedactor(slog.NewJSONHandler(&buf, nil)))
	f(log)
	return buf.String()
}

// The rule the design cares about most: a balance must not reach a log sink,
// and it is stripped because of what it IS, not because of what it is called.
func TestRedactor_StripsAmountsByType(t *testing.T) {
	amount, err := money.FromMajor(1_840_000, money.AMD)
	if err != nil {
		t.Fatal(err)
	}
	out := capture(t, func(l *slog.Logger) {
		l.Info("plan computed", "outstanding", amount)
	})
	if strings.Contains(out, "1840000") || strings.Contains(out, "1,840,000") {
		t.Errorf("an amount reached the log line: %s", out)
	}
	if !strings.Contains(out, "[redacted amount]") {
		t.Errorf("the attribute should be marked as redacted: %s", out)
	}
}

func TestRedactor_StripsDeniedKeys(t *testing.T) {
	for _, key := range []string{
		"balance", "principal_minor", "payment", "chat_id", "telegram_user",
		"token", "secret", "password", "request_body", "card_number",
	} {
		out := capture(t, func(l *slog.Logger) { l.Info("m", key, "sensitive-value") })
		if strings.Contains(out, "sensitive-value") {
			t.Errorf("%s was not redacted: %s", key, out)
		}
	}
}

func TestRedactor_KeepsOperationalFields(t *testing.T) {
	out := capture(t, func(l *slog.Logger) {
		l.Info("command completed", "command", "record_payment", "attempt", 1, "duration_ms", 47)
	})
	for _, want := range []string{"record_payment", "\"attempt\":1", "\"duration_ms\":47"} {
		if !strings.Contains(out, want) {
			t.Errorf("redaction removed something it should keep (%s): %s", want, out)
		}
	}
}

func TestRedactor_TruncatesLongValues(t *testing.T) {
	out := capture(t, func(l *slog.Logger) { l.Info("m", "note", strings.Repeat("x", 2000)) })
	if !strings.Contains(out, "truncated") {
		t.Errorf("an oversized value should be truncated: %.120s", out)
	}
	if len(out) > 1200 {
		t.Errorf("the line is still %d bytes", len(out))
	}
}

func TestRedactor_ScrubsInsideGroups(t *testing.T) {
	out := capture(t, func(l *slog.Logger) {
		l.Info("m", slog.Group("loan", slog.String("balance", "999999"), slog.String("ref", "9c1af0b2")))
	})
	if strings.Contains(out, "999999") {
		t.Errorf("a nested amount escaped redaction: %s", out)
	}
	if !strings.Contains(out, "9c1af0b2") {
		t.Errorf("a nested safe field was lost: %s", out)
	}
}

// WithAttrs is the path a contextual logger takes, and it must redact too or
// every scoped logger becomes a leak.
func TestRedactor_ScrubsWithAttrs(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(newRedactor(slog.NewJSONHandler(&buf, nil))).With("chat", "12345")
	log.Info("hello")
	if strings.Contains(buf.String(), "12345") {
		t.Errorf("WithAttrs bypassed redaction: %s", buf.String())
	}
}

func TestFanout_DeliversToEveryHandler(t *testing.T) {
	var a, b bytes.Buffer
	log := slog.New(newFanout(
		slog.NewJSONHandler(&a, nil),
		slog.NewJSONHandler(&b, nil),
	))
	log.Info("both")
	for name, buf := range map[string]*bytes.Buffer{"first": &a, "second": &b} {
		var rec map[string]any
		if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &rec); err != nil {
			t.Fatalf("%s sink: %v", name, err)
		}
		if rec["msg"] != "both" {
			t.Errorf("%s sink did not receive the record", name)
		}
	}
}

func TestFanout_OneFailingSinkDoesNotStopTheOthers(t *testing.T) {
	var good bytes.Buffer
	log := slog.New(newFanout(failingHandler{}, slog.NewJSONHandler(&good, nil)))
	log.Info("still delivered")
	if !strings.Contains(good.String(), "still delivered") {
		t.Error("a failing sink stopped delivery to a healthy one")
	}
}

type failingHandler struct{}

func (failingHandler) Enabled(context.Context, slog.Level) bool  { return true }
func (failingHandler) Handle(context.Context, slog.Record) error { return context.Canceled }
func (f failingHandler) WithAttrs([]slog.Attr) slog.Handler      { return f }
func (f failingHandler) WithGroup(string) slog.Handler           { return f }
