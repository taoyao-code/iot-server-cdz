package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	cfgpkg "github.com/taoyao-code/iot-server/internal/config"
)

// Client Redis客户端封装 (Week2.2)
type Client struct {
	*redis.Client
}

// NewClient 创建Redis客户端
func NewClient(cfg cfgpkg.RedisConfig) (*Client, error) {
	if !cfg.Enabled {
		return nil, fmt.Errorf("redis is not enabled")
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	})

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}

	return &Client{Client: rdb}, nil
}

// Close 关闭Redis连接
func (c *Client) Close() error {
	if c.Client != nil {
		return c.Client.Close()
	}
	return nil
}

// HealthCheck 健康检查
func (c *Client) HealthCheck(ctx context.Context) error {
	return c.Ping(ctx).Err()
}

// Stats 获取连接池统计
func (c *Client) Stats() *redis.PoolStats {
	return c.PoolStats()
}
