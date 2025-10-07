package thirdparty

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	// Redis Key前缀
	eventQueueKey = "thirdparty:event:queue"    // 主队列
	eventDLQKey   = "thirdparty:event:dlq"      // 死信队列（Dead Letter Queue）
	eventRetryKey = "thirdparty:event:retry:%s" // 重试计数器（event_id）

	// 配置常量
	maxRetries = 5              // 最大重试次数
	retryTTL   = 24 * time.Hour // 重试记录TTL
)

// EventQueue 异步事件队列
type EventQueue struct {
	redis   *redis.Client
	logger  *zap.Logger
	pusher  *Pusher
	baseURL string // Webhook基础URL
}

// NewEventQueue 创建事件队列
func NewEventQueue(redisClient *redis.Client, pusher *Pusher, webhookURL string, logger *zap.Logger) *EventQueue {
	return &EventQueue{
		redis:   redisClient,
		logger:  logger,
		pusher:  pusher,
		baseURL: webhookURL,
	}
}

// Enqueue 入队事件（异步，不阻塞业务逻辑）
func (q *EventQueue) Enqueue(ctx context.Context, event *StandardEvent) error {
	if q == nil || q.redis == nil {
		return fmt.Errorf("event queue not initialized")
	}

	// 序列化事件
	data, err := json.Marshal(event)
	if err != nil {
		q.logger.Error("failed to marshal event",
			zap.String("event_id", event.EventID),
			zap.String("event_type", string(event.EventType)),
			zap.Error(err))
		return fmt.Errorf("marshal event: %w", err)
	}

	// 推送到Redis List（右侧入队）
	err = q.redis.RPush(ctx, eventQueueKey, data).Err()
	if err != nil {
		q.logger.Error("failed to enqueue event",
			zap.String("event_id", event.EventID),
			zap.String("event_type", string(event.EventType)),
			zap.Error(err))
		return fmt.Errorf("redis rpush: %w", err)
	}

	q.logger.Debug("event enqueued",
		zap.String("event_id", event.EventID),
		zap.String("event_type", string(event.EventType)),
		zap.String("device_phy_id", event.DevicePhyID))

	return nil
}

// StartWorker 启动事件消费Worker
func (q *EventQueue) StartWorker(ctx context.Context, workerCount int) {
	if q == nil || q.redis == nil || q.pusher == nil {
		q.logger.Error("event queue worker cannot start: not initialized")
		return
	}

	q.logger.Info("starting event queue workers",
		zap.Int("worker_count", workerCount),
		zap.String("webhook_url", q.baseURL))

	// 启动多个Worker并发处理
	for i := 0; i < workerCount; i++ {
		workerID := i + 1
		go q.worker(ctx, workerID)
	}
}

// worker 单个Worker协程
func (q *EventQueue) worker(ctx context.Context, workerID int) {
	logger := q.logger.With(zap.Int("worker_id", workerID))
	logger.Info("event queue worker started")

	for {
		select {
		case <-ctx.Done():
			logger.Info("event queue worker stopped")
			return
		default:
			// 从队列左侧阻塞取出事件（超时5秒）
			result, err := q.redis.BLPop(ctx, 5*time.Second, eventQueueKey).Result()
			if err != nil {
				if err == redis.Nil {
					// 超时，继续循环
					continue
				}
				logger.Error("redis blpop error", zap.Error(err))
				time.Sleep(time.Second) // 出错后等待1秒
				continue
			}

			if len(result) < 2 {
				logger.Warn("invalid blpop result", zap.Any("result", result))
				continue
			}

			// result[0]是key，result[1]是value
			eventData := result[1]

			// 处理事件
			q.processEvent(ctx, eventData, logger)
		}
	}
}

