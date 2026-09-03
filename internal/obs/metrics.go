package obs

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Metrics holds the application's own instruments.
//
// Every label here is bounded by construction. No user, loan or chat
// identifier ever becomes a metric label: the free tier is billed by active
// series, and a label whose cardinality grows with users is both a bill and an
// outage. Per-entity detail belongs in logs and traces.
type Metrics struct {
	meter           metric.Meter
	DBQueryDuration metric.Float64Histogram
	DBQueryErrors   metric.Int64Counter
	AdminSignIns    metric.Int64Counter
	ReplayDuration  metric.Float64Histogram
	ReplayEvents    metric.Int64Histogram
	AccrualOverflow metric.Int64Counter
}

func newMetrics(mp metric.MeterProvider, version string) *Metrics {
	m := mp.Meter("github.com/andranikasd/marumbot")
	f := func(name, desc, unit string) metric.Float64Histogram {
		h, _ := m.Float64Histogram(name, metric.WithDescription(desc), metric.WithUnit(unit))
		return h
	}
	c := func(name, desc string) metric.Int64Counter {
		v, _ := m.Int64Counter(name, metric.WithDescription(desc))
		return v
	}

	// build_info is the only place a version appears as a label: one series
	// per build, rather than multiplying the whole catalogue by every release.
	if g, err := m.Int64Gauge("marum_build_info",
		metric.WithDescription("always 1; the version label identifies the running build")); err == nil {
		g.Record(context.Background(), 1, metric.WithAttributes(attribute.String("version", version)))
	}

	ev, _ := m.Int64Histogram("marum_replay_events",
		metric.WithDescription("events considered by one ledger replay"))

	return &Metrics{
		meter:           m,
		DBQueryDuration: f("marum_db_query_duration_seconds", "time spent in a named query", "s"),
		DBQueryErrors:   c("marum_db_query_errors_total", "named queries that returned an error"),
		AdminSignIns:    c("marum_admin_sign_ins_total", "admin interface sign-in attempts"),
		ReplayDuration:  f("marum_replay_duration_seconds", "time to rebuild a loan position", "s"),
		ReplayEvents:    ev,
		AccrualOverflow: c("marum_accrual_overflow_total",
			"interest accruals that exceeded the representable range; must stay at zero"),
	}
}

// Query names the bounded label set used for database timings. sqlc-style
// named queries give a fixed vocabulary, so this can never grow with traffic.
func Query(name string) metric.MeasurementOption {
	return metric.WithAttributes(attribute.String("query", name))
}

// Result labels an outcome with a small, closed set of values.
func Result(ok bool) metric.MeasurementOption {
	v := "error"
	if ok {
		v = "ok"
	}
	return metric.WithAttributes(attribute.String("result", v))
}
