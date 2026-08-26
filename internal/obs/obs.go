// Package obs wires telemetry: traces, metrics, logs and continuous profiles.
//
// One OTLP endpoint carries traces, metrics and logs, in development and in
// production alike - locally an OpenTelemetry Collector, in production the
// Grafana Cloud gateway. Only the URL differs, so what is exercised in dev is
// what runs in prod.
//
// An empty endpoint disables telemetry entirely, which is the right default
// for a self-hoster who has no interest in running an observability stack.
package obs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/grafana/pyroscope-go"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Config is what telemetry needs. It mirrors the standard OTEL_ environment
// variables, which the exporters read for the endpoint and headers themselves.
type Config struct {
	ServiceName   string
	Env           string
	InstanceID    string
	Version       string
	OTLPEndpoint  string // empty disables traces, metrics and logs
	PyroscopeAddr string // empty disables profiling
	PyroscopeUser string
	PyroscopePass string
}

// Providers is what the application holds on to. Shutdown flushes everything;
// a twenty-second grace beats losing the telemetry of the crash you are trying
// to explain.
type Providers struct {
	Logger   *slog.Logger
	Meter    *Metrics
	shutdown []func(context.Context) error
}

func (p *Providers) Shutdown(ctx context.Context) error {
	var errs []error
	for i := len(p.shutdown) - 1; i >= 0; i-- {
		if err := p.shutdown[i](ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Init starts telemetry. It never returns a nil Logger: if export is disabled
// or fails, logging falls back to stdout so the service is never silent.
func Init(ctx context.Context, cfg Config) (*Providers, error) {
	p := &Providers{Logger: stdoutLogger()}

	// NewSchemaless, not NewWithAttributes: pinning a semconv schema URL here
	// makes Merge fail the moment the SDK's own default resource moves to a
	// newer one, and that failure silently disables every signal. The
	// attribute keys are stable regardless of schema version.
	//
	// service.version is deliberately absent: resource attributes become
	// metric labels, and a label that changes on every deploy multiplies the
	// whole catalogue by the number of releases. The version is reported once
	// by the build_info gauge instead.
	res, err := resource.Merge(resource.Default(), resource.NewSchemaless(
		attribute.String("service.name", cfg.ServiceName),
		attribute.String("service.instance.id", cfg.InstanceID),
		// One environment attribute, not two: Loki indexes both the old and
		// the new semconv spelling, which would give two labels for one fact.
		attribute.String("deployment.environment.name", cfg.Env),
	))
	if err != nil {
		// Non-fatal, but loud: a service that reports nothing looks identical
		// to a service that is idle.
		return p, fmt.Errorf("building resource: %w", err)
	}

	if cfg.OTLPEndpoint == "" {
		p.Logger.Info("telemetry disabled: no OTLP endpoint configured")
		p.Meter = newMetrics(otel.GetMeterProvider(), cfg.Version)
		return p, nil
	}

	// Traces. Sampled at 100%: at this scale a full trace costs a few percent
	// of the allowance and removes an entire class of "the interesting request
	// wasn't sampled" investigation.
	texp, err := otlptracehttp.New(ctx)
	if err != nil {
		return p, fmt.Errorf("trace exporter: %w", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.AlwaysSample())),
		sdktrace.WithBatcher(texp),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{}) // W3C only, no B3
	p.shutdown = append(p.shutdown, tp.Shutdown)

	mexp, err := otlpmetrichttp.New(ctx)
	if err != nil {
		return p, fmt.Errorf("metric exporter: %w", err)
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(mexp,
			sdkmetric.WithInterval(15*time.Second))),
	)
	otel.SetMeterProvider(mp)
	p.shutdown = append(p.shutdown, mp.Shutdown)

	lexp, err := otlploghttp.New(ctx)
	if err != nil {
		return p, fmt.Errorf("log exporter: %w", err)
	}
	lp := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(lexp)),
	)
	p.shutdown = append(p.shutdown, lp.Shutdown)

	// Logs go to stdout and to OTLP, both through the redacting handler, so
	// there is exactly one place an amount can be stripped.
	// Order matters: enrich with the trace first so trace_id reaches both
	// sinks, then redact, so nothing sensitive can be added after the strip.
	p.Logger = slog.New(newTraceEnricher(newRedactor(newFanout(
		slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}),
		otelslog.NewHandler(cfg.ServiceName, otelslog.WithLoggerProvider(lp)),
	))))
	slog.SetDefault(p.Logger)

	// Free coverage: goroutines, heap, GC pauses, scheduler latency.
	if err := runtime.Start(runtime.WithMinimumReadMemStatsInterval(10 * time.Second)); err != nil {
		p.Logger.Warn("runtime metrics unavailable", "err", err)
	}

	p.Meter = newMetrics(mp, cfg.Version)

	if stop := startProfiler(cfg, p.Logger); stop != nil {
		p.shutdown = append(p.shutdown, func(context.Context) error { return stop() })
	}

	p.Logger.Info("telemetry started",
		"endpoint", cfg.OTLPEndpoint, "profiles", cfg.PyroscopeAddr != "")
	return p, nil
}

// startProfiler begins continuous profiling. The upload interval is 60s rather
// than the SDK default of 15s: five profile types on two instances at 15s is a
// coin flip against the free-tier allowance, and buys resolution nobody needs
// for a service handling a few requests per second.
func startProfiler(cfg Config, log *slog.Logger) func() error {
	if cfg.PyroscopeAddr == "" {
		return nil
	}
	prof, err := pyroscope.Start(pyroscope.Config{
		ApplicationName:   cfg.ServiceName,
		ServerAddress:     cfg.PyroscopeAddr,
		BasicAuthUser:     cfg.PyroscopeUser,
		BasicAuthPassword: cfg.PyroscopePass,
		UploadRate:        60 * time.Second,
		Logger:            nil,
		Tags: map[string]string{
			"env":      cfg.Env,
			"instance": cfg.InstanceID,
			// Safe here: profile tags are not metric series.
			"version": cfg.Version,
		},
		ProfileTypes: []pyroscope.ProfileType{
			pyroscope.ProfileCPU,
			pyroscope.ProfileAllocObjects,
			pyroscope.ProfileAllocSpace,
			pyroscope.ProfileInuseObjects,
			pyroscope.ProfileInuseSpace,
			// Mutex, block and goroutine profiles add runtime overhead and
			// double the volume. They are investigation settings, not defaults.
		},
	})
	if err != nil {
		log.Warn("continuous profiling unavailable", "err", err)
		return nil
	}
	return prof.Stop
}

func stdoutLogger() *slog.Logger {
	return slog.New(newTraceEnricher(newRedactor(
		slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))))
}
