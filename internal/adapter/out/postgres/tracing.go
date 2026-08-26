package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/andranikasd/marumbot/internal/obs"
)

// queryTracer times and traces every statement without any call site having to
// remember to. pgx hands it each query, which is why the coverage is total
// rather than wherever somebody added a stopwatch.
type queryTracer struct {
	metrics *obs.Metrics
	tracer  trace.Tracer
}

type tracerKey struct{}

type tracerState struct {
	start time.Time
	name  string
	span  trace.Span
}

func newQueryTracer(m *obs.Metrics) *queryTracer {
	return &queryTracer{metrics: m, tracer: otel.Tracer("marum/postgres")}
}

func (t *queryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	name := queryName(data.SQL)
	// The statement text is the span name, never an attribute: arguments would
	// otherwise ride along, and arguments are balances.
	ctx, span := t.tracer.Start(ctx, "db."+name,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.operation", name),
		))
	return context.WithValue(ctx, tracerKey{}, &tracerState{start: time.Now(), name: name, span: span})
}

func (t *queryTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	st, ok := ctx.Value(tracerKey{}).(*tracerState)
	if !ok {
		return
	}
	elapsed := time.Since(st.start).Seconds()
	if t.metrics != nil {
		t.metrics.DBQueryDuration.Record(ctx, elapsed, obs.Query(st.name))
		if data.Err != nil {
			t.metrics.DBQueryErrors.Add(ctx, 1, obs.Query(st.name))
		}
	}
	if data.Err != nil {
		st.span.RecordError(data.Err)
	}
	st.span.End()
}

// queryName reduces a statement to a bounded label. Every query the
// application runs comes from a named file, so the first meaningful token plus
// the table is a stable, closed vocabulary rather than one series per SQL text.
func queryName(sql string) string {
	fields := splitFields(sql)
	if len(fields) == 0 {
		return "unknown"
	}
	verb := lower(fields[0])
	switch verb {
	case "select", "insert", "update", "delete", "with":
		for i := 1; i < len(fields)-1; i++ {
			w := lower(fields[i])
			if w == "from" || w == "into" || w == "table" {
				return verb + "." + trimIdent(fields[i+1])
			}
		}
		if verb == "update" {
			return verb + "." + trimIdent(fields[1])
		}
		return verb
	default:
		return verb
	}
}

func splitFields(s string) []string {
	var out []string
	var cur []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ' ' || c == '\n' || c == '\t' || c == '\r' || c == '(' || c == ',' {
			if len(cur) > 0 {
				out = append(out, string(cur))
				cur = cur[:0]
			}
			continue
		}
		if c == '-' && i+1 < len(s) && s[i+1] == '-' { // skip a line comment
			for i < len(s) && s[i] != '\n' {
				i++
			}
			continue
		}
		cur = append(cur, c)
	}
	if len(cur) > 0 {
		out = append(out, string(cur))
	}
	return out
}

func lower(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}

func trimIdent(s string) string {
	for len(s) > 0 && (s[len(s)-1] == ';' || s[len(s)-1] == ')') {
		s = s[:len(s)-1]
	}
	return s
}
