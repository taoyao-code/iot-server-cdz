package pg

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool 创建 pgx 连接池
func NewPool(ctx context.Context, dsn string, maxOpen, maxIdle int, maxLifetime time.Duration) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	// pgxpool 仅提供 MaxConns；其余策略可后续通过 pgbouncer 处理
	if maxOpen > 0 {
		cfg.MaxConns = int32(maxOpen)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	// 探活
	ctxPing, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := pool.Ping(ctxPing); err != nil {
		pool.Close()
		return nil, err
	}
	_ = maxIdle
	_ = maxLifetime
	return pool, nil
}
