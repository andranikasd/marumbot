package obs

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func collectPoolMetrics(t *testing.T, reader *sdkmetric.ManualReader) map[string]float64 {
	t.Helper()
	var data metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &data); err != nil {
		t.Fatal(err)
	}
	values := map[string]float64{}
	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			if !strings.HasPrefix(m.Name, "marum_db_pool_") {
				continue
			}
			switch d := m.Data.(type) {
			case metricdata.Gauge[int64]:
				if len(d.DataPoints) != 1 || d.DataPoints[0].Attributes.Len() != 0 {
					t.Fatalf("%s must have one unlabeled point", m.Name)
				}
				values[m.Name] = float64(d.DataPoints[0].Value)
			case metricdata.Sum[int64]:
				if !d.IsMonotonic || d.Temporality != metricdata.CumulativeTemporality || len(d.DataPoints) != 1 || d.DataPoints[0].Attributes.Len() != 0 {
					t.Fatalf("%s must be an unlabeled cumulative counter", m.Name)
				}
				values[m.Name] = float64(d.DataPoints[0].Value)
			case metricdata.Sum[float64]:
				if m.Unit != "s" || !d.IsMonotonic || d.Temporality != metricdata.CumulativeTemporality || len(d.DataPoints) != 1 || d.DataPoints[0].Attributes.Len() != 0 {
					t.Fatalf("%s must be an unlabeled cumulative seconds counter", m.Name)
				}
				values[m.Name] = d.DataPoints[0].Value
			default:
				t.Fatalf("unexpected pool metric type: %T", d)
			}
		}
	}
	return values
}

func TestDBPoolMetricsSnapshotAndUnregister(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	m := newMetrics(provider, "test")
	var state atomic.Pointer[DBPoolStats]
	state.Store(&DBPoolStats{Active: 3, Idle: 2, Max: 8, WaitCount: 7, WaitDuration: 1500 * time.Millisecond, CanceledAcquires: 2})
	var calls atomic.Int64
	stop, err := m.RegisterDBPool(func() DBPoolStats {
		calls.Add(1)
		return *state.Load()
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stop() })
	want := map[string]float64{
		"marum_db_pool_active_connections": 3, "marum_db_pool_idle_connections": 2, "marum_db_pool_max_connections": 8,
		"marum_db_pool_waits_total": 7, "marum_db_pool_wait_seconds_total": 1.5, "marum_db_pool_canceled_acquires_total": 2,
	}
	if got := collectPoolMetrics(t, reader); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if calls.Load() != 1 {
		t.Fatal("one collection must read exactly one snapshot")
	}
	state.Store(&DBPoolStats{Active: 1, Idle: 4, Max: 8, WaitCount: 9, WaitDuration: 2 * time.Second, CanceledAcquires: 3})
	want["marum_db_pool_active_connections"] = 1
	want["marum_db_pool_idle_connections"] = 4
	want["marum_db_pool_waits_total"] = 9
	want["marum_db_pool_wait_seconds_total"] = 2
	want["marum_db_pool_canceled_acquires_total"] = 3
	if got := collectPoolMetrics(t, reader); !reflect.DeepEqual(got, want) {
		t.Fatalf("updated snapshot: %v", got)
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	if got := collectPoolMetrics(t, reader); len(got) != 0 || calls.Load() != 2 {
		t.Fatalf("closed pool still observed: %v calls=%d", got, calls.Load())
	}
}

func TestDBPoolMetricsConcurrentCollectionAndCleanup(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	var calls atomic.Int64
	stop, err := newMetrics(provider, "test").RegisterDBPool(func() DBPoolStats {
		calls.Add(1)
		return DBPoolStats{Max: 8}
	})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	start := make(chan struct{})
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			var data metricdata.ResourceMetrics
			if err := reader.Collect(t.Context(), &data); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		if err := stop(); err != nil {
			t.Error(err)
		}
	}()
	close(start)
	wg.Wait()
	before := calls.Load()
	if got := collectPoolMetrics(t, reader); len(got) != 0 || calls.Load() != before {
		t.Fatal("unregistered callback ran again")
	}
}

type failedPoolMeter struct {
	metric.Meter
	failure        error
	failInstrument bool
}

func (m failedPoolMeter) Int64ObservableGauge(name string, options ...metric.Int64ObservableGaugeOption) (metric.Int64ObservableGauge, error) {
	if m.failInstrument {
		return nil, m.failure
	}
	return m.Meter.Int64ObservableGauge(name, options...)
}

func (m failedPoolMeter) RegisterCallback(metric.Callback, ...metric.Observable) (metric.Registration, error) {
	return nil, m.failure
}

func TestDBPoolMetricsRegistrationErrorsAndDisabled(t *testing.T) {
	failure := errors.New("metrics unavailable")
	for _, failInstrument := range []bool{false, true} {
		m := &Metrics{meter: failedPoolMeter{Meter: noop.NewMeterProvider().Meter("test"), failure: failure, failInstrument: failInstrument}}
		stop, err := m.RegisterDBPool(func() DBPoolStats {
			t.Error("failed registration called snapshot")
			return DBPoolStats{}
		})
		if !errors.Is(err, failure) || stop != nil {
			t.Fatalf("registration error lost: %v", err)
		}
	}
	for _, m := range []*Metrics{nil, {}, newMetrics(noop.NewMeterProvider(), "test")} {
		stop, err := m.RegisterDBPool(func() DBPoolStats {
			t.Error("disabled observer called snapshot")
			return DBPoolStats{}
		})
		if err != nil || stop == nil {
			t.Fatalf("disabled metrics: %v", err)
		}
		if err := stop(); err != nil {
			t.Fatal(err)
		}
	}
	m := newMetrics(noop.NewMeterProvider(), "test")
	if _, err := m.RegisterDBPool(nil); err == nil {
		t.Fatal("nil callback accepted")
	}
}
