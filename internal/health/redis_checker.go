package health

import (
	"context"
	"fmt"
	"time"

	redisstorage "github.com/taoyao-code/iot-server/internal/storage/redis"
)

// RedisChecker Redis健康检查器 (Week2.2)
type RedisChecker struct {
	client *redisstorage.Client
}

// NewRedisChecker 创建Redis健康检查器
func NewRedisChecker(client *redisstorage.Client) *RedisChecker {
	return &RedisChecker{client: client}
}

// Name 返回检查器名称
func (c *RedisChecker) Name() string {
	return "redis"
}

// Check 执行健康检查
func (c *RedisChecker) Check(ctx context.Context) CheckResult {
	start := time.Now()

	// 1. Ping测试
	if err := c.client.HealthCheck(ctx); err != nil {
		return CheckResult{
			Status:  StatusUnhealthy,
			Message: fmt.Sprintf("ping failed: %v", err),
			Latency: time.Since(start),
		}
	}

	// 2. 获取连接池统计
	stats := c.client.Stats()

	// 3. 计算连接池利用率
	utilization := 0.0
	if stats.TotalConns > 0 {
		utilization = float64(stats.TotalConns-stats.IdleConns) / float64(stats.TotalConns)
	}

	// 4. 判断健康状态
	status := StatusHealthy
	message := "ok"

	if utilization > 0.9 {
		status = StatusDegraded
		message = "connection pool near limit"
	}

	if stats.Misses > stats.Hits && stats.Hits > 0 {
		// 连接池命中率低
		status = StatusDegraded
		message = "low connection pool hit rate"
	}

	return CheckResult{
		Status:  status,
		Message: message,
		Details: map[string]interface{}{
			"total_conns": stats.TotalConns,
			"idle_conns":  stats.IdleConns,
			"stale_conns": stats.StaleConns,
			"hits":        stats.Hits,
			"misses":      stats.Misses,
			"timeouts":    stats.Timeouts,
			"utilization": fmt.Sprintf("%.1f%%", utilization*100),
		},
		Latency: time.Since(start),
	}
}
