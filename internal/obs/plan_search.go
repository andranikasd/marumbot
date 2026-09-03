package obs

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/andranikasd/marumbot/internal/adapter/out/sysclock"
)

// PlanSearchMetrics observes cache lookups and pure search computation only.
// It excludes source reads, fingerprinting and publication. Labels are fixed
// outcomes; inputs, fingerprints and user identifiers never enter telemetry.
type PlanSearchMetrics struct {
	lookups  metric.Int64Counter
	duration metric.Float64Histogram
	now      func() time.Time
}

// NewPlanSearchMetrics creates synchronous instruments on the supplied meter.
// A global OTel meter supports registration before the SDK provider is installed.
// Instrument failures disable that measurement without affecting plan results.
func NewPlanSearchMetrics(m metric.Meter) *PlanSearchMetrics {
	lookups, err := m.Int64Counter("marum_plan_search_cache_lookups_total",
		metric.WithDescription("search cache lookups, including misses that fail to compute"))
	if err != nil {
		lookups = nil
	}
	duration, err := m.Float64Histogram("marum_plan_search_duration_seconds",
		metric.WithDescription("pure plan.Search computation on cache misses, including failures"),
		metric.WithUnit("s"))
	if err != nil {
		duration = nil
	}
	return &PlanSearchMetrics{lookups: lookups, duration: duration, now: sysclock.New().Now}
}

// CacheLookup counts hits and misses, including expired entries as misses.
func (m *PlanSearchMetrics) CacheLookup(hit bool) {
	if m == nil || m.lookups == nil {
		return
	}
	result := "miss"
	if hit {
		result = "hit"
	}
	m.lookups.Add(context.Background(), 1, metric.WithAttributes(attribute.String("result", result)))
}

// StartSearch returns a completion callback to invoke once after plan.Search.
// The clock measures telemetry only and never supplies engine or cache time.
func (m *PlanSearchMetrics) StartSearch() func(bool) {
	if m == nil || m.duration == nil {
		return func(bool) {}
	}
	start := m.now()
	return func(ok bool) {
		m.duration.Record(context.Background(), m.now().Sub(start).Seconds(), Result(ok))
	}
}
