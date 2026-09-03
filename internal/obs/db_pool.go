package obs

import (
	"context"
	"errors"
	"time"

	"go.opentelemetry.io/otel/metric"
)

// DBPoolStats is one nonblocking snapshot of a pool, without connection details.
// WaitCount and WaitDuration are cumulative successful acquisitions that waited
// for an empty pool. Canceled acquisitions are counted separately; their wait
// duration is not included. Active means checked out, not necessarily running SQL.
type DBPoolStats struct {
	Active, Idle, Max int64
	WaitCount         int64
	WaitDuration      time.Duration
	CanceledAcquires  int64
}

// RegisterDBPool observes the application's single database pool on collection.
// snapshot must be concurrency-safe and must not perform I/O. The owner must
// call the returned idempotent cleanup before closing its pool. Nil/unconfigured
// Metrics is a no-op. Instruments have no labels and need no polling goroutine.
func (m *Metrics) RegisterDBPool(snapshot func() DBPoolStats) (func() error, error) {
	if m == nil || m.meter == nil {
		return func() error { return nil }, nil
	}
	if snapshot == nil {
		return nil, errors.New("pool metrics require a snapshot callback")
	}
	active, err := m.meter.Int64ObservableGauge("marum_db_pool_active_connections",
		metric.WithDescription("connections currently checked out"), metric.WithUnit("{connection}"))
	if err != nil {
		return nil, err
	}
	idle, err := m.meter.Int64ObservableGauge("marum_db_pool_idle_connections",
		metric.WithDescription("connections currently idle"), metric.WithUnit("{connection}"))
	if err != nil {
		return nil, err
	}
	maximum, err := m.meter.Int64ObservableGauge("marum_db_pool_max_connections",
		metric.WithDescription("configured maximum connections"), metric.WithUnit("{connection}"))
	if err != nil {
		return nil, err
	}
	waits, err := m.meter.Int64ObservableCounter("marum_db_pool_waits_total",
		metric.WithDescription("successful acquisitions that waited for an empty pool"), metric.WithUnit("{acquisition}"))
	if err != nil {
		return nil, err
	}
	waitTime, err := m.meter.Float64ObservableCounter("marum_db_pool_wait_seconds_total",
		metric.WithDescription("cumulative wait for successful acquisitions from an empty pool; excludes canceled waits"), metric.WithUnit("s"))
	if err != nil {
		return nil, err
	}
	canceled, err := m.meter.Int64ObservableCounter("marum_db_pool_canceled_acquires_total",
		metric.WithDescription("acquisitions canceled by their context"), metric.WithUnit("{acquisition}"))
	if err != nil {
		return nil, err
	}
	registration, err := m.meter.RegisterCallback(func(_ context.Context, observer metric.Observer) error {
		s := snapshot()
		observer.ObserveInt64(active, s.Active)
		observer.ObserveInt64(idle, s.Idle)
		observer.ObserveInt64(maximum, s.Max)
		observer.ObserveInt64(waits, s.WaitCount)
		observer.ObserveFloat64(waitTime, s.WaitDuration.Seconds())
		observer.ObserveInt64(canceled, s.CanceledAcquires)
		return nil
	}, active, idle, maximum, waits, waitTime, canceled)
	if err != nil {
		return nil, err
	}
	return registration.Unregister, nil
}
