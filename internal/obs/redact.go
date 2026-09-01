package obs

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/andranikasd/marumbot/pkg/core/money"
)

// denied keys never reach a log sink. The list is matched case-insensitively
// against the whole attribute key, so "loan_balance" and "chat_id" are caught
// as readily as "balance" and "chat".
var denied = []string{
	"amount", "balance", "principal", "payment", "instalment", "installment",
	"interest", "fee", "penalty", "chat", "telegram", "token", "secret",
	"password", "payload", "body", "phone", "card",
}

const maxValueBytes = 512

// redactor strips anything that must not leave the trust boundary.
//
// The money check is a type switch rather than a name match: an amount is
// stripped because of what it is, not because someone happened to call the
// field "balance". Telemetry lives fourteen days in a third-party system, and
// a balance in a log line is a balance in someone else's database.
type redactor struct{ next slog.Handler }

func newRedactor(next slog.Handler) slog.Handler { return &redactor{next: next} }

func (r *redactor) Enabled(ctx context.Context, l slog.Level) bool {
	return r.next.Enabled(ctx, l)
}

func (r *redactor) Handle(ctx context.Context, rec slog.Record) error {
	clean := slog.NewRecord(rec.Time, rec.Level, rec.Message, rec.PC)
	rec.Attrs(func(a slog.Attr) bool {
		clean.AddAttrs(scrub(a))
		return true
	})
	return r.next.Handle(ctx, clean)
}

func (r *redactor) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := make([]slog.Attr, 0, len(attrs))
	for _, a := range attrs {
		out = append(out, scrub(a))
	}
	return &redactor{next: r.next.WithAttrs(out)}
}

func (r *redactor) WithGroup(name string) slog.Handler {
	return &redactor{next: r.next.WithGroup(name)}
}

func scrub(a slog.Attr) slog.Attr {
	if _, isAmount := a.Value.Any().(money.Amount); isAmount {
		return slog.String(a.Key, "[redacted amount]")
	}
	// Error text can include contextual identifiers or amounts. The request
	// span retains diagnostic detail; logs retain only the concrete error type.
	if err, ok := a.Value.Any().(error); ok {
		return slog.String(a.Key, fmt.Sprintf("[%T]", err))
	}
	key := strings.ToLower(a.Key)
	for _, d := range denied {
		if strings.Contains(key, d) {
			return slog.String(a.Key, "[redacted]")
		}
	}
	if a.Value.Kind() == slog.KindGroup {
		attrs := a.Value.Group()
		out := make([]any, 0, len(attrs))
		for _, g := range attrs {
			out = append(out, scrub(g))
		}
		return slog.Group(a.Key, out...)
	}
	if s := a.Value.String(); len(s) > maxValueBytes {
		return slog.String(a.Key, s[:maxValueBytes]+"…[truncated]")
	}
	return a
}

// fanout writes one record to several handlers, so a line reaches stdout and
// the collector without the call site knowing there are two.
type fanout struct{ handlers []slog.Handler }

func newFanout(hs ...slog.Handler) slog.Handler { return &fanout{handlers: hs} }

func (f *fanout) Enabled(ctx context.Context, l slog.Level) bool {
	for _, h := range f.handlers {
		if h.Enabled(ctx, l) {
			return true
		}
	}
	return false
}

func (f *fanout) Handle(ctx context.Context, rec slog.Record) error {
	for _, h := range f.handlers {
		if h.Enabled(ctx, rec.Level) {
			// A failing sink must not stop the others; the error is dropped
			// deliberately, because logging a logging failure loops.
			_ = h.Handle(ctx, rec.Clone())
		}
	}
	return nil
}

func (f *fanout) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := make([]slog.Handler, len(f.handlers))
	for i, h := range f.handlers {
		out[i] = h.WithAttrs(attrs)
	}
	return &fanout{handlers: out}
}

func (f *fanout) WithGroup(name string) slog.Handler {
	out := make([]slog.Handler, len(f.handlers))
	for i, h := range f.handlers {
		out[i] = h.WithGroup(name)
	}
	return &fanout{handlers: out}
}