// processEvent 处理单个事件
func (q *EventQueue) processEvent(ctx context.Context, eventData string, logger *zap.Logger) {
	// 反序列化事件
	var event StandardEvent
	if err := json.Unmarshal([]byte(eventData), &event); err != nil {
		logger.Error("failed to unmarshal event", zap.Error(err))
		// 格式错误的事件直接丢弃
		return
	}

	logger.Debug("processing event",
		zap.String("event_id", event.EventID),
		zap.String("event_type", string(event.EventType)),
		zap.String("device_phy_id", event.DevicePhyID))

	// 检查重试次数
	retryCount, err := q.getRetryCount(ctx, event.EventID)
	if err != nil {
		logger.Error("failed to get retry count",
			zap.String("event_id", event.EventID),
			zap.Error(err))
		// 出错时仍尝试推送
	}

	if retryCount >= maxRetries {
		logger.Warn("event exceeded max retries, moving to DLQ",
			zap.String("event_id", event.EventID),
			zap.String("event_type", string(event.EventType)),
			zap.Int("retry_count", retryCount))
		q.moveToDLQ(ctx, eventData, "max_retries_exceeded")
		return
	}

	// 推送事件到Webhook
	pushCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	statusCode, respBody, err := q.pusher.SendJSON(pushCtx, q.baseURL, &event)

	if err != nil || statusCode >= 500 {
		// 推送失败，增加重试计数并重新入队
		logger.Warn("event push failed, will retry",
			zap.String("event_id", event.EventID),
			zap.String("event_type", string(event.EventType)),
			zap.Int("status_code", statusCode),
			zap.Int("retry_count", retryCount+1),
			zap.Error(err))

		// 增加重试计数
		q.incrementRetryCount(ctx, event.EventID)

		// 延迟后重新入队（指数退避）
		delay := time.Duration(1<<uint(retryCount)) * time.Second // 1s, 2s, 4s, 8s, 16s
		time.Sleep(delay)

		// 重新入队
		if err := q.redis.RPush(ctx, eventQueueKey, eventData).Err(); err != nil {
			logger.Error("failed to re-enqueue event",
				zap.String("event_id", event.EventID),
				zap.Error(err))
			q.moveToDLQ(ctx, eventData, "re_enqueue_failed")
		}
		return
	}

	if statusCode >= 400 && statusCode < 500 {
		// 4xx错误，客户端错误，不重试，移到DLQ
		logger.Warn("event push client error, moving to DLQ",
			zap.String("event_id", event.EventID),
			zap.String("event_type", string(event.EventType)),
			zap.Int("status_code", statusCode),
			zap.ByteString("response", respBody))
		q.moveToDLQ(ctx, eventData, fmt.Sprintf("client_error_%d", statusCode))
		return
	}

	// 推送成功
	logger.Info("event pushed successfully",
		zap.String("event_id", event.EventID),
		zap.String("event_type", string(event.EventType)),
		zap.Int("status_code", statusCode))

	// 清理重试计数
	q.deleteRetryCount(ctx, event.EventID)
}

// moveToDLQ 移动事件到死信队列
func (q *EventQueue) moveToDLQ(ctx context.Context, eventData string, reason string) {
	// 构造DLQ记录
	dlqRecord := map[string]interface{}{
		"event_data": eventData,
		"reason":     reason,
		"timestamp":  time.Now().Unix(),
	}

	dlqData, err := json.Marshal(dlqRecord)
	if err != nil {
		q.logger.Error("failed to marshal dlq record", zap.Error(err))
		return
	}

	// 推送到DLQ
	err = q.redis.RPush(ctx, eventDLQKey, dlqData).Err()
	if err != nil {
		q.logger.Error("failed to move event to DLQ", zap.Error(err))
	}
}

// getRetryCount 获取重试次数
func (q *EventQueue) getRetryCount(ctx context.Context, eventID string) (int, error) {
	key := fmt.Sprintf(eventRetryKey, eventID)
	val, err := q.redis.Get(ctx, key).Result()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	var count int
	_, err = fmt.Sscanf(val, "%d", &count)
	return count, err
}

// incrementRetryCount 增加重试计数
func (q *EventQueue) incrementRetryCount(ctx context.Context, eventID string) {
	key := fmt.Sprintf(eventRetryKey, eventID)
	_, err := q.redis.Incr(ctx, key).Result()
	if err != nil {
		q.logger.Error("failed to increment retry count",
			zap.String("event_id", eventID),
			zap.Error(err))
		return
	}

	// 设置过期时间
	q.redis.Expire(ctx, key, retryTTL)
}

// deleteRetryCount 删除重试计数
func (q *EventQueue) deleteRetryCount(ctx context.Context, eventID string) {
	key := fmt.Sprintf(eventRetryKey, eventID)
	q.redis.Del(ctx, key)
}

// QueueLength 获取队列长度
func (q *EventQueue) QueueLength(ctx context.Context) (int64, error) {
	if q == nil || q.redis == nil {
		return 0, fmt.Errorf("queue not initialized")
	}
	return q.redis.LLen(ctx, eventQueueKey).Result()
}

// DLQLength 获取死信队列长度
func (q *EventQueue) DLQLength(ctx context.Context) (int64, error) {
	if q == nil || q.redis == nil {
		return 0, fmt.Errorf("queue not initialized")
	}
	return q.redis.LLen(ctx, eventDLQKey).Result()
}

// GetDLQEvents 获取死信队列中的事件（用于人工处理）
func (q *EventQueue) GetDLQEvents(ctx context.Context, start, stop int64) ([]string, error) {
	if q == nil || q.redis == nil {
		return nil, fmt.Errorf("queue not initialized")
	}
	return q.redis.LRange(ctx, eventDLQKey, start, stop).Result()
}

// ClearDLQ 清空死信队列
func (q *EventQueue) ClearDLQ(ctx context.Context) error {
	if q == nil || q.redis == nil {
		return fmt.Errorf("queue not initialized")
	}
	return q.redis.Del(ctx, eventDLQKey).Err()
}
