package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRedisSessionManagement 测试 Redis 会话管理
func TestRedisSessionManagement(t *testing.T) {
	rdb := getTestRedis(t)
	defer cleanupTest(t)

	ctx := context.Background()

	t.Run("SetAndGet_SessionKey", func(t *testing.T) {
		key := "session:device:TEST_001"
		value := "connected"
		ttl := 60 * time.Second

		// 设置会话
		err := rdb.Set(ctx, key, value, ttl).Err()
		require.NoError(t, err)

		// 获取会话
		result, err := rdb.Get(ctx, key).Result()
		require.NoError(t, err)
		assert.Equal(t, value, result)

		// 验证 TTL
		remainingTTL, err := rdb.TTL(ctx, key).Result()
		require.NoError(t, err)
		assert.Greater(t, remainingTTL, 50*time.Second)
		assert.LessOrEqual(t, remainingTTL, ttl)
	})

	t.Run("SessionExpiration", func(t *testing.T) {
		key := "session:device:TEST_002"
		value := "connected"
		ttl := 1 * time.Second

		// 设置短期会话
		err := rdb.Set(ctx, key, value, ttl).Err()
		require.NoError(t, err)

		// 等待过期
		time.Sleep(2 * time.Second)

		// 验证已过期
		_, err = rdb.Get(ctx, key).Result()
		assert.Error(t, err) // 应该是 redis.Nil 错误
	})

	t.Run("HeartbeatUpdate", func(t *testing.T) {
		key := "session:device:TEST_003"
		value := "connected"
		ttl := 60 * time.Second

		// 初始设置
		err := rdb.Set(ctx, key, value, ttl).Err()
		require.NoError(t, err)

		// 等待一段时间
		time.Sleep(2 * time.Second)

		// 更新心跳（刷新 TTL）
		err = rdb.Expire(ctx, key, ttl).Err()
		require.NoError(t, err)

		// 验证 TTL 已刷新（应该接近 60 秒）
		newTTL, err := rdb.TTL(ctx, key).Result()
		require.NoError(t, err)
		assert.Greater(t, newTTL, 55*time.Second, "TTL should be refreshed to near 60s")
	})

	t.Run("MultipleDevices", func(t *testing.T) {
		devices := []string{"DEVICE_A", "DEVICE_B", "DEVICE_C"}
		ttl := 60 * time.Second

		// 批量设置会话
		for _, device := range devices {
			key := "session:device:" + device
			err := rdb.Set(ctx, key, "connected", ttl).Err()
			require.NoError(t, err)
		}

		// 验证所有会话存在
		for _, device := range devices {
			key := "session:device:" + device
			exists, err := rdb.Exists(ctx, key).Result()
			require.NoError(t, err)
			assert.Equal(t, int64(1), exists)
		}

		// 清理指定会话
		err := rdb.Del(ctx, "session:device:DEVICE_B").Err()
		require.NoError(t, err)

		// 验证已删除
		exists, err := rdb.Exists(ctx, "session:device:DEVICE_B").Result()
		require.NoError(t, err)
		assert.Equal(t, int64(0), exists)
	})
}

// TestRedisHashOperations 测试 Redis Hash 操作（用于会话详情）
func TestRedisHashOperations(t *testing.T) {
	rdb := getTestRedis(t)
	defer cleanupTest(t)

	ctx := context.Background()

	t.Run("SessionDetails_Hash", func(t *testing.T) {
		key := "session:details:TEST_001"
		
		// 设置会话详情
		sessionData := map[string]interface{}{
			"device_id":  "TEST_001",
			"connected_at": time.Now().Unix(),
			"ip_addr":    "192.168.1.100",
			"protocol":   "AP3000",
		}

		err := rdb.HSet(ctx, key, sessionData).Err()
		require.NoError(t, err)

		// 设置过期时间
		err = rdb.Expire(ctx, key, 60*time.Second).Err()
		require.NoError(t, err)

		// 获取单个字段
		deviceID, err := rdb.HGet(ctx, key, "device_id").Result()
		require.NoError(t, err)
		assert.Equal(t, "TEST_001", deviceID)

		// 获取所有字段
		allData, err := rdb.HGetAll(ctx, key).Result()
		require.NoError(t, err)
		assert.Equal(t, "TEST_001", allData["device_id"])
		assert.Equal(t, "AP3000", allData["protocol"])
	})

	t.Run("UpdateSingleField", func(t *testing.T) {
		key := "session:details:TEST_002"

		// 初始化
		err := rdb.HSet(ctx, key, "device_id", "TEST_002", "status", "idle").Err()
		require.NoError(t, err)

		// 更新单个字段
		err = rdb.HSet(ctx, key, "status", "charging").Err()
		require.NoError(t, err)

		// 验证更新
		status, err := rdb.HGet(ctx, key, "status").Result()
		require.NoError(t, err)
		assert.Equal(t, "charging", status)
	})
}

// TestRedisQueueOperations 测试 Redis 队列操作（用于事件队列）
func TestRedisQueueOperations(t *testing.T) {
	rdb := getTestRedis(t)
	defer cleanupTest(t)

	ctx := context.Background()

	t.Run("EventQueue_LPUSH_RPOP", func(t *testing.T) {
		queueKey := "queue:events"

		events := []string{"event1", "event2", "event3"}

		// 入队
		for _, event := range events {
			err := rdb.LPush(ctx, queueKey, event).Err()
			require.NoError(t, err)
		}

		// 验证队列长度
		length, err := rdb.LLen(ctx, queueKey).Result()
		require.NoError(t, err)
		assert.Equal(t, int64(3), length)

		// 出队（RPOP 从右边弹出，LPUSH 从左边压入，所以是 FIFO）
		// LPUSH event1, event2, event3 => [event3, event2, event1]
		// RPOP 三次 => event1, event2, event3
		for _, expectedEvent := range events {
			event, err := rdb.RPop(ctx, queueKey).Result()
			require.NoError(t, err)
			assert.Equal(t, expectedEvent, event)
		}

		// 验证队列为空
		length, err = rdb.LLen(ctx, queueKey).Result()
		require.NoError(t, err)
		assert.Equal(t, int64(0), length)
	})

	t.Run("BlockingPop_BRPOP", func(t *testing.T) {
		queueKey := "queue:blocking"

		// 启动消费者
		done := make(chan string)
		go func() {
			result, err := rdb.BRPop(ctx, 2*time.Second, queueKey).Result()
			if err == nil {
				done <- result[1] // [0] 是 key，[1] 是 value
			} else {
				done <- ""
			}
		}()

		// 稍后生产数据
		time.Sleep(500 * time.Millisecond)
		err := rdb.LPush(ctx, queueKey, "delayed_event").Err()
		require.NoError(t, err)

		// 验证消费结果
		result := <-done
		assert.Equal(t, "delayed_event", result)
	})
}
