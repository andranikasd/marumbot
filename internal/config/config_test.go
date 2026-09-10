package config

import (
	"strings"
	"testing"
	"time"
)

func validConfig() Config {
	return Config{
		Env: "dev", Mode: "polling", DatabaseURL: "postgres://localhost/marum",
		BotToken: "test", IdentityKey: "test", DefaultCurrency: "AMD",
		DefaultTimezone: "Asia/Yerevan", TickInterval: time.Minute,
	}
}

func TestDeploymentSettingsValidation(t *testing.T) {
	for _, tc := range []struct {
		name, setting string
		change        func(*Config)
	}{
		{"unknown environment", "MARUM_ENV", func(c *Config) { c.Env = "production" }},
		{"missing production journal", "MARUM_ERASURE_JOURNAL_DIR", func(c *Config) { c.Env = "prod" }},
		{"relative journal", "MARUM_ERASURE_JOURNAL_DIR", func(c *Config) { c.ErasureJournalDir = "journal" }},
		{"unknown currency", "MARUM_DEFAULT_CURRENCY", func(c *Config) { c.DefaultCurrency = "INVALID" }},
		{"relative URL", "MARUM_MINIAPP_URL", func(c *Config) { c.MiniAppURL = "/app/" }},
		{"missing host", "MARUM_MINIAPP_URL", func(c *Config) { c.MiniAppURL = "https:///app/" }},
		{"unsupported scheme", "MARUM_MINIAPP_URL", func(c *Config) { c.MiniAppURL = "javascript:alert(1)" }},
		{"credentials", "MARUM_MINIAPP_URL", func(c *Config) { c.MiniAppURL = "https://secret:password@example.test/app/" }},
		{"query", "MARUM_MINIAPP_URL", func(c *Config) { c.MiniAppURL = "https://example.test/app/?x=1" }},
		{"empty query", "MARUM_MINIAPP_URL", func(c *Config) { c.MiniAppURL = "https://example.test/app/?" }},
		{"fragment", "MARUM_MINIAPP_URL", func(c *Config) { c.MiniAppURL = "https://example.test/app/#loans" }},
		{"invalid escape", "MARUM_MINIAPP_URL", func(c *Config) { c.MiniAppURL = "https://example.test/%zz" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := validConfig()
			tc.change(&c)
			err := c.validate()
			if err == nil || !strings.Contains(err.Error(), tc.setting) {
				t.Fatalf("error = %v, want setting %s", err, tc.setting)
			}
			if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "password") {
				t.Fatal("error disclosed URL credentials")
			}
		})
	}
}

func TestSupportedDeploymentSettings(t *testing.T) {
	for _, env := range []string{"dev", "prod"} {
		for _, appURL := range []string{"", "https://example.test/app/", "https://example.test/app", "http://localhost:8080/app/", "http://[::1]:8080/app/"} {
			c := validConfig()
			c.Env, c.MiniAppURL, c.DefaultCurrency = env, appURL, " amd "
			if env == "prod" {
				c.ErasureJournalDir = "/var/lib/marum/erasures"
			}
			if err := c.validate(); err != nil {
				t.Fatalf("%s %s: %v", env, appURL, err)
			}
		}
	}
}

func TestDevelopmentJournalMayBeDisabledOrAbsolute(t *testing.T) {
	for _, dir := range []string{"", "/var/lib/marum/erasures"} {
		c := validConfig()
		c.ErasureJournalDir = dir
		if err := c.validate(); err != nil {
			t.Fatalf("development journal configuration rejected: %v", err)
		}
	}
}
