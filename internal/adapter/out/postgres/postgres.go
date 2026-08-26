// Package postgres is the only place that talks to the database.
//
// Every statement comes from the embedded queries package; no SQL string is
// written here. Business rules do not live in this package: a rule in an
// adapter is a rule that cannot be tested without a socket.
package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/andranikasd/marumbot/internal/obs"
	"github.com/andranikasd/marumbot/queries"
)

// Store owns the connection pool.
type Store struct{ pool *pgxpool.Pool }

// Open connects and verifies the database is reachable. It fails fast: a
// service that starts and then cannot serve is worse than one that refuses.
//
// The metrics handle may be nil, in which case queries are still traced but
// not timed - useful in tests, and in a self-hosted deployment with telemetry
// switched off entirely.
func Open(ctx context.Context, dsn string, m *obs.Metrics) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parsing database url: %w", err)
	}
	cfg.MaxConns = 8
	cfg.MaxConnLifetime = time.Hour
	cfg.HealthCheckPeriod = 30 * time.Second
	cfg.ConnConfig.Tracer = newQueryTracer(m)

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connecting: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

// Ping reports whether the database is reachable. Used by readiness, never by
// liveness — a liveness probe that depends on Postgres turns a database blip
// into a restart loop.
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// Pool exposes the underlying pool for packages that need transactions.
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// MigrationVersion returns the applied schema version, so readiness can refuse
// to serve against a schema the binary does not expect.
func (s *Store) MigrationVersion(ctx context.Context) (int64, error) {
	var v int64
	err := s.pool.QueryRow(ctx,
		`SELECT max(version_id) FROM goose_db_version WHERE is_applied`).Scan(&v)
	return v, err
}

// q is a shorthand that keeps call sites readable while ensuring every
// statement is looked up by name from the embedded files.
func q(name string) string { return queries.Get(name) }
