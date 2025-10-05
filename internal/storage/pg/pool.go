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

	// Week2: 优化连接池配置
	if maxOpen > 0 {
		cfg.MaxConns = int32(maxOpen)
	} else {
		cfg.MaxConns = 20 // 默认20个连接（提升自10）
	}

	if maxIdle > 0 {
		cfg.MinConns = int32(maxIdle)
	} else {
		cfg.MinConns = 5 // 默认保持5个空闲连接（预热）
	}

	if maxLifetime > 0 {
		cfg.MaxConnLifetime = maxLifetime
	} else {
		cfg.MaxConnLifetime = 1 * time.Hour // 连接最大生命周期1小时
	}

	cfg.MaxConnIdleTime = 30 * time.Minute  // 空闲连接30分钟后关闭
	cfg.HealthCheckPeriod = 1 * time.Minute // 每分钟健康检查

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

	return pool, nil
}
