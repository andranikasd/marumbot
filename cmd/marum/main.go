// Command marum is the whole application: webhook, command worker, scheduler,
// sender and the private admin interface, in one binary selected by
// configuration.
package main

import (
	"bufio"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/andranikasd/marumbot/internal/adapter/in/admin"
	"github.com/andranikasd/marumbot/internal/adapter/in/miniapp"
	"github.com/andranikasd/marumbot/internal/adapter/in/telegram"
	"github.com/andranikasd/marumbot/internal/adapter/out/postgres"
	"github.com/andranikasd/marumbot/internal/adapter/out/sysclock"
	"github.com/andranikasd/marumbot/internal/adapter/out/telegramclient"
	"github.com/andranikasd/marumbot/internal/app"
	"github.com/andranikasd/marumbot/internal/config"
	"github.com/andranikasd/marumbot/internal/i18n"
	"github.com/andranikasd/marumbot/internal/identity"
	"github.com/andranikasd/marumbot/internal/obs"
	"github.com/andranikasd/marumbot/pkg/core/money"

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

	// Identifiers are sealed before they reach the database, so the key is built
	// here and the store never sees it. A bad key is fatal: running without one
	// would mean writing Telegram ids in the clear.
	cipher, err := identity.New(cfg.IdentityKey)
	if err != nil {
		return fmt.Errorf("identity key: %w", err)
	}

	bot := telegramclient.New(cfg.BotToken)
	worker := &app.Worker{
		Inbox: store, Users: store, Loans: store, Budgets: store, Convos: store,
		Chats:           postgres.ChatLookup{Store: store, Cipher: cipher},
		Send:            bot,
		Clock:           sysclock.New(),
		Owner:           cfg.InstanceID,
		Log:             log,
		MiniApp:         cfg.MiniAppURL,
		Menus:           bot,
		DefaultCurrency: money.MustLookup(cfg.DefaultCurrency),
	}
	// The Mini App is served from the public listener under /app, so it shares
	// the Worker's hostname and needs no second custom domain or certificate.
	mini := &miniapp.Server{
		BotToken: cfg.BotToken, Loans: store, Users: store, Budgets: store,
		Cipher: cipher, Clock: sysclock.New(), Log: log,
	}
	hook := &telegram.Webhook{
		Inbox: store, Users: store, Cipher: cipher,
		ServiceToken: cfg.ServiceToken, Timezone: cfg.DefaultTimezone,
		Clock: sysclock.New(), Handle: worker.HandleOne, Log: log,
	}

	public := &http.Server{
		Addr: cfg.Addr,
		Handler: otelhttp.NewHandler(
			publicRoutes(adminSvc, hook, worker, mini, cfg.ServiceToken, log),
			"marum", otelhttp.WithFilter(notHealthCheck)),
		ReadHeaderTimeout: 5 * time.Second,
	}
	servers := []*http.Server{public}

	// The admin interface stays down unless a password hash is configured. A
	// misconfigured deployment gets no admin interface rather than an open one.
	if cfg.AdminEnabled() {
		srv, err := admin.New(adminSvc, admin.Config{
			User: cfg.AdminUser, PasswordHash: cfg.AdminPassHash,
			Version: cfg.Version, Env: cfg.Env, Now: time.Now,
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

	// Tell Telegram what this bot offers, in both languages, and point the chat
	// menu button at the loan form. Without this the chat suggests nothing when
	// a user types "/" and the Mini App has no permanent way in.
	//
	// Not fatal: a bot with no suggestions is worse to use but still works, and
	// refusing to start because Telegram was briefly busy is worse than that.
	go func() {
		menuCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := app.PublishMenus(menuCtx, bot, cfg.MiniAppURL); err != nil {
			log.Warn("publishing the command menus failed", "err", err)
		} else {
			log.Info("command menus published", "languages", len(i18n.Supported()))
		}
	}()

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

func publicRoutes(a *app.Admin, hook *telegram.Webhook, w *app.Worker,
	mini *miniapp.Server, serviceToken string, log *slog.Logger,
) http.Handler {
	mux := http.NewServeMux()

	// The Worker forwards verified Telegram updates here. It has already checked
	// Telegram's secret token; the service token proves the call came from the
	// Worker rather than from anyone who found the container.
	mux.Handle("POST /tg/update", hook.Handler())

	// The scheduler tick. Idempotent and safe to run concurrently, so a
	// duplicate costs nothing and a missed one is caught by the next.
	mux.Handle("POST /internal/tick", tickHandler(w, serviceToken, log))

	// The Mini App. It authenticates every call with Telegram's signed
	// initData, so it needs no service token and is safe to expose.
	// StripPrefix takes "/app", not "/app/": stripping the trailing slash too
	// would leave a path with no leading slash, which the inner mux answers
	// with a redirect instead of the page.
	mux.Handle("/app/", http.StripPrefix("/app", mini.Handler()))

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

// tickHandler drains the command inbox. It is bounded rather than looping until
// empty: a tick that runs forever holds a Worker request open past its limit,
// and the next tick is only minutes away.
func tickHandler(w *app.Worker, serviceToken string, log *slog.Logger) http.Handler {
	const batch = 25
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if serviceToken != "" &&
			subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Marum-Service-Token")), []byte(serviceToken)) != 1 {
			rw.WriteHeader(http.StatusUnauthorized)
			return
		}
		ctx, span := obs.ComponentScheduler.Enter(r.Context(), "tick")
		defer span.End()

		n, err := w.Drain(ctx, batch)
		if err != nil {
			span.RecordError(err)
			log.ErrorContext(ctx, "tick failed", "error", err)
			writeJSON(rw, http.StatusInternalServerError, map[string]any{"error": "tick failed"})
			return
		}
		// Logged on every tick, including the empty ones. A scheduler that is
		// not running looks exactly like a scheduler with nothing to do, and
		// the difference matters: the tick is also what keeps the container
		// awake, so its absence turns every message into a cold start.
		log.InfoContext(ctx, "tick", "handled", n)
		writeJSON(rw, http.StatusOK, map[string]any{"handled": n})
	})
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

// printPasswordHash turns a password into the configured hash without ever
// putting the password itself on a command line, where it would land in shell
// history and in the process table.
//
// The hash goes to stdout and everything else to stderr, so the output can be
// piped straight into a file or an environment without capturing the prompt.
func printPasswordHash() error {
	// MARUM_ADMIN_PASSWORD lets this run unattended - in a provisioning script
	// or a CI job - without a terminal.
	pw := os.Getenv("MARUM_ADMIN_PASSWORD")

	if pw == "" {
		fmt.Fprint(os.Stderr, "password: ")
		// A line reader, not fmt.Scanln: Scanln stops at the first space, so a
		// passphrase silently becomes its first word.
		sc := bufio.NewScanner(os.Stdin)
		if !sc.Scan() {
			if err := sc.Err(); err != nil {
				return fmt.Errorf("reading password: %w", err)
			}
			return errors.New("no password given: pass one on stdin or set MARUM_ADMIN_PASSWORD")
		}
		pw = strings.TrimRight(sc.Text(), "\r\n")
		fmt.Fprintln(os.Stderr)
	}

	h, err := admin.HashPassword(pw)
	if err != nil {
		return err
	}
	fmt.Println(h)
	return nil
}
