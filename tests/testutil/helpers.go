package testutil

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// CleanDatabase 清理数据库（保留 schema，删除所有数据）
func CleanDatabase(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 按依赖顺序删除数据
	tables := []string{
		"outbound_messages",
		"charging_transactions",
		"charging_sessions",
		"port_status_history",
		"ports",
		"device_heartbeats",
		"devices",
		"cards",
	}

	for _, table := range tables {
		_, err := pool.Exec(ctx, "TRUNCATE TABLE "+table+" CASCADE")
		if err != nil {
			// 如果表不存在则忽略
			t.Logf("Warning: failed to truncate %s: %v", table, err)
		}
	}
}

// CleanRedis 清理 Redis 测试数据
func CleanRedis(t *testing.T, rdb *redis.Client) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 使用 SCAN 清理测试相关的 key
	iter := rdb.Scan(ctx, 0, "*", 0).Iterator()
	for iter.Next(ctx) {
		key := iter.Val()
		if err := rdb.Del(ctx, key).Err(); err != nil {
			t.Logf("Warning: failed to delete key %s: %v", key, err)
		}
	}
	if err := iter.Err(); err != nil {
		t.Logf("Warning: redis scan error: %v", err)
	}
}

// RequireNoError 断言无错误（简化版）
func RequireNoError(t *testing.T, err error, msgAndArgs ...interface{}) {
	t.Helper()
	require.NoError(t, err, msgAndArgs...)
}

// RequireEqual 断言相等
func RequireEqual(t *testing.T, expected, actual interface{}, msgAndArgs ...interface{}) {
	t.Helper()
	require.Equal(t, expected, actual, msgAndArgs...)
}

// SetupTestDB 创建测试数据库连接池
func SetupTestDB(t *testing.T, dsn string) *pgxpool.Pool {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	RequireNoError(t, err, "failed to create test db pool")

	// 验证连接
	err = pool.Ping(ctx)
	RequireNoError(t, err, "failed to ping test db")

	// 注册清理函数
	t.Cleanup(func() {
		pool.Close()
	})

	return pool
}

// SetupTestRedis 创建测试 Redis 客户端
func SetupTestRedis(t *testing.T, addr string) *redis.Client {
	t.Helper()

	rdb := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     "",
		DB:           0,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 验证连接
	err := rdb.Ping(ctx).Err()
	RequireNoError(t, err, "failed to ping test redis")

	// 注册清理函数
	t.Cleanup(func() {
		rdb.Close()
	})

	return rdb
}

// WaitForCondition 等待条件满足（用于异步操作测试）
func WaitForCondition(t *testing.T, condition func() bool, timeout time.Duration, msg string) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("Timeout waiting for condition: %s", msg)
}
