package app

import (
	"context"

	"github.com/taoyao-code/iot-server/internal/config"
	"github.com/taoyao-code/iot-server/internal/storage/redis"
	"github.com/taoyao-code/iot-server/internal/thirdparty"
	"go.uber.org/zap"
)

// NewEventQueue 创建事件队列（如果配置启用）
func NewEventQueue(
	cfg config.ThirdpartyPushConfig,
	redisClient *redis.Client,
	pusher *thirdparty.Pusher,
	logger *zap.Logger,
) (*thirdparty.EventQueue, *thirdparty.Deduper) {
	// 检查是否启用事件队列
	if !cfg.EnableQueue || cfg.WebhookURL == "" {
		logger.Info("event queue disabled (webhook_url empty or enable_queue=false)")
		return nil, nil
	}

	if redisClient == nil {
		logger.Warn("event queue disabled: redis client not available")
		return nil, nil
	}

	if pusher == nil {
		logger.Warn("event queue disabled: pusher not available")
		return nil, nil
	}

	// 创建事件队列
	queue := thirdparty.NewEventQueue(
		redisClient.Client,
		pusher,
		cfg.WebhookURL,
		logger.With(zap.String("component", "event_queue")),
	)

	logger.Info("event queue initialized",
		zap.String("webhook_url", cfg.WebhookURL),
		zap.Int("worker_count", cfg.WorkerCount))

	// 创建去重器（如果启用）
	var deduper *thirdparty.Deduper
	if cfg.EnableDedup {
		dedupTTL := cfg.DedupTTL
		if dedupTTL == 0 {
			dedupTTL = thirdparty.DefaultDedupTTL
		}

		deduper = thirdparty.NewDeduper(
			redisClient.Client,
			logger.With(zap.String("component", "deduper")),
			dedupTTL,
		)

		logger.Info("event deduper initialized", zap.Duration("ttl", dedupTTL))
	}

	return queue, deduper
}

// StartEventQueueWorkers 启动事件队列Workers
func StartEventQueueWorkers(
	ctx context.Context,
	queue *thirdparty.EventQueue,
	workerCount int,
	logger *zap.Logger,
) {
	if queue == nil {
		logger.Debug("event queue not initialized, skipping workers")
		return
	}

	if workerCount <= 0 {
		workerCount = 3 // 默认3个worker
	}

	logger.Info("starting event queue workers", zap.Int("count", workerCount))
	queue.StartWorker(ctx, workerCount)
}
