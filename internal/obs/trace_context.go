package obs

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// traceEnricher stamps every record with the trace it happened inside.
//
// Without this, a log line and the span that produced it have nothing in
// common, and "show me the logs for this span" cannot be answered. The
// OpenTelemetry log bridge attaches trace context on its own, but only when
// the caller passes a context - so this handler also covers the plain
// slog.Info form, and puts the same identifiers into the stdout JSON where no
// bridge is involved at all.
type traceEnricher struct{ next slog.Handler }

func newTraceEnricher(next slog.Handler) slog.Handler { return &traceEnricher{next: next} }

func (t *traceEnricher) Enabled(ctx context.Context, l slog.Level) bool {
	return t.next.Enabled(ctx, l)
}

func (t *traceEnricher) Handle(ctx context.Context, rec slog.Record) error {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return t.next.Handle(ctx, rec)
	}
	// Copy rather than mutate: a Record may be handed to several handlers.
	out := slog.NewRecord(rec.Time, rec.Level, rec.Message, rec.PC)
	rec.Attrs(func(a slog.Attr) bool { out.AddAttrs(a); return true })
	out.AddAttrs(
		slog.String("trace_id", sc.TraceID().String()),
		slog.String("span_id", sc.SpanID().String()),
	)
	return t.next.Handle(ctx, out)
}

func (t *traceEnricher) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &traceEnricher{next: t.next.WithAttrs(attrs)}
}

func (t *traceEnricher) WithGroup(name string) slog.Handler {
	return &traceEnricher{next: t.next.WithGroup(name)}
}
