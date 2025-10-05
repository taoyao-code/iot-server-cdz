package health

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DatabaseChecker 数据库健康检查器
type DatabaseChecker struct {
	pool *pgxpool.Pool
}

// NewDatabaseChecker 创建数据库健康检查器
func NewDatabaseChecker(pool *pgxpool.Pool) *DatabaseChecker {
	return &DatabaseChecker{pool: pool}
}

// Name 返回检查器名称
func (c *DatabaseChecker) Name() string {
	return "database"
}

// Check 执行健康检查
func (c *DatabaseChecker) Check(ctx context.Context) CheckResult {
	start := time.Now()

	// 1. Ping测试
	if err := c.pool.Ping(ctx); err != nil {
		return CheckResult{
			Status:  StatusUnhealthy,
			Message: fmt.Sprintf("ping failed: %v", err),
			Latency: time.Since(start),
		}
	}

	// 2. 获取连接池统计
	stats := c.pool.Stat()

	// 3. 计算连接池利用率
	utilization := 0.0
	if stats.MaxConns() > 0 {
		utilization = float64(stats.AcquiredConns()) / float64(stats.MaxConns())
	}

	// 4. 判断健康状态
	status := StatusHealthy
	message := "ok"

	if utilization > 0.9 {
		status = StatusDegraded
		message = "connection pool near limit"
	}

	if utilization >= 1.0 {
		status = StatusUnhealthy
		message = "connection pool exhausted"
	}

	return CheckResult{
		Status:  status,
		Message: message,
		Details: map[string]interface{}{
			"total_conns":    stats.TotalConns(),
			"idle_conns":     stats.IdleConns(),
			"acquired_conns": stats.AcquiredConns(),
			"max_conns":      stats.MaxConns(),
			"utilization":    fmt.Sprintf("%.1f%%", utilization*100),
		},
		Latency: time.Since(start),
	}
}
