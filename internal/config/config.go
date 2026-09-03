// Package config parses and validates every setting once, at startup.
//
// A service that starts successfully and then fails on the first user request
// is strictly worse than one that refuses to start.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Env             string // dev | prod
	Mode            string // polling | webhook
	Addr            string // public HTTP listener
	AdminAddr       string // private admin listener
	DatabaseURL     string
	BotToken        string
	WebhookSecret   string
	ServiceToken    string // proves a request came from the Worker, not the internet
	IdentityKey     string // base64 32 bytes; seals Telegram identifiers at rest
	MiniAppURL      string // absolute https URL of the loan form
	DefaultCurrency string
	DefaultTimezone string
	TickInterval    time.Duration
	AdminUser       string
	AdminPassHash   string // pbkdf2 encoded hash, produced by -hash-password
	OTLPEndpoint    string // empty disables telemetry entirely
	PyroscopeAddr   string
	Version         string
	InstanceID      string
}

var ErrMissing = errors.New("required setting is missing")

func Load() (Config, error) {
	tick, err := dur("MARUM_TICK_INTERVAL", 60*time.Second)
	if err != nil {
		return Config{}, err
	}
	c := Config{
		Env:             str("MARUM_ENV", "dev"),
		Mode:            str("MARUM_MODE", "polling"),
		Addr:            str("MARUM_ADDR", ":8080"),
		AdminAddr:       str("MARUM_ADMIN_ADDR", ":8081"),
		DatabaseURL:     str("MARUM_DATABASE_URL", ""),
		BotToken:        str("MARUM_BOT_TOKEN", ""),
		WebhookSecret:   str("MARUM_WEBHOOK_SECRET", ""),
		ServiceToken:    str("MARUM_SERVICE_TOKEN", ""),
		IdentityKey:     str("MARUM_IDENTITY_KEY", ""),
		MiniAppURL:      str("MARUM_MINIAPP_URL", ""),
		DefaultCurrency: str("MARUM_DEFAULT_CURRENCY", "AMD"),
		DefaultTimezone: str("MARUM_DEFAULT_TZ", "Asia/Yerevan"),
		TickInterval:    tick,
		AdminUser:       str("MARUM_ADMIN_USER", "admin"),
		AdminPassHash:   str("MARUM_ADMIN_PASSWORD_HASH", ""),
		OTLPEndpoint:    str("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
		PyroscopeAddr:   str("PYROSCOPE_SERVER_ADDRESS", ""),
		Version:         str("MARUM_VERSION", "dev"),
		InstanceID:      str("MARUM_INSTANCE_ID", "local-1"),
	}
	return c, c.validate()
}

func (c Config) validate() error {
	if c.TickInterval <= 0 {
		return errors.New("MARUM_TICK_INTERVAL must be positive")
	}
	var missing []string
	if c.DatabaseURL == "" {
		missing = append(missing, "MARUM_DATABASE_URL")
	}
	if c.BotToken == "" {
		missing = append(missing, "MARUM_BOT_TOKEN")
	}
	if c.Mode == "webhook" && c.WebhookSecret == "" {
		missing = append(missing, "MARUM_WEBHOOK_SECRET (required in webhook mode)")
	}
	// Webhook mode exposes an HTTP listener; without a service token every check
	// on it silently disappears, so an unset value must refuse to start rather
	// than fail open.
	if c.Mode == "webhook" && c.ServiceToken == "" {
		missing = append(missing, "MARUM_SERVICE_TOKEN (required in webhook mode)")
	}
	// Without a key the service would store Telegram identifiers in the clear,
	// which is worse than refusing to start: the damage is silent and permanent.
	if c.IdentityKey == "" {
		missing = append(missing, "MARUM_IDENTITY_KEY")
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: %s", ErrMissing, strings.Join(missing, ", "))
	}
	switch c.Mode {
	case "polling", "webhook":
	default:
		return fmt.Errorf("MARUM_MODE must be polling or webhook, got %q", c.Mode)
	}
	if _, err := time.LoadLocation(c.DefaultTimezone); err != nil {
		return fmt.Errorf("MARUM_DEFAULT_TZ %q: %w", c.DefaultTimezone, err)
	}
	return nil
}

// TelemetryEnabled reports whether any OTLP exporter should be started. Empty
// means off, which is the right default for a self-hoster.
func (c Config) TelemetryEnabled() bool { return c.OTLPEndpoint != "" }

// AdminEnabled reports whether the private admin listener should start. Without
// a password hash it stays down rather than coming up unauthenticated.
func (c Config) AdminEnabled() bool { return c.AdminPassHash != "" }

func str(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func dur(key string, def time.Duration) (time.Duration, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def, nil
	}
	if secs, err := strconv.Atoi(v); err == nil {
		return time.Duration(secs) * time.Second, nil
	}
	if d, err := time.ParseDuration(v); err == nil {
		return d, nil
	}
	// Falling back to the default here would run a typo silently, which is the
	// exact failure mode this package exists to prevent.
	return 0, fmt.Errorf("%s %q is not a duration", key, v)
}
