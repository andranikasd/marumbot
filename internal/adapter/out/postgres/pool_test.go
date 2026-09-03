package postgres

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/andranikasd/marumbot/internal/obs"
)

func TestPoolConfigHonorsDSNLimits(t *testing.T) {
	t.Setenv("PGSERVICE", "")
	for _, tc := range []struct {
		name, dsn        string
		maximum, minimum int32
	}{
		{"default", "postgres://localhost/marum?sslmode=disable", 8, 0},
		{"minimum only", "postgres://localhost/marum?sslmode=disable&pool_min_conns=3", 8, 3},
		{"small URL", "postgres://localhost/marum?sslmode=disable&pool_max_conns=2&pool_min_conns=1", 2, 1},
		{"large URL", "postgres://localhost/marum?sslmode=disable&pool_max_conns=32&pool_min_conns=12", 32, 12},
		{"keyword quoted", "host=localhost dbname=marum sslmode=disable pool_max_conns='12' pool_min_conns='4'", 12, 4},
		{"zero minimum", "host=localhost pool_max_conns=4 pool_min_conns=0", 4, 0},
		{"option text inside value", "host=localhost application_name='pool_max_conns=64 pool_min_conns=32'", 8, 0},
		{"encoded option name", "postgres://localhost/marum?sslmode=disable&pool%5Fmax%5Fconns=3", 3, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := poolConfig(tc.dsn)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.MaxConns != tc.maximum || cfg.MinConns != tc.minimum {
				t.Fatalf("max=%d min=%d want max=%d min=%d", cfg.MaxConns, cfg.MinConns, tc.maximum, tc.minimum)
			}
			for _, key := range []string{"pool_max_conns", "pool_min_conns"} {
				if _, ok := cfg.ConnConfig.RuntimeParams[key]; ok {
					t.Fatal("pool option would reach Postgres")
				}
			}
		})
	}
}

func TestPoolConfigRejectsInvalidLimits(t *testing.T) {
	t.Setenv("PGSERVICE", "")
	for _, options := range []string{"pool_max_conns=0", "pool_max_conns=-1", "pool_max_conns=bad", "pool_max_conns=2147483648", "pool_min_conns=-1", "pool_max_conns=2 pool_min_conns=3", "pool_min_conns=9", "pool_min_conns=bad"} {
		if _, err := poolConfig("host=localhost " + options); err == nil {
			t.Fatalf("invalid options accepted: %s", options)
		}
	}
}

var errPoolTestConnect = errors.New("test must not connect")

// Zero minimums and a failing connection hook keep this real pool socket-free.
func emptyPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	cfg, err := poolConfig("host=127.0.0.1 port=1 dbname=unused sslmode=disable pool_max_conns=3 pool_min_conns=0 pool_min_idle_conns=0")
	if err != nil {
		t.Fatal(err)
	}
	cfg.BeforeConnect = func(context.Context, *pgx.ConnConfig) error { return errPoolTestConnect }
	p, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p
}

func TestStorePoolMetricsLifecycle(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	previous := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	t.Cleanup(func() {
		otel.SetMeterProvider(previous)
		_ = provider.Shutdown(context.Background())
	})
	telemetry, err := obs.Init(t.Context(), obs.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = telemetry.Shutdown(context.Background()) })
	// Already-canceled acquisition exits before dialing. A failed startup must
	// close its pool without ever registering a collection callback.
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	failed, err := Open(canceled, "host=127.0.0.1 port=1 sslmode=disable pool_min_conns=0 pool_min_idle_conns=0", telemetry.Meter)
	if failed != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled startup: store=%v err=%v", failed, err)
	}
	var data metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &data); err != nil {
		t.Fatal(err)
	}
	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			if strings.HasPrefix(m.Name, "marum_db_pool_") {
				t.Fatal("failed startup registered pool metrics")
			}
		}
	}
	s := &Store{pool: emptyPool(t)}
	if err := s.observePool(telemetry.Meter); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	if _, err := s.pool.Acquire(canceled); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled acquire did not fail immediately")
	}
	if err := reader.Collect(t.Context(), &data); err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			if !strings.HasPrefix(m.Name, "marum_db_pool_") {
				continue
			}
			found++
			if d, ok := m.Data.(metricdata.Gauge[int64]); ok {
				want := int64(0)
				if m.Name == "marum_db_pool_max_connections" {
					want = 3
				}
				if len(d.DataPoints) != 1 || d.DataPoints[0].Value != want || d.DataPoints[0].Attributes.Len() != 0 {
					t.Fatalf("unexpected snapshot %s: %+v", m.Name, d)
				}
			}
			if d, ok := m.Data.(metricdata.Sum[int64]); ok {
				want := int64(0)
				if m.Name == "marum_db_pool_canceled_acquires_total" {
					want = 1
				}
				if len(d.DataPoints) != 1 || d.DataPoints[0].Value != want || d.DataPoints[0].Attributes.Len() != 0 {
					t.Fatalf("unexpected counter %s: %+v", m.Name, d)
				}
			}
		}
	}
	if found != 6 {
		t.Fatalf("registered metrics=%d want 6", found)
	}
	s.Close()
	s.Close()
	if err := reader.Collect(t.Context(), &data); err != nil {
		t.Fatal(err)
	}
	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			if strings.HasPrefix(m.Name, "marum_db_pool_") {
				t.Fatalf("callback survived Store.Close: %s", m.Name)
			}
		}
	}
}

func TestStoreCloseUnregistersOnceBeforePoolClose(t *testing.T) {
	p := emptyPool(t)
	var stops atomic.Int64
	s := &Store{pool: p, unregister: func() error {
		stops.Add(1)
		if _, err := p.Acquire(t.Context()); !errors.Is(err, errPoolTestConnect) {
			t.Errorf("cleanup must run before pool closes: %v", err)
		}
		return nil
	}}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Close()
		}()
	}
	wg.Wait()
	if stops.Load() != 1 {
		t.Fatalf("cleanup ran %d times", stops.Load())
	}
	if _, err := p.Acquire(t.Context()); err == nil {
		t.Fatal("pool remains open")
	}
}
