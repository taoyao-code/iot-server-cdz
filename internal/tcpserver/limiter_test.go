package tcpserver

import (
	"context"
	"testing"
	"time"
)

func TestConnectionLimiter(t *testing.T) {
	t.Run("基本限流功能", func(t *testing.T) {
		limiter := NewConnectionLimiter(3, 1*time.Second)

		// 获取3个许可
		ctx := context.Background()
		if err := limiter.Acquire(ctx); err != nil {
			t.Fatalf("第1次获取失败: %v", err)
		}
		if err := limiter.Acquire(ctx); err != nil {
			t.Fatalf("第2次获取失败: %v", err)
		}
		if err := limiter.Acquire(ctx); err != nil {
			t.Fatalf("第3次获取失败: %v", err)
		}

		// 第4次应该超时
		ctx4, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()
		if err := limiter.Acquire(ctx4); err == nil {
			t.Fatal("第4次获取应该失败")
		}

		// 释放一个
		limiter.Release()

		// 再次获取应该成功
		if err := limiter.Acquire(ctx); err != nil {
			t.Fatalf("释放后获取失败: %v", err)
		}
	})

	t.Run("统计功能", func(t *testing.T) {
		limiter := NewConnectionLimiter(10, 1*time.Second)

		// 获取5个
		for i := 0; i < 5; i++ {
			_ = limiter.Acquire(context.Background())
		}

		stats := limiter.Stats()
		if stats.ActiveConnections != 5 {
			t.Errorf("期望5个活跃连接，实际: %d", stats.ActiveConnections)
		}
		if stats.MaxConnections != 10 {
			t.Errorf("期望最大10个连接，实际: %d", stats.MaxConnections)
		}
		if stats.Utilization != 0.5 {
			t.Errorf("期望利用率0.5，实际: %.2f", stats.Utilization)
		}
	})
}

func TestRateLimiter(t *testing.T) {
	t.Run("速率限流", func(t *testing.T) {
		limiter := NewRateLimiter(10, 20) // 每秒10个，突发20个

		// 突发消费20个
		for i := 0; i < 20; i++ {
			if !limiter.Allow() {
				t.Fatalf("突发第%d个请求被拒绝", i+1)
			}
		}

		// 第21个应该被拒绝
		if limiter.Allow() {
			t.Fatal("第21个请求应该被拒绝")
		}

		// 等待100ms，应该能补充1个token
		time.Sleep(150 * time.Millisecond)
		if !limiter.Allow() {
			t.Fatal("等待后的请求应该成功")
		}
	})

	t.Run("统计功能", func(t *testing.T) {
		limiter := NewRateLimiter(100, 200)

		// 消费10个
		for i := 0; i < 10; i++ {
			limiter.Allow()
		}

		stats := limiter.Stats()
		if stats.AllowedTotal != 10 {
			t.Errorf("期望允许10个，实际: %d", stats.AllowedTotal)
		}
	})
}
