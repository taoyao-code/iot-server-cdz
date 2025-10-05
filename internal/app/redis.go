package app

import (
	"fmt"

	cfgpkg "github.com/taoyao-code/iot-server/internal/config"
	"github.com/taoyao-code/iot-server/internal/health"
	"github.com/taoyao-code/iot-server/internal/outbound"
	redisstorage "github.com/taoyao-code/iot-server/internal/storage/redis"
	"go.uber.org/zap"
)

// NewRedisClient 创建Redis客户端
// Redis是必选依赖，用于分布式会话管理和消息队列
func NewRedisClient(cfg cfgpkg.RedisConfig, logger *zap.Logger) (*redisstorage.Client, error) {
	// Redis是生产环境必选依赖
	if !cfg.Enabled {
		return nil, fmt.Errorf("redis is required for production deployment (set redis.enabled=true in config)")
	}

	client, err := redisstorage.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	logger.Info("redis client initialized",
		zap.String("addr", cfg.Addr),
		zap.Int("pool_size", cfg.PoolSize))

	return client, nil
}

// NewRedisOutboundQueue 创建Redis下行队列 (Week2.2)
func NewRedisOutboundQueue(client *redisstorage.Client) *redisstorage.OutboundQueue {
	return redisstorage.NewOutboundQueue(client)
}

// NewRedisWorker 创建Redis Worker (Week2.2)
func NewRedisWorker(
	queue *redisstorage.OutboundQueue,
	throttleMs int,
	retryMax int,
	logger *zap.Logger,
) *outbound.RedisWorker {
	return outbound.NewRedisWorker(queue, throttleMs, retryMax, logger)
}

// AddRedisChecker 添加Redis检查器到聚合器 (Week2.2)
func AddRedisChecker(aggregator *health.Aggregator, redisClient *redisstorage.Client) {
	if redisClient != nil {
		aggregator.AddChecker(health.NewRedisChecker(redisClient))
	}
}
