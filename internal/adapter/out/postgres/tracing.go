package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/andranikasd/marumbot/internal/adapter/out/sysclock"
	"github.com/andranikasd/marumbot/internal/obs"
	"github.com/andranikasd/marumbot/queries"
)

// queryTracer times and traces every statement without any call site having to
// remember to. pgx hands it each query, which is why the coverage is total
// rather than wherever somebody added a stopwatch.
type queryTracer struct {
	metrics *obs.Metrics
}

var tracingClock = sysclock.New()

type tracerKey struct{}

type tracerState struct {
	start  time.Time
	name   string
	span   trace.Span
	dbSpan trace.Span
}

func newQueryTracer(m *obs.Metrics) *queryTracer { return &queryTracer{metrics: m} }

func (t *queryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	name := queryName(data.SQL)
	// Only declared query names or fixed fallback labels reach telemetry.
	// Neither statement text nor arguments may become a span or attribute.
	// SERVER: the store is being called. Its caller opens the matching CLIENT
	// span, and that pair is what draws the edge into this node.
	ctx, span := obs.ComponentStore.Enter(ctx, name,
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.operation", name),
			// The store in turn calls Postgres. peer.service gives the database
			// its own node rather than an edge into nothing.
			attribute.String("peer.service", "postgresql"),
		))
	// The outbound leg. peer.service gives Postgres its own node on the graph
	// instead of an edge into nothing.
	_, dbSpan := obs.ComponentStore.CallService(ctx, "postgresql", "postgresql."+name)
	return context.WithValue(ctx, tracerKey{}, &tracerState{
		start: tracingClock.Now(), name: name, span: span, dbSpan: dbSpan,
	})
}

func (t *queryTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	st, ok := ctx.Value(tracerKey{}).(*tracerState)
	if !ok {
		return
	}
	elapsed := tracingClock.Now().Sub(st.start).Seconds()
	if t.metrics != nil {
		t.metrics.DBQueryDuration.Record(ctx, elapsed, obs.Query(st.name))
		if data.Err != nil {
			t.metrics.DBQueryErrors.Add(ctx, 1, obs.Query(st.name))
		}
	}
	if data.Err != nil {
		st.span.RecordError(data.Err)
		st.dbSpan.RecordError(data.Err)
	}
	st.dbSpan.End()
	st.span.End()
}

// queryName uses the embedded query's declared name. Driver-generated
// transaction commands have a fixed vocabulary; all other SQL stays unknown.
func queryName(sql string) string {
	if name := queries.Name(sql); name != "" {
		return name
	}
	switch sql {
	case "begin", "BEGIN":
		return "begin"
	case "commit", "COMMIT":
		return "commit"
	case "rollback", "ROLLBACK":
		return "rollback"
	default:
		return "unknown"
	}
}
