// Package postgres is the only place that talks to the database.
//
// Every statement comes from the embedded queries package; no SQL string is
// written here. Business rules do not live in this package: a rule in an
// adapter is a rule that cannot be tested without a socket.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/andranikasd/marumbot/internal/obs"
	"github.com/andranikasd/marumbot/queries"
)

// Store owns the connection pool.
type Store struct {
	pool       *pgxpool.Pool
	unregister func() error
	closeOnce  sync.Once
}

func poolConfig(dsn string) (*pgxpool.Config, error) {
	// Let pgx parse URL and keyword DSNs (including quoted values). Its pool
	// parser removes pool options, so inspect presence before that conversion.
	conn, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	if _, set := conn.RuntimeParams["pool_max_conns"]; !set {
		cfg.MaxConns = 8
	}
	if cfg.MinConns < 0 || cfg.MinConns > cfg.MaxConns {
		return nil, errors.New("pool_min_conns must be between zero and pool_max_conns")
	}
	cfg.MaxConnLifetime = time.Hour
	cfg.HealthCheckPeriod = 30 * time.Second
	return cfg, nil
}

// Open connects and verifies the database is reachable. It fails fast: a
// service that starts and then cannot serve is worse than one that refuses.
//
// The metrics handle may be nil, in which case queries are still traced but
// not timed - useful in tests, and in a self-hosted deployment with telemetry
// switched off entirely.
func Open(ctx context.Context, dsn string, m *obs.Metrics) (*Store, error) {
	cfg, err := poolConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parsing database url: %w", err)
	}
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
	s := &Store{pool: pool}
	if err := s.observePool(m); err != nil {
		s.Close()
		return nil, fmt.Errorf("registering pool metrics: %w", err)
	}
	return s, nil
}

func (s *Store) observePool(m *obs.Metrics) error {
	stop, err := m.RegisterDBPool(func() obs.DBPoolStats {
		stat := s.pool.Stat()
		return obs.DBPoolStats{
			Active: int64(stat.AcquiredConns()), Idle: int64(stat.IdleConns()), Max: int64(stat.MaxConns()),
			WaitCount: stat.EmptyAcquireCount(), WaitDuration: stat.EmptyAcquireWaitTime(),
			CanceledAcquires: stat.CanceledAcquireCount(),
		}
	})
	if err != nil {
		return err
	}
	s.unregister = stop
	return nil
}

func (s *Store) Close() {
	s.closeOnce.Do(func() {
		if s.unregister != nil {
			_ = s.unregister()
		}
		s.pool.Close()
	})
}

// Ping reports whether the database is reachable. Used by readiness, never by
// liveness — a liveness probe that depends on Postgres turns a database blip
// into a restart loop.
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// MigrationVersion returns the applied schema version, so readiness can refuse
// to serve against a schema the binary does not expect.
func (s *Store) MigrationVersion(ctx context.Context) (int64, error) {
	var v int64
	err := s.pool.QueryRow(ctx, q("MigrationVersion")).Scan(&v)
	return v, err
}

// q is a shorthand that keeps call sites readable while ensuring every
// statement is looked up by name from the embedded files.
func q(name string) string { return queries.Get(name) }
