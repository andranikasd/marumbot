package obs

import (
	"errors"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestPlanSearchDurationUsesElapsedClock(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(t.Context()) })
	m := NewPlanSearchMetrics(provider.Meter("test"))
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return now }
	for _, ok := range []bool{true, false} {
		finish := m.StartSearch()
		now = now.Add(250 * time.Millisecond)
		finish(ok)
	}
	var data metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &data); err != nil {
		t.Fatal(err)
	}
	points := 0
	for _, scope := range data.ScopeMetrics {
		for _, instrument := range scope.Metrics {
			if instrument.Name != "marum_plan_search_duration_seconds" {
				continue
			}
			for _, p := range instrument.Data.(metricdata.Histogram[float64]).DataPoints {
				points++
				v, _ := p.Attributes.Value(attribute.Key("result"))
				if p.Attributes.Len() != 1 || (v.AsString() != "ok" && v.AsString() != "error") || p.Count != 1 || p.Sum != 0.25 {
					t.Fatalf("unexpected observation: %+v", p)
				}
			}
		}
	}
	if points != 2 {
		t.Fatalf("got %d duration outcomes", points)
	}
}

type failedSearchMeter struct{ metric.Meter }

func (failedSearchMeter) Int64Counter(string, ...metric.Int64CounterOption) (metric.Int64Counter, error) {
	return nil, errors.New("instrument unavailable")
}

func (failedSearchMeter) Float64Histogram(string, ...metric.Float64HistogramOption) (metric.Float64Histogram, error) {
	return nil, errors.New("instrument unavailable")
}

func TestPlanSearchMetricsDisabled(t *testing.T) {
	for _, m := range []*PlanSearchMetrics{
		nil,
		{},
		NewPlanSearchMetrics(failedSearchMeter{noop.NewMeterProvider().Meter("test")}),
		NewPlanSearchMetrics(noop.NewMeterProvider().Meter("test")),
	} {
		m.CacheLookup(true)
		m.CacheLookup(false)
		m.StartSearch()(false)
	}
}
