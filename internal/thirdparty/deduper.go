package thirdparty

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	// 去重Key前缀
	dedupKeyPrefix = "thirdparty:dedup"

	// DefaultDedupTTL 默认去重TTL（1小时）
	DefaultDedupTTL = time.Hour
)

// Deduper 去重器（基于Redis实现）
type Deduper struct {
	redis  *redis.Client
	logger *zap.Logger
	ttl    time.Duration // 去重Key的TTL
}

// NewDeduper 创建去重器
func NewDeduper(redisClient *redis.Client, logger *zap.Logger, ttl time.Duration) *Deduper {
	if ttl == 0 {
		ttl = DefaultDedupTTL
	}

	return &Deduper{
		redis:  redisClient,
		logger: logger,
		ttl:    ttl,
	}
}

// IsDuplicate 检查事件是否重复
// 返回true表示是重复事件，false表示首次出现
func (d *Deduper) IsDuplicate(ctx context.Context, eventID string) (bool, error) {
	if d == nil || d.redis == nil {
		return false, fmt.Errorf("deduper not initialized")
	}

	if eventID == "" {
		return false, fmt.Errorf("event_id is empty")
	}

	key := d.buildKey(eventID)

	// 使用SetNX（SET if Not eXists）实现原子性去重
	// 如果key不存在，设置成功，返回true；如果key存在，设置失败，返回false
	success, err := d.redis.SetNX(ctx, key, "1", d.ttl).Result()
	if err != nil {
		d.logger.Error("dedup check failed",
			zap.String("event_id", eventID),
			zap.Error(err))
		return false, fmt.Errorf("redis setnx: %w", err)
	}

	// success=true 表示是首次出现（设置成功）
	// success=false 表示是重复事件（key已存在）
	isDup := !success

	if isDup {
		d.logger.Debug("duplicate event detected",
			zap.String("event_id", eventID))
	}

	return isDup, nil
}

// Mark 标记事件已处理（用于手动去重）
func (d *Deduper) Mark(ctx context.Context, eventID string) error {
	if d == nil || d.redis == nil {
		return fmt.Errorf("deduper not initialized")
	}

	if eventID == "" {
		return fmt.Errorf("event_id is empty")
	}

	key := d.buildKey(eventID)

	err := d.redis.Set(ctx, key, "1", d.ttl).Err()
	if err != nil {
		d.logger.Error("failed to mark event",
			zap.String("event_id", eventID),
			zap.Error(err))
		return fmt.Errorf("redis set: %w", err)
	}

	return nil
}

// IsMarked 检查事件是否已被标记
func (d *Deduper) IsMarked(ctx context.Context, eventID string) (bool, error) {
	if d == nil || d.redis == nil {
		return false, fmt.Errorf("deduper not initialized")
	}

	if eventID == "" {
		return false, fmt.Errorf("event_id is empty")
	}

	key := d.buildKey(eventID)

	exists, err := d.redis.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("redis exists: %w", err)
	}

	return exists > 0, nil
}

// Delete 删除去重标记（用于测试或手动清理）
func (d *Deduper) Delete(ctx context.Context, eventID string) error {
	if d == nil || d.redis == nil {
		return fmt.Errorf("deduper not initialized")
	}

	if eventID == "" {
		return fmt.Errorf("event_id is empty")
	}

	key := d.buildKey(eventID)

	return d.redis.Del(ctx, key).Err()
}

// CleanupExpired 清理过期的去重Key（Redis会自动过期，此方法仅供主动清理）
// 注意：此方法性能开销较大，不建议频繁调用
func (d *Deduper) CleanupExpired(ctx context.Context) (int64, error) {
	if d == nil || d.redis == nil {
		return 0, fmt.Errorf("deduper not initialized")
	}

	pattern := d.buildKey("*")

	var cursor uint64
	var deleted int64

	for {
		// 使用SCAN命令遍历匹配的key
		keys, nextCursor, err := d.redis.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return deleted, fmt.Errorf("redis scan: %w", err)
		}

		// 检查每个key的TTL
		for _, key := range keys {
			ttl, err := d.redis.TTL(ctx, key).Result()
			if err != nil {
				d.logger.Warn("failed to get ttl", zap.String("key", key), zap.Error(err))
				continue
			}

			// 如果TTL为-1（永不过期）或-2（key不存在），删除它
			if ttl == -1 || ttl == -2 {
				if err := d.redis.Del(ctx, key).Err(); err != nil {
					d.logger.Warn("failed to delete key", zap.String("key", key), zap.Error(err))
				} else {
					deleted++
				}
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	d.logger.Info("dedup cleanup completed", zap.Int64("deleted_count", deleted))

	return deleted, nil
}

// Stats 获取去重统计信息
func (d *Deduper) Stats(ctx context.Context) (map[string]interface{}, error) {
	if d == nil || d.redis == nil {
		return nil, fmt.Errorf("deduper not initialized")
	}

	pattern := d.buildKey("*")

	var count int64
	var cursor uint64

	for {
		keys, nextCursor, err := d.redis.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return nil, fmt.Errorf("redis scan: %w", err)
		}

		count += int64(len(keys))

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	return map[string]interface{}{
		"total_keys": count,
		"ttl":        d.ttl.String(),
	}, nil
}

// buildKey 构建去重Key
func (d *Deduper) buildKey(eventID string) string {
	return fmt.Sprintf("%s:%s", dedupKeyPrefix, eventID)
}
