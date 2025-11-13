package app

import (
	"context"
	"time"

	redisstorage "github.com/taoyao-code/iot-server/internal/storage/redis"
	"go.uber.org/zap"
)

// DeadLetterCleaner P1-6: 死信队列清理器
// 定期清理超过24小时的死信消息
type DeadLetterCleaner struct {
	queue         *redisstorage.OutboundQueue
	logger        *zap.Logger
	checkInterval time.Duration // 检查间隔

	// 统计
	statsCleaned int64
}

// NewDeadLetterCleaner 创建死信队列清理器
func NewDeadLetterCleaner(queue *redisstorage.OutboundQueue, logger *zap.Logger) *DeadLetterCleaner {
	return &DeadLetterCleaner{
		queue:         queue,
		logger:        logger,
		checkInterval: 1 * time.Hour, // 每小时清理一次
	}
}

// Start 启动死信队列清理器
func (c *DeadLetterCleaner) Start(ctx context.Context) {
	c.logger.Info("P1-6: dead letter cleaner started",
		zap.Duration("check_interval", c.checkInterval))

	ticker := time.NewTicker(c.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("P1-6: dead letter cleaner stopped",
				zap.Int64("total_cleaned", c.statsCleaned))
			return
		case <-ticker.C:
			c.cleanExpiredMessages(ctx)
		}
	}
}

// cleanExpiredMessages 清理过期的死信消息
func (c *DeadLetterCleaner) cleanExpiredMessages(ctx context.Context) {
	// 获取死信队列长度
	count, err := c.queue.GetDeadCount(ctx)
	if err != nil {
		c.logger.Error("P1-6: failed to get dead count", zap.Error(err))
		return
	}

	if count == 0 {
		return
	}

	c.logger.Info("P1-6: checking dead letter queue",
		zap.Int64("dead_count", count))

	// B修复: 清理超过24小时的死信消息
	cutoff := 24 * time.Hour
	cleaned, err := c.queue.CleanExpiredDead(ctx, cutoff, 100)
	if err != nil {
		c.logger.Error("P1-6: failed to clean expired dead messages",
			zap.Error(err),
			zap.Int64("dead_count", count))
	} else if cleaned > 0 {
		c.statsCleaned += cleaned
		c.logger.Info("P1-6: cleaned expired dead messages",
			zap.Int64("cleaned", cleaned),
			zap.Int64("remaining", count-cleaned),
			zap.Int64("total_cleaned", c.statsCleaned))
	}

	// 告警阈值检查
	remainingCount := count - cleaned
	if remainingCount > 1000 {
		c.logger.Warn("⚠️ P1-6: dead letter queue overloaded",
			zap.Int64("dead_count", remainingCount),
			zap.String("suggestion", "manual intervention required"))
	}
}

// Stats 获取统计信息
func (c *DeadLetterCleaner) Stats() map[string]interface{} {
	return map[string]interface{}{
		"total_cleaned": c.statsCleaned,
	}
}
