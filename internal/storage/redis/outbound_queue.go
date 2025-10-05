package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// Redis Key前缀
	outboundQueueKey     = "outbound:queue"          // 待处理队列（Sorted Set，按优先级+时间排序）
	outboundProcessingKey = "outbound:processing:%s" // 处理中（Hash，设备维度）
	outboundDeadKey      = "outbound:dead"           // 死信队列（List）
)

// OutboundMessage 下行消息结构 (Week2.2)
type OutboundMessage struct {
	ID        string    `json:"id"`         // 消息ID（唯一）
	DeviceID  int64     `json:"device_id"`  // 设备ID
	PhyID     string    `json:"phy_id"`     // 物理ID
	Command   []byte    `json:"command"`    // 命令数据
	Priority  int       `json:"priority"`   // 优先级（0-9，9最高）
	Retries   int       `json:"retries"`    // 已重试次数
	MaxRetry  int       `json:"max_retry"`  // 最大重试次数
	CreatedAt time.Time `json:"created_at"` // 创建时间
	UpdatedAt time.Time `json:"updated_at"` // 更新时间
	Timeout   int       `json:"timeout"`    // 超时时间（毫秒）
}

// OutboundQueue Redis下行队列 (Week2.2)
type OutboundQueue struct {
	client *Client
}

// NewOutboundQueue 创建Redis下行队列
func NewOutboundQueue(client *Client) *OutboundQueue {
	return &OutboundQueue{client: client}
}

// Enqueue 入队（添加新消息到队列）
func (q *OutboundQueue) Enqueue(ctx context.Context, msg *OutboundMessage) error {
	// 序列化消息
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	// 计算score（优先级*1e12 + 时间戳，保证优先级高的排前面）
	score := float64(msg.Priority)*1e12 + float64(msg.CreatedAt.UnixNano())

	// 添加到Sorted Set
	return q.client.ZAdd(ctx, outboundQueueKey, redis.Z{
		Score:  score,
		Member: msg.ID + ":" + string(data),
	}).Err()
}

// Dequeue 出队（获取一条待处理消息）
func (q *OutboundQueue) Dequeue(ctx context.Context) (*OutboundMessage, error) {
	// 使用ZPOPMIN原子操作（Redis 5.0+）
	result, err := q.client.ZPopMin(ctx, outboundQueueKey, 1).Result()
	if err != nil {
		return nil, err
	}

	if len(result) == 0 {
		return nil, nil // 队列为空
	}

	// 解析消息
	member := result[0].Member.(string)
	msg, err := parseMessage(member)
	if err != nil {
		return nil, fmt.Errorf("parse message: %w", err)
	}

	return msg, nil
}

// MarkProcessing 标记消息为处理中
func (q *OutboundQueue) MarkProcessing(ctx context.Context, msg *OutboundMessage) error {
	processingKey := fmt.Sprintf(outboundProcessingKey, msg.PhyID)
	
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	// 设置到Hash，带TTL（防止进程崩溃导致永久锁定）
	pipe := q.client.Pipeline()
	pipe.HSet(ctx, processingKey, msg.ID, data)
	pipe.Expire(ctx, processingKey, time.Duration(msg.Timeout)*time.Millisecond*2)
	_, err = pipe.Exec(ctx)
	
	return err
}

// MarkSuccess 标记消息处理成功（删除）
func (q *OutboundQueue) MarkSuccess(ctx context.Context, msg *OutboundMessage) error {
	processingKey := fmt.Sprintf(outboundProcessingKey, msg.PhyID)
	return q.client.HDel(ctx, processingKey, msg.ID).Err()
}

// MarkFailed 标记消息处理失败（重试或进入死信队列）
func (q *OutboundQueue) MarkFailed(ctx context.Context, msg *OutboundMessage, errMsg string) error {
	processingKey := fmt.Sprintf(outboundProcessingKey, msg.PhyID)
	
	// 先从处理中删除
	if err := q.client.HDel(ctx, processingKey, msg.ID).Err(); err != nil {
		return err
	}

	msg.Retries++
	msg.UpdatedAt = time.Now()

	// 判断是否需要重试
	if msg.Retries < msg.MaxRetry {
		// 重新入队（优先级降低）
		return q.Enqueue(ctx, msg)
	}

	// 超过最大重试次数，进入死信队列
	deadMsg := map[string]interface{}{
		"message":    msg,
		"error":      errMsg,
		"failed_at":  time.Now(),
	}
	data, _ := json.Marshal(deadMsg)
	
	return q.client.LPush(ctx, outboundDeadKey, data).Err()
}

// GetPendingCount 获取待处理消息数量
func (q *OutboundQueue) GetPendingCount(ctx context.Context) (int64, error) {
	return q.client.ZCard(ctx, outboundQueueKey).Result()
}

// GetProcessingCount 获取处理中消息数量（所有设备）
func (q *OutboundQueue) GetProcessingCount(ctx context.Context) (int64, error) {
	// 扫描所有processing:*的key
	var cursor uint64
	var count int64
	
	for {
		keys, nextCursor, err := q.client.Scan(ctx, cursor, "outbound:processing:*", 100).Result()
		if err != nil {
			return 0, err
		}
		
		for _, key := range keys {
			n, err := q.client.HLen(ctx, key).Result()
			if err != nil {
				return 0, err
			}
			count += n
		}
		
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	
	return count, nil
}

// GetDeadCount 获取死信队列消息数量
func (q *OutboundQueue) GetDeadCount(ctx context.Context) (int64, error) {
	return q.client.LLen(ctx, outboundDeadKey).Result()
}

// CleanupStale 清理过期的处理中消息（已被Expire自动清理，此方法用于手动触发）
func (q *OutboundQueue) CleanupStale(ctx context.Context, timeout time.Duration) (int64, error) {
	// Redis的Expire会自动清理，此方法保留用于兼容性
	return 0, nil
}

// parseMessage 解析消息
func parseMessage(member string) (*OutboundMessage, error) {
	// 格式: "ID:JSON"
	colonIdx := -1
	for i, c := range member {
		if c == ':' {
			colonIdx = i
			break
		}
	}
	
	if colonIdx == -1 {
		return nil, fmt.Errorf("invalid message format")
	}

	data := []byte(member[colonIdx+1:])
	var msg OutboundMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}

	return &msg, nil
}

// Stats 获取队列统计信息
func (q *OutboundQueue) Stats(ctx context.Context) (map[string]interface{}, error) {
	pending, _ := q.GetPendingCount(ctx)
	processing, _ := q.GetProcessingCount(ctx)
	dead, _ := q.GetDeadCount(ctx)

	return map[string]interface{}{
		"pending":    pending,
		"processing": processing,
		"dead":       dead,
		"total":      pending + processing,
	}, nil
}
