// Command marum is the whole application: webhook, command worker, scheduler,
// sender and the private admin interface, in one binary selected by
// configuration.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/andranikasd/marumbot/internal/adapter/in/admin"
	"github.com/andranikasd/marumbot/internal/adapter/out/postgres"
	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/internal/config"
	"github.com/andranikasd/marumbot/internal/obs"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// notHealthCheck keeps the liveness probe out of tracing. A check every few
// seconds would otherwise be the most-traced operation in the system and would
// tell nobody anything.
func notHealthCheck(r *http.Request) bool { return r.URL.Path != "/healthz" }

var version = "dev"

func main() {
	hashPassword := flag.Bool("hash-password", false,
		"read a password from stdin and print the value for MARUM_ADMIN_PASSWORD_HASH")
	flag.Parse()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	if *hashPassword {
		if err := printPasswordHash(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	if err := run(log); err != nil {
		log.Error("marum stopped", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error { //nolint:gocyclo // wiring is linear, not complex
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.Version = version

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	telemetry, err := obs.Init(ctx, obs.Config{
		ServiceName:   "marum",
		Env:           cfg.Env,
		InstanceID:    cfg.InstanceID,
		Version:       cfg.Version,
		OTLPEndpoint:  cfg.OTLPEndpoint,
		PyroscopeAddr: cfg.PyroscopeAddr,
		PyroscopeUser: os.Getenv("PYROSCOPE_BASIC_AUTH_USER"),
		PyroscopePass: os.Getenv("PYROSCOPE_BASIC_AUTH_PASSWORD"),
	})
	if err != nil {
		// Telemetry must never be the reason the service will not start, but a
		// service reporting nothing looks exactly like a service doing nothing,
		// so this is an error rather than a warning.
		telemetry.Logger.Error("TELEMETRY DEGRADED - no traces, metrics or logs will be exported", "err", err)
	}
	log = telemetry.Logger
	defer func() {
		flushCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := telemetry.Shutdown(flushCtx); err != nil {
			log.Warn("flushing telemetry", "err", err)
		}
	}()

	store, err := postgres.Open(ctx, cfg.DatabaseURL, telemetry.Meter)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer store.Close()

	adminSvc := app.NewAdmin(store)

	public := &http.Server{
		Addr:              cfg.Addr,
		Handler:           otelhttp.NewHandler(publicRoutes(adminSvc), "marum", otelhttp.WithFilter(notHealthCheck)),
		ReadHeaderTimeout: 5 * time.Second,
	}
	servers := []*http.Server{public}

	// The admin interface stays down unless a password hash is configured. A
	// misconfigured deployment gets no admin interface rather than an open one.
	if cfg.AdminEnabled() {
		srv, err := admin.New(adminSvc, admin.Config{
			User: cfg.AdminUser, PasswordHash: cfg.AdminPassHash,
			Version: cfg.Version, Env: cfg.Env,
		}, log)
		if err != nil {
			return fmt.Errorf("admin interface: %w", err)
		}
		servers = append(servers, &http.Server{
			Addr: cfg.AdminAddr,
			// Not wrapped in otelhttp: the admin handler opens its own SERVER
			// span under its own service name, which is what gives the service
			// graph an inbound edge to the admin node.
			Handler:           srv.Handler(),
			ReadHeaderTimeout: 5 * time.Second,
		})
	} else {
		log.Warn("admin interface disabled: MARUM_ADMIN_PASSWORD_HASH is not set")
	}

	for _, s := range servers {
		go func(s *http.Server) {
			if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Error("listener failed", "addr", s.Addr, "err", err)
				stop()
			}
		}(s)
	}

	log.Info("marum listening",
		"addr", cfg.Addr, "admin", cfg.AdminAddr, "mode", cfg.Mode, "env", cfg.Env,
		"admin_enabled", cfg.AdminEnabled(), "telemetry", cfg.TelemetryEnabled(),
		"version", cfg.Version)

	<-ctx.Done()
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for _, s := range servers {
		if err := s.Shutdown(shutdownCtx); err != nil {
			log.Error("graceful shutdown failed", "addr", s.Addr, "err", err)
		}
	}
	return nil
}

func publicRoutes(a *app.Admin) http.Handler {
	mux := http.NewServeMux()

	// Liveness performs no database call: a probe that depends on Postgres
	// turns a database blip into a restart loop.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	})

	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		h := a.Health(r.Context())
		code := http.StatusOK
		if !h.DatabaseOK {
			code = http.StatusServiceUnavailable
		}
		writeJSON(w, code, map[string]any{
			"status":            map[bool]string{true: "ok", false: "degraded"}[h.DatabaseOK],
			"database":          h.DatabaseOK,
			"migration_version": h.MigrationVersion,
		})
	})

	// The synthetic check reads this. It asserts more than a 200, so the
	// numbers a watchdog needs come from bounded queries rather than from
	// process-local state that dies with the process.
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
		h := a.Health(r.Context())
		body := map[string]any{"database": h.DatabaseOK, "migration_version": h.MigrationVersion}
		if o, err := a.Overview(r.Context()); err == nil {
			body["oldest_pending_command_s"] = o.OldestCommandAgeS
			body["oldest_pending_delivery_s"] = o.OldestDeliveryAgeS
			body["commands_pending"] = o.CommandsPending
			body["deliveries_pending"] = o.DeliveriesPending
		}
		writeJSON(w, http.StatusOK, body)
	})
	return mux
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

// printPasswordHash turns a password into the configured hash without ever
// putting the password itself on a command line, where it would land in shell
// history and in the process table.
func printPasswordHash() error {
	fmt.Fprint(os.Stderr, "password: ")
	var pw string
	if _, err := fmt.Scanln(&pw); err != nil {
		return fmt.Errorf("reading password: %w", err)
	}
	h, err := admin.HashPassword(pw)
	if err != nil {
		return err
	}
	fmt.Println(h)
	return nil
}
