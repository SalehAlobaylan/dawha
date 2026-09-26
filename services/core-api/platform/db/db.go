package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotConfigured = errors.New("database is not configured")

type PoolConfig struct {
	URL string
	// Tracer is an optional pgx tracer - in practice the telemetry package's,
	// which measures statement latency and nothing else. It is a field rather
	// than something this package builds, so the database layer has no opinion
	// about telemetry and a caller that wants no tracer passes none, which is
	// what every existing caller does.
	Tracer pgx.QueryTracer
}

func NewPool(ctx context.Context, config PoolConfig) (*pgxpool.Pool, error) {
	if config.URL == "" {
		return nil, nil
	}

	poolConfig, err := pgxpool.ParseConfig(config.URL)
	if err != nil {
		return nil, err
	}
	poolConfig.MaxConns = 8
	poolConfig.MinConns = 1
	poolConfig.MaxConnLifetime = 30 * time.Minute
	poolConfig.MaxConnIdleTime = 5 * time.Minute
	if config.Tracer != nil {
		poolConfig.ConnConfig.Tracer = config.Tracer
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, err
	}
	return pool, nil
}

func Ping(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return ErrNotConfigured
	}
	return pool.Ping(ctx)
}
