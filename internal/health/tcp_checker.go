package health

import (
	"context"
	"fmt"
	"time"

	"github.com/taoyao-code/iot-server/internal/tcpserver"
)

// TCPChecker TCP服务器健康检查器
type TCPChecker struct {
	server *tcpserver.Server
}

// NewTCPChecker 创建TCP健康检查器
func NewTCPChecker(server *tcpserver.Server) *TCPChecker {
	return &TCPChecker{server: server}
}

// Name 返回检查器名称
func (c *TCPChecker) Name() string {
	return "tcp"
}

// Check 执行健康检查
func (c *TCPChecker) Check(ctx context.Context) CheckResult {
	start := time.Now()

	// 获取连接统计
	activeConns := c.server.ActiveConnections()
	maxConns := c.server.MaxConnections()

	// 如果未启用限流，返回基础状态
	if maxConns == 0 {
		return CheckResult{
			Status:  StatusHealthy,
			Message: "no limiting enabled",
			Details: map[string]interface{}{
				"active_connections": activeConns,
			},
			Latency: time.Since(start),
		}
	}

	// 计算利用率
	utilization := float64(activeConns) / float64(maxConns)

	// 判断健康状态
	status := StatusHealthy
	message := "ok"

	if utilization > 0.8 {
		status = StatusDegraded
		message = "high connection usage"
	}

	if utilization > 0.95 {
		status = StatusUnhealthy
		message = "connection limit near exhausted"
	}

	details := map[string]interface{}{
		"active_connections": activeConns,
		"max_connections":    maxConns,
		"utilization":        fmt.Sprintf("%.1f%%", utilization*100),
	}

	// 添加限流器统计
	if limiterStats := c.server.GetLimiterStats(); limiterStats != nil {
		details["rejected_total"] = limiterStats.RejectedTotal
	}

	// 添加熔断器统计
	if breakerStats := c.server.GetCircuitBreakerStats(); breakerStats != nil {
		details["circuit_breaker_state"] = breakerStats.State
		details["circuit_breaker_failures"] = breakerStats.FailureCount
	}

	return CheckResult{
		Status:  status,
		Message: message,
		Details: details,
		Latency: time.Since(start),
	}
}
