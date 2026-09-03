package app

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/andranikasd/marumbot/internal/obs"
	"github.com/andranikasd/marumbot/pkg/core/plan"
)

func TestSearchCacheMetrics(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(t.Context()) })
	c := searchCache{metrics: obs.NewPlanSearchMetrics(provider.Meter("test"))}
	in, goal := cacheInput(t), plan.Goal{Kind: plan.LeastInterest}
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	first, err := c.search(in, goal, now)
	if err != nil {
		t.Fatal(err)
	}
	hit, err := c.search(in, goal, now)
	if err != nil || !reflect.DeepEqual(first, hit) {
		t.Fatal("cache hit changed result", err)
	}
	expired, err := c.search(in, goal, now.Add(searchCacheTTL))
	if err != nil || !reflect.DeepEqual(first, expired) {
		t.Fatal("expiry changed result", err)
	}
	for i := 0; i < 2; i++ {
		if _, err := c.search(plan.Input{}, goal, now); !errors.Is(err, plan.ErrNoLoans) {
			t.Fatalf("uncached error changed: %v", err)
		}
	}
	if len(c.entries) != 1 {
		t.Fatal("failed search was cached")
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.search(in, goal, now.Add(searchCacheTTL)); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()

	var data metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &data); err != nil {
		t.Fatal(err)
	}
	lookups := map[string]int64{}
	searches := map[string]uint64{}
	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			switch m.Name {
			case "marum_plan_search_cache_lookups_total":
				d := m.Data.(metricdata.Sum[int64])
				if !d.IsMonotonic {
					t.Fatal("lookup counter must be monotonic")
				}
				for _, p := range d.DataPoints {
					if p.Attributes.Len() != 1 {
						t.Fatal("unexpected labels")
					}
					v, _ := p.Attributes.Value(attribute.Key("result"))
					lookups[v.AsString()] = p.Value
				}
			case "marum_plan_search_duration_seconds":
				if m.Unit != "s" {
					t.Fatal("duration unit")
				}
				for _, p := range m.Data.(metricdata.Histogram[float64]).DataPoints {
					if p.Attributes.Len() != 1 || p.Sum < 0 {
						t.Fatal("invalid duration/labels")
					}
					v, _ := p.Attributes.Value(attribute.Key("result"))
					searches[v.AsString()] = p.Count
				}
			default:
				t.Fatalf("unexpected metric %s", m.Name)
			}
		}
	}
	if !reflect.DeepEqual(lookups, map[string]int64{"hit": 9, "miss": 4}) {
		t.Fatalf("lookups: %v", lookups)
	}
	if !reflect.DeepEqual(searches, map[string]uint64{"ok": 2, "error": 2}) {
		t.Fatalf("searches: %v", searches)
	}
}
